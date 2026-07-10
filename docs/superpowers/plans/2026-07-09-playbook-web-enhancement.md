# Playbook Web Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) for syntax checking.

**Goal:** Enhance the web playbook page with category sidebar, search, playbook detail view, enhanced run modal, add global `~/.owl/playbooks` path, and create test playbooks.

**Architecture:** Playbook uniqueness via hash of absolute path (sha256[:12]) — this replaces `name` as primary key. In-memory cache (`map[string]*Playbook` by hash) + SQLite for persistence. CLI unchanged (operates on filesystem directly). Categories derived from directory structure (subdirectory name), computed server-side during SyncFromDir. Frontend uses existing shell panel pattern (like nodes.js) for category sidebar.

**Tech Stack:** Go (Gin), SQLite, vanilla JS SPA, YAML

## Global Constraints

- No new Go dependencies beyond stdlib + yaml.v3 + gin + crypto/sha256
- No new npm/node dependencies — all frontend is vanilla JS modules
- Category derived from directory structure, not YAML metadata
- All frontend changes in `cmd/plugins/serve/web/js/pages/playbooks.js`
- Playbook ID = `sha256(absolute_path)[:12]` (hex)
- Memory cache: `map[string]*Playbook` keyed by ID, rebuilt on refresh
- CLI commands (`owl playbook list/validate/run`) are **unchanged** — they directly scan filesystem, don't depend on DB
- `owl playbook list` falls back to `~/.owl/playbooks` when `--library` not specified

---

### Task 1: Global `~/.owl/playbooks` default path

**Files:**
- Create: `cmd/cli/cmd/playbook/resolve.go`
- Modify: `cmd/cli/cmd/playbook/list.go:35-41`
- Modify: `internal/ai/tools.go:1496`
- Modify: `cmd/plugins/serve/server.go` (init section)

**Interfaces:**
- Consumes: `os.UserHomeDir()` for platform-independent home dir
- Produces: `resolvePlaybookDir()` helper (CLI), auto-creation on server start

- [ ] **Step 1: Create CLI path resolution helper**

```go
// cmd/cli/cmd/playbook/resolve.go
package playbook

import (
	"os"
	"path/filepath"
)

func defaultPlaybookDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./playbooks"
	}
	p := filepath.Join(home, ".owl", "playbooks")
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		return p
	}
	if fi, err := os.Stat("./playbooks"); err == nil && fi.IsDir() {
		abs, _ := filepath.Abs("./playbooks")
		return abs
	}
	return p
}
```

- [ ] **Step 2: Modify CLI list default**

In `cmd/cli/cmd/playbook/list.go`, change the flag default:

```go
listCmd.Flags().StringVar(&playbookListLibrary, "library", "", "剧本库目录 (default: ~/.owl/playbooks)")
```

In the Run function, at the top of `runPlaybookList`:

```go
if library == "" {
	library = defaultPlaybookDir()
}
```

- [ ] **Step 3: Modify AI tool default path**

In `internal/ai/tools.go:1495-1499`, replace:

```go
library := "./playbooks"
```

With:

```go
library := defaultPlaybookDir()
```

Add local helper function in `tools.go`:

```go
func defaultPlaybookDir() string {
    home, err := os.UserHomeDir()
    if err != nil { return "./playbooks" }
    p := filepath.Join(home, ".owl", "playbooks")
    if fi, err := os.Stat(p); err == nil && fi.IsDir() { return p }
    if fi, err := os.Stat("./playbooks"); err == nil && fi.IsDir() {
        abs, _ := filepath.Abs("./playbooks")
        return abs
    }
    return p
}
```

- [ ] **Step 4: Auto-create global dir on server startup + copy/unzip sample**

In `cmd/plugins/serve/server.go`, find the init function or a suitable place and add:

```go
home, err := os.UserHomeDir()
if err == nil {
    globalDir := filepath.Join(home, ".owl", "playbooks")
    if err := os.MkdirAll(globalDir, 0755); err == nil {
        entries, _ := os.ReadDir(globalDir)
        if len(entries) == 0 {
            copySamplePlaybooks(globalDir)
        }
    }
}
```

Add small helper:

```go
func copySamplePlaybooks(dir string) {
    samples := map[string]string{
        "example/ping-test.yaml": `name: ping_test
description: Test node connectivity via ping
hosts: []
tasks:
  - name: Ping localhost
    command: ping -c 1 127.0.0.1
`,
    }
    for relPath, content := range samples {
        p := filepath.Join(dir, relPath)
        os.MkdirAll(filepath.Dir(p), 0755)
        os.WriteFile(p, []byte(content), 0644)
    }
}
```

- [ ] **Step 5: Verify the changes build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add cmd/cli/cmd/playbook/resolve.go cmd/cli/cmd/playbook/list.go internal/ai/tools.go cmd/plugins/serve/server.go
git commit -m "feat: add global ~/.owl/playbooks path as default for CLI, AI tool, and web server"
```

---

### Task 2: Rewrite playbook model — hash-based ID, category, memory cache

**Files:**
- Modify: `cmd/plugins/serve/model/playbook.go:5-13`
- Modify: `cmd/plugins/serve/store/playbook.go` (full rewrite)
- Modify: `cmd/plugins/serve/handler/playbook.go` (update main.go to pass cache, store references)
- New: `cmd/plugins/serve/cache/playbook.go`

**DB Schema (new):**

```sql
CREATE TABLE IF NOT EXISTS playbooks (
    id           TEXT PRIMARY KEY,         -- sha256(absolute_path)[:12]
    name         TEXT NOT NULL,             -- original filename (may duplicate)
    description  TEXT DEFAULT '',
    category     TEXT DEFAULT '',
    file_path    TEXT NOT NULL,             -- absolute path
    tasks_count  INTEGER DEFAULT 0,
    task_names   TEXT DEFAULT '[]',
    file_exists  INTEGER DEFAULT 1,
    updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS playbook_runs (
    id               TEXT PRIMARY KEY,
    playbook_id      TEXT NOT NULL,
    playbook_name    TEXT NOT NULL,
    status           TEXT DEFAULT 'queued',
    target_nodes     TEXT DEFAULT '[]',
    extra_vars       TEXT DEFAULT '{}',
    tags             TEXT DEFAULT '',
    error            TEXT DEFAULT '',
    results          TEXT DEFAULT '[]',
    created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at        TIMESTAMP,
    completed_at      TIMESTAMP
);
```

**Interfaces:**

```go
// cache/playbook.go
type PlaybookStore interface {
    List(ctx context.Context) ([]*model.Playbook, error)
    Get(ctx context.Context, id string) (*model.Playbook, error)
    ListByCategory(ctx context.Context, category string) ([]*model.Playbook, error)
    Upsert(ctx context.Context, pb *model.Playbook) error
    Delete(ctx context.Context, id string) error
    SyncFromDir(ctx context.Context, dir string) ([]*model.Playbook, []string, error)
    GetCategoryCounts(ctx context.Context) (map[string]int, error)
}
```

- [ ] **Step 1: Rewrite model**

```go
// cmd/plugins/serve/model/playbook.go
package model

type Playbook struct {
    ID          string   `json:"id"`                      // hash[:12]
    Name        string   `json:"name"`
    Description string   `json:"description,omitempty"`
    Category    string   `json:"category,omitempty"`
    FilePath    string   `json:"file_path"`
    TasksCount  int      `json:"tasks_count"`
    TaskNames   []string `json:"task_names,omitempty"`
    FileExists  bool     `json:"file_exists"`
    UpdatedAt   string   `json:"updated_at,omitempty"`
}
```

- [ ] **Step 2: Create hash helper**

```go
// cmd/plugins/serve/store/playbook.go (add)
import "crypto/sha256"
import "fmt"

func playbookID(absPath string) string {
    h := sha256.Sum256([]byte(absPath))
    return fmt.Sprintf("%x", h[:6]) // 12 hex chars
}
```

- [ ] **Step 3: Rewrite PlaybookStore with new schema**

Full rewrite of `cmd/plugins/serve/store/playbook.go` (~220 lines):

```go
package store

import (
    "context"
    "database/sql"
    "encoding/json"
    "crypto/sha256"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/cangyunye/go-owl/cmd/plugins/serve/model"
    "gopkg.in/yaml.v3"
)

type PlaybookStore struct {
    db     *sql.DB
    cache  map[string]*model.Playbook // key: id (hash)
}

func NewPlaybookStore(db *sql.DB) *PlaybookStore {
    return &PlaybookStore{db: db, cache: make(map[string]*model.Playbook)}
}

func playbookID(absPath string) string {
    h := sha256.Sum256([]byte(absPath))
    return fmt.Sprintf("%x", h[:6])
}

func (s *PlaybookStore) Init(ctx context.Context) error {
    _, err := s.db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS playbooks (
            id          TEXT PRIMARY KEY,
            name        TEXT NOT NULL,
            description TEXT DEFAULT '',
            category    TEXT DEFAULT '',
            file_path   TEXT NOT NULL,
            tasks_count INTEGER DEFAULT 0,
            task_names  TEXT DEFAULT '[]',
            file_exists INTEGER DEFAULT 1,
            updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    `)
    return err
}

func (s *PlaybookStore) scanRow(scanner interface {
    Scan(dest ...interface{}) error
}) (*model.Playbook, error) {
    var id, name, desc, cat, fp string
    var tc int
    var tnJSON string
    var fe int
    var updated string
    if err := scanner.Scan(&id, &name, &desc, &cat, &fp, &tc, &tnJSON, &fe, &updated); err != nil {
        return nil, err
    }
    pb := &model.Playbook{
        ID: id, Name: name, Description: desc, Category: cat,
        FilePath: fp, TasksCount: tc, FileExists: fe == 1, UpdatedAt: updated,
    }
    json.Unmarshal([]byte(tnJSON), &pb.TaskNames)
    if pb.TaskNames == nil { pb.TaskNames = []string{} }
    return pb, nil
}

func (s *PlaybookStore) buildCache(ctx context.Context) error {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, name, description, category, file_path, tasks_count, task_names, file_exists, updated_at FROM playbooks`)
    if err != nil { return err }
    defer rows.Close()
    s.cache = make(map[string]*model.Playbook)
    for rows.Next() {
        pb, err := s.scanRow(rows)
        if err != nil { continue }
        s.cache[pb.ID] = pb
    }
    return nil
}

func (s *PlaybookStore) List(ctx context.Context) ([]*model.Playbook, error) {
    if len(s.cache) == 0 {
        if err := s.buildCache(ctx); err != nil { return nil, err }
    }
    result := make([]*model.Playbook, 0, len(s.cache))
    for _, pb := range s.cache { result = append(result, pb) }
    return result, nil
}

func (s *PlaybookStore) Get(ctx context.Context, id string) (*model.Playbook, error) {
    if len(s.cache) == 0 { s.buildCache(ctx) }
    if pb, ok := s.cache[id]; ok { return pb, nil }
    return nil, sql.ErrNoRows
}

func (s *PlaybookStore) Upsert(ctx context.Context, pb *model.Playbook) error {
    tnJSON, _ := json.Marshal(pb.TaskNames)
    fe := 0
    if pb.FileExists { fe = 1 }
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO playbooks (id, name, description, category, file_path, tasks_count, task_names, file_exists, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET
            name = excluded.name,
            description = excluded.description,
            category = excluded.category,
            file_path = excluded.file_path,
            tasks_count = excluded.tasks_count,
            task_names = excluded.task_names,
            file_exists = excluded.file_exists,
            updated_at = excluded.updated_at`,
        pb.ID, pb.Name, pb.Description, pb.Category, pb.FilePath,
        pb.TasksCount, string(tnJSON), fe, time.Now().UTC())
    if err == nil {
        s.cache[pb.ID] = pb
    }
    return err
}

func (s *PlaybookStore) Delete(ctx context.Context, id string) error {
    _, err := s.db.ExecContext(ctx, `DELETE FROM playbooks WHERE id = ?`, id)
    delete(s.cache, id)
    return err
}

type playbookYAMLMeta struct {
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
    Tasks       []any  `yaml:"tasks"`
}

func readPlaybookMeta(path string) *model.Playbook {
    data, err := os.ReadFile(path)
    if err != nil { return nil }
    var meta playbookYAMLMeta
    if err := yaml.Unmarshal(data, &meta); err != nil { return nil }
    name := meta.Name
    if name == "" {
        name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
    }
    var taskNames []string
    if len(meta.Tasks) > 0 {
        for _, t := range meta.Tasks {
            if m, ok := t.(map[string]interface{}); ok {
                for k := range m {
                    taskNames = append(taskNames, k)
                    break
                }
            }
        }
    }
    return &model.Playbook{
        ID:         playbookID(path),
        Name:       name,
        Description: meta.Description,
        FilePath:   path,
        TasksCount: len(meta.Tasks),
        TaskNames:  taskNames,
        FileExists: true,
    }
}

func (s *PlaybookStore) SyncFromDir(ctx context.Context, dir string) ([]*model.Playbook, []string, error) {
    diskMap := make(map[string]*model.Playbook)
    var errors []string

    info, err := os.Stat(dir)
    if err != nil || !info.IsDir() {
        return nil, nil, err
    }

    filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
        if err != nil || fi.IsDir() { return nil }
        if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") { return nil }
        pb := readPlaybookMeta(path)
        if pb == nil { return nil }

        // Compute category from directory structure
        rel, _ := filepath.Rel(dir, filepath.Dir(path))
        if rel != "." {
            parts := strings.SplitN(rel, string(filepath.Separator), 2)
            pb.Category = parts[0]
        }

        // Check hash collision
        if existing, ok := diskMap[pb.ID]; ok {
            errors = append(errors, fmt.Sprintf(
                "hash collision: %q and %q have the same ID %q — rename one of them",
                existing.FilePath, pb.FilePath, pb.ID))
            return nil
        }
        diskMap[pb.ID] = pb
        return nil
    })

    var results []*model.Playbook
    for _, pb := range diskMap {
        if err := s.Upsert(ctx, pb); err == nil {
            results = append(results, pb)
        }
    }

    // Mark missing
    existing, _ := s.List(ctx)
    for _, pb := range existing {
        if _, stillExists := diskMap[pb.ID]; !stillExists {
            pb.FileExists = false
            s.Upsert(ctx, pb)
        }
    }

    return results, errors, nil
}
```

- [ ] **Step 4: Modify handler to pass store, update GetSettingsPath**

In `cmd/plugins/serve/handler/playbook.go`, update `GetSettingsPath`:

```go
func (h *PlaybookHandler) GetSettingsPath(c *gin.Context) {
    var path string
    h.db.QueryRow(`SELECT value FROM settings WHERE key = 'playbook_library_path'`).Scan(&path)
    if path == "" {
        home, err := os.UserHomeDir()
        if err == nil {
            path = filepath.Join(home, ".owl", "playbooks")
        }
    }
    c.JSON(http.StatusOK, gin.H{"value": path})
}
```

Add import: `"path/filepath"` at top if missing.

Update `Refresh` to return errors:

```go
func (h *PlaybookHandler) Refresh(c *gin.Context) {
    var req refreshRequest
    if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
        c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "path is required"})
        return
    }

    upsertSetting(h.db, "playbook_library_path", req.Path)

    playbooks, syncErrors, err := h.playbooks.SyncFromDir(c.Request.Context(), req.Path)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
        return
    }

    all, _ := h.playbooks.List(c.Request.Context())
    c.JSON(http.StatusOK, gin.H{
        "data":      all,
        "refreshed": len(playbooks),
        "total":    len(all),
        "errors":   syncErrors,
    })
}
```

Make sure `List` handler still works with new store interface:

```go
func (h *PlaybookHandler) List(c *gin.Context) {
    playbooks, err := h.playbooks.List(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"data": playbooks})
}
```

Note: No change needed to `RunList`, `RunGet`, `RunCancel` — they still work.

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add cmd/plugins/serve/model/playbook.go cmd/plugins/serve/store/playbook.go cmd/plugins/serve/handler/playbook.go
git commit -m "feat: hash-based playbook ID, category field, memory cache, hash collision detection"
```

---

### Task 3: Frontend — category sidebar + search

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/playbooks.js` (full rewrite, ~450 lines)
- Add to `cmd/plugins/serve/web/js/api.js`: `playbookGet(id)` method

**Interfaces:**
- Consumes: `api.playbooks()`, `api.playbookSettingsPath()`, `api.refreshPlaybooks()`, `shell.setPanelContent()`
- Produces: Category sidebar (left panel), search box, filtered table

- [ ] **Step 1: Add API method**

In `cmd/plugins/serve/web/js/api.js`, add after `refreshPlaybooks`:

```javascript
  playbookGet: (id) =>
    request('GET', `/playbooks/${encodeURIComponent(id)}`),
```

- [ ] **Step 2: Define state + helper functions**

```javascript
export function renderPlaybooks(render, navigate, user, api) {
  let state = {
    playbooks: [],
    filteredPlaybooks: [],
    query: '',
    selectedCategory: '',
    categories: [],
  };

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
```

- [ ] **Step 3: Implement loadAll, extractCategories, applyFilters**

```javascript
  async function loadAll() {
    await loadSettingsPath();
    try {
      const res = await api.playbooks();
      state.playbooks = res.data || [];
    } catch { state.playbooks = []; }
    extractCategories();
    applyFilters();
  }

  function extractCategories() {
    const cats = new Set();
    cats.add(''); // "All"
    for (const pb of state.playbooks) {
      if (pb.category) cats.add(pb.category);
    }
    state.categories = Array.from(cats).sort();
  }

  function applyFilters() {
    state.filteredPlaybooks = state.playbooks.filter(pb => {
      if (state.selectedCategory && pb.category !== state.selectedCategory) return false;
      if (state.query) {
        const q = state.query.toLowerCase();
        const name = (pb.name || '').toLowerCase();
        const desc = (pb.description || '').toLowerCase();
        if (!name.includes(q) && !desc.includes(q)) return false;
      }
      return true;
    });
    renderTable();
    renderPanel();
  }
```

- [ ] **Step 4: Implement renderPanel**

```javascript
  function renderPanel() {
    const counts = {};
    for (const pb of state.playbooks) {
      const c = pb.category || '';
      counts[c] = (counts[c] || 0) + 1;
    }
    const html = [
      `<li class="panel-item ${state.selectedCategory === '' ? 'active' : ''}" data-category="" role="option" aria-selected="${state.selectedCategory === ''}">
        <span class="dot" style="background:var(--accent)"></span>
        <span>全部</span>
        <span class="count">${state.playbooks.length}</span>
      </li>`,
      ...state.categories.filter(c => c !== '').map(c => `
        <li class="panel-item ${state.selectedCategory === c ? 'active' : ''}" data-category="${esc(c)}" role="option" aria-selected="${state.selectedCategory === c}">
          <span class="dot" style="background:var(--muted)"></span>
          <span>${esc(c)}</span>
          <span class="count">${counts[c] || 0}</span>
        </li>`)
    ].join('');

    shell.setPanelContent(`<ul style="list-style:none;padding:0">${html}</ul>`);

    document.querySelectorAll('.panel-item[data-category]').forEach(el => {
      el.addEventListener('click', () => {
        state.selectedCategory = el.dataset.category;
        applyFilters();
      });
    });
  }
```

- [ ] **Step 5: Implement renderTable with id-based row data**

```javascript
  function renderTable() {
    const list = document.getElementById('playbook-list');
    if (state.filteredPlaybooks.length === 0) {
      list.innerHTML = '<tr><td colspan="5" class="empty-state">无匹配剧本</td></tr>';
    } else {
      list.innerHTML = state.filteredPlaybooks.map(pb =>
        `<tr class="playbook-row" data-id="${esc(pb.id)}" style="cursor:pointer">
          <td>${esc(pb.name)}${pb.file_exists === false ? ' <span class="missing-badge">missing</span>' : ''}</td>
          <td>${pb.category ? `<span class="tag">${esc(pb.category)}</span>` : '<span style="color:var(--muted);font-size:12px">-</span>'}</td>
          <td>${esc(pb.description || '')}</td>
          <td>${esc((pb.task_names || []).join(', '))}</td>
          <td class="action-cell">
            <button class="run-playbook-btn" data-id="${esc(pb.id)}" ${pb.file_exists === false ? 'disabled' : ''} style="background:none;border:1px solid var(--primary);color:var(--primary);padding:2px 10px;border-radius:var(--radius);cursor:pointer;font-size:12px">Run</button>
          </td>
        </tr>`
      ).join('');
    }
    document.querySelectorAll('.run-playbook-btn').forEach(btn => {
      btn.addEventListener('click', (e) => { e.stopPropagation(); showRunModal(btn.dataset.id); });
    });
    document.querySelectorAll('.playbook-row').forEach(row => {
      row.addEventListener('click', () => showPlaybookDetail(row.dataset.id, row.dataset.id));
    });
  }
```

- [ ] **Step 6: Full render template**

```javascript
  render(`
    <div class="card" style="margin-bottom:0">
      <div class="path-bar">
        <label for="playbook-path">Library Path</label>
        <input id="playbook-path" placeholder="/path/to/playbooks">
        <button class="btn btn-secondary btn-sm" id="refresh-playbooks-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-refresh"/></svg> 刷新</button>
      </div>
    </div>

    <div class="filter-bar">
      <div class="input" style="position:relative;padding-left:32px;width:240px">
        <svg width="14" height="14" aria-hidden="true" style="position:absolute;left:10px;top:50%;transform:translateY(-50%);color:var(--muted)"><use href="#icon-search"/></svg>
        <input type="text" id="playbook-search" placeholder="搜索剧本名称 / 描述…" style="border:none;background:transparent;outline:none;color:var(--fg);width:100%;font:13px/1.5 var(--font-body)">
      </div>
      <div class="spacer"></div>
      <button class="btn btn-primary btn-sm" id="add-playbook-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 新建</button>
    </div>

    <div class="card" style="overflow:auto">
      <table class="data-table">
        <thead><tr><th>名称</th><th>分类</th><th>描述</th><th>任务</th><th></th></tr></thead>
        <tbody id="playbook-list"><tr><td colspan="5" class="loading">加载中…</td></tr></tbody>
      </table>
    </div>

    <div class="card">
      <div class="card-header"><h3>运行历史</h3></div>
      <div class="card-body" style="padding:0">
        <table class="data-table">
          <thead><tr><th>剧本</th><th>目标节点</th><th>状态</th><th>开始时间</th><th></th></tr></thead>
          <tbody id="playbook-runs-list"><tr><td colspan="5" class="loading">加载中…</td></tr></tbody>
        </table>
      </div>
    </div>

    <div class="card" id="run-detail-card">
      <div class="card-header"><h3>运行详情</h3></div>
      <div class="card-body" id="run-detail" data-run-id=""><p class="empty-state">选择一个运行记录查看详情</p></div>
    </div>

    <div class="modal-overlay" id="run-playbook-modal">
      <div class="modal modal-sm">
        <h3>Run Playbook: <span id="run-playbook-name-display"></span></h3>
        <div class="modal-form">
          <input type="hidden" id="run-playbook-id">
          <div class="form-row"><label>Target Nodes</label><input id="run-playbook-target" placeholder="node1,node2 (comma-separated IDs)"></div>
          <div class="form-row"><label>Groups (alternative)</label><input id="run-playbook-groups" placeholder="web, db (selects all nodes in these groups)"></div>
          <div class="form-row"><label>Tags (optional)</label><input id="run-playbook-tags" placeholder="tag1,tag2"></div>
          <div class="form-row"><label>Extra Vars (optional)</label><input id="run-playbook-vars" placeholder="key=value, version=2.0"></div>
        </div>
        <p class="error-msg" id="run-playbook-error"></p>
        <div class="modal-actions">
          <button class="btn-cancel" id="run-playbook-cancel">Cancel</button>
          <button class="btn-primary" id="run-playbook-submit">Execute</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="playbook-detail-modal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3 id="detail-name"></h3>
          <button class="btn btn-ghost btn-sm" id="detail-close-btn" style="margin-left:auto;background:none;border:none;color:var(--muted);cursor:pointer;font-size:18px">&times;</button>
        </div>
        <div class="modal-body" id="detail-body" style="max-height:70vh;overflow-y:auto">
          <div id="detail-meta" style="display:flex;gap:16px;flex-wrap:wrap;font-size:13px;margin-bottom:12px;padding-bottom:12px;border-bottom:1px solid var(--border)"></div>
          <div id="detail-tasks" style="margin-bottom:12px"></div>
          <div style="margin-top:12px">
            <h4 style="font-size:13px;color:var(--muted);margin-bottom:8px">YAML 源文件</h4>
            <pre id="detail-yaml" style="background:var(--code-bg);border:1px solid var(--border);border-radius:var(--radius);padding:12px;font:12px/1.6 var(--font-mono);overflow-x:auto;white-space:pre;max-height:300px"></pre>
          </div>
        </div>
        <div class="modal-actions">
          <button class="btn-cancel" id="detail-cancel-btn">关闭</button>
          <button class="btn-primary" id="detail-run-btn">运行</button>
        </div>
      </div>
    </div>
  `, () => {
    setupWebSocket();

    let searchDebounceTimer;
    document.getElementById('playbook-search').addEventListener('input', (e) => {
      clearTimeout(searchDebounceTimer);
      searchDebounceTimer = setTimeout(() => {
        state.query = e.target.value.trim();
        applyFilters();
      }, 100);
    });

    // Download Settings Path
    document.getElementById('refresh-playbooks-btn').addEventListener('click', async () => {
      const path = document.getElementById('playbook-path').value.trim();
      if (!path) { alert('Library path is required'); return; }
      document.getElementById('refresh-playbooks-btn').textContent = 'Refreshing...';
      document.getElementById('refresh-playbooks-btn').disabled = true;
      try {
        const res = await api.refreshPlaybooks(path);
        if (res.errors && res.errors.length > 0) {
          alert('Sync completed with errors:\n' + res.errors.join('\n'));
        }
        loadAll();
      } catch (e) { alert('Refresh failed: ' + e.message); }
      document.getElementById('refresh-playbooks-btn').textContent = 'Refresh';
      document.getElementById('refresh-playbooks-btn').disabled = false;
    });

    document.getElementById('run-playbook-cancel').addEventListener('click', () => {
      document.getElementById('run-playbook-modal').classList.remove('open');
    });
    document.getElementById('run-playbook-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('run-playbook-modal').classList.remove('open');
    });
    document.getElementById('run-playbook-submit').addEventListener('click', async () => {
      const id = document.getElementById('run-playbook-id').value;
      const target = document.getElementById('run-playbook-target').value.trim();
      const groups = document.getElementById('run-playbook-groups').value.trim();
      const tags = document.getElementById('run-playbook-tags').value.trim();
      const varsStr = document.getElementById('run-playbook-vars').value.trim();
      if (!target && !groups) { document.getElementById('run-playbook-error').textContent = 'Target nodes or groups required'; return; }
      const body = {};
      if (target) body.target_nodes = target.split(',').map(s => s.trim()).filter(Boolean);
      if (groups) body.groups = groups.split(',').map(s => s.trim()).filter(Boolean);
      if (tags) body.tags = tags;
      if (varsStr) {
        const extraVars = {};
        varsStr.split(',').forEach(pair => {
          const [k, ...vs] = pair.split('=');
          if (k && vs.length) extraVars[k.trim()] = vs.join('=').trim();
        });
        if (Object.keys(extraVars).length) body.extra_vars = extraVars;
      }
      try {
        await api.runPlaybook(id, body);
        document.getElementById('run-playbook-modal').classList.remove('open');
        loadRuns();
      } catch (e) { document.getElementById('run-playbook-error').textContent = e.message; }
    });

    document.getElementById('detail-close-btn').addEventListener('click', () => {
      document.getElementById('playbook-detail-modal').classList.remove('open');
    });
    document.getElementById('detail-cancel-btn').addEventListener('click', () => {
      document.getElementById('playbook-detail-modal').classList.remove('open');
    });
    document.getElementById('playbook-detail-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('playbook-detail-modal').classList.remove('open');
    });
    document.getElementById('detail-run-btn').addEventListener('click', () => {
      const name = document.getElementById('detail-name').textContent;
      document.getElementById('playbook-detail-modal').classList.remove('open');
      showRunModal(name);
    });

    loadAll();
  });
```

Preserve existing helper functions:
- `loadSettingsPath()` (unchanged)
- `loadRuns()` (unchanged)
- `renderRuns()` (unchanged)
- `showRunDetail()` (unchanged)
- `setupWebSocket()` (unchanged)

- [ ] **Step 7: Commit**

```bash
git add cmd/plugins/serve/web/js/pages/playbooks.js cmd/plugins/serve/web/js/api.js
git commit -m "feat: playbook page category sidebar and search"
```

---

### Task 4: Frontend — playbook detail view

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/playbooks.js`
- Modify: `cmd/plugins/serve/handler/playbook.go` (+ new endpoint)
- Modify: `cmd/plugins/serve/server.go` (route registration)
- Modify: `cmd/plugins/serve/web/js/api.js`
- Modify: `cmd/plugins/serve/web/css/app.css` (modal-lg styles)

**Interfaces:**
- Consumes: `api.playbookFile(id)` → returns YAML content
- Produces: Detail modal with metadata + YAML viewer

- [ ] **Step 1: Add GetFile endpoint to handler**

In `cmd/plugins/serve/handler/playbook.go`, add:

```go
func (h *PlaybookHandler) GetFile(c *gin.Context) {
    id := c.Param("name") // route is /playbooks/:name but we'll use :id
    pb, err := h.playbooks.Get(c.Request.Context(), id)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "playbook not found"})
        return
    }
    if !pb.FileExists {
        c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "file not found"})
        return
    }
    data, err := os.ReadFile(pb.FilePath)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "read failed"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"name": pb.Name, "content": string(data)})
}
```

- [ ] **Step 2: Register route in server.go**

In the operator group, change the existing `:name` routes to use `:id`:

```go
operator.GET("/playbooks", s.playbookHandler.List)
operator.GET("/playbooks/:id", s.playbookHandler.Get)
operator.POST("/playbooks/:id/run", s.playbookHandler.Run)
operator.GET("/playbooks/:id/file", s.playbookHandler.GetFile)
```

(Keep all existing `/playbook/runs/*` routes)

- [ ] **Step 3: Add API client method**

In `api.js`:

```javascript
  playbookFile: (id) =>
    request('GET', `/playbooks/${encodeURIComponent(id)}/file`),
```

- [ ] **Step 4: Implement showPlaybookDetail in playbooks.js**

```javascript
  async function showPlaybookDetail(id) {
    const pb = state.playbooks.find(p => p.id === id);
    if (!pb) return;
    document.getElementById('detail-name').textContent = esc(pb.name);
    document.getElementById('detail-meta').innerHTML = `
      <span>ID: <strong style="font-family:var(--font-mono);font-size:11px">${esc(pb.id)}</strong></span>
      <span>分类: <strong>${esc(pb.category || '-')}</strong></span>
      <span>描述: <strong>${esc(pb.description || '-')}</strong></span>
      <span>任务数: <strong>${pb.tasks_count}</strong></span>
      <span>路径: <strong style="font-family:var(--font-mono);font-size:11px">${esc(pb.file_path)}</strong></span>
      ${pb.file_exists ? '' : '<span style="color:var(--danger)">文件缺失</span>'}
    `;
    const taskNames = pb.task_names || [];
    document.getElementById('detail-tasks').innerHTML = taskNames.length
      ? `<h4 style="font-size:13px;color:var(--muted);margin-bottom:8px">任务列表</h4>
         <ol style="padding-left:20px;font-size:13px">${taskNames.map(t => `<li style="margin-bottom:4px">${esc(t)}</li>`).join('')}</ol>`
      : '';
    document.getElementById('detail-yaml').textContent = '加载中…';
    try {
      const res = await api.playbookFile(id);
      document.getElementById('detail-yaml').textContent = res.content || '';
    } catch {
      document.getElementById('detail-yaml').textContent = '无法加载文件内容';
    }
    document.getElementById('playbook-detail-modal').classList.add('open');
  }
```

- [ ] **Step 5: Add CSS for modal-lg**

In `cmd/plugins/serve/web/css/app.css`, add after existing modal styles:

```css
.modal-lg { width: 640px; max-width: 90vw; }
.modal-header { display:flex; align-items:center; padding-bottom:12px; border-bottom:1px solid var(--border); margin-bottom:12px; }
```

- [ ] **Step 6: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add cmd/plugins/serve/handler/playbook.go cmd/plugins/serve/server.go cmd/plugins/serve/web/js/api.js cmd/plugins/serve/web/js/pages/playbooks.js cmd/plugins/serve/web/css/app.css
git commit -m "feat: playbook detail view with YAML content"
```

---

### Task 5: Enhanced run modal — group/node selector, resolve group → nodes

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/playbooks.js`
- Modify: `cmd/plugins/serve/handler/playbook.go` (resolveGroupNodes)
- Modify: `cmd/plugins/serve/store/node.go` (new method)

**Interfaces:**
- Consumes: `api.nodes()` for node list, `api.runPlaybook(id, body)`
- Produces: Node dropdown with multi-select in run modal

- [ ] **Step 1: Add ResolveNodesByGroups to store**

In `cmd/plugins/serve/store/node.go` (or create `cmd/plugins/serve/store/node.go` if it doesn't exist):

```go
func (s *NodeStore) ListByGroups(ctx context.Context, groups []string) ([]string, error) {
    if len(groups) == 0 {
        return nil, nil
    }
    placeholders := make([]string, len(groups))
    args := make([]interface{}, len(groups))
    for i, g := range groups {
        placeholders[i] = "?"
        args[i] = g
    }
    query := `SELECT DISTINCT n.id FROM nodes n JOIN node_groups ng ON n.id = ng.node_id WHERE ng."group" IN (` + strings.Join(placeholders, ",") + `)`
    rows, err := s.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var ids []string
    for rows.Next() {
        var id string
        if err := rows.Scan(&id); err == nil {
            ids = append(ids, id)
        }
    }
    return ids, nil
}
```

- [ ] **Step 2: Pass NodeStore to PlaybookHandler**

In handler construct:

```go
type PlaybookHandler struct {
    db        *sql.DB
    playbooks *store.PlaybookStore
    runs      *store.PlaybookRunStore
    nodes     *store.NodeStore  // new
    hub       *WSHub
}

func NewPlaybookHandler(db *sql.DB, ps *store.PlaybookStore, rs *store.PlaybookRunStore, ns *store.NodeStore, hub *WSHub) *PlaybookHandler {
    return &PlaybookHandler{db: db, playbooks: ps, runs: rs, nodes: ns, hub: hub}
}
```

In `server.go` where PlaybookHandler is constructed:

```go
s.playbookHandler = handler.NewPlaybookHandler(s.DB, ps, rs, s.nodeStore, s.hub)
```

- [ ] **Step 3: Update Run handler to accept groups**

In `cmd/plugins/serve/handler/playbook.go`:

```go
type runRequest struct {
    TargetNodes []string          `json:"target_nodes"`
    Groups      []string          `json:"groups,omitempty"`
    ExtraVars   map[string]string `json:"extra_vars,omitempty"`
    Tags        string            `json:"tags,omitempty"`
}
```

In the Run handler:

```go
func (h *PlaybookHandler) Run(c *gin.Context) {
    id := c.Param("id") // was "name"
    pb, err := h.playbooks.Get(c.Request.Context(), id)
    // ... (rest unchanged)

    var req runRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request"})
        return
    }

    // Resolve groups to node IDs if provided
    if len(req.TargetNodes) == 0 && len(req.Groups) > 0 {
        nodeIDs, err := h.nodes.ListByGroups(c.Request.Context(), req.Groups)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "group resolve failed"})
            return
        }
        req.TargetNodes = nodeIDs
    }

    if len(req.TargetNodes) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "target_nodes or groups is required"})
        return
    }

    run, err := h.runs.Create(c.Request.Context(), pb.ID, pb.FilePath, req.TargetNodes, req.ExtraVars, req.Tags)
    // ... rest unchanged
}
```

Wait — we also need to update `PlaybookRunStore.Create`. Let's update run model:

```go
// playbook_runs table
// playbook_id TEXT NOT NULL (instead of playbook_name)
```

Update `PlaybookRunStore.Create`:

```go
func (s *PlaybookRunStore) Create(ctx context.Context, playbookID, playbookFile string, targetNodes []string, extraVars map[string]string, tags string) (*model.PlaybookRun, error) {
    id := generateID()
    tnJSON, _ := json.Marshal(targetNodes)
    evJSON, _ := json.Marshal(extraVars)
    now := time.Now().UTC()
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO playbook_runs (id, playbook_id, playbook_name, status, target_nodes, extra_vars, tags, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
        id, playbookID, playbookID, string(model.RunStatusQueued),
        string(tnJSON), string(evJSON), tags, now)
    if err != nil { return nil, err }
    run := &model.PlaybookRun{
        ID: id, PlaybookName: playbookID, PlaybookFile: playbookFile,
        Status: model.RunStatusQueued, TargetNodes: targetNodes,
        ExtraVars: extraVars, Tags: tags, CreatedAt: now,
    }
    return run, nil
}
```

**IMPORTANT: playbook_name is now stored as the hash ID, not the human-readable name.** For display, the frontend views will see `playbook_name` and need to look it up in the playbook list to get the display name. Since the API already returns the full run data with `playbook_name` as the hash, the frontend needs to match it against existing playbook state.

- [ ] **Step 4: Update web JS API to accept id**

In `api.js`, `runPlaybook` uses `name` param which is now `id` (hash). The frontend already passes the hash from `pb.id`. No API signature change needed.

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add cmd/plugins/serve/handler/playbook.go cmd/plugins/serve/store/node.go cmd/plugins/serve/store/playbook.go cmd/plugins/serve/model/playbook.go cmd/plugins/serve/server.go cmd/plugins/serve/web/js/api.js cmd/plugins/serve/web/js/pages/playbooks.js
git commit -m "feat: enhanced run modal with group support, node list resolution"
```

---

### Task 6: Rewrite PlaybookRunStore to use playbook_id

**Files:**
- Modify: `cmd/plugins/serve/store/playbook_run.go` (rewrite)

**Interfaces:**
- Consumes: `playbook_id` (hash) instead of `playbook_name`
- Produces: `model.PlaybookRun.PlaybookID` field

- [ ] **Step 1: Add PlaybookID to model**

```go
type PlaybookRun struct {
    ID           string            `json:"id"`
    PlaybookID   string            `json:"playbook_id"`
    PlaybookName string            `json:"playbook_name"`
    PlaybookFile string           `json:"playbook_file"`
    // rest unchanged
}
```

- [ ] **Step 2: Update store Create to use playbook_id**

In `cmd/plugins/serve/store/playbook_run.go`, update query:

```go
func (s *PlaybookRunStore) Init(ctx context.Context) error {
    _, err := s.db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS playbook_runs (
            id               TEXT PRIMARY KEY,
            playbook_id      TEXT NOT NULL,
            playbook_name    TEXT NOT NULL,
            playbook_file    TEXT NOT NULL,
            status           TEXT DEFAULT 'queued',
            target_nodes     TEXT DEFAULT '[]',
            extra_vars       TEXT DEFAULT '{}',
            tags             TEXT DEFAULT '',
            error            TEXT DEFAULT '',
            results          TEXT DEFAULT '[]',
            created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            started_at       TIMESTAMP,
            completed_at     TIMESTAMP
        )
    `)
    return err
}
```

Update Create:

```go
func (s *PlaybookRunStore) Create(ctx context.Context, playbookID, playbookFile string, targetNodes []string, extraVars map[string]string, tags string) (*model.PlaybookRun, error) {
    id := uuid.New().String() // ensure ID is unique
    nodesJSON, _ := json.Marshal(targetNodes)
    varsJSON, _ := json.Marshal(extraVars)
    playbookName := playbookID // default to ID; server can enrich later if needed
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO playbook_runs (id, playbook_id, playbook_name, playbook_file, status, target_nodes, extra_vars, tags, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        id, playbookID, playbookName, playbookFile, string(model.RunStatusQueued),
        string(nodesJSON), string(varsJSON), tags, time.Now(), nil, nil)
    if err != nil { return nil, err }
    run := &model.PlaybookRun{
        ID: id, PlaybookID: playbookID, PlaybookName: playbookName, PlaybookFile: playbookFile,
        Status: model.RunStatusQueued, TargetNodes: targetNodes,
        ExtraVars: extraVars, Tags: tags, CreatedAt: time.Now(),
    }
    return run, nil
}
```

Wait, let's simplify — `Create` should accept name as well. Since the frontend already has `pb.name`, let's pass it:

```go
func (s *PlaybookRunStore) Create(ctx context.Context, playbookID, playbookName, playbookFile string, targetNodes []string, extraVars map[string]string, tags string) (*model.PlaybookRun, error) {
    // ... use playbookName directly
}
```

Update callers accordingly:

```go
// in handler/playbook.go:
run, err := h.runs.Create(c.Request.Context(), pb.ID, pb.Name, pb.FilePath, req.TargetNodes, req.ExtraVars, req.Tags)
```

- [ ] **Step 3: Update Get, List queries**

Update scan methods:

```go
func (s *PlaybookRunStore) scanRow(scanner interface {
    Scan(dest ...interface{}) error
}) (*model.PlaybookRun, error) {
    r := model.PlaybookRun{}
    var nodesJSON, varsJSON, resultsJSON, errStr, tags string
    var createdAt, startedAt, completedAt sql.NullTime
    err := scanner.Scan(
        &r.ID, &r.PlaybookID, &r.PlaybookName, &r.PlaybookFile,
        &r.Status, &nodesJSON, &varsJSON, &tags, &errStr, &resultsJSON,
        &createdAt, &startedAt, &completedAt)
    if err != nil { return nil, err }
    json.Unmarshal([]byte(nodesJSON), &r.TargetNodes)
    json.Unmarshal([]byte(varsJSON), &r.ExtraVars)
    json.Unmarshal([]byte(resultsJSON), &r.Results)
    r.Error = errStr
    r.Tags = tags
    if createdAt.Valid { r.CreatedAt = createdAt.Time }
    if startedAt.Valid { r.StartedAt = &startedAt.Time }
    if completedAt.Valid { r.CompletedAt = &completedAt.Time }
    return &r, nil
}
```

Update List and Get queries accordingly.

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add cmd/plugins/serve/model/playbook.go cmd/plugins/serve/store/playbook_run.go cmd/plugins/serve/handler/playbook.go
git commit -m "feat: playbook_run table uses playbook_id (hash) instead of name"
```

---

### Task 7: Playbook refresh enhancements

**Files:**
- Modify: `cmd/plugins/serve/handler/playbook.go`
- Modify: `cmd/plugins/serve/web/js/pages/playbooks.js` (refresh handler for errors display)

- [ ] **Step 1: Add BuildCache method to handler (if not already)**

In `handler/playbook.go`, after Refresh, call:

```go
h.playbooks.BuildCache(c.Request.Context()) // update memory cache from DB
```

- [ ] **Step 2: Show sync errors in frontend**

In `playbooks.js`, update refresh handler:

```javascript
    document.getElementById('refresh-playbooks-btn').addEventListener('click', async () => {
      const path = document.getElementById('playbook-path').value.trim();
      if (!path) { alert('Library path is required'); return; }
      const btn = document.getElementById('refresh-playbooks-btn');
      btn.textContent = 'Refreshing...';
      btn.disabled = true;
      try {
        const res = await api.refreshPlaybooks(path);
        const errorContainer = document.getElementById('refresh-errors');
        if (res.errors && res.errors.length > 0) {
          const errorList = res.errors.map(e => `<div style="color:var(--danger);font-size:12px;padding:4px 8px;background:var(--surface);border-radius:var(--radius-sm);margin-bottom:4px">${esc(e)}</div>`).join('');
          if (errorContainer) {
            errorContainer.innerHTML = errorList;
            errorContainer.style.display = 'block';
          }
        } else if (errorContainer) {
          errorContainer.style.display = 'none';
        }
        loadAll();
      } catch (e) { alert('Refresh failed: ' + e.message); }
      btn.textContent = 'Refresh';
      btn.disabled = false;
    });
```

Add in the render template, right after the path card:

```html
    <div id="refresh-errors" style="display:none;margin-top:8px"></div>
```

- [ ] **Step 3: Commit**

```bash
git add cmd/plugins/serve/handler/playbook.go cmd/plugins/serve/web/js/pages/playbooks.js
git commit -m "feat: refresh sync shows hash collision errors in UI"
```

---

### Task 8: Create test playbooks with categories

**Files:**
- Create: `~/.owl/playbooks/system/ping-test.yaml`
- Create: `~/.owl/playbooks/system/system-info.yaml`
- Create: `~/.owl/playbooks/deploy/deploy-app.yaml`
- Create: `~/.owl/playbooks/maintenance/cleanup.yaml`

**Interfaces:** Standard playbook YAML format with `command` action key

- [ ] **Step 1: Create system/ping-test.yaml**

```yaml
name: ping_test
description: 测试节点连通性 — ping + DNS 检查
hosts: []
tasks:
  - name: Ping localhost
    command: ping -c 2 127.0.0.1
  - name: Check DNS resolution
    command: nslookup localhost
  - name: Check network interfaces
    command: ifconfig lo0 2>/dev/null || ip addr show lo 2>/dev/null || echo "no lo info"
```

- [ ] **Step 2: Create system/system-info.yaml**

```yaml
name: system_info
description: 收集系统基本信息 — 主机名、负载、磁盘、内存
hosts: []
tasks:
  - name: 获取主机名
    command: hostname
  - name: 查看运行时间
    command: uptime
  - name: 查看磁盘使用
    command: df -h / | tail -1
  - name: 查看内存使用
    command: free -h 2>/dev/null || vm_stat 2>/dev/null || echo "memory info unavailable"
  - name: 查看内核版本
    command: uname -a
```

- [ ] **Step 3: Create deploy/deploy-app.yaml**

```yaml
name: deploy_app
description: 模拟应用部署 — 创建目录、拉取代码、安装依赖
hosts: []
vars:
  app_dir: /tmp/myapp
  repo_url: https://example.com/repo.git
tasks:
  - name: 创建部署目录
    command: mkdir -p {{ app_dir }}
  - name: 克隆代码
    command: echo "git clone {{ repo_url }} {{ app_dir }}"
    ignore_errors: true
  - name: 安装依赖
    command: echo "npm install --production"
```

- [ ] **Step 4: Create maintenance/cleanup.yaml**

```yaml
name: cleanup
description: 系统维护 — 清理临时文件、日志、缓存
hosts: []
tasks:
  - name: 清理临时文件
    command: rm -rf /tmp/*.tmp 2>/dev/null; echo "cleaned temp files"
  - name: 清理系统日志
    command: echo "journalctl --vacuum-time=7d"
  - name: 查看磁盘使用率
    command: df -h | grep -E '^/dev/'
```

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/plans/2026-07-09-playbook-web-enhancement.md
git commit -m "docs: update implementation plan with hash-based ID schema"
```

---

### Task 9: End-to-end verification

- [ ] **Step 1: Build and start server**

```bash
go build -o build/owl-serve ./cmd/plugins/serve && ./build/owl-serve --port 8080 --reset-admin
```

- [ ] **Step 2: Log in and navigate to /playbooks**

Expected:
- Left panel shows categories: 全部, system, deploy, maintenance
- Clicking a category filters the table
- Search input filters by name/description

- [ ] **Step 3: Click a playbook row**

Expected:
- Detail modal opens
- Shows metadata (ID, category, description, task count, path)
- Shows task list and YAML content
- "Run" button in modal opens run modal

- [ ] **Step 4: Run a playbook**

Set a target node (any existing node ID) and execute.
Expected: Run appears in history, status updates via WebSocket.

- [ ] **Step 5: Test hash collision**

Duplicate a playbook file with different name in same directory, refresh.
Expected: Error shown in refresh UI: "hash collision: ..."

- [ ] **Step 6: Build CLI and test list**

```bash
go build -o build/owl ./cmd/cli
./build/owl playbook list
```

Expected: Lists playbooks from `~/.owl/playbooks/`

- [ ] **Step 7: Commit any remaining fixes**

```bash
git add -A
git commit -m "chore: final adjustments after e2e verification"
```