package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

// 无 LLM key 时本地降级链的告警意图识别，保证 CLI/TUI/Web 三端在无模型配置下
// 依然能回答告警查询（E2E 依赖此路径）。

func TestIntentClassifierAlert(t *testing.T) {
	c := NewIntentClassifier()
	cases := []struct {
		in   string
		want IntentType
	}{
		{"有哪些节点有告警", IntentAlertList},
		{"哪些节点在告警", IntentAlertList},
		{"列出所有发生OWL_DSK_001警告的节点", IntentAlertList},
		{"查看磁盘告警", IntentAlertList},
		{"owl-oss-001 告警在哪些机器", IntentAlertList},
		{"提供 OWL-DSK-001 告警的修复方案", IntentAlertRemedy},
		{"帮我修复OWL_SVC_001告警的机器", IntentAlertRemedy},
		{"告警的机器帮我处理下", IntentAlertRemedy},
		// 回归：不抢既有意图
		{"列出所有节点", IntentQueryNodes},
		{"重启 nginx 服务", IntentGeneratePlaybook},
	}
	for _, tc := range cases {
		got := c.Classify(tc.in)
		if got.Type != tc.want {
			t.Errorf("Classify(%q) = %s (conf=%d), want %s", tc.in, got.Type, got.Confidence, tc.want)
		}
	}
}

func TestParamExtractorAlert(t *testing.T) {
	e := NewParamExtractor([]string{"web-01", "db-01"})

	t.Run("提取告警码与节点", func(t *testing.T) {
		params := e.ExtractParams(IntentAlertList, "列出所有发生OWL_DSK_001警告的节点 web-01")
		if params["alert_type_id"] != "OWL_DSK_001" {
			t.Errorf("alert_type_id = %v, want OWL_DSK_001", params["alert_type_id"])
		}
		nodes, ok := params["node"].(string)
		if !ok || nodes != "web-01" {
			t.Errorf("node = %v, want web-01", params["node"])
		}
	})

	t.Run("无告警码", func(t *testing.T) {
		params := e.ExtractParams(IntentAlertList, "有哪些节点有告警")
		if _, ok := params["alert_type_id"]; ok {
			t.Errorf("should not extract alert_type_id, got %v", params["alert_type_id"])
		}
	})

	t.Run("修复意图提取告警码", func(t *testing.T) {
		params := e.ExtractParams(IntentAlertRemedy, "提供 OWL-MEM-001 告警的修复方案")
		if params["alert_type_id"] != "OWL-MEM-001" {
			t.Errorf("alert_type_id = %v, want OWL-MEM-001", params["alert_type_id"])
		}
	})
}

func TestValidatorAlertParams(t *testing.T) {
	v := NewValidator()
	if err := v.ValidateParams(IntentAlertList, map[string]interface{}{"severity": "warn"}); err != nil {
		t.Errorf("alert_list params should validate: %v", err)
	}
	if err := v.ValidateParams(IntentAlertList, map[string]interface{}{"severity": 123}); err == nil {
		t.Error("non-string severity should fail")
	}
	if err := v.ValidateParams(IntentAlertRemedy, map[string]interface{}{"alert_type_id": "OWL-DSK-001"}); err != nil {
		t.Errorf("alert_remedy params should validate: %v", err)
	}
	if err := v.ValidateParams(IntentAlertRemedy, map[string]interface{}{}); err == nil {
		t.Error("alert_remedy without alert_type_id should fail")
	}
}

// 无 LLM key 时的完整降级路径：空配置 Agent（chatModel=defaultChatHandler）
// 本地路由 → alert_list 工具执行 → 渲染结果直接作为回复。
// Web 端无用户 key 时用的就是这条路径（E2E 回归覆盖）。
func TestAlertLocalRouteWithoutLLM(t *testing.T) {
	stub := &stubAlertExecutor{listRows: []AlertRow{
		{ID: "AL-1", TypeID: "OWL-OSS-001", NodeID: "n1", NodeName: "web-01", Severity: "critical", Status: "open", Message: "节点失联"},
	}, listTotal: 1}
	mgr := &mockNodeMgrForAI{nodes: []*model.Node{{Name: "web-01", Address: "127.0.0.1", Port: 22}}}
	agent, err := NewAgent(stub, &Config{}, mgr, nil, nil) // 空 Config → 无 LLM → 本地链
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	resp, err := agent.Process(context.Background(), "有哪些节点有告警", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !strings.Contains(resp, "OWL-OSS-001") || !strings.Contains(resp, "web-01") {
		t.Errorf("expected rendered alert list via local route, got %q", resp)
	}

	// 修复意图（有码）→ alert_remedy 工具
	resp2, err := agent.Process(context.Background(), "提供 OWL-DSK-001 告警的修复方案", nil)
	if err != nil {
		t.Fatalf("Process remedy failed: %v", err)
	}
	if !strings.Contains(resp2, "OWL-DSK-001") {
		t.Errorf("expected remedy output via local route, got %q", resp2)
	}
}
