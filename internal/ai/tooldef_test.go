package ai

import (
	"context"
	"encoding/json"
	"testing"
)

// TestToolRegistry_ToolDefinitions_ValidSchema 注册表导出的每个工具定义
// 必须携带合法的 JSON Schema 对象（原生 function calling 的 wire format 依赖它）。
func TestToolRegistry_ToolDefinitions_ValidSchema(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&QueryNodesTool{})
	registry.Register(&ExecuteCommandTool{})

	defs := registry.ToolDefinitions()
	if len(defs) != 2 {
		t.Fatalf("expected 2 tool defs, got %d", len(defs))
	}

	byName := map[string]ToolDef{}
	for _, d := range defs {
		byName[d.Name] = d
	}

	qn, ok := byName["query_nodes"]
	if !ok {
		t.Fatal("expected query_nodes in defs")
	}
	if qn.Description == "" {
		t.Error("expected non-empty description")
	}
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(qn.Schema), &schema); err != nil {
		t.Fatalf("query_nodes schema is not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected schema type object, got %v", schema["type"])
	}

	ec, ok := byName["execute_command"]
	if !ok {
		t.Fatal("expected execute_command in defs")
	}
	if err := json.Unmarshal([]byte(ec.Schema), &schema); err != nil {
		t.Fatalf("execute_command schema is not valid JSON: %v", err)
	}
}

// TestToolRegistry_ToolDefinitions_StableOrder 定义顺序必须稳定（决定 API 请求
// 与提示词的可复现性）。
func TestToolRegistry_ToolDefinitions_StableOrder(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&ExecuteCommandTool{})
	registry.Register(&QueryNodesTool{})

	first := registry.ToolDefinitions()
	second := registry.ToolDefinitions()
	if len(first) != len(second) {
		t.Fatal("def counts differ between calls")
	}
	for i := range first {
		if first[i].Name != second[i].Name {
			t.Fatalf("order unstable: index %d is %s then %s", i, first[i].Name, second[i].Name)
		}
	}
}

// TestToolRegistry_ToolDefinitions_InvalidSchemaFallbacks 单个工具的 Parameters
// 不是合法 JSON 时必须退化为 {"type":"object"}，不能拖垮整体导出。
func TestToolRegistry_ToolDefinitions_InvalidSchemaFallbacks(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&brokenSchemaTool{})

	defs := registry.ToolDefinitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}
	if defs[0].Schema != `{"type":"object"}` {
		t.Errorf("expected fallback schema, got %s", defs[0].Schema)
	}
}

type brokenSchemaTool struct{}

func (t *brokenSchemaTool) Name() string                                 { return "broken_schema" }
func (t *brokenSchemaTool) Description() string                          { return "tool with broken schema" }
func (t *brokenSchemaTool) Parameters() string                           { return "not-json{{" }
func (t *brokenSchemaTool) Validate(params map[string]interface{}) error { return nil }
func (t *brokenSchemaTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	return "", nil
}
