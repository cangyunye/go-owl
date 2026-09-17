package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockOpenAIServer 启动一个模拟 OpenAI 兼容端点，校验请求路径/头/体后返回固定回复。
func mockOpenAIServer(t *testing.T, wantPath string, check func(r *http.Request, body map[string]interface{})) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantPath != "" && r.URL.Path != wantPath {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if check != nil {
			check(r, body)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"model": "mock-model",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"role": "assistant", "content": "Hello from LLM"}},
			},
		})
	}))
}

func TestBuildChatURL_Normalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.openai.com", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"},
		{"https://api.deepseek.com/v1/chat/completions", "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.example.com/v1/", "https://api.example.com/v1/chat/completions"},
		{"https://api.deepseek.com", "https://api.deepseek.com/v1/chat/completions"},
	}
	for _, tc := range tests {
		got := buildChatURL(tc.input)
		if got != tc.expected {
			t.Errorf("buildChatURL(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestNewHTTPModel_Anthropic_RoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		if r.Header.Get("x-api-key") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("anthropic-version") == "" {
			http.Error(w, "missing anthropic-version", http.StatusBadRequest)
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		// system 消息必须抽离到顶层 system 字段，不能混入 messages
		if body["system"] != "sys prompt" {
			http.Error(w, "missing system field", http.StatusBadRequest)
			return
		}
		msgs, _ := body["messages"].([]interface{})
		for _, m := range msgs {
			mm := m.(map[string]interface{})
			if mm["role"] == "system" {
				http.Error(w, "system message must not appear in messages", http.StatusBadRequest)
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"model": "claude-mock",
			"content": []map[string]interface{}{
				{"type": "text", "text": "Hello from Claude"},
			},
		})
	}))
	defer server.Close()

	client := NewHTTPModel(ModelOptions{
		APIType: "anthropic",
		BaseURL: server.URL,
		Model:   "claude-mock",
		APIKey:  "test-key",
	})

	content, err := client.Generate(context.Background(), []Message{
		{Role: "system", Content: "sys prompt"},
		{Role: "user", Content: "Hi"},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if content != "Hello from Claude" {
		t.Fatalf("expected 'Hello from Claude', got %q", content)
	}
}

func TestNewHTTPModel_Complete_CapturesModel(t *testing.T) {
	server := mockOpenAIServer(t, "", nil)
	defer server.Close()

	client := NewHTTPModel(ModelOptions{
		APIType: "openai",
		BaseURL: server.URL,
		Model:   "mock-model",
		APIKey:  "test-key",
	})

	completion, err := client.Complete(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if completion.Content != "Hello from LLM" {
		t.Fatalf("expected 'Hello from LLM', got %q", completion.Content)
	}
	// 模型名必须取自响应体（provider 可能重定向模型），而非请求参数
	if completion.Model != "mock-model" {
		t.Fatalf("expected response model 'mock-model', got %q", completion.Model)
	}
}

func TestNewHTTPModel_OpenAI_RoundTrip(t *testing.T) {
	server := mockOpenAIServer(t, "/v1/chat/completions", func(r *http.Request, body map[string]interface{}) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected bearer auth, got %q", r.Header.Get("Authorization"))
		}
		if body["model"] != "mock-model" {
			t.Errorf("expected model in request body, got %v", body["model"])
		}
	})
	defer server.Close()

	client := NewHTTPModel(ModelOptions{
		APIType: "openai",
		BaseURL: server.URL,
		Model:   "mock-model",
		APIKey:  "test-key",
	})

	content, err := client.Generate(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if content != "Hello from LLM" {
		t.Fatalf("expected 'Hello from LLM', got %q", content)
	}
}
