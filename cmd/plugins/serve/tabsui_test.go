package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// M1 标签模型：标签栏、标签集合、URL 同步与刷新恢复的骨架必须在位。
func TestTabs_ModelAndBar(t *testing.T) {
	app := readWebFile(t, "web/js/app.js")
	tabs := readWebFile(t, "web/js/tabs.js")
	css := readWebFile(t, "web/css/app.css")

	assert.Contains(t, tabs, "export function createTabStore",
		"tabs.js must expose createTabStore()")
	assert.Contains(t, tabs, "owl-tabs",
		"标签集合必须持久化到 localStorage，刷新后恢复")
	assert.Contains(t, tabs, "MAX_TABS",
		"标签数需要上限（保活/内存可控）")
	assert.Contains(t, tabs, "snapshots",
		"每个标签要能存页面状态快照（重建式标签切回时恢复上下文）")

	assert.Contains(t, app, "createTabStore()", "app.js 必须创建标签库")
	assert.Contains(t, app, `class="tabbar"`, "壳层必须渲染标签栏")
	assert.Contains(t, app, "renderTabbar", "标签栏需要渲染函数")
	assert.Contains(t, app, "activateTab(", "必须有激活标签的入口")
	assert.Contains(t, app, "closeTab(", "必须能关闭标签")
	assert.Contains(t, app, "openView(", "导航入口要按标签语义复用/新建标签")
	assert.Contains(t, app, "bootTabs(", "启动时要对齐持久化标签与当前 URL")
	assert.Contains(t, app, "syncActiveTabRoute(", "路由变化要同步到激活标签")

	assert.Contains(t, css, ".tabbar", "app.css must style the tab bar")
	assert.Contains(t, css, ".tab {", "app.css must style tabs")
	assert.Contains(t, css, ".tab-close", "app.css must style the close button")
	assert.Contains(t, css, ".tab-menu", "app.css must style the context menu")
}

// 标签页状态快照契约：scope 提供 restoreState/persistState，列表页按标签恢复上下文。
func TestTabs_PageStateSnapshot(t *testing.T) {
	scope := readWebFile(t, "web/js/pagescope.js")
	assert.Contains(t, scope, "restoreState(defaults)",
		"scope 必须提供 restoreState(defaults) 声明状态形状并取回快照")
	assert.Contains(t, scope, "persistState(getter)",
		"scope 必须提供 persistState(getter) 注册快照提取器")
	assert.Contains(t, scope, "takeSnapshot()", "app.js 切走前要能取一次快照")
	assert.Contains(t, scope, "tabId", "页面需要 tabId 给按标签命名的存储键（如勾选状态）")

	pages := []string{
		"nodes", "playbooks", "alerts", "history", "users", "exec", "files", "settings",
	}
	for _, p := range pages {
		src := readWebFile(t, "web/js/pages/"+p+".js")
		assert.Contains(t, src, "scope.restoreState(", "%s.js 必须恢复本标签的状态快照", p)
		assert.Contains(t, src, "scope.persistState(", "%s.js 必须注册状态快照", p)
		// 用了 scope 就必须在签名里接住它：只改函数体忘改参数会得到运行期
		// "scope is not defined"（M1 实施时 users/exec/playbooks 三页正是这么漏的）
		// 取渲染函数签名：从 export 到第一个 {（不假设行尾，工作区是 CRLF）
		start := strings.Index(src, "export ")
		sig := src[start : start+strings.Index(src[start:], "{")]
		assert.Contains(t, sig, "scope)", "%s.js 的渲染函数签名必须接收 scope 参数", p)
	}
}

// 勾选类状态（sessionStorage）必须按标签命名空间，否则同一个页面的两个标签会互相覆盖。
func TestTabs_TabScopedStorage(t *testing.T) {
	for _, p := range []string{"exec", "files"} {
		src := readWebFile(t, "web/js/pages/"+p+".js")
		assert.NotContains(t, src, "sessionStorage.getItem('"+p+"_selected_nodes')",
			"%s.js 的勾选不能再存全局 sessionStorage 键（两个标签会串）", p)
		assert.Contains(t, src, "storageKey",
			"%s.js 应使用按标签命名的存储键", p)
	}
}

// 标签栏交互：中键/右键/Ctrl 点击与快捷键
func TestTabs_Interactions(t *testing.T) {
	app := readWebFile(t, "web/js/app.js")
	for _, want := range []string{
		"auxclick",          // 中键关闭标签
		"contextmenu",       // 右键菜单
		"dragstart", "drop", // 拖拽排序
		"newTab(",           // Alt+T 新建
		"nextTab(",          // Alt+PageUp/Down 切换
	} {
		assert.True(t, strings.Contains(app, want), "标签栏交互缺少 %s", want)
	}
}
