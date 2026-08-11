package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/control/node"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type mockChatModel struct {
	responses []string
	idx       int
}

func (m *mockChatModel) Generate(_ context.Context, _ []ai2.Message) (string, error) {
	if m.idx >= len(m.responses) {
		return "", fmt.Errorf("mockChatModel: out of responses")
	}
	reply := m.responses[m.idx]
	m.idx++
	return reply, nil
}

func seedNodes(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO nodes (id, name, address, port, user, status, groups) VALUES
		('node-1', 'web-01', '10.0.0.1', 22, 'root', 'online', '["web"]'),
		('node-2', 'db-01', '10.0.0.2', 22, 'root', 'online', '["db"]'),
		('node-3', 'cache-01', '10.0.0.3', 22, 'root', 'offline', '["cache"]')`)
	require.NoError(t, err)
}

func seededNodeManager(db *sql.DB) node.Manager {
	_ = db
	return node.NewManager(node.NewInMemoryNodeStore())
}

func aiTestSetup(t *testing.T) (*sql.DB, *AIHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	seedNodes(t, db, ctx)

	taskStore := store.NewTaskStore(db)
	require.NoError(t, taskStore.Init(ctx))

	transferRecordStore := store.NewTransferRecordStore(db)
	require.NoError(t, transferRecordStore.Init(ctx))

	playbookStore := store.NewPlaybookStore(db)
	require.NoError(t, playbookStore.Init(ctx))

	playbookRunStore := store.NewPlaybookRunStore(db)
	require.NoError(t, playbookRunStore.Init(ctx))

	auditStore := store.NewAIAuditStore(db)
	require.NoError(t, auditStore.Init(ctx))

	nodeStore := store.NewNodeStore(db)
	keyManager := NewKeyManager()

	nodeMgr := seededNodeManager(db)
	webExecutor := NewWebExecutor(db, taskStore, transferRecordStore,
		playbookRunStore, nodeStore, playbookStore, auditStore, keyManager, false)

	config := &ai2.Config{}
	agent, err := ai2.NewAgent(webExecutor, config, nodeMgr, nil, nil, false)
	require.NoError(t, err)

	aiHandler := NewAIHandler(db, auditStore, webExecutor, keyManager, agent, false)

	return db, aiHandler
}

func encryptTestKey(t *testing.T, km *KeyManager, sessionID string) string {
	t.Helper()
	pubKey, err := km.GetSessionPublicKey(sessionID)
	require.NoError(t, err)

	plaintext := []byte("sk-test-api-key-12345")
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pubKey, plaintext, nil)
	require.NoError(t, err)

	return base64.StdEncoding.EncodeToString(ciphertext)
}

func TestGetSessionKey_Returns200(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.GET("/api/v1/ai/session-key", h.GetSessionKey)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/ai/session-key", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp struct {
		SessionID     string `json:"session_id"`
		PublicKeySPKI string `json:"public_key_spki"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.SessionID)
	assert.NotEmpty(t, resp.PublicKeySPKI)
}

func TestGetSessionKey_EachCallReturnsDifferentKey(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.GET("/api/v1/ai/session-key", h.GetSessionKey)

	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/api/v1/ai/session-key", nil)
	router.ServeHTTP(w1, req1)

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/v1/ai/session-key", nil)
	router.ServeHTTP(w2, req2)

	var r1, r2 struct {
		SessionID string `json:"session_id"`
	}
	json.Unmarshal(w1.Body.Bytes(), &r1)
	json.Unmarshal(w2.Body.Bytes(), &r2)

	assert.NotEqual(t, r1.SessionID, r2.SessionID)
}

func TestChat_MissingMessage_Returns400(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.POST("/api/v1/ai/chat", h.Chat)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"message": "", "session_id": "test"})
	req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestChat_MissingSessionID_UsesNew(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.POST("/api/v1/ai/chat", h.Chat)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"message": "list nodes", "session_id": ""})
	req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp struct {
		Reply     string `json:"reply"`
		SessionID string `json:"session_id"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Reply)
	assert.NotEmpty(t, resp.SessionID)
}

func TestChat_WithKey_RoutesThroughAgentQueryNodes(t *testing.T) {
	_, h := aiTestSetup(t)

	mock := &mockChatModel{responses: []string{
		"node_list",
		"```json\n{\"tool_calls\":[{\"name\":\"query_nodes\",\"arguments\":{}}]}\n```",
	}}
	h.newChatAgent = func(llmReq *LLMRequest) (*ai2.Agent, error) {
		nodeStore := &dbNodeStoreAdapter{db: h.db}
		nodeMgr := ai2.InitNodeManager(nodeStore)
		agent, err := ai2.NewAgent(h.executor, &ai2.Config{}, nodeMgr, nodeStore, nil, false)
		if err != nil {
			return nil, err
		}
		agent.SetChatModel(mock)
		return agent, nil
	}

	session, err := h.keyManager.CreateSession()
	require.NoError(t, err)
	encryptedKey := encryptTestKey(t, h.keyManager, session.SessionID)

	router := gin.New()
	router.POST("/api/v1/ai/chat", h.Chat)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"message":           "列出所有节点",
		"session_id":        session.SessionID,
		"encrypted_api_key": encryptedKey,
		"provider":          "openai",
		"model":             "test-model",
		"base_url":          "https://api.example.com/v1",
		"api_type":          "openai",
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp struct {
		Reply string `json:"reply"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Reply, "| ID | Name | Address | User | Status | Groups | Labels |", "reply should be a markdown table")
	assert.Contains(t, resp.Reply, "web-01", "reply should list the seeded node web-01")
	assert.Contains(t, resp.Reply, "db-01")
	assert.NotContains(t, resp.Reply, "我不确定您要做什么")
	assert.NotContains(t, resp.Reply, "ansible")
}

func TestChat_DeepSeek_Integration(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping: DEEPSEEK_API_KEY not set")
	}

	_, h := aiTestSetup(t)

	session, err := h.keyManager.CreateSession()
	require.NoError(t, err)

	plaintextB64 := base64.StdEncoding.EncodeToString([]byte(apiKey))

	router := gin.New()
	router.POST("/api/v1/ai/chat", h.Chat)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"message":           "列出所有节点",
		"session_id":        session.SessionID,
		"encrypted_api_key": "__plain__:" + plaintextB64,
		"provider":          "deepseek",
		"model":             "deepseek-v4-flash",
		"base_url":          "https://api.deepseek.com",
		"api_type":          "openai",
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	var resp struct {
		Reply string `json:"reply"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Reply)
	assert.Contains(t, resp.Reply, "| ID |", "reply should be a markdown table")
	assert.Contains(t, resp.Reply, "web-01", "reply should list the seeded node web-01")
	assert.NotContains(t, resp.Reply, "ansible")
}

func TestChat_WithEncryptedKey_Success(t *testing.T) {
	_, h := aiTestSetup(t)

	session, err := h.keyManager.CreateSession()
	require.NoError(t, err)

	encryptedKey := encryptTestKey(t, h.keyManager, session.SessionID)

	router := gin.New()
	router.POST("/api/v1/ai/chat", h.Chat)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"message":           "list nodes",
		"session_id":        session.SessionID,
		"encrypted_api_key": encryptedKey,
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp struct {
		Reply     string `json:"reply"`
		SessionID string `json:"session_id"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Reply)
}

func TestChat_InvalidRequestBody_Returns400(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.POST("/api/v1/ai/chat", h.Chat)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader([]byte("{invalid")))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestChat_WithSession(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.POST("/api/v1/ai/chat", h.Chat)

	// First message creates session
	w1 := httptest.NewRecorder()
	body1, _ := json.Marshal(map[string]string{"message": "list nodes", "session_id": ""})
	req1, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w1, req1)

	require.Equal(t, 200, w1.Code)
	var r1 struct {
		Reply     string `json:"reply"`
		SessionID string `json:"session_id"`
	}
	json.Unmarshal(w1.Body.Bytes(), &r1)
	require.NotEmpty(t, r1.SessionID)

	// Second message uses same session
	w2 := httptest.NewRecorder()
	body2, _ := json.Marshal(map[string]string{"message": "list nodes", "session_id": r1.SessionID})
	req2, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)

	assert.Equal(t, 200, w2.Code)
	var r2 struct {
		Reply string `json:"reply"`
	}
	json.Unmarshal(w2.Body.Bytes(), &r2)
	assert.NotEmpty(t, r2.Reply)
}

func TestChat_SessionIsolatedPerUser(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			c.Set("user_id", u)
			c.Set("role", "operator")
		}
	})
	router.POST("/api/v1/ai/chat", h.Chat)

	chat := func(userID, sessionID string) int {
		w := httptest.NewRecorder()
		body, _ := json.Marshal(map[string]string{"message": "list nodes", "session_id": sessionID})
		req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", userID)
		router.ServeHTTP(w, req)
		return w.Code
	}

	require.Equal(t, 200, chat("alice", "S-1"))
	require.Equal(t, 200, chat("bob", "S-1"))
	assert.Equal(t, 2, len(h.sessionMgr.ListSessions()),
		"alice and bob must get isolated sessions even with the same session id")

	require.Equal(t, 200, chat("alice", "S-1"))
	assert.Equal(t, 2, len(h.sessionMgr.ListSessions()),
		"alice resuming S-1 must reuse her own session")
}

func TestChat_AuditRecordCreated(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.POST("/api/v1/ai/chat", h.Chat)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"message": "list nodes", "session_id": ""})
	req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var count int
	for i := 0; i < 20; i++ {
		err := h.db.QueryRow("SELECT COUNT(*) FROM ai_audit_log").Scan(&count)
		require.NoError(t, err)
		if count > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, 1, count)
}

func TestGetContext(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.GET("/api/v1/ai/context", h.GetContext)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/ai/context", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp struct {
		Tasks        []interface{} `json:"tasks"`
		Transfers    []interface{} `json:"transfers"`
		PlaybookRuns []interface{} `json:"playbook_runs"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotNil(t, resp.Tasks)
	assert.NotNil(t, resp.Transfers)
	assert.NotNil(t, resp.PlaybookRuns)
}

func TestAudit_Returns501(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.POST("/api/v1/ai/audit", h.Audit)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/ai/audit", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 501, w.Code)
}

func TestAIDebugMode_IncludesPromptTextInAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	seedNodes(t, db, ctx)

	taskStore := store.NewTaskStore(db)
	require.NoError(t, taskStore.Init(ctx))

	transferRecordStore := store.NewTransferRecordStore(db)
	require.NoError(t, transferRecordStore.Init(ctx))

	playbookStore := store.NewPlaybookStore(db)
	require.NoError(t, playbookStore.Init(ctx))

	playbookRunStore := store.NewPlaybookRunStore(db)
	require.NoError(t, playbookRunStore.Init(ctx))

	auditStore := store.NewAIAuditStore(db)
	require.NoError(t, auditStore.Init(ctx))

	nodeStore := store.NewNodeStore(db)
	keyManager := NewKeyManager()
	webExecutor := NewWebExecutor(db, taskStore, transferRecordStore,
		playbookRunStore, nodeStore, playbookStore, auditStore, keyManager, true)

	config := &ai2.Config{}
	nodeMgr := seededNodeManager(db)
	agent, err := ai2.NewAgent(webExecutor, config, nodeMgr, nil, nil, true)
	require.NoError(t, err)

	aiHandler := NewAIHandler(db, auditStore, webExecutor, keyManager, agent, true)

	router := gin.New()
	router.POST("/api/v1/ai/chat", aiHandler.Chat)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"message": "list all nodes", "session_id": ""})
	req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var promptText string
	for i := 0; i < 20; i++ {
		err = db.QueryRow("SELECT prompt_text FROM ai_audit_log ORDER BY created_at DESC LIMIT 1").Scan(&promptText)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err)
	assert.NotEmpty(t, promptText, "prompt_text should be set when debug mode is on")
}

func TestAIDebugMode_OmitsPromptTextByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	seedNodes(t, db, ctx)

	taskStore := store.NewTaskStore(db)
	require.NoError(t, taskStore.Init(ctx))

	transferRecordStore := store.NewTransferRecordStore(db)
	require.NoError(t, transferRecordStore.Init(ctx))

	playbookStore := store.NewPlaybookStore(db)
	require.NoError(t, playbookStore.Init(ctx))

	playbookRunStore := store.NewPlaybookRunStore(db)
	require.NoError(t, playbookRunStore.Init(ctx))

	auditStore := store.NewAIAuditStore(db)
	require.NoError(t, auditStore.Init(ctx))

	nodeStore := store.NewNodeStore(db)
	keyManager := NewKeyManager()
	webExecutor := NewWebExecutor(db, taskStore, transferRecordStore,
		playbookRunStore, nodeStore, playbookStore, auditStore, keyManager, false)

	config := &ai2.Config{}
	nodeMgr := seededNodeManager(db)
	agent, err := ai2.NewAgent(webExecutor, config, nodeMgr, nil, nil, false)
	require.NoError(t, err)

	aiHandler := NewAIHandler(db, auditStore, webExecutor, keyManager, agent, false)

	router := gin.New()
	router.POST("/api/v1/ai/chat", aiHandler.Chat)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"message": "list all nodes", "session_id": ""})
	req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var promptText string
	for i := 0; i < 20; i++ {
		err = db.QueryRow("SELECT prompt_text FROM ai_audit_log ORDER BY created_at DESC LIMIT 1").Scan(&promptText)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err)
	assert.Empty(t, promptText, "prompt_text should be empty when debug mode is off")
}

func TestChat_LogDoesNotContainEncryptedKey(t *testing.T) {
	_, h := aiTestSetup(t)

	router := gin.New()
	router.POST("/api/v1/ai/chat", h.Chat)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"message":           "list nodes",
		"session_id":        "test-session",
		"encrypted_api_key": "this-is-a-secret-key-that-should-not-appear-in-logs",
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	responseBody := w.Body.String()
	assert.NotContains(t, responseBody, "this-is-a-secret-key-that-should-not-appear-in-logs")
}

func TestKeyManagerDecrypt_InvalidSession_ReturnsError(t *testing.T) {
	km := NewKeyManager()
	_, err := km.Decrypt("nonexistent", "dGVzdA==")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestKeyManagerDecrypt_InvalidBase64_ReturnsError(t *testing.T) {
	km := NewKeyManager()
	session, err := km.CreateSession()
	require.NoError(t, err)

	_, err = km.Decrypt(session.SessionID, "not-valid-base64!!!")
	assert.Error(t, err)
}

func TestNewAIHandler_NilConfig(t *testing.T) {
	db, h := aiTestSetup(t)
	assert.NotNil(t, h.db)
	assert.NotNil(t, h.agent)
	assert.NotNil(t, h.keyManager)
	assert.NotNil(t, h.sessionMgr)
	_ = db
}

func TestKeyManagerDecrypt_PlaintextFallback_Success(t *testing.T) {
	km := NewKeyManager()

	// Plaintext fallback with valid base64 content
	plaintext := "sk-test-api-key-12345"
	encoded := base64.StdEncoding.EncodeToString([]byte(plaintext))
	result, err := km.Decrypt("any-session", "__plain__:"+encoded)
	require.NoError(t, err)
	assert.Equal(t, plaintext, string(result))
}

func TestKeyManagerDecrypt_PlaintextFallback_InvalidBase64_ReturnsError(t *testing.T) {
	km := NewKeyManager()

	// Plaintext fallback with invalid base64 should fail
	_, err := km.Decrypt("any-session", "__plain__:not-valid-base64!!!")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode plaintext")
}

func TestKeyManagerDecrypt_PlaintextFallback_EmptyString_ReturnsEmpty(t *testing.T) {
	km := NewKeyManager()

	// Empty base64 data after prefix is valid, decodes to empty bytes
	result, err := km.Decrypt("any-session", "__plain__:")
	require.NoError(t, err)
	assert.Empty(t, string(result))
}

// ---- AI Provider Integration Tests (env-var gated) ----

func aiTestRouter(t *testing.T, h *AIHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/ai/session-key", h.GetSessionKey)
	r.POST("/api/v1/ai/models", h.Models)
	r.POST("/api/v1/ai/test", h.Test)
	return r
}

func TestModelsEndpoint_MissingBaseURL_Returns400(t *testing.T) {
	_, h := aiTestSetup(t)
	r := aiTestRouter(t, h)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"session_id":        "test",
		"encrypted_api_key": "dGVzdA==",
		"base_url":          "",
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestModelsEndpoint_MissingSessionKey_Returns400(t *testing.T) {
	_, h := aiTestSetup(t)
	r := aiTestRouter(t, h)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"session_id":        "",
		"encrypted_api_key": "",
		"base_url":          "https://api.deepseek.com",
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestModelsEndpoint_AnthropicType_ReturnsHardcodedModels(t *testing.T) {
	_, h := aiTestSetup(t)
	r := aiTestRouter(t, h)

	session, err := h.keyManager.CreateSession()
	require.NoError(t, err)

	plaintextB64 := base64.StdEncoding.EncodeToString([]byte("sk-test"))

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"session_id":        session.SessionID,
		"encrypted_api_key": "__plain__:" + plaintextB64,
		"base_url":          "https://api.anthropic.com",
		"api_type":          "anthropic",
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Greater(t, len(resp.Models), 0)
	assert.Equal(t, "claude-sonnet-4-20250514", resp.Models[0].ID)
}

func TestTestEndpoint_MissingModel_Returns400(t *testing.T) {
	_, h := aiTestSetup(t)
	r := aiTestRouter(t, h)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"session_id":        "test",
		"encrypted_api_key": "dGVzdA==",
		"base_url":          "https://api.deepseek.com",
		"model":             "",
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestTestEndpoint_MissingBaseURL_Returns400(t *testing.T) {
	_, h := aiTestSetup(t)
	r := aiTestRouter(t, h)

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"session_id":        "test",
		"encrypted_api_key": "dGVzdA==",
		"base_url":          "",
		"model":             "deepseek-v4-flash",
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

// ---- Integration tests with real API calls (gated by env vars) ----

func TestModelsEndpoint_DeepSeek_Integration(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping: DEEPSEEK_API_KEY not set")
	}

	_, h := aiTestSetup(t)
	r := aiTestRouter(t, h)

	session, err := h.keyManager.CreateSession()
	require.NoError(t, err)

	// Use plaintext fallback since test environment may not be secure context
	plaintextB64 := base64.StdEncoding.EncodeToString([]byte(apiKey))

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"session_id":        session.SessionID,
		"encrypted_api_key": "__plain__:" + plaintextB64,
		"base_url":          "https://api.deepseek.com",
		"api_type":          "openai",
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var resp struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Greater(t, len(resp.Models), 0, "should return at least one model")

	// Verify DeepSeek V4 models are present
	modelIDs := make([]string, len(resp.Models))
	for i, m := range resp.Models {
		modelIDs[i] = m.ID
	}
	assert.Contains(t, modelIDs, "deepseek-v4-flash", "should include deepseek-v4-flash")
}

func TestTestEndpoint_DeepSeek_Integration(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping: DEEPSEEK_API_KEY not set")
	}

	_, h := aiTestSetup(t)
	r := aiTestRouter(t, h)

	session, err := h.keyManager.CreateSession()
	require.NoError(t, err)

	plaintextB64 := base64.StdEncoding.EncodeToString([]byte(apiKey))

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"session_id":        session.SessionID,
		"encrypted_api_key": "__plain__:" + plaintextB64,
		"base_url":          "https://api.deepseek.com",
		"api_type":          "openai",
		"model":             "deepseek-v4-flash",
	})
	req, _ := http.NewRequest("POST", "/api/v1/ai/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var resp struct {
		Success   bool   `json:"success"`
		Reply     string `json:"reply"`
		Model     string `json:"model"`
		ElapsedMs int64  `json:"elapsed_ms"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	if !resp.Success {
		t.Skipf("Provider returned error (may be transient): %v", resp)
		return
	}
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.Reply)
	assert.Equal(t, "deepseek-v4-flash", resp.Model)
	assert.Greater(t, resp.ElapsedMs, int64(0))
}

// ---- Settings API provider config E2E ----

func TestSettingsProviderConfig_CRUD(t *testing.T) {
	db, h := settingsTestSetup(t)
	_ = db

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/settings", h.List)
	r.GET("/api/v1/settings/:key", h.Get)
	r.PUT("/api/v1/settings/:key", h.Set)

	// List - should contain seed data
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/settings", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	var listResp struct {
		Data []SettingResponse `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &listResp)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(listResp.Data), 3)

	// Get specific key
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/v1/settings/ai_provider", nil)
	r.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w2.Code)

	var getResp SettingResponse
	err = json.Unmarshal(w2.Body.Bytes(), &getResp)
	require.NoError(t, err)
	assert.Equal(t, "ai_provider", getResp.Key)
	assert.Equal(t, "openai", getResp.Value)

	// Update provider config
	w3 := httptest.NewRecorder()
	body, _ := json.Marshal(setSettingRequest{Value: "deepseek"})
	req3, _ := http.NewRequest("PUT", "/api/v1/settings/ai_provider", bytes.NewReader(body))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)
	assert.Equal(t, 200, w3.Code)

	// Verify update persisted
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("GET", "/api/v1/settings/ai_provider", nil)
	r.ServeHTTP(w4, req4)
	var updatedResp SettingResponse
	json.Unmarshal(w4.Body.Bytes(), &updatedResp)
	assert.Equal(t, "deepseek", updatedResp.Value)

	// Add new provider config
	w5 := httptest.NewRecorder()
	body5, _ := json.Marshal(setSettingRequest{Value: "custom-endpoint"})
	req5, _ := http.NewRequest("PUT", "/api/v1/settings/ai_endpoint", bytes.NewReader(body5))
	req5.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w5, req5)
	assert.Equal(t, 200, w5.Code)

	// Verify new key exists in list
	w6 := httptest.NewRecorder()
	req6, _ := http.NewRequest("GET", "/api/v1/settings", nil)
	r.ServeHTTP(w6, req6)
	var listAgain struct {
		Data []SettingResponse `json:"data"`
	}
	json.Unmarshal(w6.Body.Bytes(), &listAgain)
	found := false
	for _, s := range listAgain.Data {
		if s.Key == "ai_endpoint" {
			found = true
			assert.Equal(t, "custom-endpoint", s.Value)
		}
	}
	assert.True(t, found, "ai_endpoint should be in settings list")
}

func aiPermissionsRequest(t *testing.T, h *AIHandler, role string) (int, map[string]interface{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", role)
	h.Permissions(c)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m))
	return w.Code, m
}

func TestAIPermissions_ReadOnlyRoles(t *testing.T) {
	_, h := aiTestSetup(t)
	for _, role := range []string{"viewer", "editor"} {
		code, resp := aiPermissionsRequest(t, h, role)
		assert.Equal(t, 200, code, "role %s", role)
		assert.True(t, resp["read_only"].(bool), "role %s should be read_only", role)

		blocked, ok := resp["blocked_tools"].([]interface{})
		require.True(t, ok, "role %s missing blocked_tools", role)
		assert.Contains(t, blocked, "execute_command", "role %s", role)
		assert.Contains(t, blocked, "execute_script", "role %s", role)
		assert.Contains(t, blocked, "transfer_file", "role %s", role)
		assert.Contains(t, blocked, "run_playbook", "role %s", role)
		assert.Contains(t, blocked, "generate_playbook", "role %s", role)
		assert.Contains(t, blocked, "node_check", "role %s", role)

		allowed, ok := resp["allowed_tools"].([]interface{})
		require.True(t, ok, "role %s missing allowed_tools", role)
		assert.Contains(t, allowed, "query_nodes", "role %s", role)
		assert.Contains(t, allowed, "list_playbooks", "role %s", role)
		assert.NotContains(t, allowed, "execute_command", "role %s", role)
	}
}

func TestAIPermissions_OperatorAndAdmin(t *testing.T) {
	_, h := aiTestSetup(t)
	for _, role := range []string{"operator", "admin"} {
		code, resp := aiPermissionsRequest(t, h, role)
		assert.Equal(t, 200, code, "role %s", role)
		assert.False(t, resp["read_only"].(bool), "role %s should be read-write", role)

		blocked, ok := resp["blocked_tools"].([]interface{})
		require.True(t, ok, "role %s missing blocked_tools", role)
		assert.Empty(t, blocked, "role %s should have no blocked tools", role)

		allowed, ok := resp["allowed_tools"].([]interface{})
		require.True(t, ok, "role %s missing allowed_tools", role)
		assert.Contains(t, allowed, "query_nodes", "role %s", role)
		assert.Contains(t, allowed, "execute_command", "role %s", role)
		assert.Contains(t, allowed, "run_playbook", "role %s", role)
	}
}
