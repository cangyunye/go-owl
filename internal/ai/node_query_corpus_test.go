package ai

import (
	"strings"
	"testing"

	aiPrompts "github.com/cangyunye/go-owl/internal/ai/prompts"
)

// 节点查询语料回归测试：用户报告的失效问法（列出下线/离线节点、查询xx的机器/主机）
// 曾从未进过版本库（见 bbf5afc 删除硬编码 group 推断后无正规提取、分类器无「机器」
// 关键词），本文件作为重建后的语料基线，先红后绿逐条修复。

func TestIntentClassifierNodeCorpus(t *testing.T) {
	c := NewIntentClassifier()
	cases := []struct {
		in   string
		want IntentType
	}{
		// 用户报告的失效语料
		{"查询web的机器", IntentQueryNodes},
		{"查询web的主机", IntentQueryNodes},
		{"查询web的服务器", IntentQueryNodes},
		{"web组有哪些机器", IntentQueryNodes},
		{"列出所有下线节点", IntentQueryNodes},
		{"列出所有离线节点", IntentQueryNodes},
		// 回归：既有语义不回退
		{"列出所有节点", IntentQueryNodes},
		{"列出所有在线节点", IntentQueryNodes},
		{"有哪些节点在告警", IntentAlertList},
		{"帮我修复OWL-DSK-001告警的机器", IntentAlertRemedy},
	}
	for _, tc := range cases {
		got := c.Classify(tc.in)
		// defaultChatHandler 对 Confidence < 20 按 Uncertain 拒绝（不调工具），
		// 语料断言必须连同置信度门槛一起校验才是用户可见行为
		if got.Type != tc.want || got.Confidence < 20 {
			t.Errorf("Classify(%q) = %s (conf=%d), want %s with conf>=20", tc.in, got.Type, got.Confidence, tc.want)
		}
	}
}

func TestParamExtractorNodeGroupCorpus(t *testing.T) {
	e := NewParamExtractor([]string{"web-01", "cache-01"}, []string{"web", "db", "cache"})
	cases := []struct {
		in        string
		wantGroup string
	}{
		// 用户报告的失效语料
		{"查询web的机器", "web"},
		{"查询db的主机", "db"},
		{"web组有哪些机器", "web"},
		{"查询web节点", "web"},
		// 不应误提
		{"列出所有节点", ""},
		{"查询root用户的主机", ""},   // root 非已知分组，owner 走标签路径
		{"查询cache-01的机器", ""}, // 命中的是具体节点名，不是分组
	}
	for _, tc := range cases {
		params := e.ExtractParams(IntentQueryNodes, tc.in)
		if got, _ := params["group"].(string); got != tc.wantGroup {
			t.Errorf("ExtractParams(%q) group = %q, want %q", tc.in, got, tc.wantGroup)
		}
	}
}

func TestParamExtractorNodeStatusCorpus(t *testing.T) {
	e := NewParamExtractor(nil, []string{"web"})
	cases := []struct {
		in         string
		wantStatus string
	}{
		// 用户报告的失效语料：「下线」曾被完全忽略，全部节点原样列出
		{"列出所有下线节点", "offline"},
		{"列出所有离线节点", "offline"},
		{"哪些节点掉线了", "offline"},
		// 回归：「不在线」含「在线」子串，曾被先匹配的 online 分支误判
		{"哪些节点不在线", "offline"},
		{"列出所有在线节点", "online"},
		{"unknown status nodes", "unknown"},
		{"未知状态的节点", "unknown"},
		{"列出所有节点", ""},
	}
	for _, tc := range cases {
		params := e.ExtractParams(IntentQueryNodes, tc.in)
		if got, _ := params["status"].(string); got != tc.wantStatus {
			t.Errorf("ExtractParams(%q) status = %q, want %q", tc.in, got, tc.wantStatus)
		}
	}
}

// TestNodeQueryCorpusPromptGuidance LLM 路径语料：合并路由下 LLM 只看工具目录与
// 场景提示词，「机器/服务器」同义词与「下线→offline」映射必须写进引导材料。
func TestNodeQueryCorpusPromptGuidance(t *testing.T) {
	prompt := aiPrompts.NodeListSystemPrompt
	for _, want := range []string{"机器", "服务器", `"查询web的机器"`, `"列出所有下线节点"`} {
		if !strings.Contains(prompt, want) {
			t.Errorf("NodeListSystemPrompt should mention %q", want)
		}
	}
	for _, shot := range []string{`"查询web的机器" → node_list`, `"列出所有下线节点" → query_nodes`} {
		if !strings.Contains(aiPrompts.RouterPrompt, shot) {
			t.Errorf("RouterPrompt missing few-shot %s", shot)
		}
	}
	if d := NewQueryNodesTool(nil, nil, nil).Description(); !strings.Contains(d, "下线") || !strings.Contains(d, "机器") {
		t.Errorf("query_nodes description should carry node/status synonyms, got %q", d)
	}
	if d := NewQueryDatabaseTool(nil, nil).Description(); !strings.Contains(d, "下线") {
		t.Errorf("query_database description should carry status synonyms, got %q", d)
	}
}
