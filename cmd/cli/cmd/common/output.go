package common

import (
	"encoding/json"
	"fmt"
	"os"
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

	// 表头
	fmt.Printf("%-10s %-15s %-18s %-6s %-10s %-20s %-15s\n",
		"ID", "Name", "Address", "Port", "Status", "Groups", "Labels")
	fmt.Println(strings.Repeat("-", 100))

	// 表格数据
	for _, n := range nodes {
		groups := strings.Join(n.Groups, ",")
		if groups == "" {
			groups = "-"
		}

		labels := formatLabelsStr(n.Labels)
		if labels == "" {
			labels = "-"
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

		fmt.Printf("%-10s %-15s %-18s %-6d %-10s %-20s %-15s\n",
			n.ID, n.Name, n.Address, n.Port, status, truncate(groups, 20), truncate(labels, 15))
	}
	fmt.Printf("\nTotal: %d nodes\n", len(nodes))
}

func (f *OutputFormatter) printNodeDetail(node *model.Node) {
	fmt.Println("==============================================")
	fmt.Printf("  Node: %s\n", node.Name)
	fmt.Println("----------------------------------------------")
	fmt.Printf("  ID:       %s\n", node.ID)
	fmt.Printf("  Address:  %s:%d\n", node.Address, node.Port)
	fmt.Printf("  Status:   %s\n", node.Status)

	groups := strings.Join(node.Groups, ", ")
	if groups == "" {
		groups = "(none)"
	}
	fmt.Printf("  Groups:   %s\n", truncate(groups, 40))

	fmt.Printf("  Labels:   %s\n", formatLabelsStr(node.Labels))
	fmt.Printf("  Created:  %s\n", node.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  Updated:  %s\n", node.UpdatedAt.Format(time.RFC3339))
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
	for k, v := range labels {
		fmt.Printf("  %s: %s\n", k, v)
	}
}

// formatLabelsStr 格式化标签为字符串
func formatLabelsStr(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ",")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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
