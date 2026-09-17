package ai

import (
	"context"
	"encoding/json"
	"strings"
)

// ToolCallingStreamModel 由支持流式原生 function calling 的模型客户端实现。
// onDelta 回调文本增量（工具调用轮通常无文本，总结轮为正文）。
type ToolCallingStreamModel interface {
	GenerateToolsStream(ctx context.Context, messages []Message, tools []ToolDef, onDelta func(string)) (*ModelResponse, error)
}

// GenerateToolsStream 流式原生 function calling：SSE 消费并聚合 tool_calls 分片。
func (m *HTTPModel) GenerateToolsStream(ctx context.Context, messages []Message, tools []ToolDef, onDelta func(string)) (*ModelResponse, error) {
	if m.apiType == "anthropic" {
		return m.generateToolsStreamAnthropic(ctx, messages, tools, onDelta)
	}
	return m.generateToolsStreamOpenAI(ctx, messages, tools, onDelta)
}

// ---- OpenAI 兼容流式 FC ----

type openAIStreamToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (m *HTTPModel) generateToolsStreamOpenAI(ctx context.Context, messages []Message, tools []ToolDef, onDelta func(string)) (*ModelResponse, error) {
	reqBody := struct {
		Model    string              `json:"model"`
		Messages []openAIWireMessage `json:"messages"`
		Tools    []openAIWireTool    `json:"tools,omitempty"`
		Stream   bool                `json:"stream"`
	}{Model: m.model, Messages: buildOpenAIWireMessages(messages), Tools: buildOpenAIWireTools(tools), Stream: true}

	out := &ModelResponse{}
	// tool_calls 按 index 跨 chunk 聚合：id/name 取首个非空值，arguments 逐块拼接
	agg := map[int]*openAIStreamToolCallDelta{}
	var order []int

	err := m.postSSE(ctx, buildChatURL(m.baseURL), map[string]string{
		"Authorization": "Bearer " + m.apiKey,
	}, reqBody, func(data string) error {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string                      `json:"content"`
					ToolCalls []openAIStreamToolCallDelta `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil // 忽略无法解析的载荷（如注释行）
		}
		if chunk.Error != nil {
			return &providerError{msg: chunk.Error.Message}
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				out.Content += choice.Delta.Content
				if onDelta != nil {
					onDelta(choice.Delta.Content)
				}
			}
			for _, tcd := range choice.Delta.ToolCalls {
				existing, ok := agg[tcd.Index]
				if !ok {
					cp := tcd
					agg[tcd.Index] = &cp
					order = append(order, tcd.Index)
					continue
				}
				if tcd.ID != "" && existing.ID == "" {
					existing.ID = tcd.ID
				}
				if tcd.Function.Name != "" && existing.Function.Name == "" {
					existing.Function.Name = tcd.Function.Name
				}
				existing.Function.Arguments += tcd.Function.Arguments
			}
			if choice.FinishReason != "" {
				out.FinishReason = choice.FinishReason
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, idx := range order {
		tcd := agg[idx]
		call := ToolCall{ID: tcd.ID, Name: tcd.Function.Name}
		if strings.TrimSpace(tcd.Function.Arguments) != "" {
			_ = json.Unmarshal([]byte(tcd.Function.Arguments), &call.Arguments)
		}
		out.ToolCalls = append(out.ToolCalls, call)
	}
	if len(out.ToolCalls) > 0 {
		out.FinishReason = "tool_calls"
	} else if out.FinishReason == "" {
		out.FinishReason = "stop"
	}
	return out, nil
}

// ---- Anthropic 流式 FC ----

func (m *HTTPModel) generateToolsStreamAnthropic(ctx context.Context, messages []Message, tools []ToolDef, onDelta func(string)) (*ModelResponse, error) {
	system, wireMsgs := buildAnthropicWire(messages)

	reqBody := struct {
		Model     string                 `json:"model"`
		MaxTokens int                    `json:"max_tokens"`
		System    string                 `json:"system,omitempty"`
		Messages  []anthropicWireMessage `json:"messages"`
		Tools     interface{}            `json:"tools,omitempty"`
		Stream    bool                   `json:"stream"`
	}{Model: m.model, MaxTokens: anthropicMaxTokens, System: system, Messages: wireMsgs, Stream: true}
	if wt := anthropicWireTools(tools); wt != nil {
		reqBody.Tools = wt
	}

	out := &ModelResponse{}
	// content block 聚合：tool_use 的 id/name 来自 content_block_start，参数来自 input_json_delta
	type blockAgg struct {
		Type string
		ID   string
		Name string
		JSON string
	}
	blocks := map[int]*blockAgg{}
	var order []int

	err := m.postSSE(ctx, buildMessagesURL(m.baseURL), map[string]string{
		"x-api-key":         m.apiKey,
		"anthropic-version": anthropicAPIVersion,
	}, reqBody, func(data string) error {
		var event struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
				Text string `json:"text"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil
		}
		if event.Error != nil {
			return &providerError{msg: event.Error.Message}
		}
		switch event.Type {
		case "content_block_start":
			blocks[event.Index] = &blockAgg{
				Type: event.ContentBlock.Type,
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
			}
			order = append(order, event.Index)
		case "content_block_delta":
			switch event.Delta.Type {
			case "text_delta":
				out.Content += event.Delta.Text
				if onDelta != nil {
					onDelta(event.Delta.Text)
				}
			case "input_json_delta":
				if b, ok := blocks[event.Index]; ok {
					b.JSON += event.Delta.PartialJSON
				}
			}
		case "message_delta":
			if event.Delta.StopReason != "" {
				out.FinishReason = event.Delta.StopReason
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, idx := range order {
		b := blocks[idx]
		switch b.Type {
		case "text":
			// 文本块已在 text_delta 阶段累积
		case "tool_use":
			call := ToolCall{ID: b.ID, Name: b.Name}
			if strings.TrimSpace(b.JSON) != "" {
				_ = json.Unmarshal([]byte(b.JSON), &call.Arguments)
			}
			out.ToolCalls = append(out.ToolCalls, call)
		}
	}
	switch out.FinishReason {
	case "tool_use":
		out.FinishReason = "tool_calls"
	case "":
		out.FinishReason = "stop"
	}
	return out, nil
}

// providerError 是 SSE 流中 provider 返回的错误载荷
type providerError struct{ msg string }

func (e *providerError) Error() string { return "API error: " + e.msg }
