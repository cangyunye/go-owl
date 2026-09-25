package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 运行/步骤状态徽章必须有配色：playbooks.js 用 status-${status} 渲染
// （queued/pending/running/completed/success/failed/cancelled/partial_failure），
// app.css 此前只定义了 online/offline/unknown，运行状态全部渲染为无色文本。
func TestPlaybooksUI_RunStatusBadgePalette(t *testing.T) {
	css := readWebFile(t, "web/css/app.css")

	for _, cls := range []string{
		".status-queued", ".status-pending", ".status-running", ".status-completed",
		".status-success", ".status-failed", ".status-cancelled", ".status-partial_failure",
	} {
		assert.True(t, strings.Contains(css, cls+",") || strings.Contains(css, cls+" {"),
			"app.css must style run-status badge %s", cls)
	}
}

// 主从分栏：左侧剧本列表、右侧详情，替代旧的四卡纵叠+详情弹窗。
// Library Path 收进主栏工具栏的折叠行，不再独占一张 section-card。
func TestPlaybooksUI_MasterDetailLayout(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")
	css := readWebFile(t, "web/css/app.css")

	assert.True(t, strings.Contains(src, "pb-layout"), "playbooks.js must render the master-detail pb-layout")
	assert.True(t, strings.Contains(src, "pb-master"), "playbooks.js must render the left master column")
	assert.True(t, strings.Contains(src, "pb-detail-col"), "playbooks.js must render the right detail column")
	assert.True(t, strings.Contains(src, "pb-detail-card"), "playbooks.js must render the playbook detail card in the right column")
	assert.False(t, strings.Contains(src, "playbook-detail-modal"),
		"the detail modal must be replaced by the right-column detail card")
	assert.True(t, strings.Contains(src, "selectPlaybook("),
		"clicking a row/card must select the playbook into the detail card")
	assert.True(t, strings.Contains(src, "pb-lib-row"),
		"library path must live in a collapsible toolbar row, not its own section card")
	assert.True(t, strings.Contains(css, ".pb-layout"),
		"app.css must define the pb-layout two-column grid")
}

// 运行历史/运行详情必须放在 pb-layout 分栏之外全宽渲染：右栏分到的宽度
// （约 560px）放不下运行详情的 meta 行 + 6 列步骤结果表，表头会竖排折行、
// 输出列被挤成一条缝。
func TestPlaybooksUI_RunsSectionFullWidth(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")

	idxDetailCol := strings.LastIndex(src, "pb-detail-col")
	idxRunsCard := strings.Index(src, `id="pb-runs-card"`)
	idxRunDetail := strings.Index(src, `id="run-detail-card"`)

	assert.Contains(t, src, `id="pb-runs-card"`, "run history must be its own full-width section card")
	assert.Greater(t, idxRunsCard, idxDetailCol,
		"run history card must be rendered after (outside) the pb-detail-col column")
	assert.Greater(t, idxRunDetail, idxRunsCard,
		"run detail card must follow the history card at full width")
	assert.False(t, strings.Contains(src, "右栏：详情 + 运行历史"),
		"the right column must no longer stack the run history/detail cards")
}

// 视图切换：表格 ↔ 卡片（.seg 切换，选择持久化到 localStorage）。
func TestPlaybooksUI_ListViewToggle(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")

	assert.True(t, strings.Contains(src, "pb-view-table"), "table view toggle button must exist")
	assert.True(t, strings.Contains(src, "pb-view-grid"), "grid view toggle button must exist")
	assert.True(t, strings.Contains(src, "playbook-card"),
		"grid view must render the existing .playbook-card component")
	assert.True(t, strings.Contains(src, "owl-pb-view"),
		"the chosen view mode must persist in localStorage")
}

// 分类彩色化：分类名哈希到 tag-rN 彩虹色，侧栏圆点与列表标签同色。
func TestPlaybooksUI_CategoryColors(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")

	assert.True(t, strings.Contains(src, "function tagColor("),
		"playbooks.js must hash category names to stable tag-rN classes")
}

// YAML 高亮：详情卡片与向导预览都要用 yaml-preview token 类上色。
func TestPlaybooksUI_YamlHighlight(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")
	css := readWebFile(t, "web/css/app.css")

	assert.True(t, strings.Contains(src, "function highlightYAML("),
		"playbooks.js must implement a YAML highlighter")
	assert.True(t, strings.Contains(src, "yaml-key"), "highlighter must emit yaml-key tokens")
	assert.True(t, strings.Contains(src, "yaml-str"), "highlighter must emit yaml-str tokens")
	assert.True(t, strings.Contains(src, "yaml-preview"), "detail card must use the yaml-preview block")
	assert.True(t, strings.Contains(css, ".yaml-num"),
		"app.css must style numeric yaml tokens")
}

// Run 弹窗对齐 exec 页设计语言：node-chip 胶囊选节点、group-chip 选分组、
// 中转站收进 adv-toggle 折叠区、Extra Vars 用 key/value 行编辑。
func TestPlaybooksUI_RunModalChips(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")

	assert.True(t, strings.Contains(src, "node-chip"), "node picker must use node-chip pills")
	assert.True(t, strings.Contains(src, "group-chip"), "group picker must use group-chip pills")
	assert.True(t, strings.Contains(src, "adv-toggle"), "staging browser must collapse behind adv-toggle")
	assert.True(t, strings.Contains(src, "run-vars-list"), "extra vars must use key/value row editor")
}

// 提示统一：alert/confirm 全部替换为 toast + 两段式取消确认；
// 取消按钮的一阶段状态必须能扛住 WS 重绘（状态存 model，不存 DOM）。
func TestPlaybooksUI_ToastAndTwoStageCancel(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")

	assert.False(t, strings.Contains(src, "alert("), "playbooks.js must not use native alert()")
	assert.False(t, strings.Contains(src, "confirm("), "playbooks.js must not use native confirm()")
	assert.False(t, strings.Contains(src, "prompt("), "playbooks.js must not use native prompt()")
	assert.True(t, strings.Contains(src, "function showToast("), "playbooks.js must implement a toast helper")
	assert.True(t, strings.Contains(src, "pendingCancelId"),
		"cancel confirmation state must live in state, surviving re-renders")
	assert.True(t, strings.Contains(src, "确认取消"),
		"cancel button must switch to an explicit confirm label")
}

// 向导瘦身：5 步合并为 3 步、步骤可点击跳转、任务卡按 action 类型渲染专属字段、
// 支持拖拽排序（复用 exec 快捷方式的 draggable 模式）。
func TestPlaybooksUI_WizardThreeSteps(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")

	assert.True(t, strings.Contains(src, "totalSteps: 3"),
		"wizard must collapse to 3 steps (基本信息+配置 / 任务与变量 / 确认)")
	assert.True(t, strings.Contains(src, "wiz-step"), "step indicator must be clickable wiz-step buttons")
	assert.True(t, strings.Contains(src, "ACTION_FIELDS"),
		"task cards must render per-action typed fields")
	assert.True(t, strings.Contains(src, `draggable="true"`), "task cards must support drag reorder")
}

// 视图切换清理：离开剧本页必须关闭 WebSocket，否则每次进入都留下一条
// 永久重连的悬挂连接。
func TestPlaybooksUI_WsClosedOnLeave(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")

	assert.True(t, strings.Contains(src, "ws.close()"),
		"the page cleanup function must close the websocket")
	assert.True(t, strings.Contains(src, "return () => {"),
		"afterRender must return a cleanup function for the shell")
}

// 运行详情要有进度反馈：进度条 + 只看失败过滤。
func TestPlaybooksUI_RunDetailProgress(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")
	css := readWebFile(t, "web/css/app.css")

	assert.True(t, strings.Contains(src, "run-progress"), "run detail must render a progress bar")
	assert.True(t, strings.Contains(src, "仅看失败"), "run detail must offer a failed-only filter")
	assert.True(t, strings.Contains(css, ".run-progress"), "app.css must style the progress bar")
}

// 运行弹窗的「目标节点」必须拿到全量节点：GET /nodes 默认 page_size=20、
// 服务端封顶 100，裸调 api.nodes() 只拿到第一页 —— 节点多时目标节点只列前 20 个、
// 「全选」也只选中这 20 个、分组徽标计数按第一页统计偏小。
// 与节点页 loadGroupCounts 同款问题，改走 api.nodesAll() 按 meta.total 翻页取全量。
func TestPlaybooksUI_RunTargetNodesLoadAll(t *testing.T) {
	apiSrc := readWebFile(t, "web/js/api.js")
	src := readWebFile(t, "web/js/pages/playbooks.js")

	assert.Contains(t, apiSrc, "nodesAll",
		"api.js must expose nodesAll() that pages /nodes until meta.total is reached")
	assert.Contains(t, apiSrc, "page_size: 100",
		"nodesAll() must page with the server-side max page_size (100)")

	assert.NotContains(t, src, "api.nodes()",
		"run dialog must not call api.nodes() bare (returns only the first page of 20)")
	assert.Contains(t, src, "api.nodesAll()",
		"run dialog target-node picker must load all nodes via api.nodesAll()")
}

// 同类缺陷：其余「节点选择器」调用点也必须走全量拉取，
// 否则节点数超过 100（服务端 page_size 上限）时同样只列部分节点、总数偏小。
func TestNodePickers_LoadAllNodes(t *testing.T) {
	tasks := readWebFile(t, "web/js/pages/tasks.js")
	alerts := readWebFile(t, "web/js/pages/alerts.js")

	assert.Contains(t, tasks, "api.nodesAll()",
		"任务中心的节点选择器必须拉全量节点（#exec-total-count 直接展示该列表长度）")
	assert.NotContains(t, tasks, "page_size: 500",
		"page_size 超上限会被服务端钳到 100，等于只取第一页")
	assert.Contains(t, alerts, "api.nodesAll()",
		"告警调试弹窗的节点下拉必须拉全量节点")
}
