package handler

import (
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
	conn *websocket.Conn
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[string]*wsClient),
	}
}

func (h *WSHub) Subscribe(ctx context.Context, conn *websocket.Conn) {
	client := &wsClient{conn: conn}
	h.mu.Lock()
	h.clients[fmt.Sprintf("%p", conn)] = client
	h.mu.Unlock()

	<-ctx.Done()
	h.Unsubscribe(fmt.Sprintf("%p", conn))
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
	defer h.mu.RUnlock()

	for _, client := range h.clients {
		go func(c *wsClient) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := wsjson.Write(ctx, c.conn, msg); err != nil {
				log.Printf("ws write error: %v", err)
			}
		}(client)
	}
}

func (h *WSHub) BroadcastTaskUpdate(task interface{}) {
	h.Broadcast(WSMessage{
		Type: "task_update",
		Data: task,
	})
}

func (h *WSHub) BroadcastTaskOutput(taskID, nodeID, line, lineType string) {
	h.Broadcast(WSMessage{
		Type: "task_output",
		Data: map[string]string{
			"task_id": taskID,
			"node_id": nodeID,
			"line":    line,
			"type":    lineType,
		},
	})
}

func (h *WSHub) WsHandler(authService *AuthHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "token required"})
			return
		}

		claims, err := authService.auth.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid token"})
			return
		}
		_ = claims

		conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
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
