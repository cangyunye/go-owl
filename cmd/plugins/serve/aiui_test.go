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
