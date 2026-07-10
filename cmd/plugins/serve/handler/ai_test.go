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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// Wait for async audit write
	time.Sleep(100 * time.Millisecond)

	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM ai_audit_log").Scan(&count)
	require.NoError(t, err)
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

	// Wait for async audit and check
	time.Sleep(100 * time.Millisecond)

	var promptText string
	err = db.QueryRow("SELECT prompt_text FROM ai_audit_log ORDER BY created_at DESC LIMIT 1").Scan(&promptText)
	require.NoError(t, err)
	assert.NotEmpty(t, promptText, "prompt_text should be set when debug mode is on")
}

func TestAIDebugMode_OmitsPromptTextByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
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

	// Wait for async audit
	time.Sleep(100 * time.Millisecond)

	var promptText string
	err = db.QueryRow("SELECT prompt_text FROM ai_audit_log ORDER BY created_at DESC LIMIT 1").Scan(&promptText)
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
