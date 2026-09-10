package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

const originTestSecret = "test-secret-32byte-long-string!!"

func wsOriginTestRouter(t *testing.T) (*gin.Engine, *service.AuthService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	as := service.NewAuthService(originTestSecret)
	// WsHandler/Terminal 只用 auth 验签，无需用户存储
	ah := NewAuthHandler(nil, as)
	hub := NewWSHub()

	r := gin.New()
	r.GET("/api/v1/ws", hub.WsHandler(ah))
	r.GET("/api/v1/session/terminal", NewTerminalHandler(nil, as).Terminal)
	return r, as
}

// TestWSHandler_RejectsUnknownRole 未知角色（签名有效但角色非法）不得建连。
func TestWSHandler_RejectsUnknownRole(t *testing.T) {
	r, as := wsOriginTestRouter(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	token, err := as.GenerateToken("ghost", "superuser")
	require.NoError(t, err)

	resp, err := http.Get(srv.URL + "/api/v1/ws?token=" + token)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestWSHandler_RejectsCrossOrigin 跨站页面发起连接必须被拒（同源校验）。
func TestWSHandler_RejectsCrossOrigin(t *testing.T) {
	r, as := wsOriginTestRouter(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	token, err := as.GenerateToken("viewer1", "viewer")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err = websocket.Dial(ctx, srv.URL+"/api/v1/ws?token="+token, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://evil.example"}},
	})
	require.Error(t, err, "跨源 WebSocket 连接必须失败")
	assert.True(t, strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "bad handshake"),
		"期望握手被拒，实际: %v", err)
}

// TestWSHandler_AcceptsSameOrigin 无 Origin 头（非浏览器客户端）与同源连接应放行。
func TestWSHandler_AcceptsSameOrigin(t *testing.T) {
	r, as := wsOriginTestRouter(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	token, err := as.GenerateToken("viewer1", "viewer")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, srv.URL+"/api/v1/ws?token="+token, nil)
	require.NoError(t, err)
	conn.CloseNow()
}

// TestTerminalHandler_RejectsCrossOrigin 终端同样不接受跨源连接。
func TestTerminalHandler_RejectsCrossOrigin(t *testing.T) {
	r, as := wsOriginTestRouter(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	token, err := as.GenerateToken("root", "admin")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err = websocket.Dial(
		ctx,
		srv.URL+"/api/v1/session/terminal?token="+token+"&node_id=whatever",
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"http://evil.example"}}},
	)
	require.Error(t, err, "跨源终端连接必须失败")
}

// TestTerminalHandler_RequiresOperator 低于 operator 的角色不得建立终端。
func TestTerminalHandler_RequiresOperator(t *testing.T) {
	r, as := wsOriginTestRouter(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	token, err := as.GenerateToken("viewer1", "viewer")
	require.NoError(t, err)

	resp, err := http.Get(srv.URL + "/api/v1/session/terminal?token=" + token + "&node_id=whatever")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestKeyManager_Cleanup 回收超期的 AI 会话密钥（此前 Cleanup 无调用方）。
func TestKeyManager_Cleanup(t *testing.T) {
	km := NewKeyManager()

	old, err := km.CreateSession()
	require.NoError(t, err)
	fresh, err := km.CreateSession()
	require.NoError(t, err)

	km.mu.Lock()
	km.sessions[old.SessionID].CreatedAt = time.Now().Add(-2 * time.Hour)
	km.mu.Unlock()

	km.Cleanup(time.Hour)

	km.mu.RLock()
	_, oldStillThere := km.sessions[old.SessionID]
	_, freshStillThere := km.sessions[fresh.SessionID]
	km.mu.RUnlock()

	assert.False(t, oldStillThere, "超期会话密钥应被回收")
	assert.True(t, freshStillThere, "未超期的会话密钥应保留")
}
