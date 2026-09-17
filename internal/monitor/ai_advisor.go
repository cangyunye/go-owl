package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cangyunye/go-owl/internal/ai"
)

// aiSystemPrompt AI 处置建议系统提示词：运维处置专家，安全优先，严格 JSON。
const aiSystemPrompt = `你是一名资深 Linux 运维处置专家，为告警生成处置计划。
要求：
1. 输出严格的 JSON（不要 markdown 代码块包裹），结构：
{
  "reasoning": "根因解读与处置思路（中文，简洁）",
  "steps": [
    {"remedy_id": "可选，引用现有对策ID", "name": "步骤名", "kind": "script",
     "content": "脚本内容或说明", "risk": "low|medium|high",
     "generated": false}
  ]
}
2. 优先引用请求中提供的现有对策（remedy_id 必须存在且能对上）；没有合适对策时才
   现场生成脚本（generated=true，kind=script）。
3. 步骤按执行顺序排列，最多 3 步；只写可自动执行的脚本步骤，人工排查步骤不要列入。
4. 安全底线：绝不生成 rm -rf /、mkfs、fdisk、dd 写盘、格式化等破坏性命令；
   高风险操作标记 risk=high（永远不要自动执行）。脚本要幂等、有超时意识、优先
   非破坏性命令（清理临时文件、重启服务、检查状态等）。
5. generated=true 的脚本必须语法正确（bash），单文件，可直接 bash 执行。
6. 处置背景提示：若请求上下文包含「历史同类处置」「告警前变更」「相关告警」等
   背景块，请充分参考：优先借鉴已确认恢复（疗效验证 recovered）的历史方案思路；
   若告警前存在相关运维变更，优先评估回退该变更的可能性；若同类型告警在多节点
   同时活跃，考虑全局性/上游原因，避免只做单点处置。`

// AIAdvisor 基于 LLM 的处置建议器：解读根因、优选现有对策，
// 无合适对策时现场生成脚本（Generated=true，走语法闸门 + 审批流）。
// LLM 调用失败时降级到规则兜底，保证告警处置不中断。
type AIAdvisor struct {
	client   ai.LLMClient
	fallback *RuleBasedAdvisor
	maxSteps int
}

// NewAIAdvisor 创建 AI 建议器。
func NewAIAdvisor(client ai.LLMClient, store *Store, maxSteps int) *AIAdvisor {
	return &AIAdvisor{
		client:   client,
		fallback: NewRuleBasedAdvisor(store, maxSteps),
		maxSteps: maxSteps,
	}
}

// Advise 生成 AI 处置建议；LLM 失败时降级规则兜底。
func (a *AIAdvisor) Advise(ctx context.Context, req DisposalRequest) (*DisposalPlan, error) {
	prompt := a.buildPrompt(req)
	raw, err := a.client.Generate(ctx, []ai.Message{
		{Role: "system", Content: aiSystemPrompt},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return a.fallback.Advise(ctx, req)
	}
	plan, err := a.parse(raw)
	if err != nil {
		return a.fallback.Advise(ctx, req)
	}
	a.normalize(req, plan)
	return plan, nil
}

// buildPrompt 组装告警上下文与可用对策清单。
func (a *AIAdvisor) buildPrompt(req DisposalRequest) string {
	var b strings.Builder
	b.WriteString("请为以下告警生成处置计划：\n\n")
	if req.Context != "" {
		b.WriteString(req.Context)
	} else if req.Alert != nil {
		b.WriteString(fmt.Sprintf("告警类型: %s\n节点: %s\n级别: %s\n消息: %s\n指标快照: %s\n",
			req.Alert.AlertTypeID, req.Alert.NodeID, req.Alert.Severity, req.Alert.Message, req.Alert.MetricSnapshot))
	}
	b.WriteString("\n\n可用对策：\n")
	if len(req.Remedies) == 0 {
		b.WriteString("（无现有对策，请现场生成脚本）\n")
	}
	for _, r := range req.Remedies {
		if r.Kind != "script" {
			continue
		}
		content := r.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		b.WriteString(fmt.Sprintf("- remedy_id=%s name=%s risk=%s source=%s\n  content: %s\n",
			r.ID, r.Name, r.Risk, r.Source, content))
	}
	b.WriteString("\n输出 JSON。")
	return b.String()
}

// planJSON LLM 输出结构。
type planJSON struct {
	Reasoning string `json:"reasoning"`
	Steps     []struct {
		RemedyID  string `json:"remedy_id"`
		Name      string `json:"name"`
		Kind      string `json:"kind"`
		Content   string `json:"content"`
		Risk      string `json:"risk"`
		Generated bool   `json:"generated"`
	} `json:"steps"`
}

// parse 解析 LLM JSON 输出（容忍 markdown 代码块包裹）。
func (a *AIAdvisor) parse(raw string) (*DisposalPlan, error) {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var pj planJSON
	if err := json.Unmarshal([]byte(text), &pj); err != nil {
		return nil, fmt.Errorf("monitor: 解析 AI 处置建议失败: %w", err)
	}
	plan := &DisposalPlan{Reasoning: pj.Reasoning}
	for _, s := range pj.Steps {
		plan.Steps = append(plan.Steps, DisposalStep{
			RemedyID:  s.RemedyID,
			Name:      s.Name,
			Kind:      s.Kind,
			Content:   s.Content,
			Risk:      s.Risk,
			Generated: s.Generated,
		})
	}
	if len(plan.Steps) > a.maxSteps {
		plan.Steps = plan.Steps[:a.maxSteps]
	}
	return plan, nil
}

// normalize 修正步骤：现有对策用库内内容与来源（不信任 LLM 改写），
// 生成项标记 AI 来源 + 待审核 + 风险校验。
func (a *AIAdvisor) normalize(req DisposalRequest, plan *DisposalPlan) {
	byID := make(map[string]Remedy, len(req.Remedies))
	for _, r := range req.Remedies {
		byID[r.ID] = r
	}
	for i := range plan.Steps {
		s := &plan.Steps[i]
		if s.RemedyID != "" {
			if rm, ok := byID[s.RemedyID]; ok {
				// 引用现有对策：内容/来源/审核状态以库为准
				s.Name = rm.Name
				s.Kind = rm.Kind
				s.Content = rm.Content
				s.Risk = rm.Risk
				s.Source = rm.Source
				s.Reviewed = rm.Reviewed
				continue
			}
			// LLM 引用了不存在的对策 ID → 降级为生成项
			s.RemedyID = ""
		}
		// 生成项：AI 来源、待审核、走语法闸门与审批流
		if s.Kind == "" {
			s.Kind = "script"
		}
		if s.Risk != "low" && s.Risk != "medium" {
			s.Risk = "medium" // 未知风险保守取中
		}
		if s.Name == "" {
			s.Name = "AI 生成脚本"
		}
		s.Source = "ai"
		s.Reviewed = false
		s.Generated = true
	}
}
