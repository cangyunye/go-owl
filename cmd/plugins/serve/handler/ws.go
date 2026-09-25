package handler

import (
	"strconv"
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type WSHub struct {
	mu      sync.RWMutex
	clients map[string]*wsClient
}

type wsClient struct {
	id     string
	conn   *websocket.Conn
	sendCh chan WSMessage
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[string]*wsClient),
	}
}

func (h *WSHub) Subscribe(ctx context.Context, conn *websocket.Conn) {
	id := fmt.Sprintf("%p", conn)
	// 512 条缓冲吸收输出洪峰；积压时由 Broadcast 断开客户端（而非丢消息），
	// 前端 3s 自动重连并按任务记录回填，输出不会缺失。
	client := &wsClient{id: id, conn: conn, sendCh: make(chan WSMessage, 512)}
	h.mu.Lock()
	h.clients[id] = client
	h.mu.Unlock()

	go h.writeLoop(ctx, client)

	<-ctx.Done()
	h.Unsubscribe(id)
}

// writeLoop 单写者串行下发消息，保证同一客户端收到的广播顺序与发送顺序一致。
func (h *WSHub) writeLoop(ctx context.Context, client *wsClient) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-client.sendCh:
			wctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := wsjson.Write(wctx, client.conn, msg); err != nil {
				log.Printf("ws write error: %v", err)
				cancel()
				return
			}
			cancel()
		}
	}
}

func (h *WSHub) Unsubscribe(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.clients[id]; ok {
		c.conn.Close(websocket.StatusNormalClosure, "closed")
		delete(h.clients, id)
	}
}

func (h *WSHub) Broadcast(msg WSMessage) {
	h.mu.RLock()
	var slow []*wsClient
	for _, client := range h.clients {
		select {
		case client.sendCh <- msg:
		default:
			slow = append(slow, client)
		}
	}
	h.mu.RUnlock()

	// 缓冲积压的客户端断开连接而不是静默丢消息：丢掉的 task_output 行会让
	// 实时输出永久缺失；断开后前端自动重连，任务终态广播携带全量 output
	// 供前端回填。（在 RLock 外执行，Unsubscribe 需要写锁。CloseNow 而非
	// Close：慢消费者不读消息，关闭握手帧没人消费只会白等超时。）
	for _, client := range slow {
		log.Printf("ws client %s too slow, disconnecting instead of dropping messages", client.id)
		client.conn.CloseNow()
		h.Unsubscribe(client.id)
	}
}

func (h *WSHub) BroadcastTaskUpdate(task interface{}) {
	h.Broadcast(WSMessage{
		Type: "task_update",
		Data: task,
	})
}

// BroadcastTaskOutput 广播一行输出。offset 是这一行在任务输出中的**起始字节位置**，
// 客户端据此维护精确游标：断线重连后可用 /tasks/:id/output?offset= 只补缺失的尾部。
func (h *WSHub) BroadcastTaskOutput(taskID, nodeID, line, lineType string, offset int64) {
	h.Broadcast(WSMessage{
		Type: "task_output",
		Data: map[string]string{
			"task_id": taskID,
			"node_id": nodeID,
			"line":    line,
			"type":    lineType,
			"offset":  strconv.FormatInt(offset, 10),
		},
	})
}

func (h *WSHub) BroadcastHistoryUpdate() {
	h.Broadcast(WSMessage{Type: "history_update", Data: nil})
}

// WsHandler 建立监控 WebSocket 连接。认证走 ?ticket= 一次性票据
// （POST /ws/ticket 签发），不再接受长期 JWT，避免凭证进入反代日志与浏览器历史。
func (h *WSHub) WsHandler(tickets *WSTicketManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		ticket := c.Query("ticket")
		if ticket == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "ticket required"})
			return
		}
		_, role, ok := tickets.Redeem(ticket)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid ticket"})
			return
		}
		if _, known := roleHierarchy[string(role)]; !known {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "unknown role"})
			return
		}

		// 不关闭来源校验：保留 nhooyr 默认的同源检查（无 Origin 头时放行非浏览器
		// 客户端），阻断跨站页面用受害者浏览器建立 WebSocket。
		conn, err := websocket.Accept(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		h.Subscribe(c.Request.Context(), conn)

		// Read loop (to detect disconnection)
		for {
			_, _, err := conn.Read(c.Request.Context())
			if err != nil {
				break
			}
		}
	}
}
