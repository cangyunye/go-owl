package monitor

import (
	"context"
	"fmt"
	"sort"
)

// DisposalStep 处置建议中的单步（对应对策或 AI 现场生成的脚本）。
type DisposalStep struct {
	RemedyID  string
	Name      string
	Kind      string // script | playbook | sop
	Content   string
	Risk      string // low | medium | high
	Source    string // builtin | ai | user
	Reviewed  bool
	Generated bool // AI 现场生成 → 必须过语法闸门
}

// DisposalPlan 处置建议：排序后的一组步骤 + 推理摘要。
type DisposalPlan struct {
	Reasoning string
	Steps     []DisposalStep
}

// DisposalRequest 处置建议请求：告警 + 富化上下文 + 可用对策。
type DisposalRequest struct {
	Alert    *Alert
	Type     AlertType
	Remedies []Remedy // 已按推荐排序的可用对策
	Context  string   // 指标快照/日志摘要等富化上下文
}

// Advisor 处置建议器：输入告警上下文，输出有序处置计划。
// AI 实现（internal/ai）与规则实现（RuleBasedAdvisor）可插拔。
type Advisor interface {
	Advise(ctx context.Context, req DisposalRequest) (*DisposalPlan, error)
}

// RuleBasedAdvisor 确定性推荐：取可用对策中可执行的脚本类，
// 低风险优先，最多 maxSteps 条。作为 AI 未接入时的兜底与测试实现。
type RuleBasedAdvisor struct {
	store    *Store
	maxSteps int
}

// NewRuleBasedAdvisor 创建规则推荐器。
func NewRuleBasedAdvisor(store *Store, maxSteps int) *RuleBasedAdvisor {
	if maxSteps <= 0 {
		maxSteps = 3
	}
	return &RuleBasedAdvisor{store: store, maxSteps: maxSteps}
}

// Advise 生成处置建议。
func (a *RuleBasedAdvisor) Advise(ctx context.Context, req DisposalRequest) (*DisposalPlan, error) {
	remedies := req.Remedies
	if remedies == nil && req.Alert != nil {
		recs, err := a.store.RecommendedRemedies(req.Alert.AlertTypeID)
		if err != nil {
			return nil, fmt.Errorf("monitor: 读取推荐对策失败: %w", err)
		}
		remedies = recs
	}

	riskRank := map[string]int{"low": 0, "medium": 1, "high": 2}
	var steps []DisposalStep
	for _, r := range remedies {
		if r.Kind != "script" {
			continue // 只推荐可自动执行的脚本
		}
		steps = append(steps, DisposalStep{
			RemedyID: r.ID,
			Name:     r.Name,
			Kind:     r.Kind,
			Content:  r.Content,
			Risk:     r.Risk,
			Source:   r.Source,
			Reviewed: r.Reviewed,
		})
	}
	sort.SliceStable(steps, func(i, j int) bool {
		return riskRank[steps[i].Risk] < riskRank[steps[j].Risk]
	})
	if len(steps) > a.maxSteps {
		steps = steps[:a.maxSteps]
	}
	return &DisposalPlan{
		Reasoning: "按对策风险排序自动推荐（规则兜底）",
		Steps:     steps,
	}, nil
}
