package test

import (
	"testing"

	"github.com/cangyunye/go-owl/internal/ai"
)

func TestParamExtractor_OwnerFilter(t *testing.T) {
	extractor := ai.NewParamExtractor([]string{})

	tests := []struct {
		name     string
		input    string
		expected map[string]interface{}
	}{
		{
			name:  "张三有哪些节点",
			input: "张三有哪些节点",
			expected: map[string]interface{}{
				"labels": map[string]interface{}{"owner": "张三"},
			},
		},
		{
			name:  "负责人张三的节点",
			input: "负责人张三的节点",
			expected: map[string]interface{}{
				"labels": map[string]interface{}{"owner": "张三"},
			},
		},
		{
			name:  "找一下李四的服务器",
			input: "找一下李四的服务器",
			expected: map[string]interface{}{
				"labels": map[string]interface{}{"owner": "李四"},
			},
		},
		{
			name:  "查看王五负责的节点",
			input: "查看王五负责的节点",
			expected: map[string]interface{}{
				"labels": map[string]interface{}{"owner": "王五"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := extractor.ExtractParams(ai.IntentQueryNodes, tt.input)
			
			// 检查 labels 参数是否存在
			labels, ok := params["labels"]
			if !ok {
				t.Errorf("Expected labels param, got: %v", params)
				return
			}
			
			labelMap, ok := labels.(map[string]interface{})
			if !ok {
				t.Errorf("Expected labels to be map, got: %T", labels)
				return
			}
			
			expectedLabels := tt.expected["labels"].(map[string]interface{})
			for k, v := range expectedLabels {
				if labelMap[k] != v {
					t.Errorf("Expected label[%s]=%v, got: %v", k, v, labelMap[k])
				}
			}
		})
	}
}

func TestExtractPersonName(t *testing.T) {
	extractor := ai.NewParamExtractor([]string{})

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"张三有哪些节点", "张三有哪些节点", "张三"},
		{"李四的服务器", "李四的服务器", "李四"},
		{"王五负责的节点", "王五负责的节点", "王五"},
		{"赵六的环境", "赵六的环境", "赵六"},
		{"节点列表", "节点列表", ""},      // 排除词
		{"在线节点", "在线节点", ""},      // 排除词
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := extractor.ExtractParams(ai.IntentQueryNodes, tt.input)
			labels, ok := params["labels"].(map[string]interface{})
			if tt.expected != "" {
				if !ok {
					t.Errorf("Expected labels param, got: %v", params)
					return
				}
				owner, ok := labels["owner"]
				if !ok {
					t.Errorf("Expected owner label, got: %v", labels)
					return
				}
				if owner != tt.expected {
					t.Errorf("Expected owner '%s', got: '%s'", tt.expected, owner)
				}
			} else {
				if ok {
					t.Errorf("Expected no labels, got: %v", labels)
				}
			}
		})
	}
}

func TestParamExtractor_UserFilter(t *testing.T) {
	extractor := ai.NewParamExtractor([]string{})

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"查询所有root用户的主机", "查询所有root用户的主机", "root"},
		{"root用户的服务器", "root用户的服务器", "root"},
		{"用户为admin的节点", "用户为admin的节点", "admin"},
		{"用户是deploy的主机", "用户是deploy的主机", "deploy"},
		{"查看user=test的节点", "查看user=test的节点", "test"},
		{"查看user=prod的主机列表", "查看user=prod的主机列表", "prod"},
		{"所有主机", "所有主机", ""}, // 不应该提取
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := extractor.ExtractParams(ai.IntentQueryNodes, tt.input)
			labels, ok := params["labels"].(map[string]interface{})
			if tt.expected != "" {
				if !ok {
					t.Errorf("Expected labels param, got: %v", params)
					return
				}
				user, ok := labels["user"]
				if !ok {
					t.Errorf("Expected user label, got: %v", labels)
					return
				}
				if user != tt.expected {
					t.Errorf("Expected user '%s', got: '%s'", tt.expected, user)
				}
			}
		})
	}
}

func TestParamExtractor_StatusFilter(t *testing.T) {
	extractor := ai.NewParamExtractor([]string{})

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"在线状态主机有哪些", "在线状态主机有哪些", "online"},
		{"在线节点列表", "在线节点列表", "online"},
		{"离线服务器", "离线服务器", "offline"},
		{"离线的主机", "离线的主机", "offline"},
		{"未知状态的节点", "未知状态的节点", "unknown"},
		{"online节点", "online节点", "online"},
		{"offline主机", "offline主机", "offline"},
		{"所有主机", "所有主机", ""}, // 不应该提取状态
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := extractor.ExtractParams(ai.IntentQueryNodes, tt.input)
			if tt.expected != "" {
				status, ok := params["status"]
				if !ok {
					t.Errorf("Expected status param, got: %v", params)
					return
				}
				if status != tt.expected {
					t.Errorf("Expected status '%s', got: '%s'", tt.expected, status)
				}
			} else {
				if _, ok := params["status"]; ok {
					t.Errorf("Expected no status, got: %v", params["status"])
				}
			}
		})
	}
}
