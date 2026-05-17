package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LLMClient 是 LLM 客户端接口
type LLMClient interface {
	Generate(ctx context.Context, messages []Message) (string, error)
}

// OpenAIClient 实现了 OpenAI 兼容 API 的客户端（用于 Qwen、DeepSeek 等）
type OpenAIClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOpenAIClient 创建一个新的 OpenAI 兼容客户端
func NewOpenAIClient(config *Config) *OpenAIClient {
	baseURL := config.AI.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return &OpenAIClient{
		apiKey:  config.AI.APIKey,
		baseURL: baseURL,
		model:   config.AI.Model,
		httpClient: &http.Client{
			Timeout: time.Duration(config.AI.Timeout) * time.Second,
		},
	}
}

// ListModels 从 API 获取可用模型列表
func (c *OpenAIClient) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp ModelsResponse
		if json.Unmarshal(respBody, &errorResp) == nil && errorResp.Error != nil {
			return nil, fmt.Errorf("API error: %s", errorResp.Error.Message)
		}
		return nil, fmt.Errorf("API error, status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	var modelsResp ModelsResponse
	if err := json.Unmarshal(respBody, &modelsResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var models = make([]string, 0, len(modelsResp.Data))
	for _, m := range modelsResp.Data {
		models = append(models, m.ID)
	}

	return models, nil
}

// OpenAIRequest 是 OpenAI API 请求结构体
type OpenAIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// OpenAIResponse 是 OpenAI API 响应结构体
type OpenAIResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ModelsResponse 是模型列表 API 响应结构体
type ModelsResponse struct {
	Data []struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Owner   string `json:"owner"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Generate 调用 LLM 生成文本
func (c *OpenAIClient) Generate(ctx context.Context, messages []Message) (string, error) {
	reqBody := OpenAIRequest{
		Model:    c.model,
		Messages: messages,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp OpenAIResponse
		if json.Unmarshal(respBody, &errorResp) == nil && errorResp.Error != nil {
			return "", fmt.Errorf("API error: %s", errorResp.Error.Message)
		}
		return "", fmt.Errorf("API error, status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	var openAIResp OpenAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return openAIResp.Choices[0].Message.Content, nil
}

// AnthropicClient 实现了 Anthropic API 的客户端
type AnthropicClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewAnthropicClient 创建一个新的 Anthropic 客户端
func NewAnthropicClient(config *Config) *AnthropicClient {
	return &AnthropicClient{
		apiKey: config.AI.APIKey,
		model:  config.AI.Model,
		httpClient: &http.Client{
			Timeout: time.Duration(config.AI.Timeout) * time.Second,
		},
	}
}

// AnthropicRequest 是 Anthropic API 请求结构体
type AnthropicRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	System    string `json:"system,omitempty"`
	Messages  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// AnthropicResponse 是 Anthropic API 响应结构体
type AnthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Generate 调用 Anthropic API 生成文本
func (c *AnthropicClient) Generate(ctx context.Context, messages []Message) (string, error) {
	// 转换消息格式并分离系统消息
	var systemMessage string
	var anthropicMessages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	for _, msg := range messages {
		if msg.Role == "system" {
			systemMessage = msg.Content
		} else {
			// 确保 role 是 user 或 assistant
			role := msg.Role
			if role != "user" && role != "assistant" {
				role = "user"
			}
			anthropicMessages = append(anthropicMessages, struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{
				Role:    role,
				Content: msg.Content,
			})
		}
	}

	reqBody := AnthropicRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemMessage,
		Messages:  anthropicMessages,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.anthropic.com/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp AnthropicResponse
		if json.Unmarshal(respBody, &errorResp) == nil && errorResp.Error != nil {
			return "", fmt.Errorf("API error: %s", errorResp.Error.Message)
		}
		return "", fmt.Errorf("API error, status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	var anthropicResp AnthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return anthropicResp.Content[0].Text, nil
}

// CreateLLMClient 根据配置创建相应的 LLM 客户端
func CreateLLMClient(config *Config) (LLMClient, error) {
	if config.AI.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	switch config.AI.Provider {
	case "openai":
		if config.AI.BaseURL == "" {
			config.AI.BaseURL = "https://api.openai.com/v1"
		}
		return NewOpenAIClient(config), nil

	case "anthropic":
		if config.AI.Model == "" {
			config.AI.Model = "claude-3-opus-20240229"
		}
		return NewAnthropicClient(config), nil

	case "qwen", "dashscope":
		if config.AI.BaseURL == "" {
			config.AI.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		}
		if config.AI.Model == "" {
			config.AI.Model = "qwen-turbo"
		}
		return NewOpenAIClient(config), nil

	case "deepseek":
		if config.AI.BaseURL == "" {
			config.AI.BaseURL = "https://api.deepseek.com"
		}
		if config.AI.Model == "" {
			config.AI.Model = "deepseek-chat"
		}
		return NewOpenAIClient(config), nil

	case "":
		return NewOpenAIClient(config), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", config.AI.Provider)
	}
}
