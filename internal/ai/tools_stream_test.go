package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// OpenAI 流式原生 FC：tool_calls 跨 chunk 聚合（id/name 首块 + arguments 分片拼接）。
func TestHTTPModel_GenerateToolsStream_OpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		chunks := []string{
			`data: {"choices":[{"delta":{"role":"assistant","content":"让我查一下","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"query_nodes","arguments":""}}]}}]}`,
			``,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"gro"}}]}}]}`,
			``,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"up\":\"web\"}"}}]}}]}`,
			``,
			`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
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
	resp, err := client.GenerateToolsStream(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, testToolDefs, func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("GenerateToolsStream failed: %v", err)
	}

	if resp.Content != "让我查一下" {
		t.Errorf("expected content '让我查一下', got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "query_nodes" {
		t.Errorf("unexpected tool call id/name: %+v", tc)
	}
	if tc.Arguments["group"] != "web" {
		t.Errorf("expected aggregated arguments group=web, got %v", tc.Arguments)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish tool_calls, got %q", resp.FinishReason)
	}
	if len(deltas) != 1 || deltas[0] != "让我查一下" {
		t.Errorf("unexpected deltas: %v", deltas)
	}
}

// Anthropic 流式原生 FC：content_block_start(tool_use) + input_json_delta 聚合。
func TestHTTPModel_GenerateToolsStream_Anthropic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		events := []string{
			`data: {"type":"message_start"}`,
			``,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"query_nodes"}}`,
			``,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"group\":"}}`,
			``,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"web\"}"}}`,
			``,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
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

	resp, err := client.GenerateToolsStream(context.Background(),
		[]Message{{Role: "user", Content: "hi"}}, testToolDefs, nil)
	if err != nil {
		t.Fatalf("GenerateToolsStream failed: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_1" || tc.Name != "query_nodes" {
		t.Errorf("unexpected tool call: %+v", tc)
	}
	if tc.Arguments["group"] != "web" {
		t.Errorf("expected aggregated input group=web, got %v", tc.Arguments)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish tool_calls, got %q", resp.FinishReason)
	}
}
