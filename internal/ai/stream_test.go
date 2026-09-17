package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// OpenAI SSE：delta 逐段回调并拼接为完整内容；请求必须带 stream:true。
func TestHTTPModel_GenerateStream_OpenAI(t *testing.T) {
	var sawStreamFlag bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		sawStreamFlag = body["stream"] == true

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		chunks := []string{
			`data: {"choices":[{"delta":{"content":"He"}}]}`,
			``,
			`data: {"choices":[{"delta":{"content":"llo"}}]}`,
			``,
			`data: [DONE]`,
			``,
		}
		for _, c := range chunks {
			w.Write([]byte(c + "\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := NewHTTPModel(ModelOptions{APIType: "openai", BaseURL: server.URL, Model: "m", APIKey: "k"})

	var deltas []string
	content, err := client.GenerateStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatalf("GenerateStream failed: %v", err)
	}
	if content != "Hello" {
		t.Fatalf("expected 'Hello', got %q", content)
	}
	if len(deltas) != 2 || deltas[0] != "He" || deltas[1] != "llo" {
		t.Fatalf("unexpected deltas: %v", deltas)
	}
	if !sawStreamFlag {
		t.Error("expected stream:true in request body")
	}
}

// Anthropic SSE：content_block_delta 事件解析。
func TestHTTPModel_GenerateStream_Anthropic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		events := []string{
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Bon"}}`,
			``,
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"jour"}}`,
			``,
			`data: {"type":"message_stop"}`,
			``,
		}
		for _, e := range events {
			w.Write([]byte(e + "\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := NewHTTPModel(ModelOptions{APIType: "anthropic", BaseURL: server.URL, Model: "m", APIKey: "k"})

	content, err := client.GenerateStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("GenerateStream failed: %v", err)
	}
	if content != "Bonjour" {
		t.Fatalf("expected 'Bonjour', got %q", content)
	}
}

// SSE 中途断开必须返回错误而非静默截断。
func TestHTTPModel_GenerateStream_OpenAI_ErrorEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		w.Write([]byte(`data: {"error":{"message":"overloaded"}}` + "\n\n"))
	}))
	defer server.Close()

	client := NewHTTPModel(ModelOptions{APIType: "openai", BaseURL: server.URL, Model: "m", APIKey: "k"})
	_, err := client.GenerateStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error from SSE error payload")
	}
	if !strings.Contains(err.Error(), "overloaded") {
		t.Errorf("expected provider error message in error, got %v", err)
	}
}
