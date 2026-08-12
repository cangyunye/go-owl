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
