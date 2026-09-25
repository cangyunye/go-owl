package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// M0 资源纪律：页面需要回收的资源（定时器/事件监听/浮层/WS/流）统一走
// `scope.resources` 注册，由 app.js 在每次导航时释放。历史上这类资源靠各页
// 自己写 cleanup，漏一处就永久残留（files 5s 轮询的 stopAutoRefresh 全仓无调用点、
// nodes/settings 的 document keydown 每次挂载加一条）。
func TestPageScope_RegistrarExists(t *testing.T) {
	app := readWebFile(t, "web/js/app.js")
	scope := readWebFile(t, "web/js/pagescope.js")

	assert.Contains(t, scope, "export function createPageScope",
		"pagescope.js must expose createPageScope()")
	for _, m := range []string{"setInterval", "setTimeout", "on(", "onDispose", "overlay", "ws("} {
		assert.True(t, strings.Contains(scope, m),
			"createPageScope must provide %s for reclaimable resources", m)
	}

	assert.Contains(t, app, "createPageScope(",
		"app.js must create a page scope per navigation")
	assert.Contains(t, app, "currentScope.dispose()",
		"app.js must dispose the previous page scope when navigating away")
	assert.Contains(t, app, "beginPage(",
		"app.js must funnel page lifecycle through beginPage()")
	assert.Contains(t, app, "scope.resources",
		"pages are handed the scope so they can register reclaimable resources")
	assert.Contains(t, scope, "host.appendChild(el)",
		"resources.overlay 必须自己把浮层挂到 body（只登记不挂载会让弹窗永不出现）")
}

// 已知泄漏点必须改走 scope：这些断言是防回退，不是风格要求。
func TestPageScope_LeakSitesUseRegistrar(t *testing.T) {
	files := readWebFile(t, "web/js/pages/files.js")
	assert.Contains(t, files, "resources.setInterval(loadTransfers",
		"文件页 5s 轮询必须注册到页面作用域（原 stopAutoRefresh 全仓无调用点）")
	assert.NotContains(t, files, "function stopAutoRefresh",
		"轮询回收由作用域负责，留着无人调用的 stopAutoRefresh 是回退信号")

	nodes := readWebFile(t, "web/js/pages/nodes.js")
	assert.NotContains(t, nodes, "document.addEventListener('keydown'",
		"节点页 keydown 必须走 scope.resources.on（历史上每次挂载泄漏一条）")

	settings := readWebFile(t, "web/js/pages/settings.js")
	assert.NotContains(t, settings, "document.addEventListener('keydown'",
		"设置页 keydown 必须走 scope.resources.on（历史上每次挂载泄漏一条）")

	alerts := readWebFile(t, "web/js/pages/alerts.js")
	assert.NotContains(t, alerts, "document.body.appendChild",
		"告警页浮层必须用 scope.resources.overlay 注册，避免切页残留")
	assert.Contains(t, alerts, "resources.setTimeout",
		"告警轮询必须走 scope.resources.setTimeout，切页可取消")

	history := readWebFile(t, "web/js/pages/history.js")
	assert.NotContains(t, history, "document.body.appendChild",
		"任务历史详情浮层必须用 scope.resources.overlay 注册")

	sftp := readWebFile(t, "web/js/pages/sftp.js")
	assert.NotContains(t, sftp, "document.body.appendChild",
		"sftp 浮层必须用 scope.resources.overlay 注册")
}

// AI 流式回答要在切页时中止：否则 SSE 继续读、继续写已卸载的 DOM。
func TestAIChatStream_Abortable(t *testing.T) {
	api := readWebFile(t, "web/js/api.js")
	ai := readWebFile(t, "web/js/pages/ai.js")

	assert.Contains(t, api, "signal: handlers.signal",
		"aiChatStream must pass the caller's AbortSignal to fetch")
	assert.Contains(t, ai, "new AbortController()",
		"ai.js must create an AbortController per stream")
	assert.Contains(t, ai, "signal.aborted",
		"ai.js must not fall back to blocking chat / show errors after an abort")
	assert.Contains(t, ai, "ac.abort()",
		"ai.js must abort the in-flight stream when the page scope is disposed")
}
