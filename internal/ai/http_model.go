package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ModelOptions 描述一个直连 LLM HTTP API 的模型客户端。
type ModelOptions struct {
	// APIType 协议类型："openai"（默认，覆盖所有 OpenAI 兼容端点）或 "anthropic"
	APIType string
	BaseURL string
	Model   string
	APIKey  string
	// TimeoutSeconds 请求超时秒数，0 表示默认 120 秒
	TimeoutSeconds int
}

// HTTPModel 是 internal/ai 的统一 LLM 客户端：按 APIType 分派协议实现，
// 同时服务 CLI（配置文件构造）与 Web（用户 per-request key 构造）两个宿主。
type HTTPModel struct {
	apiType    string
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// 默认请求超时（秒）
const defaultModelTimeoutSeconds = 120

// Anthropic 协议常量
const (
	anthropicAPIVersion = "2023-06-01"
	anthropicMaxTokens  = 4096
)

// AnthropicMessage 是 Anthropic API 的对话消息
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicRequest 是 Anthropic API 请求结构体
type AnthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []AnthropicMessage `json:"messages"`
}

// AnthropicResponse 是 Anthropic API 响应结构体
type AnthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model string `json:"model"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// NewHTTPModel 创建统一模型客户端。
func NewHTTPModel(opts ModelOptions) *HTTPModel {
	apiType := opts.APIType
	if apiType == "" {
		apiType = "openai"
	}
	timeout := opts.TimeoutSeconds
	if timeout <= 0 {
		timeout = defaultModelTimeoutSeconds
	}
	return &HTTPModel{
		apiType: apiType,
		apiKey:  opts.APIKey,
		baseURL: opts.BaseURL,
		model:   opts.Model,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// Generate 调用 LLM 生成文本（实现 ChatModel/LLMClient 接口）。
func (m *HTTPModel) Generate(ctx context.Context, messages []Message) (string, error) {
	completion, err := m.Complete(ctx, messages)
	if err != nil {
		return "", err
	}
	return completion.Content, nil
}

// Completion 是一次补全的结果
type Completion struct {
	Content string
	// Model 取自响应体（provider 可能重定向模型）
	Model string
}

// Complete 调用 LLM 并返回带元数据的补全结果。
func (m *HTTPModel) Complete(ctx context.Context, messages []Message) (*Completion, error) {
	switch m.apiType {
	case "anthropic":
		return m.completeAnthropic(ctx, messages)
	default:
		return m.completeOpenAI(ctx, messages)
	}
}

// ---- Anthropic 协议 ----

func (m *HTTPModel) completeAnthropic(ctx context.Context, messages []Message) (*Completion, error) {
	// system 消息抽离到顶层 system 字段，其余归一为 user/assistant
	var systemMessage string
	var anthropicMessages []AnthropicMessage
	for _, msg := range messages {
		if msg.Role == "system" {
			systemMessage = msg.Content
			continue
		}
		role := msg.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}
		anthropicMessages = append(anthropicMessages, AnthropicMessage{Role: role, Content: msg.Content})
	}

	reqBody := AnthropicRequest{
		Model:     m.model,
		MaxTokens: anthropicMaxTokens,
		System:    systemMessage,
		Messages:  anthropicMessages,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := buildMessagesURL(m.baseURL)
	llmDebug("[Anthropic] Request URL: %s", url)
	llmDebug("[Anthropic] Request Model: %s", m.model)
	llmDebug("[Anthropic] Request Body: %s", string(bodyBytes))

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", m.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		llmDebug("[Anthropic] Request failed: %v", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	llmDebug("[Anthropic] Response Status: %d", resp.StatusCode)
	llmDebug("[Anthropic] Response Body: %s", string(respBody))

	if resp.StatusCode != http.StatusOK {
		var errorResp AnthropicResponse
		if json.Unmarshal(respBody, &errorResp) == nil && errorResp.Error != nil {
			return nil, fmt.Errorf("API error: %s", errorResp.Error.Message)
		}
		return nil, fmt.Errorf("API error, status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	var anthropicResp AnthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	return &Completion{
		Content: anthropicResp.Content[0].Text,
		Model:   anthropicResp.Model,
	}, nil
}

// buildMessagesURL 把 base URL 归一化为 .../v1/messages。
func buildMessagesURL(baseURL string) string {
	u := trimBaseURL(baseURL)
	if u == "" {
		return u
	}
	if strings.HasSuffix(u, "/v1/messages") {
		return u
	}
	if strings.HasSuffix(u, "/v1") {
		u += "/messages"
	} else {
		u += "/v1/messages"
	}
	return u
}

// ---- OpenAI 兼容协议 ----

func (m *HTTPModel) completeOpenAI(ctx context.Context, messages []Message) (*Completion, error) {
	reqBody := OpenAIRequest{
		Model:    m.model,
		Messages: messages,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	llmDebug("[OpenAI] Request URL: %s", buildChatURL(m.baseURL))
	llmDebug("[OpenAI] Request Model: %s", m.model)
	llmDebug("[OpenAI] Request Body: %s", string(bodyBytes))

	req, err := http.NewRequestWithContext(ctx, "POST",
		buildChatURL(m.baseURL), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		llmDebug("[OpenAI] Request failed: %v", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	llmDebug("[OpenAI] Response Status: %d", resp.StatusCode)
	llmDebug("[OpenAI] Response Body: %s", string(respBody))

	if resp.StatusCode != http.StatusOK {
		var errorResp OpenAIResponse
		if json.Unmarshal(respBody, &errorResp) == nil && errorResp.Error != nil {
			return nil, fmt.Errorf("API error: %s", errorResp.Error.Message)
		}
		return nil, fmt.Errorf("API error, status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	var openAIResp OpenAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &Completion{
		Content: openAIResp.Choices[0].Message.Content,
		Model:   openAIResp.Model,
	}, nil
}

// buildChatURL 把任意形态的 base URL 归一化为 .../v1/chat/completions。
func buildChatURL(baseURL string) string {
	u := trimBaseURL(baseURL)
	if u == "" {
		return u
	}
	if strings.HasSuffix(u, "/v1") {
		u += "/chat/completions"
	} else if strings.Contains(u, "/v1/") {
		idx := strings.LastIndex(u, "/v1/")
		u = u[:idx] + "/v1/chat/completions"
	} else if strings.HasSuffix(u, "/chat/completions") {
		return u
	} else {
		u += "/v1/chat/completions"
	}
	return u
}

func trimBaseURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/")
}
