package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 「新建对话」曾把 sessionId 重置回页面加载时的服务端会话（initialSessionId）。
// 用户先聊过再点新建时，该会话已带上一轮完整历史，服务端 GetOrLoadSession
// 恢复上下文后"新对话"延续旧话题，表现为 fork。修复后新建对话必须向服务端
// 请求全新会话键（GET /ai/session-key 每次生成新 UUID），并在发送前确保
// 会话键就绪（加密 API key 与会话 RSA 密钥对成对绑定，不能用旧公钥配新会话）。
func TestWebUIAINewConversation_FreshServerSession(t *testing.T) {
	js := readWebLF(t, "web/js/pages/ai.js")

	assert.NotContains(t, js, "initialSessionId",
		"newConversation must not fall back to the page-load session; that session may already hold prior history")

	start := strings.Index(js, "function newConversation")
	require.NotEqual(t, -1, start, "newConversation function must exist")
	end := strings.Index(js[start:], "\n  }")
	require.NotEqual(t, -1, end, "newConversation function body must be closable")
	body := js[start : start+end]

	assert.Contains(t, body, "ensureSessionKey()",
		"newConversation must request a fresh server session key")
	assert.Contains(t, body, "sessionId = null",
		"newConversation must drop the stale session id before the new one arrives")
}

// 会话列表的搜索 / 无限滚动 / 常驻删除，以及对话上下文分页的静态断言：
// 这些能力依赖 IndexedDB 全量取回 + IntersectionObserver 哨兵 + 后端 has_more，
// 缺一即静默失效（列表卡在首屏、搜索无效、删按钮触屏不可见）。
func TestWebUIAIConvSearchAndInfiniteScroll(t *testing.T) {
	storage := readWebLF(t, "web/js/storage.js")
	ai := readWebLF(t, "web/js/pages/ai.js")
	api := readWebLF(t, "web/js/api.js")
	css := readWebLF(t, "web/css/app.css")

	assert.Contains(t, storage, "getConversationsPage",
		"storage.js must expose cursor pagination via compound index")
	assert.Contains(t, storage, "user_created",
		"storage.js must define the [userId, createdAt] compound index")
	assert.Contains(t, storage, "searchConversations",
		"storage.js must expose title+body search")

	assert.Contains(t, ai, "getConversationsPage(userId",
		"ai.js must page conversations by user via the compound index (no full load)")
	assert.Contains(t, ai, "searchConversations",
		"ai.js must search conversations by title + body")
	assert.Contains(t, ai, "ai-conv-search", "ai.js must wire the conversation search box")
	assert.Contains(t, ai, "ai-conv-sentinel", "ai.js must render an infinite-scroll sentinel for conversations")
	assert.Contains(t, ai, "ai-ctx-sentinel", "ai.js must render an infinite-scroll sentinel for context")
	assert.Contains(t, ai, "IntersectionObserver", "ai.js must use IntersectionObserver for lazy loading")
	assert.Contains(t, ai, "has_more", "ai.js must honor the backend has_more flag for context paging")

	assert.Contains(t, api, "/ai/context?offset=", "api.getAiContext must pass offset/limit")

	assert.Contains(t, css, ".ai-conv-search", "css must style the conversation search box")
	assert.Contains(t, css, ".ai-conv-sentinel", "css must style the conversation sentinel")
	// 删除按钮常驻（不再依赖 :hover，触屏可用）
	deleteBlock := css[strings.Index(css, ".ai-conv-item-delete {"):]
	deleteBlock = deleteBlock[:strings.Index(deleteBlock, "}")]
	assert.Contains(t, deleteBlock, "display: grid", "delete button must be always visible, not hover-only")
}
