package handler

import (
	"context"

	ai2 "github.com/cangyunye/go-owl/internal/ai"
)

type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMRequest struct {
	APIKey   string
	BaseURL  string
	Model    string
	APIType  string // "openai" or "anthropic"
	Messages []LLMMessage
	// RequestedBy 请求者用户名（serve 内部：安全审计身份），非 API 字段
	RequestedBy string `json:"-"`
}

type LLMResponse struct {
	Content string `json:"content"`
	Model   string `json:"model"`
}

// CallLLM 调用 LLM 补全。协议实现统一委托 internal/ai 的 HTTPModel，
// 本文件仅保留 Web 侧的请求/响应契约。
func CallLLM(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	client := ai2.NewHTTPModel(ai2.ModelOptions{
		APIType: req.APIType,
		BaseURL: req.BaseURL,
		Model:   req.Model,
		APIKey:  req.APIKey,
	})

	msgs := make([]ai2.Message, len(req.Messages))
	for i, msg := range req.Messages {
		msgs[i] = ai2.Message{Role: msg.Role, Content: msg.Content}
	}

	completion, err := client.Complete(ctx, msgs)
	if err != nil {
		return nil, err
	}
	return &LLMResponse{
		Content: completion.Content,
		Model:   completion.Model,
	}, nil
}
