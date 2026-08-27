package monitor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRuleBasedAdvisor_SelectsScripts 验证规则推荐：只选可执行脚本、按风险排序。
func TestRuleBasedAdvisor_SelectsScripts(t *testing.T) {
	s := newTestStore(t)
	for _, r := range []Remedy{
		{ID: "RM-H", AlertTypeID: "OWL-DSK-001", Name: "高风险", Kind: "script", Content: "echo hi", Risk: "high", Source: "user", Reviewed: true},
		{ID: "RM-L", AlertTypeID: "OWL-DSK-001", Name: "低风险", Kind: "script", Content: "echo hi", Risk: "low", Source: "user", Reviewed: true},
		{ID: "RM-S", AlertTypeID: "OWL-DSK-001", Name: "指引", Kind: "sop", Content: "1. df -h", Risk: "low", Source: "builtin", Reviewed: true},
	} {
		require.NoError(t, s.UpsertRemedy(r))
	}

	a := NewRuleBasedAdvisor(s, 5)
	plan, err := a.Advise(context.Background(), DisposalRequest{
		Alert: &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, plan.Steps, "应推荐出步骤")

	// sop 不应入选
	for _, st := range plan.Steps {
		require.NotEqual(t, "sop", st.Kind, "规则推荐只选可执行脚本")
	}
	// 低风险在前
	require.Equal(t, "RM-L", plan.Steps[0].RemedyID, "低风险对策应优先")
}

// TestRuleBasedAdvisor_MaxSteps 验证步数上限。
func TestRuleBasedAdvisor_MaxSteps(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		require.NoError(t, s.UpsertRemedy(Remedy{
			ID: string(rune('A' + i)), AlertTypeID: "OWL-DSK-001",
			Name: "对策", Kind: "script", Content: "echo hi", Risk: "low",
			Source: "user", Reviewed: true,
		}))
	}
	a := NewRuleBasedAdvisor(s, 2)
	plan, err := a.Advise(context.Background(), DisposalRequest{
		Alert: &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"},
	})
	require.NoError(t, err)
	require.Len(t, plan.Steps, 2, "应遵守最大步数")
}

// TestRuleBasedAdvisor_NoRemedies 验证无对策时不报错、返回空计划。
func TestRuleBasedAdvisor_NoRemedies(t *testing.T) {
	s := newTestStore(t)
	a := NewRuleBasedAdvisor(s, 3)
	plan, err := a.Advise(context.Background(), DisposalRequest{
		Alert: &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"},
	})
	require.NoError(t, err)
	require.Empty(t, plan.Steps)
}
