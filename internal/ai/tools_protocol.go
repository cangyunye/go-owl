package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrToolsUnsupported 表示 provider 拒绝了原生 function calling 请求（4xx 类），
// 调用方应降级为文本协议工具调用。
var ErrToolsUnsupported = errors.New("tools not supported by provider")

// ToolCallingChatModel 由支持原生 function calling 的模型客户端实现。
type ToolCallingChatModel interface {
	GenerateTools(ctx context.Context, messages []Message, tools []ToolDef) (*ModelResponse, error)
}

// ModelResponse 是原生 function calling 的一次响应。
type ModelResponse struct {
	Content   string
	ToolCalls []ToolCall
	// FinishReason 归一化："tool_calls"（要求调工具）或 "stop"
	FinishReason string
}

// GenerateTools 以原生 function calling 协议调用模型。
func (m *HTTPModel) GenerateTools(ctx context.Context, messages []Message, tools []ToolDef) (*ModelResponse, error) {
	if m.apiType == "anthropic" {
		return m.generateToolsAnthropic(ctx, messages, tools)
	}
	return m.generateToolsOpenAI(ctx, messages, tools)
}

// ---- OpenAI 兼容协议 ----

type openAIWireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIWireMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content"`
	ToolCalls  []openAIWireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}

type openAIWireTool struct {
	Type     string `json:"type"` // 固定 "function"
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

func buildOpenAIWireMessages(messages []Message) []openAIWireMessage {
	wire := make([]openAIWireMessage, 0, len(messages))
	for _, msg := range messages {
		wm := openAIWireMessage{Role: msg.Role, Content: msg.Content, ToolCallID: msg.ToolCallID}
		if msg.Role == "tool" && wm.Content == "" {
			// OpenAI 要求 tool 消息 content 可为空串，但空串比缺省更稳
			wm.Content = ""
		}
		for _, tc := range msg.ToolCalls {
			wtc := openAIWireToolCall{ID: tc.ID, Type: "function"}
			wtc.Function.Name = tc.Name
			wtc.Function.Arguments = tc.ArgsJSON
			wm.ToolCalls = append(wm.ToolCalls, wtc)
		}
		wire = append(wire, wm)
	}
	return wire
}

func buildOpenAIWireTools(tools []ToolDef) []openAIWireTool {
	wire := make([]openAIWireTool, 0, len(tools))
	for _, td := range tools {
		wt := openAIWireTool{Type: "function"}
		wt.Function.Name = td.Name
		wt.Function.Description = td.Description
		wt.Function.Parameters = json.RawMessage(td.Schema)
		wire = append(wire, wt)
	}
	return wire
}

func (m *HTTPModel) generateToolsOpenAI(ctx context.Context, messages []Message, tools []ToolDef) (*ModelResponse, error) {
	reqBody := struct {
		Model    string              `json:"model"`
		Messages []openAIWireMessage `json:"messages"`
		Tools    []openAIWireTool    `json:"tools,omitempty"`
	}{Model: m.model, Messages: buildOpenAIWireMessages(messages), Tools: buildOpenAIWireTools(tools)}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := buildChatURL(m.baseURL)
	llmDebug("[OpenAI:tools] Request URL: %s", url)
	llmDebug("[OpenAI:tools] Request Body: %s", string(bodyBytes))

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	llmDebug("[OpenAI:tools] Response Status: %d", resp.StatusCode)
	llmDebug("[OpenAI:tools] Response Body: %s", string(respBody))

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("API error, status: %d, body: %s", resp.StatusCode, string(respBody))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			err = fmt.Errorf("%w: %v", ErrToolsUnsupported, err)
		}
		return nil, err
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content   string               `json:"content"`
				ToolCalls []openAIWireToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Model string `json:"model"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := raw.Choices[0]
	out := &ModelResponse{Content: choice.Message.Content, FinishReason: choice.FinishReason}
	for _, tc := range choice.Message.ToolCalls {
		call := ToolCall{ID: tc.ID, Name: tc.Function.Name}
		if strings.TrimSpace(tc.Function.Arguments) != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &call.Arguments)
		}
		out.ToolCalls = append(out.ToolCalls, call)
	}
	if out.ToolCalls != nil {
		out.FinishReason = "tool_calls"
	} else if out.FinishReason == "" {
		out.FinishReason = "stop"
	}
	return out, nil
}

// ---- Anthropic 协议 ----

type anthropicWireBlock struct {
	Type string `json:"type"` // text | tool_use | tool_result
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type anthropicWireMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string 或 []anthropicWireBlock
}

func buildAnthropicWire(messages []Message) (system string, wire []anthropicWireMessage) {
	var systems []string
	appendBlock := func(role string, blocks []anthropicWireBlock) {
		// Anthropic 严格要求 user/assistant 交替：同类消息合并 content 块
		if n := len(wire); n > 0 && wire[n-1].Role == role {
			if arr, ok := wire[n-1].Content.([]anthropicWireBlock); ok {
				wire[n-1].Content = append(arr, blocks...)
				return
			}
		}
		wire = append(wire, anthropicWireMessage{Role: role, Content: blocks})
	}

	for _, msg := range messages {
		switch {
		case msg.Role == "system":
			systems = append(systems, msg.Content)

		case msg.Role == "tool":
			// 连续 tool 结果合并为一条 user 消息的 tool_result 块
			blocks := []anthropicWireBlock{{
				Type: "tool_result", ToolUseID: msg.ToolCallID, Content: msg.Content,
			}}
			if n := len(wire); n > 0 && wire[n-1].Role == "user" {
				if arr, ok := wire[n-1].Content.([]anthropicWireBlock); ok {
					wire[n-1].Content = append(arr, blocks...)
					continue
				}
			}
			appendBlock("user", blocks)

		case len(msg.ToolCalls) > 0:
			var blocks []anthropicWireBlock
			if msg.Content != "" {
				blocks = append(blocks, anthropicWireBlock{Type: "text", Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, anthropicWireBlock{
					Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: json.RawMessage(tc.ArgsJSON),
				})
			}
			appendBlock("assistant", blocks)

		default:
			role := msg.Role
			if role != "user" && role != "assistant" {
				role = "user"
			}
			appendBlock(role, []anthropicWireBlock{{Type: "text", Text: msg.Content}})
		}
	}

	if len(systems) > 0 {
		system = strings.Join(systems, "\n\n")
	}
	return system, wire
}

// anthropicWireTool 是 Anthropic tools 参数的 wire 结构
type anthropicWireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// anthropicWireTools 把 ToolDef 列表转为 Anthropic wire 格式
func anthropicWireTools(tools []ToolDef) []anthropicWireTool {
	wire := make([]anthropicWireTool, 0, len(tools))
	for _, td := range tools {
		wire = append(wire, anthropicWireTool{
			Name: td.Name, Description: td.Description, InputSchema: json.RawMessage(td.Schema),
		})
	}
	return wire
}

func (m *HTTPModel) generateToolsAnthropic(ctx context.Context, messages []Message, tools []ToolDef) (*ModelResponse, error) {
	system, wireMsgs := buildAnthropicWire(messages)

	var reqTools interface{}
	if len(tools) > 0 {
		reqTools = anthropicWireTools(tools)
	}

	reqBody := struct {
		Model     string                 `json:"model"`
		MaxTokens int                    `json:"max_tokens"`
		System    string                 `json:"system,omitempty"`
		Messages  []anthropicWireMessage `json:"messages"`
		Tools     interface{}            `json:"tools,omitempty"`
	}{Model: m.model, MaxTokens: anthropicMaxTokens, System: system, Messages: wireMsgs, Tools: reqTools}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := buildMessagesURL(m.baseURL)
	llmDebug("[Anthropic:tools] Request URL: %s", url)
	llmDebug("[Anthropic:tools] Request Body: %s", string(bodyBytes))

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", m.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	llmDebug("[Anthropic:tools] Response Status: %d", resp.StatusCode)
	llmDebug("[Anthropic:tools] Response Body: %s", string(respBody))

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("API error, status: %d, body: %s", resp.StatusCode, string(respBody))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			err = fmt.Errorf("%w: %v", ErrToolsUnsupported, err)
		}
		return nil, err
	}

	var raw struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Model      string `json:"model"`
		Error      *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	out := &ModelResponse{}
	for _, block := range raw.Content {
		switch block.Type {
		case "text":
			out.Content += block.Text
		case "tool_use":
			call := ToolCall{ID: block.ID, Name: block.Name}
			if len(block.Input) > 0 {
				_ = json.Unmarshal(block.Input, &call.Arguments)
			}
			out.ToolCalls = append(out.ToolCalls, call)
		}
	}
	switch raw.StopReason {
	case "tool_use":
		out.FinishReason = "tool_calls"
	default:
		out.FinishReason = "stop"
	}
	return out, nil
}
