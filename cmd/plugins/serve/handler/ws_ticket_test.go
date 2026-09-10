package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

// TestWSTicket_RedeemIsOneShot 同一票据只能核销一次（防重放）。
func TestWSTicket_RedeemIsOneShot(t *testing.T) {
	tm := NewWSTicketManager()
	ticket, err := tm.Issue("alice", model.RoleViewer)
	require.NoError(t, err)

	_, role, ok := tm.Redeem(ticket)
	require.True(t, ok)
	assert.Equal(t, model.RoleViewer, role)

	_, _, ok = tm.Redeem(ticket)
	assert.False(t, ok, "已核销的票据必须失效")
}

// TestWSTicket_ExpiredRejected 过期票据不可用。
func TestWSTicket_ExpiredRejected(t *testing.T) {
	now := time.Now()
	tm := NewWSTicketManager()
	tm.now = func() time.Time { return now }

	ticket, err := tm.Issue("alice", model.RoleViewer)
	require.NoError(t, err)

	now = now.Add(wsTicketTTL + time.Second)
	_, _, ok := tm.Redeem(ticket)
	assert.False(t, ok, "过期票据必须失效")
}

// TestWSTicket_ConcurrentRedeem 并发核销同一票据恰好成功一次。
func TestWSTicket_ConcurrentRedeem(t *testing.T) {
	tm := NewWSTicketManager()
	ticket, err := tm.Issue("alice", model.RoleViewer)
	require.NoError(t, err)

	const n = 16
	var success atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, ok := tm.Redeem(ticket); ok {
				success.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), success.Load(), "同一票据并发核销只应成功一次")
}

// TestWSTicket_CleanupKeepsMapBounded 签发时顺带清理过期项，map 不无界增长。
func TestWSTicket_CleanupKeepsMapBounded(t *testing.T) {
	now := time.Now()
	tm := NewWSTicketManager()
	tm.now = func() time.Time { return now }

	for i := 0; i < 100; i++ {
		_, err := tm.Issue("alice", model.RoleViewer)
		require.NoError(t, err)
		now = now.Add(time.Minute) // 每次签发跨过上一张的过期时间
	}

	tm.mu.Lock()
	size := len(tm.tickets)
	tm.mu.Unlock()
	assert.LessOrEqual(t, size, 2, "过期票据应被顺带清理, size=%d", size)
}

// TestWSHandler_TicketFlowViaServer 端到端：取票据 → 建连 → 重放被拒；
// 直接拿 JWT 冒充 ticket 同样被拒。
func TestWSHandler_TicketFlowViaServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(nil, as)
	tm := NewWSTicketManager()
	hub := NewWSHub()

	r := gin.New()
	auth := r.Group("/api/v1", ah.AuthMiddleware())
	auth.POST("/ws/ticket", tm.IssueHandler())
	r.GET("/api/v1/ws", hub.WsHandler(tm))

	s := httptest.NewServer(r)
	defer s.Close()

	adminJWT, err := as.GenerateToken("admin", "admin")
	require.NoError(t, err)

	// 无凭证取票据 → 401
	resp, err := http.Post(s.URL+"/api/v1/ws/ticket", "application/json", nil)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// 取票据 → 建连
	ticket := wsFetchTicket(t, r, as, "admin", "admin")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, s.URL+"/api/v1/ws?ticket="+ticket, nil)
	require.NoError(t, err)
	conn.CloseNow()

	// 重放同一票据 → 401；JWT 冒充 ticket → 401
	_, _, err = websocket.Dial(ctx, s.URL+"/api/v1/ws?ticket="+ticket, nil)
	require.Error(t, err, "已用票据不得再次建连")
	_, _, err = websocket.Dial(ctx, s.URL+"/api/v1/ws?ticket="+adminJWT, nil)
	require.Error(t, err, "JWT 不得作为 ticket")
}

// TestTerminalHandler_OperatorTicketAccepted operator 票据可通过终端的角色校验
// （建连后进入节点拨号段；node 不存在时以 WS 消息回报失败）。
func TestTerminalHandler_OperatorTicketAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tm := NewWSTicketManager()
	ticket, err := tm.Issue("op1", model.RoleOperator)
	require.NoError(t, err)

	r := gin.New()
	r.GET("/session/terminal", NewTerminalHandler(nil, tm).Terminal)

	s := httptest.NewServer(r)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, s.URL+"/session/terminal?ticket="+ticket+"&node_id=nope", nil)
	require.NoError(t, err, "operator 票据应通过角色校验并完成升级")
	conn.CloseNow()
}
