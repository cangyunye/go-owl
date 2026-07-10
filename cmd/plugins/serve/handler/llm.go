package handler

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
}

type LLMResponse struct {
	Content string `json:"content"`
	Model   string `json:"model"`
}

func CallLLM(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	switch req.APIType {
	case "anthropic":
		return callAnthropic(ctx, req)
	default:
		return callOpenAI(ctx, req)
	}
}

func buildChatURL(baseURL string) string {
	u := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(u, "/v1") {
		u += "/chat/completions"
	} else if strings.Contains(u, "/v1/") {
		if idx := strings.LastIndex(u, "/v1/"); idx >= 0 {
			u = u[:idx] + "/v1/chat/completions"
		} else {
			u += "/v1/chat/completions"
		}
	} else if strings.HasSuffix(u, "/chat/completions") {
		return u
	} else {
		u += "/v1/chat/completions"
	}
	return u
}

func callOpenAI(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	url := buildChatURL(req.BaseURL)

	chatReq := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
	}
	body, _ := json.Marshal(chatReq)

	httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("parse response failed: %w", err)
	}
	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &LLMResponse{
		Content: raw.Choices[0].Message.Content,
		Model:   raw.Model,
	}, nil
}

func callAnthropic(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	url := strings.TrimRight(req.BaseURL, "/")
	if !strings.HasSuffix(url, "/v1/messages") {
		if strings.HasSuffix(url, "/v1") {
			url += "/messages"
		} else {
			url += "/v1/messages"
		}
	}

	var msgs []map[string]interface{}
	for _, m := range req.Messages {
		if m.Role == "system" {
			continue
		}
		msgs = append(msgs, map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	systemPrompt := ""
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			break
		}
	}

	chatReq := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": 4096,
		"messages":   msgs,
	}
	if systemPrompt != "" {
		chatReq["system"] = systemPrompt
	}

	body, _ := json.Marshal(chatReq)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	httpReq.Header.Set("x-api-key", req.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var raw struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("parse response failed: %w", err)
	}
	if len(raw.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	return &LLMResponse{
		Content: raw.Content[0].Text,
		Model:   raw.Model,
	}, nil
}
