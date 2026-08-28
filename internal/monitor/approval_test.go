package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestApprovalDecide_DefaultOff 验证默认全关：类型/对策未放行 → 仅人工。
func TestApprovalDecide_DefaultOff(t *testing.T) {
	in := ApprovalInput{
		Severity: SeverityWarning, Risk: "low", ScopeCount: 1,
		TypeApproved: false, RemedyApproved: true, Source: "builtin", Reviewed: true,
	}
	require.Equal(t, DecisionHuman, DecideApproval(in), "类型未放行必须仅人工")

	in.TypeApproved = true
	in.RemedyApproved = false
	require.Equal(t, DecisionHuman, DecideApproval(in), "对策未放行必须仅人工")
}

// TestApprovalDecide_HighRisk 验证高风险永不自愈。
func TestApprovalDecide_HighRisk(t *testing.T) {
	in := ApprovalInput{
		Severity: SeverityWarning, Risk: "high", ScopeCount: 1,
		TypeApproved: true, RemedyApproved: true, Source: "user", Reviewed: true,
	}
	require.Equal(t, DecisionHuman, DecideApproval(in), "高风险对策必须仅人工")
}

// TestApprovalDecide_AIUnreviewed 验证 AI 生成未审核 → 待审批（进审批流人工审核）。
func TestApprovalDecide_AIUnreviewed(t *testing.T) {
	in := ApprovalInput{
		Severity: SeverityWarning, Risk: "low", ScopeCount: 1,
		TypeApproved: true, RemedyApproved: true, Source: "ai", Reviewed: false,
	}
	require.Equal(t, DecisionApproval, DecideApproval(in), "AI 未审核应进审批流待人工审核")
}

// TestApprovalDecide_Critical 验证 critical 必须人工审批（紧急不改动）。
func TestApprovalDecide_Critical(t *testing.T) {
	in := ApprovalInput{
		Severity: SeverityCritical, Risk: "low", ScopeCount: 1,
		TypeApproved: true, RemedyApproved: true, Source: "builtin", Reviewed: true,
	}
	require.Equal(t, DecisionApproval, DecideApproval(in), "critical 告警必须审批")
}

// TestApprovalDecide_MultiNode 验证多节点必须审批（canary 先行）。
func TestApprovalDecide_MultiNode(t *testing.T) {
	in := ApprovalInput{
		Severity: SeverityWarning, Risk: "low", ScopeCount: 3,
		TypeApproved: true, RemedyApproved: true, Source: "builtin", Reviewed: true,
	}
	require.Equal(t, DecisionApproval, DecideApproval(in), "批量执行必须审批")
}

// TestApprovalDecide_Auto 验证低风险单节点 + 全放行 + 非 AI → 自动。
func TestApprovalDecide_Auto(t *testing.T) {
	for _, source := range []string{"builtin", "user"} {
		in := ApprovalInput{
			Severity: SeverityWarning, Risk: "low", ScopeCount: 1,
			TypeApproved: true, RemedyApproved: true, Source: source, Reviewed: true,
		}
		require.Equal(t, DecisionAuto, DecideApproval(in), "来源 %s 低风险单节点应自动", source)
	}

	// AI 已审核 + 低风险 + 单节点 → 允许自动
	in := ApprovalInput{
		Severity: SeverityInfo, Risk: "low", ScopeCount: 1,
		TypeApproved: true, RemedyApproved: true, Source: "ai", Reviewed: true,
	}
	require.Equal(t, DecisionAuto, DecideApproval(in), "AI 已审核低风险单节点应自动")
}

// TestApprovalDecide_MediumRisk 验证中风险单节点可审批（不自动也不仅人工）。
func TestApprovalDecide_MediumRisk(t *testing.T) {
	in := ApprovalInput{
		Severity: SeverityWarning, Risk: "medium", ScopeCount: 1,
		TypeApproved: true, RemedyApproved: true, Source: "builtin", Reviewed: true,
	}
	require.Equal(t, DecisionApproval, DecideApproval(in), "中风险应审批")
}
