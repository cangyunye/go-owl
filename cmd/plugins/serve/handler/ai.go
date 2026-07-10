package handler

import (
	"database/sql"
	"net/http"
	"time"

	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AIHandler struct {
	db         *sql.DB
	auditStore *store.AIAuditStore
	executor   *WebExecutor
	agent      *ai2.Agent
	sessionMgr *ai2.SessionManager
	keyManager *KeyManager
	debugMode  bool
}

func NewAIHandler(db *sql.DB, auditStore *store.AIAuditStore, executor *WebExecutor,
	keyManager *KeyManager, agent *ai2.Agent, debugMode bool) *AIHandler {
	return &AIHandler{
		db: db, auditStore: auditStore, executor: executor,
		keyManager: keyManager, agent: agent,
		sessionMgr: ai2.NewSessionManager(),
		debugMode:  debugMode,
	}
}

func (h *AIHandler) GetSessionKey(c *gin.Context) {
	session, err := h.keyManager.CreateSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "generate session key failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"session_id":     session.SessionID,
		"public_key_spki": session.PublicKeySPKI,
	})
}

type aiChatRequest struct {
	Message         string `json:"message"`
	SessionID       string `json:"session_id"`
	EncryptedAPIKey string `json:"encrypted_api_key"`
}

func (h *AIHandler) Chat(c *gin.Context) {
	var req aiChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "message is required"})
		return
	}

	// Decrypt API Key if provided
	if req.EncryptedAPIKey != "" {
		apiKey, err := h.keyManager.Decrypt(req.SessionID, req.EncryptedAPIKey)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid session or encrypted key"})
			return
		}
		_ = apiKey // stored for future LLM use
	}

	userID := c.GetString("user_id")
	if userID == "" {
		userID = "anonymous"
	}

	// Get or create session
	sessionID := req.SessionID
	if sessionID == "" {
		// Use request-level session
		sessionID = uuid.New().String()
	}

	// Get or create AI session
	session, exists := h.sessionMgr.GetSession(sessionID)
	if !exists {
		session = h.sessionMgr.CreateSession(sessionID, h.agent)
	}

	startTime := time.Now()
	reply, err := session.Send(c.Request.Context(), req.Message)
	durationMs := time.Since(startTime).Milliseconds()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	// Async audit logging
	go func() {
		promptText := ""
		if h.debugMode {
			promptText = req.Message
		}
		h.auditStore.Create(c.Request.Context(), &store.AIAuditRecord{
			UserID:    userID,
			Intent:    "conversation",
			Result:    "success",
			ReplyText: reply,
			PromptText: promptText,
			LLMDurationMs: durationMs,
		})
	}()

	c.JSON(http.StatusOK, gin.H{
		"reply":      reply,
		"session_id": sessionID,
	})
}

func (h *AIHandler) GetContext(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"tasks":         []interface{}{},
		"transfers":     []interface{}{},
		"playbook_runs": []interface{}{},
	})
}

func (h *AIHandler) Audit(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"code": 501, "message": "server-side audit is the primary path"})
}

// Status returns node counts (used by health check / analytics)
func (h *AIHandler) Status() (int, int, int, error) {
	var total, online, offline int
	h.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&total)
	h.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE status = 'online'").Scan(&online)
	h.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE status = 'offline' OR status = 'unknown'").Scan(&offline)
	return total, online, offline, nil
}
