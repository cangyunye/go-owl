package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readWebFile(t *testing.T, name string) string {
	t.Helper()
	b, err := webFS.ReadFile(name)
	require.NoError(t, err, "read %s", name)
	return string(b)
}

func TestFilesJS_UploadButton_SingleClickTriggersTransfer(t *testing.T) {
	src := readWebFile(t, "web/js/pages/files.js")

	assert.True(t, strings.Contains(src, "upload-btn"), "files.js must reference upload-btn")
	assert.True(t, strings.Contains(src, "handleTransfer('push')"), "upload must trigger handleTransfer('push')")

	for _, btn := range []string{"upload-btn", "download-btn"} {
		assert.False(t, strings.Contains(src, "dblclick"),
			"transfer must not be gated behind double-click for %s", btn)
	}
}

func TestAIStorage_ConversationsScopedPerUser(t *testing.T) {
	src := readWebFile(t, "web/js/storage.js")

	assert.True(t, strings.Contains(src, "DB_VERSION: 2"),
		"storage.js must bump DB version to add the user_id index")
	assert.True(t, strings.Contains(src, "createIndex('user_id'"),
		"storage.js must create a user_id index on conversations")
	assert.True(t, strings.Contains(src, "conv.userId = userId"),
		"storage.js must stamp the owner on saved conversations")
	assert.True(t, strings.Contains(src, "IDBKeyRange.only(userId)"),
		"storage.js must filter conversations by userId")
}

func TestAIJS_PassesUserIdToStorage(t *testing.T) {
	src := readWebFile(t, "web/js/pages/ai.js")

	assert.True(t, strings.Contains(src, "saveConversation(conv, userId)"),
		"ai.js must persist conversations with the current user id")
	assert.True(t, strings.Contains(src, "getConversations(userId, 50, 0)"),
		"ai.js must load conversations filtered by the current user id")
	assert.True(t, strings.Contains(src, "userId + '::'"),
		"ai.js must namespace new conversation ids by user to avoid cross-user collisions")
}

func TestPlaybooksJS_RunViewClickSurvivesRerender(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")

	assert.True(t, strings.Contains(src, "addEventListener('pointerdown'"),
		"playbooks.js must capture run action intent on pointerdown")
	assert.True(t, strings.Contains(src, "addEventListener('pointerup'"),
		"playbooks.js must execute run actions on pointerup (click can be swallowed by re-render)")
	assert.True(t, strings.Contains(src, "closest('.view-run-btn, .cancel-run-btn')"),
		"playbooks.js must resolve run action buttons via closest() under delegation")
	assert.True(t, strings.Contains(src, "runDelegated"),
		"playbooks.js must bind the delegated listener only once")
	assert.True(t, strings.Contains(src, "showRunDetailError"),
		"playbooks.js must surface run-detail load failures instead of swallowing them")
}

func TestUsersJS_PaginationAndSearch(t *testing.T) {
	src := readWebFile(t, "web/js/pages/users.js")

	assert.True(t, strings.Contains(src, "api.users("),
		"users.js must load users via api with query params")
	assert.True(t, strings.Contains(src, "page_size"),
		"users.js must request a bounded page_size")
	assert.True(t, strings.Contains(src, "meta"),
		"users.js must read meta.total for pagination")
	assert.True(t, strings.Contains(src, "user-search-input"),
		"users.js must render a search input")
	assert.True(t, strings.Contains(src, "user-prev-btn"),
		"users.js must render a prev page button")
	assert.True(t, strings.Contains(src, "user-next-btn"),
		"users.js must render a next page button")
}

func TestUsersJS_RolePanel(t *testing.T) {
	src := readWebFile(t, "web/js/pages/users.js")

	assert.True(t, strings.Contains(src, "setPanelContent"),
		"users.js must populate the left 用户角色 panel via shell.setPanelContent")
	assert.True(t, strings.Contains(src, "role_counts"),
		"users.js must read per-role counts from the users API meta")
	assert.True(t, strings.Contains(src, "data-panel-role"),
		"users.js must render one panel item per role, filterable by role")
	assert.True(t, strings.Contains(src, "state.role"),
		"users.js must keep a role filter state")
	assert.False(t, strings.Contains(src, "panel-user-search"),
		"users.js must not render a per-user search box in the role panel anymore")
}

func TestExecJS_SafetyConfirmations(t *testing.T) {
	src := readWebFile(t, "web/js/pages/exec.js")

	assert.True(t, strings.Contains(src, "hasTargetFilter()"),
		"exec.js must detect whether any target condition (nodes/groups/labels) is set")
	assert.True(t, strings.Contains(src, "countExecTargetNodes()"),
		"exec.js must count the actual execution target nodes before submitting")
	assert.True(t, strings.Contains(src, "未选择任何分组/标签"),
		"exec.js must warn when no group/label condition is set (full-scope execution)")
	assert.True(t, strings.Contains(src, "targetCount > 50"),
		"exec.js must confirm before executing on more than 50 nodes")
}

func TestFilesJS_TransferSafetyConfirmations(t *testing.T) {
	src := readWebFile(t, "web/js/pages/files.js")

	assert.True(t, strings.Contains(src, "hasTargetFilter()"),
		"files.js must detect whether any target condition (nodes/groups/labels) is set")
	assert.True(t, strings.Contains(src, "countTransferTargetNodes()"),
		"files.js must count the actual transfer target nodes before submitting")
	assert.True(t, strings.Contains(src, "未选择任何分组/标签"),
		"files.js must warn when no group/label condition is set (full-scope transfer)")
	assert.True(t, strings.Contains(src, "targetCount > 50"),
		"files.js must confirm before transferring to more than 50 nodes")
}

// 传输记录展示必须能区分同名文件的新旧记录，且提交≠完成：
// - 记录行用绝对时间（相对时间每 5s 轮询都会跳动，旧记录看起来像在被刷新）
// - 记录行带方向徽标（上传/下载）
// - 批量传输提交后不得声称"完成"（HTTP 202 只代表任务已排队）
func TestFilesJS_TransferRecordClarity(t *testing.T) {
	src := readWebFile(t, "web/js/pages/files.js")

	assert.True(t, strings.Contains(src, "fmtTime(r.created_at)"),
		"record rows must show absolute creation time (ticking relative time makes old rows look refreshed)")
	assert.True(t, strings.Contains(src, "r.direction === 'pull'"),
		"record rows must render a direction badge so same-name records are distinguishable")
	assert.False(t, strings.Contains(src, "批量传输完成"),
		"batch transfer must not claim completion right after submit (202 means queued, not finished)")
	assert.True(t, strings.Contains(src, "后台进行"),
		"batch submit message must point users to the transfer records for progress")
}

// 运行历史必须真分页（用户已确认分页控件形态）：带 page/page_size 请求、
// 读 meta.total、渲染翻页按钮与页码信息。此前固定只显示最近 50 条。
func TestPlaybooksJS_RunHistoryPagination(t *testing.T) {
	src := readWebFile(t, "web/js/pages/playbooks.js")

	assert.True(t, strings.Contains(src, "page_size"),
		"playbooks.js must request a bounded page_size for run history")
	assert.True(t, strings.Contains(src, "meta"),
		"playbooks.js must read meta.total for run-history pagination")
	assert.True(t, strings.Contains(src, "runs-prev-btn"),
		"playbooks.js must render a prev page button")
	assert.True(t, strings.Contains(src, "runs-next-btn"),
		"playbooks.js must render a next page button")
	assert.True(t, strings.Contains(src, "runs-page-info"),
		"playbooks.js must render page position info")
}

// 传输列表轮询(5s)失败时必须保留上一次数据：任何一次请求抖动都把列表
// 清空，会表现为"传输记录突然丢失显示暂无、下一轮又闪现回来"。
func TestFilesJS_PollFailureKeepsLastData(t *testing.T) {
	src := readWebFile(t, "web/js/pages/files.js")

	assert.False(t, strings.Contains(src, "catch { transfers = [];"),
		"loadTransfers must not wipe transfer data on a transient fetch error")
	assert.True(t, strings.Contains(src, "loadTransfers"),
		"files.js must keep the polling loadTransfers flow")
}

// 节点页分组计数必须按 meta.total 翻页拉全量节点后再统计：
// 单次超大 page_size 会被服务端钳制到 100，统计只覆盖第一页节点。
func TestNodesJS_GroupCountsPageThroughAllNodes(t *testing.T) {
	src := readWebFile(t, "web/js/pages/nodes.js")

	assert.False(t, strings.Contains(src, "page_size: 1000"),
		"group counts must not rely on a single oversized page request")
	assert.True(t, strings.Contains(src, "page_size: 100"),
		"group counts must fetch nodes in pages of 100")
	assert.True(t, strings.Contains(src, "meta.total"),
		"group counts must loop pages using meta.total")
}

func TestExecJS_ShortcutBar(t *testing.T) {
	src := readWebFile(t, "web/js/pages/exec.js")

	assert.True(t, strings.Contains(src, "id=\"shortcut-chips\""), "exec.js must render a shortcut chip container")
	assert.True(t, strings.Contains(src, "id=\"add-shortcut-btn\""), "exec.js must expose an add-shortcut button")
	assert.True(t, strings.Contains(src, "api.shortcuts()"), "exec.js must load shortcuts via api")
	assert.True(t, strings.Contains(src, "reorderShortcuts"), "exec.js must persist drag-drop order")
	assert.True(t, strings.Contains(src, "draggable=\"true\""), "exec.js must make chips draggable")
	assert.True(t, strings.Contains(src, "switchExecMode('command')"), "exec.js must switch to command mode when a chip is clicked")
	assert.True(t, strings.Contains(src, "openShortcutModal"), "exec.js must support add/edit modal")
	assert.True(t, strings.Contains(src, "deleteShortcut"), "exec.js must support delete")
}

func TestExecJS_StagingScriptSource(t *testing.T) {
	src := readWebFile(t, "web/js/pages/exec.js")

	assert.True(t, strings.Contains(src, `data-script-src="staging"`),
		"exec.js must expose a staging script source")
	assert.True(t, strings.Contains(src, "script-staging-row"),
		"exec.js must render the staging picker row")
	assert.True(t, strings.Contains(src, "script-staging-select"),
		"exec.js must render a staging file select")
	assert.True(t, strings.Contains(src, "api.staging.files()"),
		"exec.js must load staging files via the staging api")
	assert.True(t, strings.Contains(src, "payload.script_ref"),
		"exec.js must send script_ref in the exec payload")
	assert.True(t, strings.Contains(src, "script-save-staging"),
		"exec.js must offer saving an uploaded script to staging")
}
