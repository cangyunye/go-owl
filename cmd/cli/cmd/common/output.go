package common

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
	"gopkg.in/yaml.v3"
)

// HeaderField 定义表格列
type HeaderField struct {
	Name   string
	Width  int
	Label  string
}

// FieldWidthMap 定义字段的默认宽度和显示名称
var FieldWidthMap = map[string]struct{ Width int; Label string }{
	"id":         {20, "ID"},
	"name":       {25, "Name"},
	"address":    {25, "Address"},
	"port":       {8, "Port"},
	"user":       {10, "User"},
	"status":     {12, "Status"},
	"groups":     {20, "Groups"},
	"labels":     {30, "Labels"},
	"last_check": {20, "Last Check"},
	"metadata":   {30, "Metadata"},
}

// DefaultFields 定义默认的8个字段及其显示顺序
var DefaultFields = []string{"id", "name", "address", "user", "status", "groups", "labels", "last_check"}

// ParseHeaderFields 解析字段定义字符串
// 格式支持:
//   - id,address,labels:60 (仅显示指定字段)
//   - * (显示默认8个字段)
//   - *,id (默认字段 + id放最后)
//   - labels:60,* (labels先显示，然后其他默认字段)
// 返回解析后的字段列表，无效字段会被忽略
func ParseHeaderFields(header string) []HeaderField {
	if header == "" {
		return nil
	}

	parts := strings.Split(header, ",")

	// 解析各个部分，同时记录是否包含通配符
	type parsedPart struct {
		name  string
		width int
		isWildcard bool
	}

	var parsedParts []parsedPart
	hasWildcard := false
	wildcardIndex := -1

	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "*" {
			hasWildcard = true
			wildcardIndex = i
			parsedParts = append(parsedParts, parsedPart{isWildcard: true})
			continue
		}

		// 解析宽度: fieldname:width
		name := part
		width := 0
		if idx := strings.Index(part, ":"); idx != -1 {
			name = strings.TrimSpace(part[:idx])
			widthStr := strings.TrimSpace(part[idx+1:])
			if w, err := strconv.Atoi(widthStr); err == nil && w > 0 {
				width = w
			}
		}

		// 检查字段是否有效
		if _, ok := FieldWidthMap[name]; !ok {
			continue
		}

		parsedParts = append(parsedParts, parsedPart{name: name, width: width})
	}

	// 构建最终字段列表
	seenFields := make(map[string]bool)
	var resultFields []HeaderField

	// 先添加通配符前的字段
	for _, p := range parsedParts[:wildcardIndex+1] {
		if p.isWildcard {
			// 继续
		} else {
			if !seenFields[p.name] {
				fieldInfo := FieldWidthMap[p.name]
				width := p.width
				if width == 0 {
					width = fieldInfo.Width
				}
				resultFields = append(resultFields, HeaderField{
					Name:  p.name,
					Width: width,
					Label: fieldInfo.Label,
				})
				seenFields[p.name] = true
			}
		}
	}

	// 添加通配符的默认字段
	if hasWildcard {
		for _, fieldName := range DefaultFields {
			if !seenFields[fieldName] {
				fieldInfo := FieldWidthMap[fieldName]
				resultFields = append(resultFields, HeaderField{
					Name:  fieldName,
					Width: fieldInfo.Width,
					Label: fieldInfo.Label,
				})
				seenFields[fieldName] = true
			}
		}
	}

	// 添加通配符后的字段
	if hasWildcard && wildcardIndex+1 < len(parsedParts) {
		for _, p := range parsedParts[wildcardIndex+1:] {
			if !p.isWildcard && !seenFields[p.name] {
				fieldInfo := FieldWidthMap[p.name]
				width := p.width
				if width == 0 {
					width = fieldInfo.Width
				}
				resultFields = append(resultFields, HeaderField{
					Name:  p.name,
					Width: width,
					Label: fieldInfo.Label,
				})
				seenFields[p.name] = true
			}
		}
	}

	// 如果没有通配符，就用原来的逻辑
	if !hasWildcard {
		resultFields = nil
		for _, p := range parsedParts {
			if !seenFields[p.name] {
				fieldInfo := FieldWidthMap[p.name]
				width := p.width
				if width == 0 {
					width = fieldInfo.Width
				}
				resultFields = append(resultFields, HeaderField{
					Name:  p.name,
					Width: width,
					Label: fieldInfo.Label,
				})
				seenFields[p.name] = true
			}
		}
	}

	return resultFields
}

// FormatNodesWithFields 使用自定义字段格式化节点列表
func (f *OutputFormatter) FormatNodesWithFields(nodes []*model.Node, fields []HeaderField) {
	if len(nodes) == 0 {
		fmt.Println("No nodes found.")
		return
	}

	if len(fields) == 0 {
		f.printTable(nodes)
		return
	}

	// 打印表头
	headerParts := make([]string, len(fields))
	for i, field := range fields {
		headerParts[i] = PadRight(field.Label, field.Width)
	}
	fmt.Println(strings.Join(headerParts, " "))

	// 打印分隔线
	totalWidth := 0
	for _, field := range fields {
		totalWidth += field.Width + 1
	}
	fmt.Println(strings.Repeat("-", totalWidth))

	// 打印数据行
	for _, n := range nodes {
		rowParts := make([]string, len(fields))
		for i, field := range fields {
			rowParts[i] = PadRight(f.getFieldValue(n, field.Name), field.Width)
		}
		fmt.Println(strings.Join(rowParts, " "))
	}

	fmt.Printf("\nTotal: %d nodes\n", len(nodes))
}

// getFieldValue 获取节点的指定字段值
func (f *OutputFormatter) getFieldValue(n *model.Node, fieldName string) string {
	switch fieldName {
	case "id":
		return n.ID
	case "name":
		return TruncateByWidth(n.Name, 25)
	case "address":
		return truncate(fmt.Sprintf("%s:%d", n.Address, n.Port), 25)
	case "port":
		return fmt.Sprintf("%d", n.Port)
	case "user":
		if n.User == "" {
			return "-"
		}
		return n.User
	case "status":
		status := string(n.Status)
		if f.Color {
			switch n.Status {
			case model.NodeStatusOnline:
				status = greenStr(status)
			case model.NodeStatusOffline:
				status = redStr(status)
			default:
				status = yellowStr(status)
			}
		}
		return status
	case "groups":
		groups := strings.Join(n.Groups, ",")
		if groups == "" {
			return "-"
		}
		return TruncateByWidth(groups, 20)
	case "labels":
		labels := formatLabelsStr(n.Labels)
		if labels == "" {
			return "-"
		}
		return labels
	case "last_check":
		if n.LastCheckAt == "" {
			return "-"
		}
		return truncate(n.LastCheckAt, 20)
	case "metadata":
		metadata := formatLabelsStr(n.Metadata)
		if metadata == "" {
			return "-"
		}
		return metadata
	default:
		return ""
	}
}

// OutputFormatter 输出格式化器
type OutputFormatter struct {
	Format OutputFormat
	Color  bool
}

// NewOutputFormatter 创建输出格式化器
func NewOutputFormatter(format string, color bool) *OutputFormatter {
	f := OutputFormatTable
	switch strings.ToLower(format) {
	case "json", "js":
		f = OutputFormatJSON
	case "yaml", "yml", "y":
		f = OutputFormatYAML
	default:
		f = OutputFormatTable
	}
	return &OutputFormatter{Format: f, Color: color}
}

// FormatNodes 格式化节点列表
func (f *OutputFormatter) FormatNodes(nodes []*model.Node) {
	switch f.Format {
	case OutputFormatJSON:
		f.printJSON(nodes)
	case OutputFormatYAML:
		f.printYAML(nodes)
	default:
		f.printTable(nodes)
	}
}

// FormatNode 格式化单个节点
func (f *OutputFormatter) FormatNode(node *model.Node) {
	switch f.Format {
	case OutputFormatJSON:
		f.printJSON(node)
	case OutputFormatYAML:
		f.printYAML(node)
	default:
		f.printNodeDetail(node)
	}
}

// FormatTaskResults 格式化任务结果
func (f *OutputFormatter) FormatTaskResults(results map[string]*model.Node) {
	switch f.Format {
	case OutputFormatJSON:
		f.printJSON(results)
	case OutputFormatYAML:
		f.printYAML(results)
	default:
		f.printTaskResultsSimple(results)
	}
}

func (f *OutputFormatter) printJSON(v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func (f *OutputFormatter) printYAML(v interface{}) {
	data, err := yaml.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "YAML marshal error: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func (f *OutputFormatter) printTable(nodes []*model.Node) {
	if len(nodes) == 0 {
		fmt.Println("No nodes found.")
		return
	}

	fmt.Printf("%s %s %s %s %s %s %s %s\n",
		PadRight("ID", 20), PadRight("Name", 25), PadRight("Address", 25),
		PadRight("User", 10), PadRight("Status", 12), PadRight("Groups", 20),
		PadRight("Labels", 30), PadRight("Last Check", 20))
	fmt.Println(strings.Repeat("-", 169))

	for _, n := range nodes {
		groups := strings.Join(n.Groups, ",")
		if groups == "" {
			groups = "-"
		}

		labels := formatLabelsStr(n.Labels)
		if labels == "" {
			labels = "-"
		}

		user := n.User
		if user == "" {
			user = "-"
		}

		address := fmt.Sprintf("%s:%d", n.Address, n.Port)

		lastCheck := n.LastCheckAt
		if lastCheck == "" {
			lastCheck = "-"
		}

		status := string(n.Status)
		if f.Color {
			switch n.Status {
			case model.NodeStatusOnline:
				status = greenStr(status)
			case model.NodeStatusOffline:
				status = redStr(status)
			default:
				status = yellowStr(status)
			}
		}

		fmt.Printf("%s %s %s %s %s %s %s %s\n",
			PadRight(n.ID, 20),
			PadRight(TruncateByWidth(n.Name, 25), 25),
			PadRight(truncate(address, 25), 25),
			PadRight(user, 10),
			PadRight(status, 12),
			PadRight(TruncateByWidth(groups, 20), 20),
			PadRight(TruncateByWidth(labels, 30), 30),
			PadRight(truncate(lastCheck, 20), 20))
	}
	fmt.Printf("\nTotal: %d nodes\n", len(nodes))
}

func (f *OutputFormatter) printNodeDetail(node *model.Node) {
	fmt.Println("==============================================")
	fmt.Printf("  Node: %s\n", node.Name)
	fmt.Println("----------------------------------------------")
	fmt.Printf("  ID:          %s\n", node.ID)
	fmt.Printf("  Address:     %s:%d\n", node.Address, node.Port)
	if node.User != "" {
		fmt.Printf("  User:        %s\n", node.User)
	}
	fmt.Printf("  Status:      %s\n", node.Status)

	groups := strings.Join(node.Groups, ", ")
	if groups == "" {
		groups = "(none)"
	}
	fmt.Printf("  Groups:      %s\n", truncate(groups, 40))

	fmt.Printf("  Labels:      %s\n", formatLabelsStr(node.Labels))
	fmt.Printf("  Created:     %s\n", node.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  Updated:     %s\n", node.UpdatedAt.Format(time.RFC3339))
	if node.LastCheckAt != "" {
		fmt.Printf("  Last Check:  %s\n", node.LastCheckAt)
	}
	fmt.Println("==============================================")
}

func (f *OutputFormatter) printTaskResultsSimple(results map[string]*model.Node) {
	success := 0
	failed := 0

	for nodeID, node := range results {
		status := greenStr("OK")
		if node == nil {
			status = redStr("FAIL")
			failed++
		} else {
			success++
		}
		fmt.Printf("[%s] %s: %s\n", status, nodeID, formatNodeStatusStr(node))
	}

	successStr := fmt.Sprintf("%d", success)
	failedStr := fmt.Sprintf("%d", failed)
	if f.Color {
		successStr = greenStr(successStr)
		failedStr = redStr(failedStr)
	}
	fmt.Printf("\nSummary: %s succeeded, %s failed\n", successStr, failedStr)
}

// Helper functions

// PrintLabels 打印标签
func PrintLabels(labels map[string]string) {
	if len(labels) == 0 {
		fmt.Println("  (no labels)")
		return
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s: %s\n", k, labels[k])
	}
}

// formatLabelsStr 格式化标签为字符串
func formatLabelsStr(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(labels))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(parts, ",")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func TruncateByWidth(s string, maxWidth int) string {
	w := 0
	for i, r := range s {
		if r > 127 {
			w += 2
		} else {
			w += 1
		}
		if w > maxWidth {
			return s[:i]
		}
	}
	return s
}

func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r > 127 {
			w += 2
		} else {
			w += 1
		}
	}
	return w
}

func PadRight(s string, width int) string {
	dw := DisplayWidth(s)
	if dw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-dw)
}

func formatNodeStatusStr(node *model.Node) string {
	if node == nil {
		return "not found"
	}
	return string(node.Status)
}

// Color codes - 使用函数避免与常量冲突
const (
	_colorRed    = "\033[31m"
	_colorGreen  = "\033[32m"
	_colorYellow = "\033[33m"
	_colorReset  = "\033[0m"
)

func redStr(s string) string {
	return _colorRed + s + _colorReset
}

func greenStr(s string) string {
	return _colorGreen + s + _colorReset
}

func yellowStr(s string) string {
	return _colorYellow + s + _colorReset
}
