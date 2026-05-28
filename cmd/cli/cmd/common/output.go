package common

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
	"gopkg.in/yaml.v3"
)

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
