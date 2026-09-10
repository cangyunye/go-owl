package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/gin-gonic/gin"
)

// wsTicketTTL 票据有效期：一次性、短时，泄露后很快失效。
const wsTicketTTL = 60 * time.Second

// wsTicket 是 /ws 与 /session/terminal 的建连凭证，替代 query 中的长期 JWT：
// JWT 会进入外部反向代理日志与浏览器历史，一次性票据即使被记录也难以复用。
type wsTicket struct {
	username  string
	role      model.Role
	expiresAt time.Time
	used      bool
}

// WSTicketManager 签发与核销一次性建连票据。票据只在内存中，重启即全部失效。
type WSTicketManager struct {
	mu      sync.Mutex
	tickets map[string]*wsTicket
	now     func() time.Time
}

func NewWSTicketManager() *WSTicketManager {
	return &WSTicketManager{tickets: make(map[string]*wsTicket), now: time.Now}
}

// Issue 为指定用户签发一张一次性票据。
func (m *WSTicketManager) Issue(username string, role model.Role) (string, error) {
	if _, ok := roleHierarchy[string(role)]; !ok {
		return "", fmt.Errorf("unknown role: %s", role)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate ticket: %w", err)
	}
	ticket := hex.EncodeToString(buf)

	m.mu.Lock()
	m.cleanupLocked()
	m.tickets[ticket] = &wsTicket{
		username:  username,
		role:      role,
		expiresAt: m.now().Add(wsTicketTTL),
	}
	m.mu.Unlock()
	return ticket, nil
}

// Redeem 核销票据并返回签发时的身份：不存在、已过期、已使用均返回 false。
// 已用标记在锁内原子置位，同一票据并发核销只有一次成功。
func (m *WSTicketManager) Redeem(ticket string) (string, model.Role, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tickets[ticket]
	if !ok {
		return "", "", false
	}
	if t.used || m.now().After(t.expiresAt) {
		delete(m.tickets, ticket)
		return "", "", false
	}
	t.used = true
	return t.username, t.role, true
}

func (m *WSTicketManager) cleanupLocked() {
	now := m.now()
	for k, t := range m.tickets {
		if t.used || now.After(t.expiresAt) {
			delete(m.tickets, k)
		}
	}
}

// IssueHandler 返回签发票据的 HTTP handler；必须挂在认证中间件之后
// （从 context 读取 username/role）。
func (m *WSTicketManager) IssueHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ticket, err := m.Issue(c.GetString("username"), model.Role(c.GetString("role")))
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "unknown role"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ticket": ticket, "expires_in": int(wsTicketTTL.Seconds())})
	}
}
