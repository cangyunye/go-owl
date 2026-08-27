package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/cangyunye/go-owl/internal/control/blacklist"
)

// AutoHealer 自愈管线：告警触发 → 建议 → 闸门链（语法/黑名单/审批矩阵）
// → 执行（复用 RunExecutor）→ 反馈。所有被拦截的步骤带原因入计划，可审计。
type AutoHealer struct {
	store   *Store
	advisor Advisor
	runner  *RunExecutor
	gate    *SyntaxGate
	checker *blacklist.Checker
	decide  func(ApprovalInput) Decision
	now     func() int64
}

// NewAutoHealer 创建自愈协调器。
func NewAutoHealer(store *Store, advisor Advisor, runner *RunExecutor) *AutoHealer {
	return &AutoHealer{
		store:   store,
		advisor: advisor,
		runner:  runner,
		gate:    NewSyntaxGate(),
		checker: blacklist.NewDefaultChecker(),
		decide:  DecideApproval,
		now:     func() int64 { return time.Now().Unix() },
	}
}

// Heal 对一条告警执行自愈管线并异步执行获准步骤。
// 返回创建的处置计划；无任何可执行步骤时返回 nil（不创建）。
func (h *AutoHealer) Heal(ctx context.Context, alert *Alert, at AlertType, target *Target) (*RemedyRun, error) {
	req := DisposalRequest{Alert: alert, Type: at}
	plan, err := h.advisor.Advise(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("monitor: 生成处置建议失败: %w", err)
	}
	if len(plan.Steps) == 0 {
		return nil, nil
	}

	now := h.now()
	var steps []RemedyStep
	executable := 0
	for i, ds := range plan.Steps {
		st := RemedyStep{
			Order:    i,
			RemedyID: ds.RemedyID,
			Name:     ds.Name,
			Kind:     ds.Kind,
			Content:  ds.Content,
			Status:   StepPending,
			NodeID:   alert.NodeID,
		}
		// 1. 语法闸门：AI 现场生成的脚本必须通过语法校验
		if ds.Generated {
			if ok, reason := h.gate.Validate(ds.Kind, ds.Content); !ok {
				st.Status = StepSkipped
				st.Output = "语法闸门拦截: " + reason
				st.FinishedAt = now
				steps = append(steps, st)
				continue
			}
		}
		// 2. 黑名单闸门
		if res := h.checker.Check("", ds.Content); res.Blocked {
			st.Status = StepSkipped
			st.Output = "命中危险命令黑名单，已拦截"
			st.FinishedAt = now
			steps = append(steps, st)
			continue
		}
		// 3. 审批矩阵
		remedyApproved := false
		if ds.RemedyID != "" {
			if rm, ok, err := h.store.GetRemedy(ds.RemedyID); err == nil && ok {
				remedyApproved = rm.AutoApprove
			}
		}
		decision := h.decide(ApprovalInput{
			Severity:       alert.Severity,
			Risk:           ds.Risk,
			ScopeCount:     1, // 自愈首轮单节点（canary）
			TypeApproved:   at.AutoApprove,
			RemedyApproved: remedyApproved,
			Source:         ds.Source,
			Reviewed:       ds.Reviewed,
		})
		switch decision {
		case DecisionAuto:
			executable++
		case DecisionApproval:
			st.Status = StepSkipped
			st.Output = "需人工审批后执行（P2-M3 审批流）"
			st.FinishedAt = now
		case DecisionHuman:
			st.Status = StepSkipped
			st.Output = "仅人工处置（类型/对策未放行或风险过高）"
			st.FinishedAt = now
		}
		steps = append(steps, st)
	}
	if executable == 0 {
		// 全被拦截：仍记录一次「尝试」便于审计
		if len(steps) == 0 {
			return nil, nil
		}
	}

	run := &RemedyRun{
		ID:          fmt.Sprintf("RUN-%d", now),
		AlertID:     alert.ID,
		NodeID:      alert.NodeID,
		Status:      RunPending,
		StopOnError: true,
		CreatedBy:   "auto-heal",
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps:       steps,
	}
	if err := h.store.CreateRemedyRun(run); err != nil {
		return nil, err
	}
	if executable > 0 {
		go func() { _ = h.runner.ExecuteRun(context.Background(), run.ID, h.store) }()
	} else {
		_ = h.store.UpdateRemedyRunStatus(run.ID, RunDone)
	}
	return run, nil
}
