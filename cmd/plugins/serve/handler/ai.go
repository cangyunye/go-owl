package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AIHandler struct {
	db          *sql.DB
	auditStore  *store.AIAuditStore
	executor    *WebExecutor
	agent       *ai2.Agent
	sessionMgr  *ai2.SessionManager
	keyManager  *KeyManager
	debugMode   bool
	rateLimiter *aiRateLimiter
	// newChatAgent builds an agent bound to the user-provided LLM config.
	// Overridable in tests to inject a mock chat model.
	newChatAgent func(llmReq *LLMRequest) (*ai2.Agent, error)
}

func NewAIHandler(db *sql.DB, auditStore *store.AIAuditStore, executor *WebExecutor,
	keyManager *KeyManager, agent *ai2.Agent, debugMode bool) *AIHandler {
	// 会话持久化到 serve 主库（ai_sessions 表，host="web"），重启后可恢复上下文；
	// 建表失败降级为纯内存会话（不阻塞服务启动）
	sessionMgr := ai2.NewSessionManager()
	if sessionStore, err := ai2.NewSQLiteSessionStore(db); err == nil {
		sessionMgr = ai2.NewSessionManagerWithStore(sessionStore, "web")
	}
	h := &AIHandler{
		db: db, auditStore: auditStore, executor: executor,
		keyManager: keyManager, agent: agent,
		sessionMgr:  sessionMgr,
		debugMode:   debugMode,
		rateLimiter: newAIRateLimiter(),
	}
	h.newChatAgent = h.buildChatAgent
	return h
}

// buildChatAgent constructs an agent that executes against the serve database
// (via WebExecutor + dbNodeStoreAdapter) and uses the given LLM credentials.
// 用户自带的 key/model 经 internal/ai.HTTPModel 直连，与 CLI 共用同一内核客户端。
func (h *AIHandler) buildChatAgent(llmReq *LLMRequest) (*ai2.Agent, error) {
	nodeStore := &dbNodeStoreAdapter{db: h.db}
	nodeMgr := ai2.InitNodeManager(nodeStore)
	agent, err := ai2.NewAgent(h.executor, &ai2.Config{}, nodeMgr, nodeStore, nil, h.debugMode)
	if err != nil {
		return nil, err
	}
	agent.SetChatModel(ai2.NewHTTPModel(ai2.ModelOptions{
		APIType: llmReq.APIType,
		BaseURL: llmReq.BaseURL,
		Model:   llmReq.Model,
		APIKey:  llmReq.APIKey,
	}))
	agent.SetSafetyIdentity(func() string { return h.executor.userName })
	return agent, nil
}

func (h *AIHandler) GetSessionKey(c *gin.Context) {
	session, err := h.keyManager.CreateSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "generate session key failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"session_id":      session.SessionID,
		"public_key_spki": session.PublicKeySPKI,
	})
}

type aiChatRequest struct {
	Message         string `json:"message"`
	SessionID       string `json:"session_id"`
	EncryptedAPIKey string `json:"encrypted_api_key"`
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	BaseURL         string `json:"base_url"`
	APIType         string `json:"api_type"`
}

// aiChatContext 是解析后的聊天请求上下文（Chat 与 StreamChat 共用）
type aiChatContext struct {
	req        *aiChatRequest
	userID     string
	sessionID  string
	sessionKey string
	session    *ai2.Session
}

// resolveChatContext 解析并创建/复用用户会话。
func (h *AIHandler) resolveChatContext(c *gin.Context) (*aiChatContext, bool) {
	var req aiChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return nil, false
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "message is required"})
		return nil, false
	}

	userID := c.GetString("user_id")
	if userID == "" {
		userID = "anonymous"
	}

	// 每用户滑动窗口限流（ai.rate_limit_per_min，0=不限）；
	// 在写 SSE 头之前拒绝，Chat 与 StreamChat 都能以 JSON 返回 429
	if retryAfter, ok := h.allowChat(userID); !ok {
		secs := int(retryAfter.Seconds() + 0.9)
		c.Header("Retry-After", strconv.Itoa(secs))
		c.JSON(http.StatusTooManyRequests, gin.H{
			"code":        429,
			"message":     fmt.Sprintf("请求过于频繁，请 %d 秒后重试", secs),
			"retry_after": secs,
		})
		return nil, false
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// 服务端会话键按用户命名空间隔离:同一 sessionID 在不同用户下是不同会话,
	// 防止用户之间共享/续接彼此的对话上下文。
	sessionKey := sessionID
	if userID != "anonymous" {
		sessionKey = userID + ":" + sessionID
	}

	h.executor.userRole = c.GetString("role")
	h.executor.userName = c.GetString("username")

	// 先构建本次请求使用的 agent（用户自带 key 时），命中持久化会话则以
	// 该 agent 恢复上下文（新 key + 旧上下文），未命中则创建新会话
	agent := h.agent
	if req.EncryptedAPIKey != "" && req.Model != "" {
		if apiKeyBytes, err := h.keyManager.Decrypt(req.SessionID, req.EncryptedAPIKey); err == nil {
			apiType := req.APIType
			if apiType == "" {
				apiType = "openai"
			}
			baseURL := req.BaseURL
			if baseURL == "" {
				baseURL = defaultBaseURL(req.Provider)
			}
			llmReq := &LLMRequest{
				APIKey:  string(apiKeyBytes),
				BaseURL: baseURL,
				Model:   req.Model,
				APIType: apiType,
			}
			if chatAgent, err := h.newChatAgent(llmReq); err == nil {
				agent = chatAgent
			}
		}
	}
	session, exists := h.sessionMgr.GetOrLoadSession(sessionKey, agent)
	if !exists {
		session = h.sessionMgr.CreateSession(sessionKey, agent)
	}

	return &aiChatContext{req: &req, userID: userID, sessionID: sessionID, sessionKey: sessionKey, session: session}, true
}

func (h *AIHandler) Chat(c *gin.Context) {
	cc, ok := h.resolveChatContext(c)
	if !ok {
		return
	}
	req, userID, sessionID, session, sessionKey := cc.req, cc.userID, cc.sessionID, cc.session, cc.sessionKey

	startTime := time.Now()
	reply, err := session.Send(c.Request.Context(), req.Message)
	if err != nil {
		// The LLM-backed agent failed (invalid key, provider error, etc.).
		// Degrade gracefully to the rule-based default agent.
		fallbackID := "fallback:" + sessionKey
		if fallback, ok := h.sessionMgr.GetSession(fallbackID); ok {
			session = fallback
		} else {
			session = h.sessionMgr.CreateSession(fallbackID, h.agent)
		}
		reply, err = session.Send(c.Request.Context(), req.Message)
	}
	durationMs := time.Since(startTime).Milliseconds()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("ai request failed", err)})
		return
	}

	go h.logAudit(userID, "conversation", "success", req.Message, reply, durationMs, h.debugMode)

	c.JSON(http.StatusOK, gin.H{
		"reply":      reply,
		"session_id": sessionID,
	})
}

func defaultBaseURL(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com"
	case "deepseek":
		return "https://api.deepseek.com"
	case "anthropic":
		return "https://api.anthropic.com"
	default:
		return ""
	}
}

type aiTestRequest struct {
	SessionID       string `json:"session_id"`
	EncryptedAPIKey string `json:"encrypted_api_key"`
	BaseURL         string `json:"base_url"`
	APIType         string `json:"api_type"`
	Model           string `json:"model"`
}

func (h *AIHandler) Test(c *gin.Context) {
	var req aiTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request"})
		return
	}

	if req.EncryptedAPIKey == "" || req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "session_id and encrypted_api_key are required"})
		return
	}
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "model is required"})
		return
	}
	if req.BaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "base_url is required"})
		return
	}

	apiKeyBytes, err := h.keyManager.Decrypt(req.SessionID, req.EncryptedAPIKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid session or encrypted key"})
		return
	}
	apiKey := string(apiKeyBytes)

	apiType := req.APIType
	if apiType == "" {
		apiType = "openai"
	}

	msgs := []LLMMessage{
		{Role: "user", Content: "只需要回答hello"},
	}

	startTime := time.Now()
	llmResp, err := CallLLM(c.Request.Context(), &LLMRequest{
		APIKey:   apiKey,
		BaseURL:  req.BaseURL,
		Model:    req.Model,
		APIType:  apiType,
		Messages: msgs,
	})
	elapsed := time.Since(startTime).Milliseconds()

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"error":      err.Error(),
			"elapsed_ms": elapsed,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"reply":      llmResp.Content,
		"model":      llmResp.Model,
		"elapsed_ms": elapsed,
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

func (h *AIHandler) logAudit(userID, intent, result, prompt, reply string, durationMs int64, debug bool) {
	promptText := ""
	if debug {
		promptText = prompt
	}
	h.auditStore.Create(context.Background(), &store.AIAuditRecord{
		UserID:        userID,
		Intent:        intent,
		Result:        result,
		ReplyText:     reply,
		PromptText:    promptText,
		LLMDurationMs: durationMs,
	})
}

type aiModelsRequest struct {
	SessionID       string `json:"session_id"`
	EncryptedAPIKey string `json:"encrypted_api_key"`
	BaseURL         string `json:"base_url"`
	APIType         string `json:"api_type"`
}

func (h *AIHandler) Models(c *gin.Context) {
	var req aiModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request"})
		return
	}
	if req.BaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "base_url is required"})
		return
	}
	if req.EncryptedAPIKey == "" || req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "session_id and encrypted_api_key are required"})
		return
	}

	apiKeyBytes, err := h.keyManager.Decrypt(req.SessionID, req.EncryptedAPIKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid session or encrypted key"})
		return
	}
	apiKey := string(apiKeyBytes)

	apiType := req.APIType
	if apiType == "" {
		apiType = "openai"
	}

	switch apiType {
	case "anthropic":
		models := []gin.H{
			{"id": "claude-sonnet-4-20250514", "owned_by": "anthropic"},
			{"id": "claude-sonnet-4-20250514-thinking", "owned_by": "anthropic"},
			{"id": "claude-3-5-sonnet-20241022", "owned_by": "anthropic"},
			{"id": "claude-3-5-haiku-20241022", "owned_by": "anthropic"},
			{"id": "claude-3-opus-20240229", "owned_by": "anthropic"},
			{"id": "claude-3-haiku-20240307", "owned_by": "anthropic"},
		}
		c.JSON(http.StatusOK, gin.H{"models": models})
		return

	default:
		baseURL := strings.TrimRight(req.BaseURL, "/")
		if strings.HasSuffix(baseURL, "/v1") {
			baseURL += "/models"
		} else if strings.Contains(baseURL, "/v1/") {
			if idx := strings.Index(baseURL, "/v1/"); idx >= 0 {
				baseURL = baseURL[:idx] + "/v1/models"
			} else {
				baseURL += "/v1/models"
			}
		} else {
			baseURL += "/v1/models"
		}

		client := &http.Client{Timeout: 15 * time.Second}
		httpReq, _ := http.NewRequestWithContext(c.Request.Context(), "GET", baseURL, nil)
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": fmt.Sprintf("request failed: %v", err)})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": fmt.Sprintf("provider returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))})
			return
		}

		var raw struct {
			Data []struct {
				ID      string `json:"id"`
				OwnedBy string `json:"owned_by,omitempty"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": fmt.Sprintf("parse response failed: %v", err)})
			return
		}

		models := make([]gin.H, 0, len(raw.Data))
		for _, m := range raw.Data {
			models = append(models, gin.H{"id": m.ID, "owned_by": m.OwnedBy})
		}
		if models == nil {
			models = []gin.H{}
		}
		c.JSON(http.StatusOK, gin.H{"models": models})
	}
}

// AI 会话权限：读工具任何已登录角色可用；写工具仅 operator/admin。
// 前端按 /api/v1/ai/permissions 的 allowed_tools 过滤 UI，
// 执行层 requireOperator() 兜底拦截写工具，双保险。
var (
	aiReadTools = []string{
		"query_nodes", "query_database", "node_status",
		"node_groups", "node_labels",
		"list_playbooks", "playbook_info",
		"playbook_template_list", "playbook_template_info",
		"playbook_state_list", "playbook_state_show",
		"validate_playbook",
	}
	aiWriteTools = []string{
		"execute_command", "execute_script",
		"generate_playbook", "transfer_file",
		"run_playbook", "node_check",
	}
)

// Permissions 返回当前用户在 AI 会话中的权限视图：
// read_only 为 true 时仅允许 aiReadTools；operator/admin 全量可用。
func (h *AIHandler) Permissions(c *gin.Context) {
	role := c.GetString("role")
	if role == "" {
		role = string(model.RoleViewer)
	}

	readOnly := role != string(model.RoleOperator) && role != string(model.RoleAdmin)

	allowed := append([]string{}, aiReadTools...)
	blocked := append([]string{}, aiWriteTools...)
	if !readOnly {
		allowed = append(allowed, aiWriteTools...)
		blocked = []string{}
	}

	c.JSON(http.StatusOK, gin.H{
		"role":          role,
		"read_only":     readOnly,
		"allowed_tools": allowed,
		"blocked_tools": blocked,
	})
}

func (h *AIHandler) Status() (int, int, int, error) {
	var total, online, offline int
	h.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&total)
	h.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE status = 'online'").Scan(&online)
	h.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE status = 'offline' OR status = 'unknown'").Scan(&offline)
	return total, online, offline, nil
}

// StreamChat 是 /ai/chat/stream 的 SSE 流式聊天：请求体与 Chat 相同，
// 响应为 text/event-stream，事件：delta（文本增量）/ tool（工具调用进度）/
// progress（路由等阶段进度）/ done（最终 reply+session_id）/ error。
func (h *AIHandler) StreamChat(c *gin.Context) {
	cc, ok := h.resolveChatContext(c)
	if !ok {
		return
	}
	req, userID, sessionID, session := cc.req, cc.userID, cc.sessionID, cc.session

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, canFlush := c.Writer.(http.Flusher)
	if !canFlush {
		return
	}
	flusher.Flush()

	writeSSE := func(event string, data interface{}) {
		payload, err := json.Marshal(data)
		if err != nil {
			return
		}
		fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, payload)
		flusher.Flush()
	}

	// 桥接 agent 进度到 SSE。会话级回调，请求结束即清理。
	session.OnProgress = func(step, detail string) {
		switch step {
		case "delta":
			writeSSE("delta", gin.H{"text": detail})
		case "generate", "execute":
			writeSSE("tool", gin.H{"name": detail, "phase": step})
		default:
			writeSSE("progress", gin.H{"step": step, "detail": detail})
		}
	}
	defer func() { session.OnProgress = nil }()

	startTime := time.Now()
	reply, err := session.Send(c.Request.Context(), req.Message)
	if err != nil {
		// 与 Chat 一致：LLM agent 失败时优雅降级到规则兜底 agent
		fallbackID := "fallback:" + cc.sessionKey
		if fallback, ok := h.sessionMgr.GetSession(fallbackID); ok {
			session = fallback
		} else {
			session = h.sessionMgr.CreateSession(fallbackID, h.agent)
		}
		session.OnProgress = func(step, detail string) {
			switch step {
			case "delta":
				writeSSE("delta", gin.H{"text": detail})
			case "generate", "execute":
				writeSSE("tool", gin.H{"name": detail, "phase": step})
			default:
				writeSSE("progress", gin.H{"step": step, "detail": detail})
			}
		}
		reply, err = session.Send(c.Request.Context(), req.Message)
	}
	durationMs := time.Since(startTime).Milliseconds()

	if err != nil {
		writeSSE("error", gin.H{"message": internalErr("ai request failed", err)})
		return
	}

	go h.logAudit(userID, "conversation", "success", req.Message, reply, durationMs, h.debugMode)

	writeSSE("done", gin.H{"reply": reply, "session_id": sessionID})
}

// allowChat 读取 ai.rate_limit_per_min 并做滑动窗口限流判定。
func (h *AIHandler) allowChat(userID string) (time.Duration, bool) {
	var v string
	_ = h.db.QueryRow(`SELECT value FROM settings WHERE key = 'ai.rate_limit_per_min'`).Scan(&v)
	limit := parseIntOrDefault(v, 0)
	return h.rateLimiter.Allow(userID, limit, time.Minute)
}

func parseIntOrDefault(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

// ListAISessions 列出指定宿主（默认 cli）的持久化会话，供跨端续聊导入。
func (h *AIHandler) ListAISessions(c *gin.Context) {
	host := c.DefaultQuery("host", "cli")
	metas, err := h.sessionMgr.ListPersisted(host, 50)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"items": []ai2.SessionMeta{}})
		return
	}
	if metas == nil {
		metas = []ai2.SessionMeta{}
	}
	c.JSON(http.StatusOK, gin.H{"items": metas})
}

// ImportAISession 把 CLI 会话复制到当前用户的 web 命名空间下，
// 之后以同一 session_id 发消息即可延续 CLI 里的对话上下文。
func (h *AIHandler) ImportAISession(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id"`
		Host      string `json:"host"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "session_id is required"})
		return
	}
	fromHost := req.Host
	if fromHost == "" {
		fromHost = "cli"
	}
	userID := c.GetString("user_id")
	if userID == "" {
		userID = "anonymous"
	}
	targetID := req.SessionID
	if userID != "anonymous" {
		targetID = userID + ":" + req.SessionID
	}
	if _, err := h.sessionMgr.ImportSession(fromHost, req.SessionID, "web", targetID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "source session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"session_id": req.SessionID, "host": "web"})
}
