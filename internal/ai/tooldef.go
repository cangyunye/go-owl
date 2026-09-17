package ai

import (
	"encoding/json"
	"sort"
)

// ToolDef 是工具的原生 function calling 定义。
// Schema 为 JSON Schema 对象的字符串形式（来源于工具自身的 Parameters()）。
type ToolDef struct {
	Name        string
	Description string
	Schema      string
}

// fallbackSchema 是 Parameters() 无法解析为合法 JSON 时的兜底 schema，
// 保证单个工具定义损坏不拖垮整体导出。
const fallbackSchema = `{"type":"object"}`

// ToolDefinitions 导出全部工具的原生 function calling 定义，按名称排序保证稳定。
func (r *ToolRegistry) ToolDefinitions() []ToolDef {
	tools := r.ListAll()
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name() < tools[j].Name() })

	defs := make([]ToolDef, 0, len(tools))
	for _, tool := range tools {
		schema := tool.Parameters()
		if schema == "" || !json.Valid([]byte(schema)) {
			schema = fallbackSchema
		}
		defs = append(defs, ToolDef{
			Name:        tool.Name(),
			Description: tool.Description(),
			Schema:      schema,
		})
	}
	return defs
}
