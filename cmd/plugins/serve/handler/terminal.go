package handler

import (
	"context"
	"database/sql"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type TerminalHandler struct {
	db   *sql.DB
	auth *service.AuthService
}

func NewTerminalHandler(db *sql.DB, auth *service.AuthService) *TerminalHandler {
	return &TerminalHandler{db: db, auth: auth}
}

type termMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Code int    `json:"code,omitempty"`
}

func writeTermMsg(ctx context.Context, conn *websocket.Conn, msg termMessage) {
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := wsjson.Write(wctx, conn, msg); err != nil {
		log.Printf("terminal ws write: %v", err)
	}
}

func (h *TerminalHandler) Terminal(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "token required"})
		return
	}
	claims, err := h.auth.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid token"})
		return
	}
	if roleHierarchy[claims.Role] < roleHierarchy["operator"] {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "operator role required"})
		return
	}

	nodeID := c.Query("node_id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "node_id required"})
		return
	}

	cols, _ := strconv.Atoi(c.DefaultQuery("cols", "80"))
	rows, _ := strconv.Atoi(c.DefaultQuery("rows", "24"))
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := c.Request.Context()

	client, err := (&sshExecutor{db: h.db}).dial(nodeID)
	if err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "连接失败: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "创建会话失败: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "请求终端失败: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "标准输入管道失败: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "标准输出管道失败: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}

	if err := session.Shell(); err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "启动 shell 失败: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := stdout.Read(buf)
			if n > 0 {
				writeTermMsg(ctx, conn, termMessage{Type: "output", Data: string(buf[:n])})
			}
			if readErr != nil {
				exitCode := 0
				if waitErr := session.Wait(); waitErr != nil {
					if ee, ok := waitErr.(*ssh.ExitError); ok {
						exitCode = ee.ExitStatus()
					}
				}
				writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: exitCode})
				conn.Close(websocket.StatusNormalClosure, "shell exited")
				return
			}
		}
	}()

	for {
		var msg termMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			break
		}
		switch msg.Type {
		case "input":
			if _, err := io.WriteString(stdin, msg.Data); err != nil {
				break
			}
		case "resize":
			if msg.Cols > 0 && msg.Rows > 0 {
				_ = session.WindowChange(msg.Rows, msg.Cols)
			}
		}
	}
}
