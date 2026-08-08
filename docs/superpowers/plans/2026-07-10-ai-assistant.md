# AI Assistant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the stub `handler/ai.go` with a full Agent-powered AI assistant using the existing `internal/ai/` engine, plus RSA-key-encrypted API Key transport, audit logging, and a frontend chat UI.

**Architecture:** Extract an `Executor` interface from `tools.go` so the same Agent can be driven by both `runOwlCommand()` (CLI) and direct Go calls (Web). New `WebExecutor` in `handler/` calls stores/SSH directly. RSA-2048 ephemeral keypair per server startup encrypts API Key in transit. Audit table stores structured tool calls. Frontend uses IndexedDB for encrypted conversation history.

**Tech Stack:** Go (gin, sqlite), JavaScript (vanilla, Web Crypto API, IndexedDB)

## Global Constraints

- `internal/ai/` package must remain usable by both CLI and Web — no new dependencies from `internal/ai/` to `cmd/plugins/serve/`
- `handler/` package imports `internal/ai/` but not vice versa
- All existing CLI AI tests (`agent_test.go`) must remain green after refactoring
- The `RunPlaybookTool` is not yet connected to a real playbook runner — keep its current mock implementation in WebExecutor (it just formats info, doesn't actually run)
- RSA keypair regenerates every server restart (ephemeral, never persisted)
- `encrypted_api_key` field must be explicitly filtered from all log output
- `--ai-debug` is a command-line flag only, not a settings toggle
- All new frontend files go in `cmd/plugins/serve/web/js/`
- New backend files: `executor.go` in `internal/ai/`, `ai_audit.go` in `store/`, `aiexecutor.go` in `handler/`
- Modified backend files: `internal/ai/tools.go`, `internal/ai/agent.go`, `handler/ai.go`, `server.go`, `cmd/plugins/serve/main.go` (or wherever flags are parsed)
- Follow existing code style: gin handlers in handler/, DB access in store/, no new dependencies

---

## File Structure

```
internal/ai/
├── executor.go          — NEW: Executor interface (shared by CLI & Web)
├── tools.go             — MODIFY: constructors accept Executor; CLIExecutor impl
├── agent.go             — MODIFY: NewAgent accepts Executor
├── agent_test.go        — UNCHANGED (tests pass through executor)

cmd/plugins/serve/
├── handler/
│   ├── ai.go            — MODIFY: full Agent handler with session-key, chat, context
│   ├── aiexecutor.go    — NEW: WebExecutor impl (direct Go calls)
│   ├── ai_keys.go       — NEW: RSA keypair management
├── store/
│   ├── ai_audit.go      — NEW: audit log CRUD
├── server.go            — MODIFY: aiHandler setup, routes, --ai-debug flag
├── main.go              — MODIFY (or config.go): add --ai-debug flag

web/js/
├── crypto.js            — NEW: Web Crypto wallet (RSA encrypt + AES-GCM local)
├── storage.js           — NEW: IndexedDB conversation store + localStorage key vault
├── audit.js             — NEW: audit reporting (optional frontend path)
├── pages/
│   ├── files.js         — UNCHANGED
│   └── settings.js      — or wherever settings pages are
├── app.js               — MODIFY: add AI assistant page route/panel
├── api.js               — MODIFY: add getSessionKey(), aiChat(), getAiContext(), reportAudit()

web/css/
└── app.css              — MODIFY: AI chat styles (messages, input, debug banner)
```

---

### Task 1: Executor Interface + Refactor CLI Tools

**Files:**
- Create: `internal/ai/executor.go`
- Modify: `internal/ai/tools.go:66-72` (Tool interface unchanged), tool constructors, CLIExecutor
- Modify: `internal/ai/agent.go:122` (NewAgent signature)
- Test: `internal/ai/agent_test.go` (verify existing tests pass)

**Interfaces:**
- Consumes: `internal/ai/tools.go` Tool interface (unchanged — `Name()`, `Description()`, `Parameters()`, `Validate()`, `Execute()`)
- Produces: `Executor` interface, `CLIExecutor` struct, modified `NewAgent(executor Executor, ...)`, modified tool constructors

**E2E Test Specification:**
```
1. `go build ./...` passes (no compile errors from interface change)
2. `go test ./internal/ai/ -v` passes all existing tests (agent_test.go, llm_test.go, etc.)
3. Build and run `owl ai "list nodes"` → CLI still works end-to-end
4. `go vet ./internal/ai/` clean
```

- [ ] **Step 1: Create `executor.go` with the `Executor` interface**

```go
package ai

import "context"

type (
	QueryNodesParams struct {
		Group  string                 `json:"group"`
		Labels map[string]interface{} `json:"labels"`
		Status string                 `json:"status"`
		Search string                 `json:"search"`
		Format string                 `json:"format"`
	}

	ExecCommandParams struct {
		Nodes   []string `json:"nodes"`
		Command string   `json:"command"`
		Group   string   `json:"group"`
		Label   string   `json:"label"`
		Search  string   `json:"search"`
		Timeout int      `json:"timeout"`
		Format  string   `json:"format"`
		Mode    string   `json:"mode"`
	}

	ExecScriptParams struct {
		Script  string   `json:"script"`
		Nodes   []string `json:"nodes"`
		Group   string   `json:"group"`
		Label   string   `json:"label"`
		Search  string   `json:"search"`
		Dest    string   `json:"dest"`
		Args    string   `json:"args"`
		Timeout int      `json:"timeout"`
		Inline  bool     `json:"inline"`
		Keep    bool     `json:"keep"`
	}

	GeneratePlaybookParams struct {
		Requirement string                 `json:"requirement"`
		Vars        map[string]interface{} `json:"vars"`
	}

	TransferFileParams struct {
		SourceFile string   `json:"source_file"`
		Nodes      []string `json:"nodes"`
		DestDir    string   `json:"dest_dir"`
		Mode       string   `json:"mode"`
		Permission string   `json:"permission"`
		Search     string   `json:"search"`
	}

	RunPlaybookParams struct {
		Name   string                 `json:"name"`
		Nodes  []string               `json:"nodes"`
		Group  string                 `json:"group"`
		Label  string                 `json:"label"`
		Search string                 `json:"search"`
		Vars   map[string]interface{} `json:"vars"`
		Tags   string                 `json:"tags"`
		Check  bool                   `json:"check"`
	}

	QueryNodesResult      struct { Text string }
	ExecResult            struct { Text string }
	ExecScriptResult      struct { Text string }
	GeneratePlaybookResult struct { Text string }
	TransferResult        struct { Text string }
	RunPlaybookResult     struct { Text string }
	ListPlaybooksResult   struct { Text string }
	PlaybookInfoResult    struct { Text string }
	ValidateResult        struct { Text string }
	NodeCheckResult       struct { Text string }
	QueryDatabaseResult   struct { Text string }
)

type Executor interface {
	QueryNodes(ctx context.Context, params QueryNodesParams) (*QueryNodesResult, error)
	ExecuteCommand(ctx context.Context, params ExecCommandParams) (*ExecResult, error)
	ExecuteScript(ctx context.Context, params ExecScriptParams) (*ExecScriptResult, error)
	GeneratePlaybook(ctx context.Context, params GeneratePlaybookParams) (*GeneratePlaybookResult, error)
	TransferFile(ctx context.Context, params TransferFileParams) (*TransferResult, error)
	ListPlaybooks(ctx context.Context) (*ListPlaybooksResult, error)
	PlaybookInfo(ctx context.Context, params PlaybookInfoParams) (*PlaybookInfoResult, error)
	ValidatePlaybook(ctx context.Context, params ValidatePlaybookParams) (*ValidateResult, error)
	NodeCheck(ctx context.Context, params NodeCheckParams) (*NodeCheckResult, error)
	QueryDatabase(ctx context.Context, params QueryDatabaseParams) (*QueryDatabaseResult, error)
}

type PlaybookInfoParams struct {
	Name string `json:"name"`
}

type ValidatePlaybookParams struct {
	File string `json:"file"`
}

type NodeCheckParams struct {
	Nodes   []string `json:"nodes"`
	Group   string   `json:"group"`
	All     bool     `json:"all"`
	Timeout int      `json:"timeout"`
	Update  bool     `json:"update"`
}

type QueryDatabaseParams struct {
	Query  string                 `json:"query"`
	Group  string                 `json:"group"`
	Labels map[string]interface{} `json:"labels"`
	Status string                 `json:"status"`
	Search string                 `json:"search"`
	Format string                 `json:"format"`
}
```

- [ ] **Step 2: Create `CLIExecutor` inside `tools.go`**

Add to `internal/ai/tools.go` (before the existing tool types, after imports):

```go
// CLIExecutor calls the owl CLI binary via runOwlCommand
type CLIExecutor struct {
	nodeMgr   node.Manager
	nodeStore NodeStoreAdapter
}

func NewCLIExecutor(nodeMgr node.Manager, nodeStore NodeStoreAdapter) *CLIExecutor {
	return &CLIExecutor{nodeMgr: nodeMgr, nodeStore: nodeStore}
}

func (e *CLIExecutor) QueryNodes(ctx context.Context, p QueryNodesParams) (*QueryNodesResult, error) {
	args := []string{"node", "list", "--no-color"}
	if p.Group != "" { args = append(args, "--groups", p.Group) }
	if p.Status != "" { args = append(args, "--status", p.Status) }
	if p.Format != "" && p.Format != "table" { args = append(args, "--format", p.Format) }
	if p.Search != "" {
		nodes := e.nodeMgr.SearchByName(p.Search)
		if len(nodes) == 0 { return &QueryNodesResult{"No matching nodes found"}, nil }
		var names []string
		for _, n := range nodes { names = append(names, n.Name) }
		args = append(args, "--nodes", strings.Join(names, ","))
	}
	if p.Labels != nil {
		for k, v := range labelKeyMap {
			if val, ok := p.Labels[k]; ok {
				args = append(args, "--label", fmt.Sprintf("%s=%s", v, val))
			}
		}
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &QueryNodesResult{Text: result}, nil
}

func (e *CLIExecutor) ExecuteCommand(ctx context.Context, p ExecCommandParams) (*ExecResult, error) {
	args := []string{"exec", "run", p.Command, "--no-color", "--format", p.Format}
	if p.Format == "" { args = append(args, "--format", "simple") }
	if p.Mode == "serial" { args = append(args, "--serial") }
	if p.Timeout > 0 && p.Timeout != 30 { args = append(args, "--timeout", fmt.Sprintf("%d", p.Timeout)) }
	if len(p.Nodes) > 0 { args = append(args, "--nodes", strings.Join(p.Nodes, ",")) }
	if p.Group != "" { args = append(args, "--groups", p.Group) }
	if p.Label != "" { args = append(args, "--label", p.Label) }
	result, err := runOwlCommand(ctx, args)
	if err != nil { return nil, err }
	return &ExecResult{Text: result}, nil
}

func (e *CLIExecutor) ExecuteScript(ctx context.Context, p ExecScriptParams) (*ExecScriptResult, error) {
	args := []string{"exec", "script", p.Script, "--no-color"}
	if p.Inline { args = append(args, "--inline") }
	if p.Dest != "" && p.Dest != "/tmp" { args = append(args, "--dest", p.Dest) }
	if p.Args != "" { args = append(args, "--args", p.Args) }
	if p.Timeout > 0 && p.Timeout != 300 { args = append(args, "--timeout", fmt.Sprintf("%d", p.Timeout)) }
	if p.Keep { args = append(args, "--keep") }
	if len(p.Nodes) > 0 { args = append(args, "--nodes", strings.Join(p.Nodes, ",")) }
	if p.Group != "" { args = append(args, "--groups", p.Group) }
	if p.Label != "" { args = append(args, "--label", p.Label) }
	result, err := runOwlCommand(ctx, args)
	if err != nil { return nil, err }
	return &ExecScriptResult{Text: result}, nil
}

func (e *CLIExecutor) TransferFile(ctx context.Context, p TransferFileParams) (*TransferResult, error) {
	mode := p.Mode
	if mode == "" {
		if len(p.Nodes) >= 5 { mode = "diffusion" } else { mode = "direct" }
	}
	var args []string
	if mode == "diffusion" {
		args = []string{"file", "transfer", p.SourceFile, "--nodes", strings.Join(p.Nodes, ","), "--dest", p.DestDir}
	} else {
		args = []string{"file", "upload", p.SourceFile, "--nodes", strings.Join(p.Nodes, ","), "--dest", p.DestDir}
	}
	if p.Permission != "" && p.Permission != "0644" { args = append(args, "--mode", p.Permission) }
	result, err := runOwlCommand(ctx, args)
	if err != nil { return nil, err }
	return &TransferResult{Text: result}, nil
}

func (e *CLIExecutor) GeneratePlaybook(ctx context.Context, p GeneratePlaybookParams) (*GeneratePlaybookResult, error) {
	args := []string{"playbook", "generate", p.Requirement}
	if p.Vars != nil {
		vJSON, _ := json.Marshal(p.Vars)
		args = append(args, "--vars", string(vJSON))
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil { return nil, err }
	return &GeneratePlaybookResult{Text: result}, nil
}

func (e *CLIExecutor) ListPlaybooks(ctx context.Context) (*ListPlaybooksResult, error) {
	result, err := runOwlCommand(ctx, []string{"playbook", "list", "--no-color"})
	if err != nil { return nil, err }
	return &ListPlaybooksResult{Text: result}, nil
}

func (e *CLIExecutor) PlaybookInfo(ctx context.Context, p PlaybookInfoParams) (*PlaybookInfoResult, error) {
	result, err := runOwlCommand(ctx, []string{"playbook", "info", p.Name})
	if err != nil { return nil, err }
	return &PlaybookInfoResult{Text: result}, nil
}

func (e *CLIExecutor) ValidatePlaybook(ctx context.Context, p ValidatePlaybookParams) (*ValidateResult, error) {
	result, err := runOwlCommand(ctx, []string{"playbook", "validate", p.File})
	if err != nil { return nil, err }
	return &ValidateResult{Text: result}, nil
}

func (e *CLIExecutor) NodeCheck(ctx context.Context, p NodeCheckParams) (*NodeCheckResult, error) {
	args := []string{"node", "check", "--no-color"}
	if p.All { args = append(args, "--all") }
	if p.Group != "" { args = append(args, "--groups", p.Group) }
	if len(p.Nodes) > 0 { args = append(args, p.Nodes...) }
	if p.Timeout > 0 && p.Timeout != 10 { args = append(args, "--timeout", fmt.Sprintf("%ds", p.Timeout)) }
	result, err := runOwlCommand(ctx, args)
	if err != nil { return nil, err }
	return &NodeCheckResult{Text: result}, nil
}

func (e *CLIExecutor) QueryDatabase(ctx context.Context, p QueryDatabaseParams) (*QueryDatabaseResult, error) {
	args := []string{"node", "list", "--no-color"}
	if p.Group != "" { args = append(args, "--groups", p.Group) }
	if p.Status != "" { args = append(args, "--status", p.Status) }
	if p.Format != "" && p.Format != "table" { args = append(args, "--format", p.Format) }
	result, err := runOwlCommand(ctx, args)
	if err != nil { return nil, err }
	return &QueryDatabaseResult{Text: result}, nil
}
```

- [ ] **Step 3: Refactor each tool to use `Executor`**

Each tool's `Execute()` delegates to the executor instead of calling `runOwlCommand()` directly. For example, `ExecuteCommandTool`:

```go
type ExecuteCommandTool struct {
	executor Executor
}

func NewExecuteCommandTool(executor Executor) *ExecuteCommandTool {
	return &ExecuteCommandTool{executor: executor}
}

func (t *ExecuteCommandTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	p := ExecCommandParams{}
	p.Command, _ = params["command"].(string)
	if p.Command == "" { return "", fmt.Errorf("missing command") }
	p.Timeout = 30
	if tv, ok := params["timeout"].(float64); ok { p.Timeout = int(tv) }
	p.Format, _ = params["format"].(string)
	if p.Format == "" { p.Format = "simple" }
	p.Mode, _ = params["mode"].(string)
	if p.Mode == "" { p.Mode = "parallel" }
	if nodeList, ok := params["nodes"].([]interface{}); ok {
		for _, n := range nodeList { if s, ok := n.(string); ok { p.Nodes = append(p.Nodes, s) } }
	}
	p.Group, _ = params["group"].(string)
	p.Label, _ = params["label"].(string)
	p.Search, _ = params["search"].(string)

	result, err := t.executor.ExecuteCommand(ctx, p)
	if err != nil {
		// Fallback: use nodeMgr-based mock (keep existing fallback logic)
	}
	return result.Text, nil
}
```

Similarly refactor: `QueryNodesTool`, `ExecuteScriptTool`, `GeneratePlaybookTool`, `TransferFileTool`, `ListPlaybooksTool`, `RunPlaybookTool`, `PlaybookInfoTool`, `ValidatePlaybookTool`, `NodeCheckTool`, `QueryDatabaseTool`.

**Important:** Each tool's `Execute()` must keep its fallback logic (the in-process mock that uses nodeMgr directly) as a second path when the executor returns an error, to maintain backward compatibility with tests that set `DisableRealCommands = true`.

- [ ] **Step 4: Modify `NewAgent` in `agent.go`**

```go
func NewAgent(executor Executor, config *Config, nodeMgr node.Manager, nodeStore NodeStoreAdapter, playbookParser *playbook.Parser, debug ...bool) (*Agent, error) {
	registry := NewToolRegistry()
	registry.Register(NewQueryNodesTool(executor))
	registry.Register(NewExecuteCommandTool(executor))
	registry.Register(NewGeneratePlaybookTool(executor))
	registry.Register(NewTransferFileTool(executor))
	registry.Register(NewExecuteScriptTool(executor))
	registry.Register(NewQueryDatabaseTool(executor))
	registry.Register(NewListPlaybooksTool(executor))
	registry.Register(NewRunPlaybookTool(executor))
	registry.Register(NewPlaybookInfoTool(executor))
	registry.Register(NewValidatePlaybookTool(executor))
	registry.Register(NewNodeCheckTool(executor))
	// ... rest unchanged
}
```

- [ ] **Step 5: Update CLI caller of `NewAgent`**

Find where `NewAgent` is called from the CLI and pass `NewCLIExecutor(nodeMgr, nodeStore)` as the first arg.

- [ ] **Step 6: Run existing tests**

Run: `go test ./internal/ai/ -v -run TestAgent`
Expected: All tests PASS (they work through fallback mock logic since `DisableRealCommands = true`)

- [ ] **Step 7: Commit**

```bash
git add internal/ai/executor.go internal/ai/tools.go internal/ai/agent.go
git commit -m "refactor(ai): extract Executor interface, add CLIExecutor, inject into tools"
```

---

### Task 2: AI Audit Store

**Files:**
- Create: `cmd/plugins/serve/store/ai_audit.go`

**Interfaces:**
- Consumes: `database/sql.DB`
- Produces: `AIAuditStore` with `Init`, `Create`, `List`, `Get`

**E2E Test Specification:**
```
1. `go build ./cmd/plugins/serve/...` passes
2. Unit test: create audit record → List returns it → Get by ID matches
3. Unit test: List with userID filter returns only that user's records
4. Unit test: List pagination (limit/offset) works
5. Server starts: `go run ./cmd/plugins/serve --port 8090` → DB has ai_audit_log table
```

- [ ] **Step 1: Create `store/ai_audit.go`**

```go
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type AIAuditRecord struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	Intent         string    `json:"intent"`
	Tool           string    `json:"tool"`
	ParamsSnapshot string    `json:"params_snapshot"`
	Result         string    `json:"result"`
	TargetType     string    `json:"target_type"`
	TargetIDs      string    `json:"target_ids"`
	PromptText     string    `json:"prompt_text,omitempty"`
	ReplyText      string    `json:"reply_text,omitempty"`
	LLMModel       string    `json:"llm_model"`
	LLMDurationMs  int64     `json:"llm_duration_ms"`
	CreatedAt      time.Time `json:"created_at"`
}

type AIAuditStore struct {
	db *sql.DB
}

func NewAIAuditStore(db *sql.DB) *AIAuditStore {
	return &AIAuditStore{db: db}
}

func (s *AIAuditStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS ai_audit_log (
			id              TEXT PRIMARY KEY,
			user_id         TEXT NOT NULL,
			intent          TEXT NOT NULL DEFAULT '',
			tool            TEXT NOT NULL DEFAULT '',
			params_snapshot TEXT NOT NULL DEFAULT '{}',
			result          TEXT NOT NULL DEFAULT 'success',
			target_type     TEXT DEFAULT '',
			target_ids      TEXT DEFAULT '[]',
			prompt_text     TEXT DEFAULT '',
			reply_text      TEXT DEFAULT '',
			llm_model       TEXT DEFAULT '',
			llm_duration_ms INTEGER DEFAULT 0,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create ai_audit_log table: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_ai_audit_user ON ai_audit_log(user_id)
	`)
	if err != nil {
		return fmt.Errorf("create ai_audit_user index: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_ai_audit_time ON ai_audit_log(created_at)
	`)
	return err
}

func (s *AIAuditStore) Create(ctx context.Context, r *AIAuditRecord) error {
	r.ID = uuid.New().String()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_audit_log (id, user_id, intent, tool, params_snapshot, result,
			target_type, target_ids, prompt_text, reply_text, llm_model, llm_duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.UserID, r.Intent, r.Tool, r.ParamsSnapshot, r.Result,
		r.TargetType, r.TargetIDs, r.PromptText, r.ReplyText,
		r.LLMModel, r.LLMDurationMs, r.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert ai audit: %w", err)
	}
	return nil
}

func (s *AIAuditStore) List(ctx context.Context, userID string, offset, limit int) ([]*AIAuditRecord, int, error) {
	where := ""
	args := []interface{}{}
	if userID != "" {
		where = " WHERE user_id = ?"
		args = append(args, userID)
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM ai_audit_log" + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count ai audit: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT id, user_id, intent, tool, params_snapshot, result, target_type, target_ids, prompt_text, reply_text, llm_model, llm_duration_ms, created_at FROM ai_audit_log"+where+" ORDER BY created_at DESC LIMIT ? OFFSET ?",
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list ai audit: %w", err)
	}
	defer rows.Close()

	var records []*AIAuditRecord
	for rows.Next() {
		r := &AIAuditRecord{}
		var createdAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.Intent, &r.Tool, &r.ParamsSnapshot, &r.Result,
			&r.TargetType, &r.TargetIDs, &r.PromptText, &r.ReplyText, &r.LLMModel, &r.LLMDurationMs, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("scan ai audit: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		records = append(records, r)
	}
	return records, total, nil
}

func (s *AIAuditStore) Get(ctx context.Context, id string) (*AIAuditRecord, error) {
	r := &AIAuditRecord{}
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, user_id, intent, tool, params_snapshot, result, target_type, target_ids, prompt_text, reply_text, llm_model, llm_duration_ms, created_at FROM ai_audit_log WHERE id = ?", id).
		Scan(&r.ID, &r.UserID, &r.Intent, &r.Tool, &r.ParamsSnapshot, &r.Result,
			&r.TargetType, &r.TargetIDs, &r.PromptText, &r.ReplyText, &r.LLMModel, &r.LLMDurationMs, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("get ai audit: %w", err)
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return r, nil
}
```

- [ ] **Step 2: Commit**

```bash
git add cmd/plugins/serve/store/ai_audit.go
git commit -m "feat: add AI audit store"
```

---

### Task 3: RSA Keypair + AI Handler Rewrite

**Files:**
- Create: `cmd/plugins/serve/handler/aiexecutor.go`
- Create: `cmd/plugins/serve/handler/ai_keys.go`
- Modify: `cmd/plugins/serve/handler/ai.go` (replace stub with full handler)
- Modify: `cmd/plugins/serve/server.go` (init AI store, routes, --ai-debug)
- Modify (or find flag parser): `cmd/plugins/serve/main.go` (add --ai-debug flag)

**Interfaces:**
- Consumes: `internal/ai.Executor` (WebExecutor), `store.AIAuditStore`, `database/sql.DB`
- Produces: `AIHandler` with `SessionKey`, `Chat`, `Context`, `Audit` endpoints

**E2E Test Specification:**
```
1. curl -v http://localhost:8090/api/v1/ai/session-key → 200, returns {session_id, public_key_spki}
2. curl with encrypted key + valid session → /ai/chat returns reply
3. curl with invalid session_id → /ai/chat returns 400
4. viewer role can call GET /ai/session-key, POST /ai/chat (moved from operator)
5. operator role can still call /exec, /transfer (unchanged)
6. After chat → ai_audit_log table has a new record
7. Server logs do NOT contain encrypted_api_key (grep test)
8. --ai-debug flag: when set, audit records include prompt_text; when unset, prompt_text is empty
9. `go build ./cmd/plugins/serve/...` passes
10. `go vet ./cmd/plugins/serve/...` clean
```

- [ ] **Step 1: Create `handler/ai_keys.go`**

```go
package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SessionKey struct {
	SessionID    string
	PrivateKey   *rsa.PrivateKey
	PublicKeySPKI string
	CreatedAt    time.Time
}

type KeyManager struct {
	mu       sync.RWMutex
	sessions map[string]*SessionKey
}

func NewKeyManager() *KeyManager {
	return &KeyManager{sessions: make(map[string]*SessionKey)}
}

func (km *KeyManager) CreateSession() (*SessionKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}

	session := &SessionKey{
		SessionID:     uuid.New().String(),
		PrivateKey:    key,
		PublicKeySPKI: base64.StdEncoding.EncodeToString(pubKeyBytes),
		CreatedAt:     time.Now(),
	}

	km.mu.Lock()
	km.sessions[session.SessionID] = session
	km.mu.Unlock()

	return session, nil
}

func (km *KeyManager) Decrypt(sessionID string, ciphertextB64 string) ([]byte, error) {
	km.mu.RLock()
	session, ok := km.sessions[sessionID]
	km.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}

	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, session.PrivateKey, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}

func (km *KeyManager) Cleanup(maxAge time.Duration) {
	km.mu.Lock()
	defer km.mu.Unlock()
	for id, s := range km.sessions {
		if time.Since(s.CreatedAt) > maxAge {
			delete(km.sessions, id)
		}
	}
}
```

- [ ] **Step 2: Create `handler/aiexecutor.go`**

```go
package handler

import (
	"context"
	"fmt"
	"strings"

	ai2 "github.com/cangyunye/go-owl/cmd/plugins/serve/handler"  // will use store packages
	// Actually, WebExecutor needs access to TaskStore, PlaybookRunStore, etc.
	// It will be in handler/ package and call those stores directly.
)
```

The WebExecutor implements `internal/ai.Executor`. It receives TaskStore, TransferRecordStore, PlaybookRunStore, NodeStore, db, etc. via constructor and calls their methods directly instead of shelling out.

```go
type WebExecutor struct {
	db                 *sql.DB
	taskStore          *store.TaskStore
	transferRecordStore *store.TransferRecordStore
	playbookRunStore   *store.PlaybookRunStore
	nodeStore          *store.NodeStore
	playbookStore      *store.PlaybookStore
	auditStore         *store.AIAuditStore
	keyManager         *KeyManager
	debugMode          bool
}

func NewWebExecutor(db *sql.DB, taskStore *store.TaskStore, transferRecordStore *store.TransferRecordStore,
	playbookRunStore *store.PlaybookRunStore, nodeStore *store.NodeStore,
	playbookStore *store.PlaybookStore, auditStore *store.AIAuditStore,
	keyManager *KeyManager, debugMode bool) *WebExecutor {
	return &WebExecutor{
		db: db, taskStore: taskStore, transferRecordStore: transferRecordStore,
		playbookRunStore: playbookRunStore, nodeStore: nodeStore,
		playbookStore: playbookStore, auditStore: auditStore,
		keyManager: keyManager, debugMode: debugMode,
	}
}
```

Each method on WebExecutor:
- `QueryNodes` — calls `nodeStore.List()` / `GetByGroup()` / etc. with filters, returns formatted text
- `ExecuteCommand` — creates a task via `taskStore.Create()` with `Type: "exec"`, returns task info
- `ExecuteScript` — creates a task via `taskStore.Create()` with `Type: "script"`, returns task info
- `TransferFile` — creates a transfer record via `transferRecordStore.Create()`, returns record info
- `GeneratePlaybook` — generates YAML content (same logic as current tool), returns the generated playbook text
- `ListPlaybooks` — lists playbooks from `playbookStore`
- `PlaybookInfo` — gets playbook detail from `playbookStore`
- `ValidatePlaybook` — validates playbook YAML from `playbookStore`
- `NodeCheck` — creates a task via `taskStore.Create()` with `Type: "check"`, returns task info
- `QueryDatabase` — queries the nodes table directly via `db.QueryContext()`, returns formatted results
- `RunPlaybook` — creates a playbook run via `playbookRunStore.Create()`, returns run info

Each method wraps the result in a `*Result{Text: ...}` where Text is a human-readable string (same format as CLI output).

- [ ] **Step 3: Rewrite `handler/ai.go`**

Replace the stub with full handler:

```go
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
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
}

func NewAIHandler(db *sql.DB, auditStore *store.AIAuditStore, executor *WebExecutor,
	keyManager *KeyManager, agent *ai2.Agent, debugMode bool) *AIHandler {
	return &AIHandler{
		db: db, auditStore: auditStore, executor: executor,
		keyManager: keyManager, agent: agent,
		sessionMgr: ai2.NewSessionManager(),
		debugMode: debugMode,
	}
}

func (h *AIHandler) GetSessionKey(c *gin.Context) {
	session, err := h.keyManager.CreateSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "generate session key failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"session_id":   session.SessionID,
		"public_key_spki": session.PublicKeySPKI,
	})
}

type chatRequest struct {
	Message         string `json:"message"`
	SessionID       string `json:"session_id"`
	EncryptedAPIKey string `json:"encrypted_api_key"`
}

func (h *AIHandler) Chat(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "message is required"})
		return
	}

	// Decrypt API Key
	apiKey, err := h.keyManager.Decrypt(req.SessionID, req.EncryptedAPIKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid session or encrypted key"})
		return
	}

	// Skip logging the encrypted key
	// encrypted_api_key is intentionally not captured in any log or audit record

	// Set up LLM client with the decrypted API Key
	// For now, reuse the fallback classifier (no LLM call) until LLM integration
	userID := c.GetString("user_id")
	role := c.GetString("role")

	// Get or create AI session
	sessionID := c.GetHeader("X-Ai-Session")
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	session, exists := h.sessionMgr.GetSession(sessionID)
	if !exists {
		// Inject system prompt with role and context
		sysPrompt := fmt.Sprintf("Current user role: %s. You are an OWL operations assistant.", role)

		// Create new agent with per-request API key for LLM
		session = h.sessionMgr.CreateSession(sessionID, h.agent)
	}

	startTime := time.Now()
	reply, err := session.Send(c.Request.Context(), req.Message)
	durationMs := time.Since(startTime).Milliseconds()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	// Conversation-level audit — tool-level audit is handled inside WebExecutor methods
	go func() {
		h.auditStore.Create(c.Request.Context(), &store.AIAuditRecord{
			UserID:    userID,
			Intent:    "conversation",
			Result:    "success",
			ReplyText: reply,
		})
	}()

	c.JSON(http.StatusOK, gin.H{
		"reply":      reply,
		"session_id": sessionID,
	})
}



func (h *AIHandler) GetContext(c *gin.Context) {
	userID := c.GetString("user_id")
	_ = userID
	// Return recent 20 operations summary (tasks + transfers + playbook runs)
	// This is a lightweight DB query, no LLM call
	c.JSON(http.StatusOK, gin.H{
		"tasks":          []interface{}{},
		"transfers":      []interface{}{},
		"playbook_runs":  []interface{}{},
	})
}

func (h *AIHandler) Audit(c *gin.Context) {
	// Backup endpoint for frontend direct audit reporting
	// (not used in normal flow, audit happens server-side)
	c.JSON(http.StatusNotImplemented, gin.H{"code": 501, "message": "server-side audit is the primary path"})
}
```

- [ ] **Step 4: Update `server.go`**

Add to `Server` struct:
```go
	aiDebugMode        bool
	auditStore         *store.AIAuditStore
	keyManager         *handler.KeyManager
```

In `Init()`:
```go
s.auditStore = store.NewAIAuditStore(db)
if err := s.auditStore.Init(context.Background()); err != nil {
	return nil, fmt.Errorf("init ai audit store: %w", err)
}

s.keyManager = handler.NewKeyManager()

// Create WebExecutor
webExecutor := handler.NewWebExecutor(db, s.Tasks, s.transferRecordStore,
	playbookRunStore, nodeStore, playbookStore, s.auditStore, s.keyManager, s.aiDebugMode)

// Create AI Agent with WebExecutor
// Config for non-LLM fallback: AI Agent can work without API Key
config := &ai2.Config{
	// Model and APIKey come from DB settings (key: ai_model, ai_provider) and frontend request
}
agent, err := ai2.NewAgent(webExecutor, config,
	node.NewManager(store.NewNodeStore(db).Adapter()),
	nil, nil, s.aiDebugMode)
if err != nil {
	return nil, fmt.Errorf("init ai agent: %w", err)
}

s.aiHandler = handler.NewAIHandler(db, s.auditStore, webExecutor, s.keyManager, agent, s.aiDebugMode)
```

Update routes — move AI chat from `operator` to `reader` group (viewer+):
```go
reader.GET("/ai/session-key", s.aiHandler.GetSessionKey)
reader.GET("/ai/context", s.aiHandler.GetContext)
reader.POST("/ai/chat", s.aiHandler.Chat)
reader.POST("/ai/audit", s.aiHandler.Audit)
```

Add `--ai-debug` field to Config struct, parse in main.go.

- [ ] **Step 5: Commit**

```bash
git add cmd/plugins/serve/handler/ai.go cmd/plugins/serve/handler/aiexecutor.go cmd/plugins/serve/handler/ai_keys.go cmd/plugins/serve/server.go
git commit -m "feat(ai): add RSA key exchange, WebExecutor, full AI handler with audit"
```

---

### Task 4: Frontend — crypto.js, storage.js, audit.js

**Files:**
- Create: `cmd/plugins/serve/web/js/crypto.js`
- Create: `cmd/plugins/serve/web/js/storage.js`
- Create: `cmd/plugins/serve/web/js/audit.js`

**E2E Test Specification:**
```
1. In browser console: CryptoWallet.encryptApiKey(pubKeyB64, "sk-test") returns base64 string
2. In browser console: CryptoWallet.encryptLocal({foo:"bar"}, "user1") returns {salt, iv, data}
3. In browser console: CryptoWallet.decryptLocal(packet, "user1") returns original data
4. In browser console: CryptoWallet.decryptLocal(packet, "user2") throws (wrong user)
5. IndexedDB: AIStorage.saveConversation → AIStorage.getConversations → returns it
6. localStorage: AIStorage.saveApiKey → AIStorage.loadApiKey → returns saved data
7. After loading wrong user key, loadApiKey returns null (decrypt fails)
```

- [ ] **Step 1: Create `web/js/crypto.js`**

```javascript
// Crypto wallet for AI page
const CryptoWallet = {
  async encryptApiKey(publicKeySpkiB64, apiKey) {
    const spkiBytes = Uint8Array.from(atob(publicKeySpkiB64), c => c.charCodeAt(0));
    const publicKey = await crypto.subtle.importKey(
      'spki', spkiBytes, { name: 'RSA-OAEP', hash: 'SHA-256' }, false, ['encrypt']
    );
    const encrypted = await crypto.subtle.encrypt(
      { name: 'RSA-OAEP' }, publicKey, new TextEncoder().encode(apiKey)
    );
    return btoa(String.fromCharCode(...new Uint8Array(encrypted)));
  },

  async deriveKey(userId, salt) {
    const enc = new TextEncoder();
    const keyMaterial = await crypto.subtle.importKey(
      'raw', enc.encode(userId + ':' + salt),
      { name: 'PBKDF2' }, false, ['deriveKey']
    );
    return crypto.subtle.deriveKey(
      { name: 'PBKDF2', salt: enc.encode(salt), iterations: 100000, hash: 'SHA-256' },
      keyMaterial, { name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt']
    );
  },

  async encryptLocal(data, userId) {
    const salt = crypto.randomUUID();
    const key = await this.deriveKey(userId, salt);
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const encoded = new TextEncoder().encode(JSON.stringify(data));
    const encrypted = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, encoded);
    return {
      salt,
      iv: btoa(String.fromCharCode(...iv)),
      data: btoa(String.fromCharCode(...new Uint8Array(encrypted)))
    };
  },

  async decryptLocal(packet, userId) {
    const key = await this.deriveKey(userId, packet.salt);
    const iv = Uint8Array.from(atob(packet.iv), c => c.charCodeAt(0));
    const data = Uint8Array.from(atob(packet.data), c => c.charCodeAt(0));
    const decrypted = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, data);
    return JSON.parse(new TextDecoder().decode(decrypted));
  }
};
```

- [ ] **Step 2: Create `web/js/storage.js`**

```javascript
// AI conversation + API key storage
const AIStorage = {
  DB_NAME: 'owl_ai_chat',
  DB_VERSION: 1,

  async openDb() {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(this.DB_NAME, this.DB_VERSION);
      req.onupgradeneeded = (e) => {
        const db = e.target.result;
        if (!db.objectStoreNames.contains('conversations')) {
          const store = db.createObjectStore('conversations', { keyPath: 'id' });
          store.createIndex('created_at', 'createdAt', { unique: false });
        }
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
  },

  async saveConversation(conv) {
    const db = await this.openDb();
    return new Promise((resolve, reject) => {
      const tx = db.transaction('conversations', 'readwrite');
      tx.objectStore('conversations').put(conv);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
  },

  async getConversations(limit = 50, offset = 0) {
    const db = await this.openDb();
    return new Promise((resolve, reject) => {
      const tx = db.transaction('conversations', 'readonly');
      const store = tx.objectStore('conversations');
      const index = store.index('created_at');
      const req = index.openCursor(null, 'prev');
      const results = [];
      let skipped = 0;
      req.onsuccess = () => {
        const cursor = req.result;
        if (!cursor) { resolve(results); return; }
        if (skipped < offset) { skipped++; cursor.continue(); return; }
        if (results.length < limit) {
          results.push(cursor.value);
          cursor.continue();
        } else {
          resolve(results);
        }
      };
      req.onerror = () => reject(req.error);
    });
  },

  async deleteConversation(id) {
    const db = await this.openDb();
    return new Promise((resolve, reject) => {
      const tx = db.transaction('conversations', 'readwrite');
      tx.objectStore('conversations').delete(id);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
  },

  // API Key in localStorage (encrypted)
  async saveApiKey(userId, apiKey, provider, model) {
    const packet = await CryptoWallet.encryptLocal({ apiKey, provider, model }, userId);
    localStorage.setItem('owl_ai_key', JSON.stringify(packet));
  },

  async loadApiKey(userId) {
    const raw = localStorage.getItem('owl_ai_key');
    if (!raw) return null;
    try {
      const packet = JSON.parse(raw);
      return await CryptoWallet.decryptLocal(packet, userId);
    } catch {
      localStorage.removeItem('owl_ai_key');
      return null;
    }
  }
};
```

- [ ] **Step 3: Create `web/js/audit.js`**

```javascript
// AI audit reporter (backup path — primary audit is server-side)
const AIAudit = {
  async report(record) {
    try {
      const res = await API.aiAudit(record);
      return res.ok;
    } catch {
      return false;  // silent fail
    }
  }
};
```

- [ ] **Step 4: Commit**

```bash
git add cmd/plugins/serve/web/js/crypto.js cmd/plugins/serve/web/js/storage.js cmd/plugins/serve/web/js/audit.js
git commit -m "feat(ai): add frontend crypto wallet, storage, and audit modules"
```

---

### Task 5: Frontend — API wrapper and settings page

**Files:**
- Modify: `cmd/plugins/serve/web/js/api.js` (add ai methods)
- Modify: `cmd/plugins/serve/web/js/app.js` (add AI route/panel)

**E2E Test Specification:**
```
1. Panel sidebar: "AI 助手" icon appears and clicks open the AI panel
2. API.aiChat() sends POST with correct body
3. Panel content area for 'ai' loads ai.js
```

- [ ] **Step 1: Add AI API methods to `api.js`**

```javascript
// In the API object (find existing and add these methods):
  async getSessionKey() {
    return this._get('/api/v1/ai/session-key');
  },

  async aiChat(message, sessionId, encryptedApiKey) {
    return this._post('/api/v1/ai/chat', {
      message,
      session_id: sessionId,
      encrypted_api_key: encryptedApiKey
    });
  },

  async getAiContext() {
    return this._get('/api/v1/ai/context');
  },

  async aiAudit(record) {
    return this._post('/api/v1/ai/audit', record);
  }
```

- [ ] **Step 2: Add AI panel to `app.js`**

Add a new panel definition (following existing pattern like "files" panel):
```javascript
// In PANELS object:
  {
    id: 'ai',
    icon: '🤖',
    title: 'AI 助手',
    file: 'ai.js'
  }
```

Add `updatePanelContent` entry for 'ai' if whitelist exists.

- [ ] **Step 3: Commit**

```bash
git add cmd/plugins/serve/web/js/api.js cmd/plugins/serve/web/js/app.js
git commit -m "feat(ai): add AI API methods and panel route"
```

---

### Task 6: Frontend — AI chat page

**Files:**
- Create: `cmd/plugins/serve/web/js/pages/ai.js`

**E2E Test Specification:**
```
1. AI panel opens → messages container + input + send button render
2. Type message + send → user bubble appears, typing indicator shows
3. No API Key configured → "请先在设置页面配置" message shown
4. Reply received → assistant bubble renders with text
5. On page refresh → last conversation restored from IndexedDB
6. Debug mode (if --ai-debug) → yellow banner displayed at top
7. CSS: messages scroll, user/assistant styles correct, input at bottom
```

- [ ] **Step 1: Create `web/js/pages/ai.js`**

Full AI chat page with:
- Message rendering (user bubbles + assistant bubbles)
- Input box + send button
- Loading state with typing indicator
- Session key fetch on page load
- API Key check from localStorage → redirect to settings if missing
- Conversation persistence to IndexedDB
- Streaming-like response from `/ai/chat`
- Debug mode banner (detected from response headers or a separate endpoint)

```javascript
const AiPage = {
  sessionId: null,
  messages: [],
  initialized: false,

  async init() {
    this.container = document.getElementById('panel-content');
    this.render();
    await this.loadSessionKey();
    this.initialized = true;
  },

  render() {
    this.container.innerHTML = `
      <div id="ai-chat" class="ai-container">
        <div id="ai-messages" class="ai-messages"></div>
        <div id="ai-debug-banner" class="ai-debug-banner hidden">🔧 调试模式已开启，所有对话将被记录</div>
        <div class="ai-input-row">
          <input type="text" id="ai-input" placeholder="输入运维指令..." />
          <button id="ai-send" class="btn btn-primary">发送</button>
        </div>
      </div>
    `;

    document.getElementById('ai-send').onclick = () => this.send();
    document.getElementById('ai-input').onkeydown = (e) => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); this.send(); }
    };

    // Restore previous conversation
    this.restoreConversation();
  },

  async loadSessionKey() {
    try {
      const data = await API.getSessionKey();
      this.sessionId = data.session_id;
      // Check debug mode
      if (data.debug_mode) {
        document.getElementById('ai-debug-banner').classList.remove('hidden');
      }
    } catch (e) {
      console.error('Failed to load session key', e);
    }
  },

  async send() {
    const input = document.getElementById('ai-input');
    const text = input.value.trim();
    if (!text) return;
    input.value = '';

    this.addMessage('user', text);
    this.showTyping();

    // Load API Key
    const userId = await API.getSessionUserId();  // implement in api.js
    const keyData = await AIStorage.loadApiKey(userId);
    if (!keyData || !keyData.apiKey) {
      this.addMessage('assistant', '⚠️ 请先在设置页面配置 API Key');
      this.hideTyping();
      return;
    }

    const encryptedKey = await CryptoWallet.encryptApiKey(
      window.aiPublicKeySpki, keyData.apiKey
    );

    try {
      const res = await API.aiChat(text, this.sessionId, encryptedKey);
      this.addMessage('assistant', res.reply);
      // Persist
      await AIStorage.saveConversation({
        id: Date.now().toString(),
        sessionId: this.sessionId,
        messages: this.messages,
        createdAt: new Date().toISOString()
      });
    } catch (e) {
      this.addMessage('assistant', '❌ 请求失败: ' + e.message);
    } finally {
      this.hideTyping();
    }
  },

  addMessage(role, content) {
    this.messages.push({ role, content });
    const container = document.getElementById('ai-messages');
    const div = document.createElement('div');
    div.className = `ai-message ai-${role}`;
    div.textContent = content;
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
  },

  showTyping() {
    const container = document.getElementById('ai-messages');
    const div = document.createElement('div');
    div.className = 'ai-message ai-assistant ai-typing';
    div.id = 'ai-typing';
    div.textContent = '...';
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
  },

  hideTyping() {
    const el = document.getElementById('ai-typing');
    if (el) el.remove();
  },

  async restoreConversation() {
    // Load last conversation from IndexedDB
    const convs = await AIStorage.getConversations(1);
    if (convs.length > 0) {
      this.messages = convs[0].messages;
      const container = document.getElementById('ai-messages');
      container.innerHTML = '';
      for (const msg of this.messages) {
        const div = document.createElement('div');
        div.className = `ai-message ai-${msg.role}`;
        div.textContent = msg.content;
        container.appendChild(div);
      }
      container.scrollTop = container.scrollHeight;
    }
  }
};

// Auto-init when panel is shown
document.addEventListener('panel-change', (e) => {
  if (e.detail.panel === 'ai') {
    AiPage.init();
  }
});
```

- [ ] **Step 2: Add CSS styles to `web/css/app.css`**

```css
.ai-container {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.ai-messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ai-message {
  max-width: 80%;
  padding: 10px 14px;
  border-radius: 8px;
  line-height: 1.5;
  white-space: pre-wrap;
}

.ai-user {
  align-self: flex-end;
  background: var(--accent);
  color: white;
}

.ai-assistant {
  align-self: flex-start;
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.ai-typing {
  opacity: 0.6;
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 0.6; }
  50% { opacity: 1; }
}

.ai-input-row {
  display: flex;
  gap: 8px;
  padding: 12px;
  border-top: 1px solid var(--border);
}

.ai-input-row input {
  flex: 1;
}

.ai-debug-banner {
  background: #fff3cd;
  color: #856404;
  padding: 8px 16px;
  text-align: center;
  font-size: 13px;
}

.ai-debug-banner.hidden {
  display: none;
}
```

- [ ] **Step 3: Commit**

```bash
git add cmd/plugins/serve/web/js/pages/ai.js cmd/plugins/serve/web/css/app.css
git commit -m "feat(ai): add AI chat page with crypto, storage, and UI"
```

---

### Task 7: Frontend — Settings page AI config

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/settings.js` (or wherever the settings panel is defined)

- [ ] **Step 1: Add AI configuration section to settings page**

Find the settings page file (likely `web/js/pages/settings.js`) and add an AI config block:

```javascript
// After existing settings sections (like user management, etc.):
function renderAiSettings() {
  const userId = getCurrentUserId();  // from app state
  AIStorage.loadApiKey(userId).then(keyData => {
    const section = document.getElementById('ai-settings-section');
    if (!section) return;
    section.innerHTML = `
      <h3>AI 助手配置</h3>
      <div class="settings-field">
        <label>API Key</label>
        <input type="password" id="ai-api-key" value="${keyData?.apiKey || ''}" placeholder="sk-..." />
      </div>
      <div class="settings-field">
        <label>Provider</label>
        <select id="ai-provider">
          <option value="openai" ${keyData?.provider === 'openai' ? 'selected' : ''}>OpenAI</option>
          <option value="anthropic" ${keyData?.provider === 'anthropic' ? 'selected' : ''}>Anthropic</option>
          <option value="deepseek" ${keyData?.provider === 'deepseek' ? 'selected' : ''}>DeepSeek</option>
          <option value="custom" ${keyData?.provider === 'custom' ? 'selected' : ''}>Custom</option>
        </select>
      </div>
      <div class="settings-field">
        <label>Model</label>
        <input type="text" id="ai-model" value="${keyData?.model || ''}" placeholder="gpt-4o" />
      </div>
      <button onclick="saveAiSettings()" class="btn btn-primary">保存</button>
    `;
  });
}

async function saveAiSettings() {
  const apiKey = document.getElementById('ai-api-key').value;
  const provider = document.getElementById('ai-provider').value;
  const model = document.getElementById('ai-model').value;
  const userId = getCurrentUserId();
  await AIStorage.saveApiKey(userId, apiKey, provider, model);
  showToast('AI 配置已保存');
}
```

**E2E Test Specification:**
```
1. Settings page shows "AI 助手配置" section heading
2. API Key field is type="password" (hidden input)
3. Provider dropdown has OpenAI / Anthropic / DeepSeek / Custom options
4. Save button → reload page → values persist
5. Empty API Key → AI chat page shows guidance message
```

- [ ] **Step 2: Add AI settings HTML container in settings page**

Find where the settings page's main HTML is rendered and add:
```html
<div id="ai-settings-section" class="settings-section">
  <!-- rendered by renderAiSettings() -->
</div>
```

Call `renderAiSettings()` when the settings panel loads.

- [ ] **Step 3: Commit**

```bash
git add cmd/plugins/serve/web/js/pages/settings.js
git commit -m "feat(ai): add AI configuration section to settings page"
```
