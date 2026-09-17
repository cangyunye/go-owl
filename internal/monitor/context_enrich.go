package monitor

import (
	"fmt"
	"strings"
	"time"
)

// ChangeRecord 是告警前发生在目标节点上的一次运维变更（来自 serve operations 表）。
type ChangeRecord struct {
	Time    string
	User    string
	Origin  string // web/ai/cli/...
	OpType  string
	Command string
}

// HistoryLookup 按节点 + 起始时间检索变更记录（serve 装配时包装 HistoryStore 注入）。
type HistoryLookup func(nodeID string, since time.Time, limit int) ([]ChangeRecord, error)

// ContextEnricher 处置上下文增强接口（AutoHealer 可选依赖）。
type ContextEnricher interface {
	Enrich(alert *Alert, at AlertType) string
}

// enrichLimits 控制 prompt 注入量，防止上下文膨胀。
const (
	enrichMaxRuns      = 3
	enrichMaxChanges   = 5
	enrichMaxAlerts    = 5
	enrichChangeWindow = 30 * time.Minute
)

// DisposalContextEnricher 汇集三类高价值上下文（历史同类处置、告警前变更、
// 相关告警），拼装为文本块供 AI advisor 生成更精准的处置计划。
// 任一数据源缺失即省略对应块，全部缺失时返回空串。
type DisposalContextEnricher struct {
	store   *Store
	history HistoryLookup
	now     func() time.Time
}

// NewDisposalContextEnricher 创建增强器（history 可为 nil，跳过变更关联）。
func NewDisposalContextEnricher(store *Store, history HistoryLookup) *DisposalContextEnricher {
	return &DisposalContextEnricher{store: store, history: history, now: func() time.Time { return time.Now() }}
}

// Enrich 拼装增强上下文（无数据返回空串）。
func (e *DisposalContextEnricher) Enrich(alert *Alert, at AlertType) string {
	var blocks []string

	if block := e.historyBlock(at); block != "" {
		blocks = append(blocks, block)
	}
	if block := e.changeBlock(alert); block != "" {
		blocks = append(blocks, block)
	}
	if block := e.relatedAlertsBlock(alert); block != "" {
		blocks = append(blocks, block)
	}
	if len(blocks) == 0 {
		return ""
	}
	return strings.Join(blocks, "\n\n")
}

// historyBlock 历史同类处置：近 N 次计划的终态与疗效验证结果，
// 供模型优先推荐已被验证有效的对策。
func (e *DisposalContextEnricher) historyBlock(at AlertType) string {
	runs, err := e.store.ListRemedyRunsByAlertType(at.ID, enrichMaxRuns)
	if err != nil || len(runs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[历史同类处置]（该告警类型最近的处置计划与疗效：）")
	for _, r := range runs {
		line := fmt.Sprintf("\n- %s @ %s：%s", r.RunID, r.NodeID, runStatusText(r.Status))
		if v, err := e.store.GetVerificationByRun(r.RunID); err == nil {
			switch v.Status {
			case VerifyRecovered:
				line += "，疗效验证：已确认恢复 ✅"
			case VerifyNotRecovered:
				line += "，疗效验证：告警未消除 ⚠️"
			case VerifyInconclusive:
				line += "，疗效验证：无法判定"
			}
		}
		sb.WriteString(line)
	}
	sb.WriteString("\n说明：优先参考已确认恢复（recovered）的历史方案思路。")
	return sb.String()
}

// changeBlock 告警前变更：目标节点近 30 分钟内的运维操作，
// 人为变更往往是告警的直接诱因。
func (e *DisposalContextEnricher) changeBlock(alert *Alert) string {
	if e.history == nil {
		return ""
	}
	records, err := e.history(alert.NodeID, e.now().Add(-enrichChangeWindow), enrichMaxChanges)
	if err != nil || len(records) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[告警前变更]（该节点最近 30 分钟内的运维操作，可能与本次告警相关：）")
	for _, r := range records {
		sb.WriteString(fmt.Sprintf("\n- %s [%s/%s] %s: %s", r.Time, r.Origin, r.User, r.OpType, r.Command))
	}
	return sb.String()
}

// relatedAlertsBlock 相关告警：同节点其他活跃告警（同一节点的多信号因果链）
// 与同类型全网活跃数（区分个体故障与全局性故障）。
func (e *DisposalContextEnricher) relatedAlertsBlock(alert *Alert) string {
	var lines []string

	sameNode, err := e.store.ListAlerts(AlertFilter{NodeID: alert.NodeID, Status: "active", Limit: enrichMaxAlerts + 1})
	if err == nil {
		for _, a := range sameNode {
			if a.ID == alert.ID || a.AlertTypeID == alert.AlertTypeID {
				continue
			}
			lines = append(lines, fmt.Sprintf("- 同节点 %s：%s（%s，%s）", a.NodeID, a.AlertTypeID, a.Severity, a.Message))
		}
	}

	sameType, err := e.store.ListAlerts(AlertFilter{AlertTypeID: alert.AlertTypeID, Status: "active", Limit: 50})
	if err == nil {
		nodes := make([]string, 0, len(sameType))
		for _, a := range sameType {
			if a.NodeID == alert.NodeID {
				continue
			}
			nodes = append(nodes, a.NodeID)
		}
		if len(nodes) > 0 {
			if len(nodes) > enrichMaxAlerts {
				lines = append(lines, fmt.Sprintf("- 同类型告警同时在 %d 个节点活跃（疑似全局性/上游故障）：%s 等",
					len(nodes), strings.Join(nodes[:enrichMaxAlerts], ", ")))
			} else {
				lines = append(lines, fmt.Sprintf("- 同类型告警同时在以下节点活跃：%s", strings.Join(nodes, ", ")))
			}
		}
	}

	if len(lines) == 0 {
		return ""
	}
	return "[相关告警]（可能与本次告警存在因果或关联：）\n" + strings.Join(lines, "\n")
}

func runStatusText(status RemedyRunStatus) string {
	switch status {
	case RunDone:
		return "完成"
	case RunFailed:
		return "失败"
	case RunStopped:
		return "已停止"
	case RunWaitingApproval:
		return "待审批"
	case RunRunning:
		return "执行中"
	default:
		return string(status)
	}
}
