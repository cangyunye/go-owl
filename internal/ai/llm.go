package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	llmLogger *zap.SugaredLogger
	llmLogLv  zap.AtomicLevel
)

func init() {
	llmLogLv = zap.NewAtomicLevelAt(zap.WarnLevel)
	config := zap.Config{
		Level:            llmLogLv,
		Development:      false,
		Encoding:         "console",
		EncoderConfig:    zap.NewDevelopmentEncoderConfig(),
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	logger, _ := config.Build()
	llmLogger = logger.Sugar().Named("llm")
}

func SetLLMLogVerbose(verbose bool) {
	if verbose {
		llmLogLv.SetLevel(zap.DebugLevel)
	} else {
		llmLogLv.SetLevel(zap.WarnLevel)
	}
}

func llmDebug(format string, args ...interface{}) {
	llmLogger.Debugf(format, args...)
}

// LLMClient 是 LLM 客户端接口
type LLMClient interface {
	Generate(ctx context.Context, messages []Message) (string, error)
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
	Model string `json:"model"`
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

// ListModels 从 OpenAI 兼容 API 获取可用模型列表
func (m *HTTPModel) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		trimBaseURL(m.baseURL)+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

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
	for _, mo := range modelsResp.Data {
		models = append(models, mo.ID)
	}

	return models, nil
}

// NewOpenAIClient 用 Config 创建 OpenAI 兼容客户端（qwen/dashscope/deepseek 等通用）。
// 兼容构造：内部统一走 HTTPModel。
func NewOpenAIClient(config *Config) *HTTPModel {
	baseURL := config.AI.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return NewHTTPModel(ModelOptions{
		APIType:        "openai",
		BaseURL:        baseURL,
		Model:          config.AI.Model,
		APIKey:         config.AI.APIKey,
		TimeoutSeconds: config.AI.Timeout,
	})
}

// NewAnthropicClient 用 Config 创建 Anthropic 客户端。
// 兼容构造：内部统一走 HTTPModel。
func NewAnthropicClient(config *Config) *HTTPModel {
	baseURL := config.AI.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	model := config.AI.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return NewHTTPModel(ModelOptions{
		APIType:        "anthropic",
		BaseURL:        baseURL,
		Model:          model,
		APIKey:         config.AI.APIKey,
		TimeoutSeconds: config.AI.Timeout,
	})
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
		return NewAnthropicClient(config), nil

	case "qwen", "dashscope":
		if config.AI.BaseURL == "" {
			config.AI.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		}
		if config.AI.Model == "" {
			config.AI.Model = "qwen-max"
		}
		return NewOpenAIClient(config), nil

	case "deepseek":
		if config.AI.BaseURL == "" {
			config.AI.BaseURL = "https://api.deepseek.com"
		}
		if config.AI.Model == "" {
			config.AI.Model = "deepseek-flash"
		}
		return NewOpenAIClient(config), nil

	case "":
		return NewOpenAIClient(config), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", config.AI.Provider)
	}
}
