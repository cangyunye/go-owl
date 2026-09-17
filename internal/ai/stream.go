package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// StreamingChatModel 由支持流式输出的模型客户端实现。
// onDelta 逐段回调增量文本（可为 nil），返回值为拼接后的完整内容。
type StreamingChatModel interface {
	GenerateStream(ctx context.Context, messages []Message, onDelta func(string)) (string, error)
}

// GenerateStream 实现 StreamingChatModel：按 APIType 分派 SSE 协议。
func (m *HTTPModel) GenerateStream(ctx context.Context, messages []Message, onDelta func(string)) (string, error) {
	if m.apiType == "anthropic" {
		return m.generateStreamAnthropic(ctx, messages, onDelta)
	}
	return m.generateStreamOpenAI(ctx, messages, onDelta)
}

// consumeSSE 逐行读取 SSE 流，把每个 data: 载荷交给 handle，遇 [DONE] 终止。
func consumeSSE(resp *http.Response, handle func(data string) error) error {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}
		if err := handle(data); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// ---- OpenAI 兼容流式协议 ----

func (m *HTTPModel) generateStreamOpenAI(ctx context.Context, messages []Message, onDelta func(string)) (string, error) {
	reqBody := struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
	}{Model: m.model, Messages: messages, Stream: true}

	var content strings.Builder
	err := m.postSSE(ctx, buildChatURL(m.baseURL), map[string]string{
		"Authorization": "Bearer " + m.apiKey,
	}, reqBody, func(data string) error {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("failed to parse stream chunk: %w", err)
		}
		if chunk.Error != nil {
			return fmt.Errorf("API error: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			delta := chunk.Choices[0].Delta.Content
			content.WriteString(delta)
			if onDelta != nil {
				onDelta(delta)
			}
		}
		return nil
	})
	return content.String(), err
}

// ---- Anthropic 流式协议 ----

func (m *HTTPModel) generateStreamAnthropic(ctx context.Context, messages []Message, onDelta func(string)) (string, error) {
	system, wireMsgs := buildAnthropicWire(messages)

	reqBody := struct {
		Model     string                 `json:"model"`
		MaxTokens int                    `json:"max_tokens"`
		System    string                 `json:"system,omitempty"`
		Messages  []anthropicWireMessage `json:"messages"`
		Stream    bool                   `json:"stream"`
	}{Model: m.model, MaxTokens: anthropicMaxTokens, System: system, Messages: wireMsgs, Stream: true}

	var content strings.Builder
	err := m.postSSE(ctx, buildMessagesURL(m.baseURL), map[string]string{
		"x-api-key":         m.apiKey,
		"anthropic-version": anthropicAPIVersion,
	}, reqBody, func(data string) error {
		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil // 忽略非 JSON 载荷
		}
		if event.Error != nil {
			return fmt.Errorf("API error: %s", event.Error.Message)
		}
		if event.Type == "content_block_delta" && event.Delta.Text != "" {
			content.WriteString(event.Delta.Text)
			if onDelta != nil {
				onDelta(event.Delta.Text)
			}
		}
		return nil
	})
	return content.String(), err
}

// postSSE 发起流式 POST 并消费 SSE 流，内容累积由调用方闭包负责。
func (m *HTTPModel) postSSE(ctx context.Context, url string, headers map[string]string, reqBody interface{}, handle func(data string) error) error {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	llmDebug("[SSE] Request URL: %s", url)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return &statusCodeError{Code: resp.StatusCode, Body: string(respBody)}
	}

	return consumeSSE(resp, handle)
}

// statusCodeError 是流式请求的非 200 响应
type statusCodeError struct {
	Code int
	Body string
}

func (e *statusCodeError) Error() string {
	return fmt.Sprintf("API error, status: %d, body: %s", e.Code, e.Body)
}
