package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 告警 AI 工具层：DTO 与渲染函数在本包定义（三端共享，防文案漂移），
// 数据访问由宿主注入（WebExecutor 直连共享 monitor.Store，CLIExecutor 经
// AlertData 委托读取共享 owl.db）。internal/ai 不得直接依赖 internal/monitor：
// monitor 包已导入本包（ai_advisor.go），反向引用会成环。

// AlertRow 告警实例视图（渲染用）。
type AlertRow struct {
	ID        string
	TypeID    string
	NodeID    string
	NodeName  string
	Severity  string
	Status    string
	Message   string
	FirstSeen int64
	LastSeen  int64
}

// AlertTypeRow 告警类型（告警码）视图。
type AlertTypeRow struct {
	ID              string
	Category        string
	Name            string
	Description     string
	DefaultSeverity string
	Rule            string // 人类可读触发规则描述
	Enabled         bool
}

// RemedyRow 告警对策视图（SOP/脚本/剧本）。
type RemedyRow struct {
	ID       string
	Name     string
	Kind     string // sop | script | playbook
	Risk     string
	Source   string // builtin | ai | user
	Rollback string
	Content  string
}

// AlertData 告警数据访问接口，由宿主实现注入。
// 实现方负责：告警码归一/模糊匹配（结合可用告警类型表）、类别展开、
// 节点名→ID 解析、按 AlertListParams 组装过滤条件。
type AlertData interface {
	// ListAlerts 返回告警行与符合条件的总数（用于"共 N 条"汇总）。
	ListAlerts(p AlertListParams) ([]AlertRow, int, error)
	ListAlertTypes() ([]AlertTypeRow, error)
	ListRemedies(alertTypeID string) ([]RemedyRow, error)
}

// NormalizeAlertCode 归一化告警码写法：OWL_DSK_001 / owl dsk 001 → OWL-DSK-001。
func NormalizeAlertCode(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

// MatchAlertTypeCodes 将用户输入的告警码/类别解析为具体告警码列表。
// rawCode 与 category 均为空时返回 (nil, nil) 表示不过滤；
// 给定了条件但无可匹配项时返回带可用码清单的错误，便于 LLM 自纠错。
func MatchAlertTypeCodes(types []AlertTypeRow, rawCode, category string) ([]string, error) {
	rawCode = strings.TrimSpace(rawCode)
	category = strings.ToLower(strings.TrimSpace(category))
	if rawCode == "" && category == "" {
		return nil, nil
	}

	if rawCode != "" {
		code := NormalizeAlertCode(rawCode)
		for _, at := range types {
			if at.ID == code {
				return []string{at.ID}, nil
			}
		}
		// 后缀匹配：用户省略 OWL- 前缀（如 "DSK-001"）
		var suffix []string
		for _, at := range types {
			if strings.HasSuffix(at.ID, code) {
				suffix = append(suffix, at.ID)
			}
		}
		if len(suffix) > 0 {
			return suffix, nil
		}
		return nil, fmt.Errorf("未知告警码 %s，可用告警码：%s", code, joinAlertTypeIDs(types))
	}

	var codes []string
	for _, at := range types {
		if strings.EqualFold(at.Category, category) {
			codes = append(codes, at.ID)
		}
	}
	if len(codes) == 0 {
		return nil, fmt.Errorf("未知告警类别 %s，可用类别：%s", category, joinAlertCategories(types))
	}
	return codes, nil
}

func joinAlertTypeIDs(types []AlertTypeRow) string {
	ids := make([]string, 0, len(types))
	for _, at := range types {
		ids = append(ids, at.ID)
	}
	return strings.Join(ids, ", ")
}

func joinAlertCategories(types []AlertTypeRow) string {
	seen := map[string]bool{}
	var cats []string
	for _, at := range types {
		c := strings.ToLower(at.Category)
		if c != "" && !seen[c] {
			seen[c] = true
			cats = append(cats, c)
		}
	}
	sort.Strings(cats)
	return strings.Join(cats, ", ")
}

// ---------- 渲染（三端共享输出格式） ----------

func formatUnixTS(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}

func RenderAlertList(rows []AlertRow, total int) string {
	if len(rows) == 0 {
		return "当前没有符合条件的告警。"
	}
	var sb strings.Builder
	if total > len(rows) {
		fmt.Fprintf(&sb, "共 %d 条告警（显示前 %d 条，按严重级别与发生时间排序）：\n\n", total, len(rows))
	} else {
		fmt.Fprintf(&sb, "共 %d 条告警：\n\n", total)
	}
	sb.WriteString("| 告警码 | 节点 | 级别 | 状态 | 告警信息 | 首次发生 | 最近发生 |\n")
	sb.WriteString("|---|---|---|---|---|---|---|\n")
	nodeCount := map[string]int{}
	for _, r := range rows {
		name := r.NodeName
		if name == "" {
			name = r.NodeID
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s | %s |\n",
			r.TypeID, name, r.Severity, r.Status, r.Message,
			formatUnixTS(r.FirstSeen), formatUnixTS(r.LastSeen))
		nodeCount[name]++
	}
	sb.WriteString("\n按节点汇总：")
	names := make([]string, 0, len(nodeCount))
	for n := range nodeCount {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s(%d)", n, nodeCount[n]))
	}
	sb.WriteString(strings.Join(parts, "、"))
	return sb.String()
}

func RenderAlertTypes(rows []AlertTypeRow) string {
	if len(rows) == 0 {
		return "当前没有已定义的告警类型。"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "共 %d 种告警类型：\n\n", len(rows))
	sb.WriteString("| 告警码 | 类别 | 名称 | 默认级别 | 触发规则 | 启用 |\n")
	sb.WriteString("|---|---|---|---|---|---|\n")
	for _, r := range rows {
		enabled := "是"
		if !r.Enabled {
			enabled = "否"
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n",
			r.ID, r.Category, r.Name, r.DefaultSeverity, r.Rule, enabled)
	}
	return sb.String()
}

func RenderRemedies(code string, typeRow *AlertTypeRow, remedies []RemedyRow) string {
	var sb strings.Builder
	if typeRow != nil {
		fmt.Fprintf(&sb, "## 告警码 %s：%s\n\n", typeRow.ID, typeRow.Name)
		if typeRow.Description != "" {
			fmt.Fprintf(&sb, "说明：%s\n", typeRow.Description)
		}
		if typeRow.Rule != "" {
			fmt.Fprintf(&sb, "触发规则：%s\n", typeRow.Rule)
		}
		if typeRow.DefaultSeverity != "" {
			fmt.Fprintf(&sb, "默认级别：%s\n\n", typeRow.DefaultSeverity)
		}
	} else {
		fmt.Fprintf(&sb, "## 告警码 %s\n\n未在告警类型表中找到该码，请先用 alert_types 工具查询有效告警码。\n\n", code)
	}

	if len(remedies) == 0 {
		sb.WriteString("该告警码暂无预置对策（SOP/脚本/剧本）。可结合触发规则给出排查思路，或建议用户在告警页面配置对策。")
		return sb.String()
	}

	fmt.Fprintf(&sb, "### 可用对策（%d 条）\n\n", len(remedies))
	for i, r := range remedies {
		fmt.Fprintf(&sb, "#### %d. %s [%s] 风险:%s 来源:%s\n\n", i+1, r.Name, strings.ToUpper(r.Kind), r.Risk, r.Source)
		if r.Content != "" {
			sb.WriteString(r.Content)
			sb.WriteString("\n\n")
		}
		if r.Rollback != "" {
			fmt.Fprintf(&sb, "回滚方案：%s\n\n", r.Rollback)
		}
	}
	return sb.String()
}

// ---------- 工具实现 ----------

// AlertListTool 查询告警列表（场景：有哪些节点有告警/哪些节点有 OWL_XXX 警告）。
type AlertListTool struct {
	executor Executor
}

func NewAlertListTool(executor Executor) *AlertListTool {
	return &AlertListTool{executor: executor}
}

func (t *AlertListTool) Name() string        { return "alert_list" }
func (t *AlertListTool) Description() string { return "List monitor alerts. Use this to answer questions like 'which nodes have alerts' or 'which nodes have OWL-XXX-NNN alerts'. Supports filtering by alert code, node, group, severity and status." }
func (t *AlertListTool) Parameters() string  { return alertListParamsSchema }

const alertListParamsSchema = `{
	"type": "object",
	"properties": {
		"alert_type_id": {"type": "string", "description": "Alert code filter, e.g. OWL-DSK-001 (OWL_DSK_001 also accepted)"},
		"category": {"type": "string", "description": "Filter by category: disk/mem/cpu/net/svc/err/avail/custom"},
		"node": {"type": "string", "description": "Node name or ID"},
		"group": {"type": "string", "description": "Comma-separated node groups, e.g. 'web,db'"},
		"severity": {"type": "string", "enum": ["info", "warn", "critical"], "description": "Filter by severity"},
		"status": {"type": "string", "enum": ["active", "open", "acked", "resolved"], "description": "Default active (not resolved)"},
		"limit": {"type": "integer", "description": "Max rows to return, default 50"}
	}
}`

func (t *AlertListTool) Validate(p map[string]interface{}) error {
	switch s := strOf(p["severity"]); s {
	case "", "info", "warn", "critical":
	default:
		return fmt.Errorf("severity 必须是 info/warn/critical，got %q", s)
	}
	switch s := strOf(p["status"]); s {
	case "", "active", "open", "acked", "resolved":
	default:
		return fmt.Errorf("status 必须是 active/open/acked/resolved，got %q", s)
	}
	return nil
}

func (t *AlertListTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor == nil {
		return "", fmt.Errorf("alert_list 不可用：executor 未注入")
	}
	p := AlertListParams{
		AlertTypeID: NormalizeAlertCode(strOf(params["alert_type_id"])),
		Category:    strOf(params["category"]),
		Node:        strOf(params["node"]),
		Group:       strOf(params["group"]),
		Severity:    strOf(params["severity"]),
		Status:      strOf(params["status"]),
		Limit:       intOf(params["limit"]),
	}
	result, err := t.executor.ListAlerts(ctx, p)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// AlertTypesTool 列出全部告警码及触发规则（供 AI 解释 OWL_XXX 含义/纠正常见码写法）。
type AlertTypesTool struct {
	executor Executor
}

func NewAlertTypesTool(executor Executor) *AlertTypesTool {
	return &AlertTypesTool{executor: executor}
}

func (t *AlertTypesTool) Name() string        { return "alert_types" }
func (t *AlertTypesTool) Description() string { return "List all alert type codes (OWL-XXX-NNN) with their trigger rules. Use it to explain what an alert code means or to find the exact code for a category like disk/mem/cpu." }
func (t *AlertTypesTool) Parameters() string {
	return `{"type": "object", "properties": {}}`
}
func (t *AlertTypesTool) Validate(p map[string]interface{}) error { return nil }

func (t *AlertTypesTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor == nil {
		return "", fmt.Errorf("alert_types 不可用：executor 未注入")
	}
	result, err := t.executor.ListAlertTypes(ctx)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// AlertRemedyTool 查询告警码的修复对策（SOP/脚本/剧本/回滚），只读不执行。
type AlertRemedyTool struct {
	executor Executor
}

func NewAlertRemedyTool(executor Executor) *AlertRemedyTool {
	return &AlertRemedyTool{executor: executor}
}

func (t *AlertRemedyTool) Name() string { return "alert_remedy" }
func (t *AlertRemedyTool) Description() string {
	return "Get remedies for an alert code (OWL-XXX-NNN): step-by-step SOP, script/playbook remedies, risk level and rollback plan. Read-only: it NEVER executes anything."
}
func (t *AlertRemedyTool) Parameters() string { return alertRemedyParamsSchema }

const alertRemedyParamsSchema = `{
	"type": "object",
	"properties": {
		"alert_type_id": {"type": "string", "description": "Alert code, e.g. OWL-DSK-001 (required)"}
	},
	"required": ["alert_type_id"]
}`

func (t *AlertRemedyTool) Validate(p map[string]interface{}) error {
	if strOf(p["alert_type_id"]) == "" {
		return fmt.Errorf("alert_type_id 必填，如 OWL-DSK-001")
	}
	return nil
}

func (t *AlertRemedyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor == nil {
		return "", fmt.Errorf("alert_remedy 不可用：executor 未注入")
	}
	p := AlertRemedyParams{AlertTypeID: NormalizeAlertCode(strOf(params["alert_type_id"]))}
	result, err := t.executor.GetAlertRemedies(ctx, p)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// ---------- CLIExecutor 告警方法（数据经 AlertData 委托） ----------

// SetAlertData 注入告警数据委托（owl ai / TUI 装配时调用）。
// 未注入时告警工具返回友好错误，其余功能不受影响。
func (e *CLIExecutor) SetAlertData(d AlertData) {
	e.alertData = d
}

func (e *CLIExecutor) ListAlerts(ctx context.Context, p AlertListParams) (*AlertListResult, error) {
	if e.alertData == nil {
		return nil, fmt.Errorf("告警数据不可用：本地监控数据库未初始化（请先运行 owl serve 初始化监控）")
	}
	rows, total, err := e.alertData.ListAlerts(p)
	if err != nil {
		return nil, err
	}
	return &AlertListResult{Text: RenderAlertList(rows, total)}, nil
}

func (e *CLIExecutor) ListAlertTypes(ctx context.Context) (*AlertTypesResult, error) {
	if e.alertData == nil {
		return nil, fmt.Errorf("告警数据不可用：本地监控数据库未初始化（请先运行 owl serve 初始化监控）")
	}
	rows, err := e.alertData.ListAlertTypes()
	if err != nil {
		return nil, err
	}
	return &AlertTypesResult{Text: RenderAlertTypes(rows)}, nil
}

func (e *CLIExecutor) GetAlertRemedies(ctx context.Context, p AlertRemedyParams) (*AlertRemedyResult, error) {
	if e.alertData == nil {
		return nil, fmt.Errorf("告警数据不可用：本地监控数据库未初始化（请先运行 owl serve 初始化监控）")
	}
	remedies, err := e.alertData.ListRemedies(p.AlertTypeID)
	if err != nil {
		return nil, err
	}
	types, err := e.alertData.ListAlertTypes()
	if err != nil {
		return nil, err
	}
	var typeRow *AlertTypeRow
	for i := range types {
		if types[i].ID == p.AlertTypeID {
			typeRow = &types[i]
			break
		}
	}
	return &AlertRemedyResult{Text: RenderRemedies(p.AlertTypeID, typeRow, remedies)}, nil
}
