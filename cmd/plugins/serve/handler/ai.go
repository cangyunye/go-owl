package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	ai2 "github.com/cangyunye/go-owl/internal/ai"
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
	// newChatAgent builds an agent bound to the user-provided LLM config.
	// Overridable in tests to inject a mock chat model.
	newChatAgent func(llmReq *LLMRequest) (*ai2.Agent, error)
}

func NewAIHandler(db *sql.DB, auditStore *store.AIAuditStore, executor *WebExecutor,
	keyManager *KeyManager, agent *ai2.Agent, debugMode bool) *AIHandler {
	h := &AIHandler{
		db: db, auditStore: auditStore, executor: executor,
		keyManager: keyManager, agent: agent,
		sessionMgr: ai2.NewSessionManager(),
		debugMode:  debugMode,
	}
	h.newChatAgent = h.buildChatAgent
	return h
}

// webLLMChatModel adapts the generic CallLLM HTTP client to ai2.ChatModel so
// the agent framework (router + group prompts + tool calling) runs on the
// user-provided API key/model instead of a bare chat completion.
type webLLMChatModel struct {
	req *LLMRequest
}

func (m *webLLMChatModel) Generate(ctx context.Context, messages []ai2.Message) (string, error) {
	msgs := make([]LLMMessage, len(messages))
	for i, msg := range messages {
		msgs[i] = LLMMessage{Role: msg.Role, Content: msg.Content}
	}
	req := *m.req
	req.Messages = msgs
	resp, err := CallLLM(ctx, &req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// buildChatAgent constructs an agent that executes against the serve database
// (via WebExecutor + dbNodeStoreAdapter) and uses the given LLM credentials.
func (h *AIHandler) buildChatAgent(llmReq *LLMRequest) (*ai2.Agent, error) {
	nodeStore := &dbNodeStoreAdapter{db: h.db}
	nodeMgr := ai2.InitNodeManager(nodeStore)
	agent, err := ai2.NewAgent(h.executor, &ai2.Config{}, nodeMgr, nodeStore, nil, h.debugMode)
	if err != nil {
		return nil, err
	}
	agent.SetChatModel(&webLLMChatModel{req: llmReq})
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

	userID := c.GetString("user_id")
	if userID == "" {
		userID = "anonymous"
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

	session, exists := h.sessionMgr.GetSession(sessionKey)
	if !exists {
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
		session = h.sessionMgr.CreateSession(sessionKey, agent)
	}

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
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
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
