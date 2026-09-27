package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// 慢客户端（发送缓冲积压）必须被断开连接，而不是静默丢行：
// 丢掉的 task_output 行会让实时输出永久缺失且无感知；断开后前端自动重连，
// 任务终态广播携带全量 output，前端可据此回填完整输出。
func TestWSHub_SlowClientDisconnectedOnOverflow(t *testing.T) {
	hub := NewWSHub()

	// 服务端连接经 channel 交给测试（不使用包级状态：跨 -count 轮次会污染，
	// 且断言若放在锁内失败会以持锁状态 Goexit，导致后续轮次永久卡死）。
	serverConns := make(chan *websocket.Conn, 4)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		require.NoError(t, err)
		serverConns <- conn
		hub.Subscribe(r.Context(), conn)
		<-r.Context().Done()
	}))
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slowConn, _, err := websocket.Dial(ctx, s.URL, nil)
	require.NoError(t, err)
	defer slowConn.CloseNow()

	fastConn, _, err := websocket.Dial(ctx, s.URL, nil)
	require.NoError(t, err)
	defer fastConn.CloseNow()

	// 拨号顺序 = 记录顺序：先 slow 后 fast
	var slowServerConn, fastServerConn *websocket.Conn
	select {
	case slowServerConn = <-serverConns:
	case <-time.After(3 * time.Second):
		t.Fatal("慢客户端未在服务端建立连接")
	}
	select {
	case fastServerConn = <-serverConns:
	case <-time.After(3 * time.Second):
		t.Fatal("快客户端未在服务端建立连接")
	}
	require.NotNil(t, fastServerConn)
	hub.mu.RLock()
	var slow *wsClient
	for _, c := range hub.clients {
		if c.conn == slowServerConn {
			slow = c
		}
	}
	hub.mu.RUnlock()
	require.NotNil(t, slow, "慢客户端应已订阅")

	filler := WSMessage{Type: "filler"}
	for {
		select {
		case slow.sendCh <- filler:
			continue
		default:
		}
		break
	}

	// 缓冲已满的下一次广播应触发慢客户端断开（而非静默丢弃本条消息）
	hub.BroadcastTaskOutput("task-1", "node-1", "trigger", "stdout", 0)

	// 慢客户端被移出 hub（Broadcast 内同步执行 Close + Unsubscribe）
	hub.mu.RLock()
	_, slowStillThere := hub.clients[slow.id]
	hub.mu.RUnlock()
	require.False(t, slowStillThere, "慢客户端应被移出 hub")

	// 慢客户端连接被关闭：已缓冲的数据帧可能先被读出，持续读到错误即证明断开
	var readErr error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		readCtx, readCancel := context.WithTimeout(ctx, time.Second)
		_, _, readErr = slowConn.Read(readCtx)
		readCancel()
		if readErr != nil {
			break
		}
	}
	require.Error(t, readErr, "慢客户端连接应被服务端关闭")

	// 快客户端不受影响，仍能收到后续广播（跳过途中可能收到的 task_output）
	hub.BroadcastTaskUpdate(map[string]string{"id": "after-flood"})
	var msg struct {
		Type string            `json:"type"`
		Data map[string]string `json:"data"`
	}
	for {
		require.NoError(t, wsjson.Read(ctx, fastConn, &msg))
		if msg.Type == "task_update" {
			break
		}
	}
	require.Equal(t, "after-flood", msg.Data["id"])
}
