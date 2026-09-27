package handler

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type TerminalHandler struct {
	db       *sql.DB
	tickets  *WSTicketManager
	sessions *terminalSessions
}

// NewTerminalHandler 的 tickets 为建连凭证来源（POST /ws/ticket 签发的一次性票据）。
func NewTerminalHandler(db *sql.DB, tickets *WSTicketManager) *TerminalHandler {
	return &TerminalHandler{db: db, tickets: tickets, sessions: newTerminalSessions()}
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

// terminalSessions 记录每用户的活动终端数（进程内注册表；owl-serve 重启即清零，
// 与「连接随进程终止」的语义一致）。
type terminalSessions struct {
	mu   sync.Mutex
	used map[string]int
}

func newTerminalSessions() *terminalSessions {
	return &terminalSessions{used: map[string]int{}}
}

func (t *terminalSessions) acquire(user string, limit int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if limit > 0 && t.used[user] >= limit {
		return false
	}
	t.used[user]++
	return true
}

func (t *terminalSessions) release(user string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.used[user] > 0 {
		t.used[user]--
	}
}

// terminalLimits 从系统设置读终端限额（键不存在时用默认值）：
//
//	terminal.idle_timeout_min —— 空闲超时（分钟），<=0 表示不启用
//	terminal.max_per_user     —— 每用户并发终端上限，<=0 表示不限制
func terminalLimits(db *sql.DB) (idle time.Duration, perUser int) {
	idle, perUser = 30*time.Minute, 5
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'terminal.idle_timeout_min'`).Scan(&v); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(v)); e == nil {
			// 能解析：>0 启用；<=0 显式关闭
			if n > 0 {
				idle = time.Duration(n) * time.Minute
			} else {
				idle = 0
			}
		}
		// 解析失败（非法字符串）：保留默认值
	}
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'terminal.max_per_user'`).Scan(&v); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(v)); e == nil {
			if n > 0 {
				perUser = n
			} else {
				perUser = 0
			}
		}
	}
	return idle, perUser
}

func (h *TerminalHandler) Terminal(c *gin.Context) {
	ticket := c.Query("ticket")
	if ticket == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "ticket required"})
		return
	}
	username, role, ok := h.tickets.Redeem(ticket)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid ticket"})
		return
	}
	if roleHierarchy[string(role)] < roleHierarchy["operator"] {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "operator role required"})
		return
	}

	nodeID := c.Query("node_id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "node_id required"})
		return
	}
	// 节点范围授权：终端只允许连接授权内节点
	if filtered := NewScopeChecker(h.db).FilterNodeIDs(c.Request.Context(), username, []string{nodeID}); len(filtered) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "node out of your scope"})
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

	// nil options 即保留 nhooyr 默认的同源校验（无 Origin 头时放行非浏览器客户端）
	conn, err := websocket.Accept(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := c.Request.Context()

	// 终端限额与空闲超时（系统设置里可调；读不到键时用默认值）
	idleTimeout, perUser := terminalLimits(h.db)
	if !h.sessions.acquire(username, perUser) {
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Data: "终端数已达上限（每用户 " + fmt.Sprint(perUser) + " 个），请先关闭闲置终端\r\n"})
		conn.Close(websocket.StatusNormalClosure, "terminal limit")
		return
	}
	defer h.sessions.release(username)

	client, err := (&sshExecutor{db: h.db}).dialNode(ctx, nodeID)
	if err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "connect failed: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "create session failed: " + err.Error() + "\r\n"})
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
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "request pty failed: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "stdin pipe failed: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "stdout pipe failed: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}

	if err := session.Shell(); err != nil {
		writeTermMsg(ctx, conn, termMessage{Type: "output", Data: "start shell failed: " + err.Error() + "\r\n"})
		writeTermMsg(ctx, conn, termMessage{Type: "exit", Code: 1})
		return
	}

	// 空闲看门狗：终端连续 idleTimeout 无任何输入/输出 → 推提示并断开，
	// 释放 SSH 连接与远端 shell（正在产出的命令不算空闲，不会被打断）。
	var activeMu sync.Mutex
	lastActive := time.Now()
	touch := func() { activeMu.Lock(); lastActive = time.Now(); activeMu.Unlock() }
	if idleTimeout > 0 {
		stopIdle := make(chan struct{})
		defer close(stopIdle)
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-stopIdle:
					return
				case <-ticker.C:
					activeMu.Lock()
					idleFor := time.Since(lastActive)
					activeMu.Unlock()
					if idleFor > idleTimeout {
						writeTermMsg(ctx, conn, termMessage{Type: "exit", Data: "因空闲超过 " + idleTimeout.Round(time.Second).String() + " 无任何输入/输出，已自动断开\r\n"})
						conn.Close(websocket.StatusNormalClosure, "idle timeout")
						return
					}
				}
			}
		}()
	}

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := stdout.Read(buf)
			if n > 0 {
				touch()
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
		touch()
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
