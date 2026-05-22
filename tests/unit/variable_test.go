package unit

import (
	"strings"
	"testing"
)

func TestVariableInterpolation(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     map[string]string
		expected string
	}{
		{
			name:     "简单变量替换",
			template: "echo {{name}}",
			vars:     map[string]string{"name": "world"},
			expected: "echo world",
		},
		{
			name:     "多个变量",
			template: "{{greeting}} {{name}}!",
			vars:     map[string]string{"greeting": "Hello", "name": "World"},
			expected: "Hello World!",
		},
		{
			name:     "带路径的变量",
			template: "cd {{base_path}}/{{app_name}}",
			vars:     map[string]string{"base_path": "/opt", "app_name": "myapp"},
			expected: "cd /opt/myapp",
		},
		{
			name:     "无变量",
			template: "echo hello",
			vars:     map[string]string{},
			expected: "echo hello",
		},
		{
			name:     "不存在的变量保留原样",
			template: "echo {{undefined}}",
			vars:     map[string]string{},
			expected: "echo {{undefined}}",
		},
		{
			name:     "版本号变量",
			template: "app-{{version}}.tar.gz",
			vars:     map[string]string{"version": "1.0.0"},
			expected: "app-1.0.0.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := interpolate(tt.template, tt.vars)
			if result != tt.expected {
				t.Errorf("interpolate(%q, %v) = %q, want %q", tt.template, tt.vars, result, tt.expected)
			}
		})
	}
}

func interpolate(template string, vars map[string]string) string {
	result := template
	for key, value := range vars {
		placeholder := "{{" + key + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

func TestSplitLabelEq(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "正常标签",
			input:    "env=prod",
			expected: []string{"env", "prod"},
		},
		{
			name:     "包含数字",
			input:    "count=123",
			expected: []string{"count", "123"},
		},
		{
			name:     "没有等号",
			input:    "nolabel",
			expected: []string{"nolabel"},
		},
		{
			name:     "多个等号",
			input:    "k=v=x",
			expected: []string{"k", "v=x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLabelEq(tt.input)
			if !sliceEqual(result, tt.expected) {
				t.Errorf("splitLabelEq(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func splitLabelEq(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substrs  []string
		expected bool
	}{
		{
			name:     "包含第一个",
			s:        "auth failed",
			substrs:  []string{"auth", "password"},
			expected: true,
		},
		{
			name:     "包含第二个",
			s:        "password incorrect",
			substrs:  []string{"auth", "password"},
			expected: true,
		},
		{
			name:     "都不包含",
			s:        "connection timeout",
			substrs:  []string{"auth", "password"},
			expected: false,
		},
		{
			name:     "空字符串",
			s:        "",
			substrs:  []string{"auth"},
			expected: false,
		},
		{
			name:     "空子串列表",
			s:        "some text",
			substrs:  []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAny(tt.s, tt.substrs...)
			if result != tt.expected {
				t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.substrs, result, tt.expected)
			}
		})
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
