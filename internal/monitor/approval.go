package monitor

// Decision 处置审批决策。
type Decision string

const (
	DecisionAuto     Decision = "auto"     // 自动执行（无人值守）
	DecisionApproval Decision = "approval" // 需人工审批后执行
	DecisionHuman    Decision = "human"    // 仅人工（禁止自动）
)

// ApprovalInput 审批矩阵输入。
type ApprovalInput struct {
	Severity       Severity
	Risk           string // low | medium | high
	ScopeCount     int    // 目标节点数（>1 即批量）
	TypeApproved   bool   // 告警类型 auto_approve 放行
	RemedyApproved bool   // 对策 auto_approve 放行
	Source         string // builtin | ai | user
	Reviewed       bool   // AI 生成是否已审核
}

// DecideApproval 审批矩阵（安全优先）：
// 1. 类型/对策未放行 → 仅人工（默认全关）
// 2. 高风险 → 仅人工（永不自愈）
// 3. AI 未审核 → 待审批（人工在审批流中审核脚本后批准执行）
// 4. critical 级别 → 审批（紧急操作必须人确认）
// 5. 多节点 → 审批（canary 先行，批量须确认）
// 6. 其余低/中风险单节点 → 自动
func DecideApproval(in ApprovalInput) Decision {
	if !in.TypeApproved || !in.RemedyApproved {
		return DecisionHuman
	}
	if in.Risk == "high" {
		return DecisionHuman
	}
	if in.Source == "ai" && !in.Reviewed {
		return DecisionApproval
	}
	if in.Severity == SeverityCritical {
		return DecisionApproval
	}
	if in.ScopeCount > 1 {
		return DecisionApproval
	}
	if in.Risk == "medium" {
		return DecisionApproval
	}
	return DecisionAuto
}
