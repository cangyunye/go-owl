package monitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/internal/ai"
)

// TestManual_RealDeepSeek 真实 LLM 冒烟：OWL_LLM_TEST=1 时运行。
func TestManual_RealDeepSeek(t *testing.T) {
	if os.Getenv("OWL_LLM_TEST") != "1" {
		t.Skip("manual")
	}
	home, _ := os.UserHomeDir()
	cfg, err := ai.LoadConfig(filepath.Join(home, ".owl", "config.yaml"))
	if err != nil || cfg.AI.APIKey == "" {
		t.Fatalf("无 AI 配置: %v", err)
	}
	t.Logf("provider=%s model=%s base=%s", cfg.AI.Provider, cfg.AI.Model, cfg.AI.BaseURL)
	client, err := ai.CreateLLMClient(cfg)
	if err != nil {
		t.Fatalf("创建 LLM 客户端失败: %v", err)
	}
	a := NewAIAdvisor(client, newTestStore(t), 3)
	plan, err := a.Advise(context.Background(), DisposalRequest{
		Alert: &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "web-01",
			Severity: SeverityWarning, Status: StatusOpen,
			Message:        "磁盘使用率过高：disk.usage./ 当前 93.50（规则 > 90）",
			MetricSnapshot: `{"disk.usage./":93.5}`},
		Remedies: []Remedy{{
			ID: "R-DSK-001-SOP", AlertTypeID: "OWL-DSK-001", Name: "磁盘使用率过高排查指引",
			Kind: "sop", Content: "1. du -h --max-depth=1 / | sort -h", Risk: "low",
			Source: "builtin", Reviewed: true,
		}},
		Context: "告警类型: OWL-DSK-001（磁盘使用率过高）\n节点: web-01\n级别: warn\n消息: 磁盘使用率过高：disk.usage./ 当前 93.50（规则 > 90）\n指标快照: {\"disk.usage./\":93.5}",
	})
	if err != nil {
		t.Fatalf("Advise 失败: %v", err)
	}
	t.Logf("reasoning: %s", plan.Reasoning)
	for i, s := range plan.Steps {
		t.Logf("step%d: remedy=%q name=%q kind=%q risk=%q generated=%v source=%q reviewed=%v",
			i, s.RemedyID, s.Name, s.Kind, s.Risk, s.Generated, s.Source, s.Reviewed)
		t.Logf("  content: %s", s.Content)
	}
}
