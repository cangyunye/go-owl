package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var testToolDefs = []ToolDef{
	{
		Name:        "query_nodes",
		Description: "List nodes",
		Schema:      `{"type":"object","properties":{"group":{"type":"string"}}}`,
	},
}

// OpenAI：GenerateTools 请求必须携带 tools 数组，响应 tool_calls 必须解析为 ToolCall。
func TestHTTPModel_GenerateTools_OpenAI(t *testing.T) {
	var gotTools []interface{}
	var gotMessages []interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		gotTools, _ = body["tools"].([]interface{})
		gotMessages, _ = body["messages"].([]interface{})

		json.NewEncoder(w).Encode(map[string]interface{}{
			"model": "mock-model",
			"choices": []map[string]interface{}{
				{
					"finish_reason": "tool_calls",
					"message": map[string]interface{}{
						"role": "assistant",
						"content": "",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "query_nodes",
									"arguments": `{"group":"web"}`,
								},
							},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewHTTPModel(ModelOptions{APIType: "openai", BaseURL: server.URL, Model: "mock-model", APIKey: "k"})

	// 模拟上一轮：assistant 发起调用 → tool 返回结果（验证消息编码）
	msgs := []Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []MessageToolCall{{ID: "call_1", Name: "query_nodes", ArgsJSON: `{"group":"web"}`}}},
		{Role: "tool", ToolCallID: "call_1", Content: "node-a, node-b"},
	}

	resp, err := client.GenerateTools(context.Background(), msgs, testToolDefs)
	if err != nil {
		t.Fatalf("GenerateTools failed: %v", err)
	}

	// 请求侧：tools 数组按 OpenAI function 格式编码
	if len(gotTools) != 1 {
		t.Fatalf("expected 1 tool in request, got %d", len(gotTools))
	}
	tool := gotTools[0].(map[string]interface{})
	if tool["type"] != "function" {
		t.Errorf("expected tool type function, got %v", tool["type"])
	}
	fn := tool["function"].(map[string]interface{})
	if fn["name"] != "query_nodes" {
		t.Errorf("expected function name query_nodes, got %v", fn["name"])
	}

	// 请求侧：assistant tool_calls / tool 结果消息按 wire format 编码
	var assistantToolCalls, toolRoleMsg interface{}
	for _, m := range gotMessages {
		mm := m.(map[string]interface{})
		if mm["role"] == "assistant" && mm["tool_calls"] != nil {
			assistantToolCalls = mm["tool_calls"]
		}
		if mm["role"] == "tool" {
			toolRoleMsg = mm
		}
	}
	if assistantToolCalls == nil {
		t.Error("expected assistant message with tool_calls in request")
	}
	if toolRoleMsg == nil {
		t.Fatal("expected role=tool message in request")
	}
	tm := toolRoleMsg.(map[string]interface{})
	if tm["tool_call_id"] != "call_1" {
		t.Errorf("expected tool_call_id call_1, got %v", tm["tool_call_id"])
	}
	if tm["content"] != "node-a, node-b" {
		t.Errorf("expected tool result content, got %v", tm["content"])
	}

	// 响应侧：tool_calls 解析为 ToolCall
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "query_nodes" {
		t.Errorf("expected tool name query_nodes, got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].ID != "call_1" {
		t.Errorf("expected tool call id call_1, got %q", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Arguments["group"] != "web" {
		t.Errorf("expected group=web argument, got %v", resp.ToolCalls[0].Arguments)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason tool_calls, got %q", resp.FinishReason)
	}
}

// OpenAI：provider 不支持 tools（400）时返回可识别的 ErrToolsUnsupported。
func TestHTTPModel_GenerateTools_OpenAI_Unsupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"tools is not supported"}}`))
	}))
	defer server.Close()

	client := NewHTTPModel(ModelOptions{APIType: "openai", BaseURL: server.URL, Model: "m", APIKey: "k"})
	_, err := client.GenerateTools(context.Background(), []Message{{Role: "user", Content: "hi"}}, testToolDefs)
	if !errors.Is(err, ErrToolsUnsupported) {
		t.Fatalf("expected ErrToolsUnsupported, got %v", err)
	}
}

// Anthropic：tools 编码为 input_schema；tool_use 解析；role=tool 消息归并为
// user 消息中的 tool_result 块；system 多条合并。
func TestHTTPModel_GenerateTools_Anthropic(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"model": "claude-mock",
			"content": []map[string]interface{}{
				{"type": "tool_use", "id": "toolu_1", "name": "query_nodes", "input": map[string]interface{}{"group": "web"}},
			},
			"stop_reason": "tool_use",
		})
	}))
	defer server.Close()

	client := NewHTTPModel(ModelOptions{APIType: "anthropic", BaseURL: server.URL, Model: "claude-mock", APIKey: "k"})

	msgs := []Message{
		{Role: "system", Content: "sys one"},
		{Role: "system", Content: "sys two"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []MessageToolCall{{ID: "toolu_1", Name: "query_nodes", ArgsJSON: `{"group":"web"}`}}},
		{Role: "tool", ToolCallID: "toolu_1", Content: "node-a"},
	}

	resp, err := client.GenerateTools(context.Background(), msgs, testToolDefs)
	if err != nil {
		t.Fatalf("GenerateTools failed: %v", err)
	}

	// tools → input_schema
	tools, _ := gotBody["tools"].([]interface{})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool in request, got %d", len(tools))
	}
	td := tools[0].(map[string]interface{})
	if td["input_schema"] == nil {
		t.Error("expected input_schema in anthropic tool def")
	}

	// system 多条合并
	if gotBody["system"] != "sys one\n\nsys two" {
		t.Errorf("expected merged system, got %v", gotBody["system"])
	}

	// role=tool → user 消息中的 tool_result 块
	msgArr, _ := gotBody["messages"].([]interface{})
	var sawToolResult bool
	for _, m := range msgArr {
		mm := m.(map[string]interface{})
		if mm["role"] == "tool" {
			t.Error("role=tool must not appear in anthropic messages")
		}
		if blocks, ok := mm["content"].([]interface{}); ok {
			for _, b := range blocks {
				bb := b.(map[string]interface{})
				if bb["type"] == "tool_result" {
					sawToolResult = true
					if bb["tool_use_id"] != "toolu_1" {
						t.Errorf("expected tool_use_id toolu_1, got %v", bb["tool_use_id"])
					}
				}
			}
		}
	}
	if !sawToolResult {
		t.Error("expected tool_result block in a user message")
	}

	// 响应：tool_use 解析
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "query_nodes" || resp.ToolCalls[0].ID != "toolu_1" {
		t.Errorf("unexpected tool call %+v", resp.ToolCalls[0])
	}
	if resp.ToolCalls[0].Arguments["group"] != "web" {
		t.Errorf("expected group=web, got %v", resp.ToolCalls[0].Arguments)
	}
	if !strings.Contains(resp.FinishReason, "tool") {
		t.Errorf("expected finish reason to reflect tool use, got %q", resp.FinishReason)
	}
}
