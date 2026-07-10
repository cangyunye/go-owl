package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCallLLM_OpenAI(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "gpt-4o" {
			http.Error(w, "wrong model", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "Hello from LLM"}},
			},
			"model": "gpt-4o",
		})
	}))
	defer mock.Close()

	resp, err := CallLLM(context.Background(), &LLMRequest{
		APIKey:   "test-key",
		BaseURL:  mock.URL,
		Model:    "gpt-4o",
		APIType:  "openai",
		Messages: []LLMMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("CallLLM failed: %v", err)
	}
	if resp.Content != "Hello from LLM" {
		t.Fatalf("expected 'Hello from LLM', got %q", resp.Content)
	}
	if resp.Model != "gpt-4o" {
		t.Fatalf("expected model 'gpt-4o', got %q", resp.Model)
	}
}

func TestCallLLM_OpenAI_WrongKey(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
	}))
	defer mock.Close()

	_, err := CallLLM(context.Background(), &LLMRequest{
		APIKey:   "bad-key",
		BaseURL:  mock.URL,
		Model:    "gpt-4o",
		APIType:  "openai",
		Messages: []LLMMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for bad key")
	}
}

func TestCallLLM_Anthropic(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"text": "Hello from Claude"},
			},
			"model": "claude-sonnet-4-20250514",
		})
	}))
	defer mock.Close()

	resp, err := CallLLM(context.Background(), &LLMRequest{
		APIKey:   "test-key",
		BaseURL:  mock.URL,
		Model:    "claude-sonnet-4-20250514",
		APIType:  "anthropic",
		Messages: []LLMMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("CallLLM failed: %v", err)
	}
	if resp.Content != "Hello from Claude" {
		t.Fatalf("expected 'Hello from Claude', got %q", resp.Content)
	}
	if resp.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected model 'claude-sonnet-4-20250514', got %q", resp.Model)
	}
}

func TestBuildChatURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.openai.com", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"},
		{"https://api.deepseek.com/v1/chat/completions", "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.example.com/v1/", "https://api.example.com/v1/chat/completions"},
	}
	for _, tc := range tests {
		got := buildChatURL(tc.input)
		if got != tc.expected {
			t.Errorf("buildChatURL(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
