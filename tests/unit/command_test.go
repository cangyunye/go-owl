package unit

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseNodeList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "单个节点",
			input:    "node1",
			expected: []string{"node1"},
		},
		{
			name:     "两个节点",
			input:    "node1,node2",
			expected: []string{"node1", "node2"},
		},
		{
			name:     "多个节点",
			input:    "node1,node2,node3",
			expected: []string{"node1", "node2", "node3"},
		},
		{
			name:     "带空格的节点列表",
			input:    "node1, node2, node3",
			expected: []string{"node1", "node2", "node3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNodeList(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseNodeList(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func parseNodeList(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			if current != "" {
				result = append(result, strings.TrimSpace(current))
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, strings.TrimSpace(current))
	}
	return result
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "多行文本",
			input:    "line1\nline2\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "单行文本",
			input:    "line1",
			expected: []string{"line1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLines(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("splitLines(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func splitLines(s string) []string {
	var lines []string
	current := ""
	for _, c := range s {
		if c == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "短字符串",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "等长字符串",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "长字符串",
			input:    "hello world",
			maxLen:   8,
			expected: "hello...",
		},
		{
			name:     "空字符串",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func TestEscapeJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "普通字符串",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "包含引号",
			input:    `hello "world"`,
			expected: `hello \"world\"`,
		},
		{
			name:     "包含换行",
			input:    "line1\nline2",
			expected: "line1\\nline2",
		},
		{
			name:     "包含反斜杠",
			input:    `path\to\file`,
			expected: `path\\to\\file`,
		},
		{
			name:     "包含制表符",
			input:    "col1\tcol2",
			expected: "col1\\tcol2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeJSON(tt.input)
			if result != tt.expected {
				t.Errorf("escapeJSON(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func escapeJSON(s string) string {
	var result strings.Builder
	for _, c := range s {
		switch c {
		case '"':
			result.WriteString(`\"`)
		case '\\':
			result.WriteString(`\\`)
		case '\n':
			result.WriteString(`\n`)
		case '\r':
			result.WriteString(`\r`)
		case '\t':
			result.WriteString(`\t`)
		default:
			result.WriteRune(c)
		}
	}
	return result.String()
}

func TestContainsStringList(t *testing.T) {
	tests := []struct {
		name     string
		list     []string
		s        string
		expected bool
	}{
		{
			name:     "列表包含元素",
			list:     []string{"a", "b", "c"},
			s:        "b",
			expected: true,
		},
		{
			name:     "列表不包含元素",
			list:     []string{"a", "b", "c"},
			s:        "d",
			expected: false,
		},
		{
			name:     "空列表",
			list:     []string{},
			s:        "a",
			expected: false,
		},
		{
			name:     "单元素列表",
			list:     []string{"only"},
			s:        "only",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsStringList(tt.list, tt.s)
			if result != tt.expected {
				t.Errorf("containsStringList(%v, %q) = %v, want %v", tt.list, tt.s, result, tt.expected)
			}
		})
	}
}

func containsStringList(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
