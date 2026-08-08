# 统一执行共享层 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按 spec `docs/superpowers/specs/2026-08-04-unified-exec-layer-design.md` 统一 CLI/serve 的节点选择、SSH 拨号、安全黑名单三个共享层。

**Architecture:** 新增 `internal/node/select` 精确选择包（NodeSource 接口对接 CLI resolver 与 serve DB）；`internal/ssh` 新增统一 `Dial`（含 ProxyJump）替换三处自研拨号；serve 三个执行入口接入现有 `internal/control/blacklist`（403 + `danger_confirmed` 放行）。保持 go.work 三模块结构与现有响应格式不变。

**Tech Stack:** Go 1.26、cobra、gin、`golang.org/x/crypto/ssh`、modernc.org/sqlite、testify

## Global Constraints

- **CLI 功能优先**：任何任务结束前必须先跑 CLI 侧测试（`go test ./cmd/cli/... ./internal/... ./pkg/...`）且全绿，再动/再测 serve 模块。凡 CLI 既有行为（交互确认、`--force`、flag 语义），一律不改。
- CLI label flag 现状是只用 `execLabel[0]`，接入后保持只取第一个（不做行为升级）。
- serve 响应体保持现有 `{"code": N, "message": "..."}` 约定（spec §4 提到的 `{"error","detail"}` 与现有前端约定冲突，**偏离**：沿用 code/message，详情放进 message）。
- serve 已有请求字段 `"force": "true"` 语义是"覆盖同节点冲突中的任务"（`handler/exec.go:395-401`），**不可**复用于黑名单放行（否则合并冲突绕过 = 安全绕过）。黑名单放行用新字段 `"danger_confirmed": true`（bool）。**偏离** spec 的 `"force": true` 命名，原因在此记录。
- `HostKeyCallback` 维持 `InsecureIgnoreHostKey`（known_hosts 为范围外单项）。
- 不新增第三方依赖；不改动 `handler/node.go` 的列表/搜索 LIKE（界面模糊搜索是合法功能）。
- 每个任务以原子 commit 结束。提交信息格式沿用仓库风格（`feat(scope): ...` / `fix(scope): ...` / `test(scope): ...`）。

## 文件结构总览

| 文件 | 职责 | 动作 |
|---|---|---|
| `internal/node/select/select.go` | NodeRow/SelectOptions/Selector，精确选择语义 | 新增 |
| `internal/node/select/select_test.go` | 选择语义单测 | 新增 |
| `internal/node/select/resolver_source.go` | CLI NodeSource：包装 *node.NodeResolver | 新增 |
| `cmd/cli/cmd/exec/run.go` | CLI exec run 接入共享选择 | 修改 :133-168 |
| `cmd/plugins/serve/handler/node_source.go` | serve NodeSource：读 nodes 表 | 新增 |
| `cmd/plugins/serve/handler/exec.go` | 删除 resolveNodeIDs，接入 Selector + 黑名单 | 修改 |
| `cmd/plugins/serve/handler/aiexecutor.go` | AI 节点解析改精确匹配 + 黑名单 | 修改 |
| `internal/ssh/dial.go` | 统一 Dial（认证链/超时/ProxyJump） | 新增 |
| `internal/ssh/dial_test.go` | 进程内 SSH server 测试 Dial/ProxyJump | 新增 |
| `internal/ssh/connection_manager.go` | ConnectionInfo +ProxyJump +KeyContent | 修改 |
| `internal/ssh/native_executor.go` | execute/WriteFile 改用共享 Dial | 修改 |
| `internal/ssh/executor_factory.go` | GetExecutorForNode +proxyJump 参数 | 修改 |
| `internal/ssh/connection_pool.go` | Get 传递 nodeInfo.ProxyJump | 修改 |
| `cmd/plugins/serve/handler/ssh_executor.go` | 改用共享 Dial，读 proxy_jump 列 | 修改 |
| `internal/control/blacklist/check_exec.go` | CheckForExec + BlockedError | 新增 |
| `cmd/plugins/serve/handler/playbook_engine.go` | webCommandExecutor 加黑名单检查 | 修改 |
| `cmd/plugins/serve/model/playbook.go` + `store/playbook_run.go` | PlaybookRun +DangerConfirmed | 修改 |
| `internal/history/history.go` / `db_sqlite3.go` | Operation +Forced、schema 迁移 | 修改 |
| `cmd/plugins/serve/store/history.go` | 同上（schema 双侧一致） | 修改 |
| `tests/unit/command_test.go`、`tests/unit/variable_test.go` | 假测试 | 删除 |

---

### Task 1: `internal/node/select` — 精确节点选择包

**Files:**
- Create: `internal/node/select/select.go`
- Test: `internal/node/select/select_test.go`

**Interfaces:**
- Produces（后续任务依赖）:
  - `type NodeRow struct { ID, Name, Status string; Groups []string; Labels map[string]string }`
  - `type NodeSource interface { List(ctx context.Context) ([]NodeRow, error) }`
  - `type SelectOptions struct { NodeIDs, Groups []string; Labels map[string]string; Status string }`
  - `func NewSelector(source NodeSource) *Selector`
  - `func (s *Selector) Select(ctx context.Context, opts SelectOptions) ([]NodeRow, error)`
  - Labels 语义：值为 `""` 表示"键存在即可"（兼容 CLI `--label env` 写法）

- [ ] **Step 1: 写失败测试**

```go
// internal/node/select/select_test.go
package select

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	rows []NodeRow
	err  error
}

func (f *fakeSource) List(ctx context.Context) ([]NodeRow, error) {
	return f.rows, f.err
}

func sampleRows() []NodeRow {
	return []NodeRow{
		{ID: "n1", Name: "web-01", Groups: []string{"web", "prod"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "web-k8s-01", Groups: []string{"web-k8s"}, Labels: map[string]string{"env": "prod", "zone": "a"}},
		{ID: "n3", Name: "db-01", Groups: []string{"db"}, Labels: map[string]string{"env": "staging"}},
		{ID: "n4", Name: "web-02", Groups: []string{"web"}, Labels: map[string]string{}, Status: "offline"},
	}
}

func TestSelect_GroupExactMatch(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{Groups: []string{"web"}})
	require.NoError(t, err)
	var ids []string
	for _, n := range got {
		ids = append(ids, n.ID)
	}
	assert.ElementsMatch(t, []string{"n1", "n4"}, ids)
}

func TestSelect_LabelsAND(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{
		Labels: map[string]string{"env": "prod", "zone": "a"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n2", got[0].ID)
}

func TestSelect_LabelKeyOnly(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{
		Labels: map[string]string{"zone": ""},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n2", got[0].ID)
}

func TestSelect_IDAndName(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{NodeIDs: []string{"n1", "db-01"}})
	require.NoError(t, err)
	var ids []string
	for _, n := range got {
		ids = append(ids, n.ID)
	}
	assert.ElementsMatch(t, []string{"n1", "n3"}, ids)
}

func TestSelect_UnknownIDErrors(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	_, err := s.Select(context.Background(), SelectOptions{NodeIDs: []string{"n1", "ghost"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestSelect_PriorityNodesOverGroups(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{
		NodeIDs: []string{"n3"},
		Groups:  []string{"web"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n3", got[0].ID)
}

func TestSelect_Status(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{Status: "offline"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n4", got[0].ID)
}

func TestSelect_EmptyReturnsAll(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{})
	require.NoError(t, err)
	assert.Len(t, got, 4)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/node/select/`
Expected: FAIL，编译错误（package 不存在）

- [ ] **Step 3: 实现**

```go
// internal/node/select/select.go
// Package select 提供 CLI 与 Web 共用的执行目标选择语义。
// 精确匹配（组名完整相等、标签键值相等），不做子串模糊匹配——
// 界面搜索框的模糊查询在各自的列表 API 中，与此无关。
package select

import (
	"context"
	"fmt"
	"strings"
)

type NodeRow struct {
	ID     string
	Name   string
	Groups []string
	Labels map[string]string
	Status string
}

type NodeSource interface {
	List(ctx context.Context) ([]NodeRow, error)
}

type SelectOptions struct {
	NodeIDs []string
	Groups  []string
	Labels  map[string]string
	Status  string
}

func (o SelectOptions) Empty() bool {
	return len(o.NodeIDs) == 0 && len(o.Groups) == 0 && len(o.Labels) == 0 && o.Status == ""
}

type Selector struct {
	source NodeSource
}

func NewSelector(source NodeSource) *Selector {
	return &Selector{source: source}
}

// Select 按 CLI 语义解析执行目标。
// 优先级：NodeIDs > Groups > Labels > Status（多条件并存时取其一，不做交集）。
// 空选项返回全部节点。NodeIDs 中任一 id/name 无法解析则整体报错。
func (s *Selector) Select(ctx context.Context, opts SelectOptions) ([]NodeRow, error) {
	all, err := s.source.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("节点列表获取失败: %w", err)
	}
	switch {
	case opts.Empty():
		return all, nil
	case len(opts.NodeIDs) > 0:
		return selectByIDOrName(all, opts.NodeIDs)
	case len(opts.Groups) > 0:
		return selectByGroups(all, opts.Groups), nil
	case len(opts.Labels) > 0:
		return selectByLabels(all, opts.Labels), nil
	default:
		return selectByStatus(all, opts.Status), nil
	}
}

func selectByIDOrName(all []NodeRow, ids []string) ([]NodeRow, error) {
	byID := make(map[string]NodeRow, len(all))
	byName := make(map[string]NodeRow, len(all))
	for _, n := range all {
		byID[n.ID] = n
		if n.Name != "" {
			byName[n.Name] = n
		}
	}
	var out []NodeRow
	var missing []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if n, ok := byID[id]; ok {
			out = append(out, n)
			continue
		}
		if n, ok := byName[id]; ok {
			out = append(out, n)
			continue
		}
		missing = append(missing, id)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("节点不存在: %v", missing)
	}
	return out, nil
}

func selectByGroups(all []NodeRow, groups []string) []NodeRow {
	want := make(map[string]bool, len(groups))
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g != "" {
			want[g] = true
		}
	}
	var out []NodeRow
	for _, n := range all {
		for _, g := range n.Groups {
			if want[g] {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

func selectByLabels(all []NodeRow, labels map[string]string) []NodeRow {
	var out []NodeRow
	for _, n := range all {
		match := true
		for k, v := range labels {
			got, ok := n.Labels[k]
			if !ok || (v != "" && got != v) {
				match = false
				break
			}
		}
		if match {
			out = append(out, n)
		}
	}
	return out
}

func selectByStatus(all []NodeRow, status string) []NodeRow {
	var out []NodeRow
	for _, n := range all {
		if n.Status == status {
			out = append(out, n)
		}
	}
	return out
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/node/select/ -v`
Expected: PASS（8 个测试全绿）

- [ ] **Step 5: Commit**

```bash
git add internal/node/select/
git commit -m "feat(node): add shared exact-match node selector"
```

---

### Task 2: CLI `exec run` 接入共享选择

**Files:**
- Create: `internal/node/select/resolver_source.go`
- Test: `internal/node/select/resolver_source_test.go`
- Modify: `cmd/cli/cmd/exec/run.go:133-168`（目标解析块）

**Interfaces:**
- Consumes: Task 1 的 `Selector/SelectOptions/NodeRow`
- Produces: `func NewResolverSource(resolver *node.NodeResolver) *ResolverSource`（实现 NodeSource）

- [ ] **Step 1: 写 ResolverSource 失败测试**

```go
// internal/node/select/resolver_source_test.go
package select

import (
	"context"
	"testing"

	"github.com/cangyunye/go-owl/internal/node"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolverSource_List(t *testing.T) {
	resolver := node.NewNodeResolver()
	src := NewResolverSource(resolver)
	rows, err := src.List(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, rows)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/node/select/ -run TestResolverSource`
Expected: FAIL，编译错误（NewResolverSource 未定义）

- [ ] **Step 3: 实现 ResolverSource**

```go
// internal/node/select/resolver_source.go
package select

import (
	"context"

	"github.com/cangyunye/go-owl/internal/node"
)

// ResolverSource 把 CLI 的 NodeResolver（合并 local/API/ssh-config 来源）
// 适配为共享选择器的 NodeSource。
type ResolverSource struct {
	resolver *node.NodeResolver
}

func NewResolverSource(resolver *node.NodeResolver) *ResolverSource {
	return &ResolverSource{resolver: resolver}
}

func (s *ResolverSource) List(ctx context.Context) ([]NodeRow, error) {
	nodes, err := s.resolver.ListNodes(&node.ListOptions{})
	if err != nil {
		return nil, err
	}
	rows := make([]NodeRow, 0, len(nodes))
	for _, n := range nodes {
		rows = append(rows, NodeRow{
			ID:     n.ID,
			Name:   n.Name,
			Groups: n.Groups,
			Labels: n.Labels,
		})
	}
	return rows, nil
}
```

- [ ] **Step 4: 修改 `cmd/cli/cmd/exec/run.go`**

替换 :133-168 的目标解析块（`nodeResolver := ...` 之后到 `if len(targetNodeIDs) == 0` 之前）。imports 增加 `nodeselect "github.com/cangyunye/go-owl/internal/node/select"` 与 `"strings"`（若未有）。

原代码（删除）：

```go
	if execNodes != "" {
		targetNodeIDs = parseNodeList(execNodes)
	} else if len(execGroup) > 0 {
		nodes, err := node.ListNodesByGroups(nodeResolver, execGroup)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("exec.run.err_list", err))
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	} else if len(execLabel) > 0 {
		nodes, err := nodeResolver.ListNodes(&node.ListOptions{
			Label: execLabel[0],
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("exec.run.err_list", err))
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	} else {
		nodes, err := nodeResolver.ListNodes(&node.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("exec.run.err_list", err))
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	}
```

新代码（替换为）：

```go
	selector := nodeselect.NewSelector(nodeselect.NewResolverSource(nodeResolver))
	selectOpts := nodeselect.SelectOptions{}
	switch {
	case execNodes != "":
		selectOpts.NodeIDs = parseNodeList(execNodes)
	case len(execGroup) > 0:
		selectOpts.Groups = execGroup
	case len(execLabel) > 0:
		labels := make(map[string]string)
		if k, v, ok := strings.Cut(execLabel[0], "="); ok {
			labels[k] = v
		} else {
			labels[execLabel[0]] = ""
		}
		selectOpts.Labels = labels
	}
	selected, err := selector.Select(context.Background(), selectOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("exec.run.err_list", err))
		os.Exit(1)
	}
	for _, n := range selected {
		targetNodeIDs = append(targetNodeIDs, n.ID)
	}
```

注意：`context` 包如未 import 需补充。CLI 行为差异仅一处（spec §1 已批准）：指定了不存在的节点 id/name 时从"执行阶段逐节点失败"变为"选择阶段整体报错退出"，属于 bug 修复方向。`--label` 仍只取 `execLabel[0]`，与旧行为一致。

- [ ] **Step 5: CLI 门禁（优先）**

Run: `go build ./... && go test ./cmd/cli/... ./internal/... ./pkg/...`
Expected: PASS。若 `cmd/cli/cmd/exec` 有用例依赖"未知节点不报错"，检查后按新语义（整体报错）更新断言，并在 commit message 注明。

- [ ] **Step 6: Commit**

```bash
git add internal/node/select/resolver_source.go internal/node/select/resolver_source_test.go cmd/cli/cmd/exec/run.go
git commit -m "refactor(cli): exec run uses shared exact-match node selector"
```

---

### Task 3: serve 节点选择接入（exec + AI）

**Files:**
- Create: `cmd/plugins/serve/handler/node_source.go`、`cmd/plugins/serve/handler/node_source_test.go`
- Modify: `cmd/plugins/serve/handler/exec.go`（删除 resolveNodeIDs :111-181，Create 处调用点 :223）
- Modify: `cmd/plugins/serve/handler/aiexecutor.go`（resolveAINodeIDs :55-90 改精确匹配，保留 search 模糊）

**Interfaces:**
- Consumes: Task 1 的 `nodeselect.Selector/SelectOptions`
- Produces: serve 内 `type dbNodeSource struct{ db *sql.DB }` 实现 `nodeselect.NodeSource`

- [ ] **Step 1: 写 dbNodeSource + 精确选择失败测试**

```go
// cmd/plugins/serve/handler/node_source_test.go
package handler

import (
	"context"
	"database/sql"
	"testing"

	nodeselect "github.com/cangyunye/go-owl/internal/node/select"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nodeSelectTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT ''
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, user, status, groups, labels) VALUES
		('n1', 'web-01', 'root', 'online', '["web","prod"]', '{"env":"prod"}'),
		('n2', 'web-k8s-01', 'root', 'online', '["web-k8s"]', '{"env":"prod","zone":"a"}'),
		('n3', 'db-01', 'admin', 'online', '["db"]', '{}')`)
	require.NoError(t, err)
	return db
}

func TestDBNodeSource_List(t *testing.T) {
	db := nodeSelectTestDB(t)
	src := &dbNodeSource{db: db}
	rows, err := src.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, rows, 3)
}

func TestResolveNodeIDs_GroupExact(t *testing.T) {
	db := nodeSelectTestDB(t)
	ids, err := resolveNodeIDs(context.Background(), db, execRequest{Groups: []string{"web"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"n1"}, ids, "web 不得命中 web-k8s")
}

func TestResolveNodeIDs_LabelExact(t *testing.T) {
	db := nodeSelectTestDB(t)
	ids, err := resolveNodeIDs(context.Background(), db, execRequest{
		Labels: map[string]string{"env": "prod", "zone": "a"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"n2"}, ids)
}

func TestResolveNodeIDs_UnknownNodeErrors(t *testing.T) {
	db := nodeSelectTestDB(t)
	_, err := resolveNodeIDs(context.Background(), db, execRequest{NodeIDs: []string{"ghost"}})
	require.Error(t, err)
}

func TestSelector_GroupExact_NoFalsePositive(t *testing.T) {
	db := nodeSelectTestDB(t)
	sel := nodeselect.NewSelector(&dbNodeSource{db: db})
	got, err := sel.Select(context.Background(), nodeselect.SelectOptions{Groups: []string{"web"}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n1", got[0].ID)
}
```

- [ ] **Step 2: 运行确认失败**

Run（workdir `cmd/plugins/serve`）: `go test ./handler/ -run 'TestDBNodeSource|TestResolveNodeIDs|TestSelector_GroupExact'`
Expected: FAIL，编译错误（dbNodeSource / 新签名 resolveNodeIDs 未定义）

- [ ] **Step 3: 实现 dbNodeSource**

```go
// cmd/plugins/serve/handler/node_source.go
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	nodeselect "github.com/cangyunye/go-owl/internal/node/select"
)

// dbNodeSource 从 serve 的 nodes 表加载节点，供共享选择器精确过滤。
// 界面搜索框的模糊查询在 node.go 的列表 API，与此无关。
type dbNodeSource struct {
	db *sql.DB
}

func (s *dbNodeSource) List(ctx context.Context) ([]nodeselect.NodeRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, COALESCE(groups,'[]'), COALESCE(labels,'{}'), COALESCE(status,'') FROM nodes`)
	if err != nil {
		return nil, fmt.Errorf("查询节点表失败: %w", err)
	}
	defer rows.Close()

	var out []nodeselect.NodeRow
	for rows.Next() {
		var r nodeselect.NodeRow
		var groupsJSON, labelsJSON string
		if err := rows.Scan(&r.ID, &r.Name, &groupsJSON, &labelsJSON, &r.Status); err != nil {
			return nil, fmt.Errorf("读取节点行失败: %w", err)
		}
		if err := json.Unmarshal([]byte(groupsJSON), &r.Groups); err != nil {
			r.Groups = nil
		}
		if err := json.Unmarshal([]byte(labelsJSON), &r.Labels); err != nil {
			r.Labels = nil
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: 替换 `exec.go` 的 resolveNodeIDs**

删除 `exec.go:111-181` 整个旧函数，替换为：

```go
func resolveNodeIDs(ctx context.Context, db *sql.DB, req execRequest) ([]string, error) {
	sel := nodeselect.NewSelector(&dbNodeSource{db: db})

	opts := nodeselect.SelectOptions{NodeIDs: req.NodeIDs}
	if len(opts.NodeIDs) == 0 && req.NodeID != "" {
		opts.NodeIDs = []string{req.NodeID}
	}
	groups := req.Groups
	if len(groups) == 0 && req.Group != "" {
		groups = strings.Split(req.Group, ",")
	}
	for _, g := range groups {
		if g = strings.TrimSpace(g); g != "" {
			opts.Groups = append(opts.Groups, g)
		}
	}
	opts.Labels = req.Labels
	opts.Status = req.Status

	nodes, err := sel.Select(ctx, opts)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids, nil
}
```

imports 增加 `nodeselect "github.com/cangyunye/go-owl/internal/node/select"`。

调用点 `exec.go:223` 改为：

```go
	nodeIDs, err := resolveNodeIDs(c.Request.Context(), h.db, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if len(nodeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "no target nodes specified"})
		return
	}
```

- [ ] **Step 5: 修正 `aiexecutor.go` 的 resolveAINodeIDs（精确组/标签，保留 search 模糊）**

`aiexecutor.go:55-90` 整体替换为（保留方法签名与调用点 `:145`）：

```go
func (e *WebExecutor) resolveAINodeIDs(ctx context.Context, nodes []string, group, label, search string) []string {
	src := &dbNodeSource{db: e.db}
	rows, err := src.List(ctx)
	if err != nil {
		return nil
	}

	var out []string
	for _, r := range rows {
		if len(nodes) > 0 {
			hit := false
			for _, id := range nodes {
				if id == r.ID || id == r.Name {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		if group != "" {
			hit := false
			for _, g := range r.Groups {
				if g == group {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		if label != "" && strings.Contains(label, "=") {
			k, v, _ := strings.Cut(label, "=")
			if r.Labels[k] != v {
				continue
			}
		}
		if search != "" {
			needle := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(r.Name), needle) &&
				!strings.Contains(strings.ToLower(r.ID), needle) {
				continue
			}
		}
		out = append(out, r.ID)
	}
	return out
}
```

（语义变化仅为组/标签由 LIKE 子串改为精确匹配；search 保留模糊。原实现里的 address 匹配在 AI 场景无对应 NodeRow 字段，按 id/name 匹配即可——若现有 AI 测试依赖 address 搜索，用 NodeRow 中补充 Address 字段的方式保留：本任务不改 NodeRow，测试若失败则说明依赖脆弱，按 name/id 更新断言。）

- [ ] **Step 6: 运行测试**

Run（workdir `cmd/plugins/serve`）: `go test ./handler/ -v`
Expected: PASS。重点检查既有 `TestExecCreate_ByGroup`、`TestExecCreate_ByLabel`、`aiexecutor_test.go`：若用例数据含子串误匹配场景，按精确语义更新断言。

CLI 门禁（未改 CLI 但必须确认）：Run（仓库根）: `go test ./cmd/cli/... ./internal/... ./pkg/...` → PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/plugins/serve/handler/node_source.go cmd/plugins/serve/handler/node_source_test.go cmd/plugins/serve/handler/exec.go cmd/plugins/serve/handler/aiexecutor.go
git commit -m "fix(serve): exec/AI node selection uses exact match instead of LIKE"
```

---

### Task 4: `internal/ssh` 统一 Dial（含 ProxyJump）

**Files:**
- Modify: `internal/ssh/connection_manager.go`（ConnectionInfo 加 ProxyJump、KeyContent 字段）
- Create: `internal/ssh/dial.go`
- Test: `internal/ssh/dial_test.go`
- Modify: `internal/ssh/native_executor.go`（execute/WriteFile 改用 Dial）、`internal/ssh/executor_factory.go`（GetExecutorForNode 加 proxyJump 参数）、`internal/ssh/connection_pool.go`（Get 传 nodeInfo.ProxyJump）

**Interfaces:**
- Produces:
  - `type Client struct { *gossh.Client }`，`Close()` 同时关闭跳板连接
  - `func Dial(ctx context.Context, addr string, opts DialOptions) (*Client, error)`
  - `type DialOptions struct { User, Password, KeyFile, KeyContent, ProxyJump string; ConnectTimeout time.Duration }`
  - 错误类型复用现有 `*SSHAuthError` / `*ConnectionError`

- [ ] **Step 1: ConnectionInfo 加字段**

`internal/ssh/connection_manager.go` 的 ConnectionInfo 结构改为：

```go
type ConnectionInfo struct {
	User       string
	Address    string
	Port       int
	KeyFile    string
	KeyContent string // 内联 PEM 私钥（Web 数据库存储场景）
	Password   string // SSH 密码
	ProxyJump  string // 跳板机 "host" 或 "host:port"
	UseConfig  bool   // 是否使用 SSH config 中的配置
}
```

- [ ] **Step 2: 写 dial_test.go（进程内 SSH server）**

```go
// internal/ssh/dial_test.go
package ssh

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"github.com/stretchr/testify/require"
)

func genHostKey(t *testing.T) gossh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromSigner(priv)
	require.NoError(t, err)
	return signer
}

// startSSHServer 启动一个最简 SSH server：接受认证（密码 "pass"），
// 对 exec 请求回 "ok"；allowForward=true 时支持 direct-tcpip 转发。
func startSSHServer(t *testing.T, allowForward bool) string {
	t.Helper()
	cfg := &gossh.ServerConfig{
		PasswordCallback: func(c gossh.ConnMetadata, pass []byte) (*gossh.Permissions, error) {
			if string(pass) == "pass" {
				return nil, nil
			}
			return nil, io.ErrUnexpectedEOF
		},
	}
	cfg.AddHostKey(genHostKey(t))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				sconn, chans, reqs, err := gossh.NewServerConn(c, cfg)
				if err != nil {
					c.Close()
					return
				}
				defer sconn.Close()
				go gossh.DiscardRequests(reqs)
				for newChan := range chans {
					switch newChan.ChannelType() {
					case "session":
						ch, chReqs, err := newChan.Accept()
						if err != nil {
							continue
						}
						go func() {
							for req := range chReqs {
								if req.Type == "exec" {
									req.Reply(true, nil)
									ch.Write([]byte("ok"))
									ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
									ch.Close()
									return
								}
								req.Reply(false, nil)
							}
						}()
					case "direct-tcpip":
						if !allowForward {
							newChan.Reject(gossh.Prohibited, "forwarding disabled")
							continue
						}
						var payload struct{ DestAddr string; DestPort uint32 }
						raw := newChan.ExtraData()
						// direct-tcpip payload: addr(4+n) port(4) srcAddr(4+n) srcPort(4)
						if len(raw) < 4 {
							newChan.Reject(gossh.ConnectionFailed, "bad payload")
							continue
						}
						al := binary.BigEndian.Uint32(raw)
						payload.DestAddr = string(raw[4 : 4+al])
						payload.DestPort = binary.BigEndian.Uint32(raw[4+al : 8+al])
						ch, reqs, err := newChan.Accept()
						if err != nil {
							continue
						}
						go gossh.DiscardRequests(reqs)
						go func() {
							dst, err := net.Dial("tcp", net.JoinHostPort(payload.DestAddr, itoa(int(payload.DestPort))))
							if err != nil {
								ch.Close()
								return
							}
							go io.Copy(dst, ch)
							io.Copy(ch, dst)
						}()
					default:
						newChan.Reject(gossh.UnknownChannelType, "unsupported")
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

func TestDial_Basic(t *testing.T) {
	addr := startSSHServer(t, false)
	client, err := Dial(context.Background(), addr, DialOptions{
		User: "u", Password: "pass", ConnectTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer client.Close()

	session, err := client.NewSession()
	require.NoError(t, err)
	defer session.Close()
	out, err := session.CombinedOutput("uptime")
	require.NoError(t, err)
	require.Equal(t, "ok", string(out))
}

func TestDial_AuthFailed(t *testing.T) {
	addr := startSSHServer(t, false)
	_, err := Dial(context.Background(), addr, DialOptions{
		User: "u", Password: "wrong", ConnectTimeout: 5 * time.Second,
	})
	require.Error(t, err)
}

func TestDial_NoAuthMethods(t *testing.T) {
	addr := startSSHServer(t, false)
	_, err := Dial(context.Background(), addr, DialOptions{
		User: "u", ConnectTimeout: 5 * time.Second,
	})
	require.Error(t, err)
	var authErr *SSHAuthError
	require.ErrorAs(t, err, &authErr)
}

func TestDial_ProxyJump(t *testing.T) {
	target := startSSHServer(t, false)
	jump := startSSHServer(t, true) // 跳板需允许 direct-tcpip

	client, err := Dial(context.Background(), target, DialOptions{
		User: "u", Password: "pass", ProxyJump: jump, ConnectTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer client.Close()

	session, err := client.NewSession()
	require.NoError(t, err)
	defer session.Close()
	out, err := session.CombinedOutput("uptime")
	require.NoError(t, err)
	require.Equal(t, "ok", string(out))
}

func TestDial_ConnectTimeout(t *testing.T) {
	// 不可达地址应超时返回而不是无限等待
	_, err := Dial(context.Background(), "10.255.255.1:22", DialOptions{
		User: "u", Password: "pass", ConnectTimeout: 500 * time.Millisecond,
	})
	require.Error(t, err)
}
```

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/ssh/ -run TestDial`
Expected: FAIL，编译错误（Dial/DialOptions 未定义）

- [ ] **Step 4: 实现 `internal/ssh/dial.go`**

```go
// internal/ssh/dial.go
package ssh

import (
	"context"
	"fmt"
	"net"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

type DialOptions struct {
	User           string
	Password       string
	KeyFile        string
	KeyContent     string // 内联 PEM 私钥
	ProxyJump      string // "host" 或 "host:port"
	ConnectTimeout time.Duration
}

// Client 包装 gossh.Client；Close 会连带关闭经由 ProxyJump 建立的跳板连接。
type Client struct {
	*gossh.Client
	jump *gossh.Client
}

func (c *Client) Close() error {
	err := c.Client.Close()
	if c.jump != nil {
		c.jump.Close()
	}
	return err
}

// Dial 建立 SSH 连接。认证链：密钥文件/内联密钥优先，密码兜底，
// 两者皆无时尝试默认密钥（~/.ssh/id_ed25519 等）。
// ProxyJump 非空时先连跳板机，再经跳板转发到目标。
func Dial(ctx context.Context, addr string, opts DialOptions) (*Client, error) {
	timeout := opts.ConnectTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	auths := buildDialAuth(opts)
	if len(auths) == 0 {
		return nil, &SSHAuthError{
			ExitCode: -1,
			NodeID:   addr,
			Stderr:   "没有可用的认证方式：请配置 SSH 密钥或密码",
			Cause:    fmt.Errorf("no authentication methods available"),
		}
	}

	config := &gossh.ClientConfig{
		User:            opts.User,
		Auth:            auths,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	if opts.ProxyJump != "" {
		jumpAddr := opts.ProxyJump
		if _, _, err := net.SplitHostPort(jumpAddr); err != nil {
			jumpAddr = net.JoinHostPort(jumpAddr, "22")
		}
		jump, err := Dial(ctx, jumpAddr, DialOptions{
			User:           opts.User,
			Password:       opts.Password,
			KeyFile:        opts.KeyFile,
			KeyContent:     opts.KeyContent,
			ConnectTimeout: opts.ConnectTimeout,
		})
		if err != nil {
			return nil, fmt.Errorf("跳板机 %s 连接失败: %w", jumpAddr, err)
		}
		forwarded, err := jump.Dial("tcp", addr)
		if err != nil {
			jump.Close()
			return nil, connErr(addr, fmt.Errorf("经跳板转发到 %s 失败: %w", addr, err))
		}
		client, chans, reqs, err := gossh.NewClientConn(forwarded, addr, config)
		if err != nil {
			forwarded.Close()
			jump.Close()
			return nil, connErr(addr, err)
		}
		go gossh.DiscardRequests(reqs)
		_ = chans
		return &Client{Client: client, jump: jump.Client}, nil
	}

	netConn, err := dialTCP(ctx, addr, timeout)
	if err != nil {
		return nil, connErr(addr, err)
	}
	client, chans, reqs, err := gossh.NewClientConn(netConn, addr, config)
	if err != nil {
		netConn.Close()
		return nil, connErr(addr, err)
	}
	go gossh.DiscardRequests(reqs)
	_ = chans
	return &Client{Client: client}, nil
}

func dialTCP(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, "tcp", addr)
}

func connErr(addr string, cause error) *ConnectionError {
	errType := ErrorTypeConnection
	msg := cause.Error()
	if containsAnySSH(msg, "auth", "password", "key", "permission", "authentication",
		"no supported methods", "unable to authenticate") {
		errType = ErrorTypeAuth
	}
	return &ConnectionError{NodeID: addr, ErrorType: errType, Stderr: msg, Cause: cause}
}

func buildDialAuth(opts DialOptions) []gossh.AuthMethod {
	var auths []gossh.AuthMethod

	if opts.KeyFile != "" {
		if signers, err := loadKeyFile(opts.KeyFile); err == nil && len(signers) > 0 {
			auths = append(auths, gossh.PublicKeys(signers...))
		}
	}
	if opts.KeyContent != "" {
		if signer, err := gossh.ParsePrivateKey([]byte(opts.KeyContent)); err == nil {
			auths = append(auths, gossh.PublicKeys(signer))
		}
	}
	if opts.Password != "" {
		auths = append(auths, gossh.Password(opts.Password))
		auths = append(auths, gossh.KeyboardInteractive(
			func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = opts.Password
				}
				return answers, nil
			}))
	}
	if len(auths) == 0 {
		if signers := tryDefaultKeys(); len(signers) > 0 {
			auths = append(auths, gossh.PublicKeys(signers...))
		}
	}
	return auths
}
```

- [ ] **Step 5: 重构 native_executor.go 复用包级函数**

把 `native_executor.go` 中三处改为包级函数并复用 Dial：

1. `buildAuthMethods/ loadKeyFile/ tryDefaultKeys` 从方法改为包级函数（签名加 `ci *ConnectionInfo` 或 `opts`）：

```go
func loadKeyFile(keyPath string) ([]gossh.Signer, error) { /* 原逻辑不变 */ }
func tryDefaultKeys() []gossh.Signer { /* 原逻辑不变，loadKeyFile 调用去掉接收者 */ }
```

2. `NativeNodeExecutor.execute` 的连接建立部分（原 :102-142）替换为：

```go
func (e *NativeNodeExecutor) execute(command string, dialTimeout, commandTimeout time.Duration) (int, string, error) {
	addr := fmt.Sprintf("%s:%d", e.connInfo.Address, e.connInfo.Port)

	client, err := Dial(context.Background(), addr, DialOptions{
		User:           e.connInfo.GetUser(),
		Password:       e.connInfo.Password,
		KeyFile:        e.connInfo.KeyFile,
		KeyContent:     e.connInfo.KeyContent,
		ProxyJump:      e.connInfo.ProxyJump,
		ConnectTimeout: dialTimeout,
	})
	if err != nil {
		return -1, "", err
	}
	defer client.Close()
	// ...以下 session/exec 逻辑不变
```

（错误已是 `*SSHAuthError`/`*ConnectionError` 类型，删除原 containsAnySSH 分类块。）

3. `WriteFile`（:32-89）同样替换 `gossh.Dial` 为共享 `Dial`，删除重复分类逻辑。

- [ ] **Step 6: 工厂与连接池传 ProxyJump**

`executor_factory.go` 的 GetExecutorForNode 签名与实现：

```go
func (f *NodeExecutorFactory) GetExecutorForNode(nodeID, nodeAddress string, nodePort int, nodeUser, nodeKeyFile, nodePassword, proxyJump string) (NodeExecutor, error) {
	if isLocalNode(nodeAddress) {
		return &LocalNodeExecutor{}, nil
	}
	connInfo, err := ResolveConnection(nodeID, nodeAddress, nodePort, nodeUser, nodeKeyFile, nodePassword, f.sshConfigPath)
	if err != nil {
		return nil, err
	}
	if proxyJump != "" {
		connInfo.ProxyJump = proxyJump
	}
	return &NativeNodeExecutor{connInfo: connInfo}, nil
}
```

`connection_pool.go:55-62` 调用改为追加 `nodeInfo.ProxyJump` 参数。

全仓检查其他调用点：Run `rg -n "GetExecutorForNode" --type go`，把其余调用点（若有，如 file transfer/session）同步加参数（可从上下文取 ProxyJump，无则传 `""`）。

- [ ] **Step 7: 运行测试（CLI 门禁）**

Run: `go test ./internal/ssh/ ./internal/control/... ./cmd/cli/... ./pkg/... -v`
Expected: PASS（含 5 个新 Dial 测试与既有 timeout_test 等）

- [ ] **Step 8: Commit**

```bash
git add internal/ssh/
git commit -m "feat(ssh): unified Dial with timeout, ProxyJump and shared auth chain"
```

---

### Task 5: serve sshExecutor 改用共享 Dial

**Files:**
- Modify: `cmd/plugins/serve/handler/ssh_executor.go`（整体重写 dial/auth，保留 Execute/ExecuteStream 语义）
- Test: `cmd/plugins/serve/handler/ssh_executor_test.go`

**Interfaces:**
- Consumes: Task 4 的 `owlssh.Dial/DialOptions/Client`（import alias `owlssh "github.com/cangyunye/go-owl/internal/ssh"`）
- 不变: `Executor` 接口（`exec.go:28-31`）、`OutputLine`、逐行流式语义

- [ ] **Step 1: 写失败测试**

```go
// cmd/plugins/serve/handler/ssh_executor_test.go
package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNodeInfo_WithProxyJump(t *testing.T) {
	db := nodeSelectTestDB(t)
	db.Exec(`UPDATE nodes SET proxy_jump = 'jump:2222', password = 'pw' WHERE id = 'n1'`)
	e := &sshExecutor{db: db}
	info, err := e.getNodeInfo("n1")
	require.NoError(t, err)
	assert.Equal(t, "jump:2222", info.ProxyJump)
	assert.Equal(t, "pw", info.Password)
	assert.Equal(t, "root", info.User)
}

func TestGetNodeInfo_NotFound(t *testing.T) {
	db := nodeSelectTestDB(t)
	e := &sshExecutor{db: db}
	_, err := e.getNodeInfo("ghost")
	require.Error(t, err)
}

func TestDialNode_BadAddress(t *testing.T) {
	db := nodeSelectTestDB(t)
	db.Exec(`UPDATE nodes SET address = '10.255.255.1' WHERE id = 'n1'`)
	e := &sshExecutor{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 2*1000*1000*1000)
	defer cancel()
	_, err := e.dialNode(ctx, "n1")
	require.Error(t, err)
}
```

（复用 Task 3 的 `nodeSelectTestDB`。）

- [ ] **Step 2: 运行确认失败**

Run（workdir `cmd/plugins/serve`）: `go test ./handler/ -run 'TestGetNodeInfo|TestDialNode'`
Expected: FAIL（dialNode 未定义 / ProxyJump 字段缺失）

- [ ] **Step 3: 重写 `ssh_executor.go`**

```go
package handler

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	owlssh "github.com/cangyunye/go-owl/internal/ssh"
	gossh "golang.org/x/crypto/ssh"
)

const sshConnectTimeout = 10 * time.Second

type sshExecutor struct {
	db *sql.DB
}

type nodeSSHInfo struct {
	Address   string
	Port      int
	User      string
	Password  string
	SSHKey    string
	ProxyJump string
}

func (e *sshExecutor) getNodeInfo(nodeID string) (*nodeSSHInfo, error) {
	var info nodeSSHInfo
	var pw, key, jump sql.NullString
	err := e.db.QueryRow(
		`SELECT address, port, user, password, ssh_key, COALESCE(proxy_jump, '') FROM nodes WHERE id = ?`, nodeID,
	).Scan(&info.Address, &info.Port, &info.User, &pw, &key, &jump)
	if err != nil {
		return nil, err
	}
	if pw.Valid {
		info.Password = pw.String
	}
	if key.Valid {
		info.SSHKey = key.String
	}
	info.ProxyJump = jump.String
	return &info, nil
}

func (e *sshExecutor) dialNode(ctx context.Context, nodeID string) (*owlssh.Client, error) {
	info, err := e.getNodeInfo(nodeID)
	if err != nil {
		return nil, fmt.Errorf("resolve node: %w", err)
	}
	addr := info.Address + ":" + strconv.Itoa(info.Port)
	return owlssh.Dial(ctx, addr, owlssh.DialOptions{
		User:           info.User,
		Password:       info.Password,
		KeyContent:     info.SSHKey,
		ProxyJump:      info.ProxyJump,
		ConnectTimeout: sshConnectTimeout,
	})
}

func (e *sshExecutor) Execute(ctx context.Context, nodeID, command string) (string, int, error) {
	client, err := e.dialNode(ctx, nodeID)
	if err != nil {
		return "", -1, err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", -1, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*gossh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return "", -1, fmt.Errorf("ssh exec: %w", err)
		}
	}
	return string(output), exitCode, nil
}

func (e *sshExecutor) ExecuteStream(ctx context.Context, nodeID, command string, outputCh chan<- OutputLine) (int, error) {
	client, err := e.dialNode(ctx, nodeID)
	if err != nil {
		return -1, err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return -1, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return -1, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return -1, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := session.Start(command); err != nil {
		return -1, fmt.Errorf("ssh start: %w", err)
	}

	done := make(chan struct{}, 2)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case outputCh <- OutputLine{NodeID: nodeID, Line: scanner.Text(), Type: "stdout"}:
			case <-ctx.Done():
				done <- struct{}{}
				return
			}
		}
		done <- struct{}{}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			select {
			case outputCh <- OutputLine{NodeID: nodeID, Line: scanner.Text(), Type: "stderr"}:
			case <-ctx.Done():
				done <- struct{}{}
				return
			}
		}
		done <- struct{}{}
	}()

	err = session.Wait()
	<-done
	<-done

	exitCode := 0
	if exitErr, ok := err.(*gossh.ExitError); ok {
		exitCode = exitErr.ExitStatus()
		err = nil
	}
	return exitCode, err
}
```

- [ ] **Step 4: 运行测试**

Run（workdir `cmd/plugins/serve`）: `go test ./handler/ ./store/ ./...`
Expected: PASS（含既有 16 个 handler 测试文件）

CLI 门禁：Run（仓库根）: `go test ./cmd/cli/... ./internal/... ./pkg/...` → PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/plugins/serve/handler/ssh_executor.go cmd/plugins/serve/handler/ssh_executor_test.go
git commit -m "refactor(serve): ssh executor uses shared Dial with timeout and ProxyJump"
```

---

### Task 6: `blacklist.CheckForExec`

**Files:**
- Create: `internal/control/blacklist/check_exec.go`
- Test: `internal/control/blacklist/check_exec_test.go`

**Interfaces:**
- Produces:
  - `type BlockedError struct { Result *CheckResult }`（实现 error）
  - `func (c *Checker) CheckForExec(user, command string, force bool) (*CheckResult, error)`

- [ ] **Step 1: 写失败测试**

```go
// internal/control/blacklist/check_exec_test.go
package blacklist

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckForExec_BlockedCommands(t *testing.T) {
	c := NewDefaultChecker()

	_, err := c.CheckForExec("root", "rm -rf /var/log", false)
	require.Error(t, err)

	var blocked *BlockedError
	require.ErrorAs(t, err, &blocked)
	assert.NotEmpty(t, blocked.Result.Matches)
	assert.Equal(t, "root", blocked.Result.User)
}

func TestCheckForExec_ForceAllows(t *testing.T) {
	c := NewDefaultChecker()

	result, err := c.CheckForExec("root", "rm -rf /var/log", true)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Blocked, "force 放行时仍需返回匹配详情供审计")
}

func TestCheckForExec_SafeCommand(t *testing.T) {
	c := NewDefaultChecker()

	result, err := c.CheckForExec("root", "uptime", false)
	require.NoError(t, err)
	assert.False(t, result.Blocked)
}

func TestCheckForExec_UserScope(t *testing.T) {
	c := NewDefaultChecker()

	// sudo 规则仅对 root 用户生效
	_, err := c.CheckForExec("www-data", "echo hello", false)
	require.NoError(t, err)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/control/blacklist/ -run TestCheckForExec`
Expected: FAIL，编译错误（CheckForExec/BlockedError 未定义）

- [ ] **Step 3: 实现**

```go
// internal/control/blacklist/check_exec.go
package blacklist

import (
	"fmt"
	"strings"
)

// BlockedError 命令命中黑名单且未获 force 放行。
type BlockedError struct {
	Result *CheckResult
}

func (e *BlockedError) Error() string {
	var lines []string
	for _, m := range e.Result.Matches {
		lines = append(lines, fmt.Sprintf("%q 匹配规则 %q", m.Line, m.Pattern))
	}
	return fmt.Sprintf("危险命令已被黑名单拦截: %s", strings.Join(lines, "; "))
}

// CheckForExec 供 API 场景使用：
// force=false 且命中时返回 *BlockedError；force=true 时放行，
// 但仍返回 CheckResult（Blocked=true）供调用方审计记录。
func (c *Checker) CheckForExec(user, command string, force bool) (*CheckResult, error) {
	result := c.Check(user, command)
	if result.Blocked && !force {
		return result, &BlockedError{Result: result}
	}
	return result, nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/control/blacklist/ -v`
Expected: PASS（含既有 checker 测试）

- [ ] **Step 5: Commit**

```bash
git add internal/control/blacklist/
git commit -m "feat(blacklist): add CheckForExec API for serve integration"
```

---

### Task 7: serve 三个执行入口接入黑名单

**Files:**
- Modify: `cmd/plugins/serve/handler/exec.go`（Create 内加检查 + `danger_confirmed` 字段 + 记录 Forced 于 Task 8）
- Modify: `cmd/plugins/serve/handler/playbook_engine.go`（webCommandExecutor 加检查）
- Modify: `cmd/plugins/serve/handler/aiexecutor.go`（ExecuteCommand/ExecuteScript 前检查）
- Modify: `cmd/plugins/serve/model/playbook.go`（PlaybookRun +DangerConfirmed）、`cmd/plugins/serve/handler/playbook.go`（runRequest +DangerConfirmed 并传递）、`cmd/plugins/serve/store/playbook_run.go`（持久化字段——若该 store 以 JSON 存整个 run 对象，仅需模型加字段即可，先看文件再定）
- Test: `cmd/plugins/serve/handler/blacklist_integration_test.go`

**Interfaces:**
- Consumes: Task 6 `blacklist.CheckForExec/BlockedError`、Task 3 `dbNodeSource`

**约定：**
- API 字段 `"danger_confirmed": true`（bool）。禁止复用现有 `"force"`（语义为任务合并冲突覆盖，见 Global Constraints）。
- 命中拒绝响应：`403 {"code":403, "message":"<BlockedError.Error()>", "blocked":true, "matches":[{node,pattern,line}]}`

- [ ] **Step 1: 写失败测试（exec 入口）**

```go
// cmd/plugins/serve/handler/blacklist_integration_test.go
package handler

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecCreate_DangerousCommandBlocked(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id": "test-node",
		"command": "rm -rf /var/data",
	})
	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "blocked")
}

func TestExecCreate_DangerousCommandConfirmed(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id":          "test-node",
		"command":          "rm -rf /var/data",
		"danger_confirmed": true,
	})
	require.NotEqual(t, http.StatusForbidden, w.Code)
}

func TestExecCreate_SafeCommandNotBlocked(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id": "test-node",
		"command": "uptime",
	})
	require.NotEqual(t, http.StatusForbidden, w.Code)
}
```

注：execTestSetup 的 test-node user 为 `root`，命中默认 root 规则 `rm -rf `。

- [ ] **Step 2: 运行确认失败**

Run（workdir `cmd/plugins/serve`）: `go test ./handler/ -run TestExecCreate_Dangerous`
Expected: FAIL（无拦截，Blocked 场景返回 2xx/202）

- [ ] **Step 3: exec.go 接入**

3a. execRequest 加字段：

```go
	DangerConfirmed bool `json:"danger_confirmed"`
```

3b. ExecHandler 加 checker：

```go
type ExecHandler struct {
	db      *sql.DB
	task    *store.TaskStore
	exec    Executor
	hub     *WSHub
	History *store.HistoryStore
	checker *blacklist.Checker
}

func NewExecHandler(db *sql.DB, ts *store.TaskStore, hub *WSHub) *ExecHandler {
	cfg, err := blacklist.LoadConfig()
	if err != nil {
		log.Printf("load blacklist config: %v (using defaults)", err)
		cfg = &blacklist.Config{Rules: blacklist.DefaultRules()}
	}
	return &ExecHandler{
		db:      db,
		task:    ts,
		exec:    &sshExecutor{db: db},
		hub:     hub,
		checker: blacklist.NewChecker(cfg),
	}
}
```

import 增加 `"github.com/cangyunye/go-owl/internal/control/blacklist"`。

3c. Create 中，在 nodeIDs 解析成功、command 确定之后（现 :250 附近，cfg 构造之前）插入检查。script 模式检查 scriptContent，command 模式检查 command：

```go
	checkCmd := command
	if isScript {
		checkCmd = scriptContent
	}
	users := map[string]string{}
	if rows, err := h.db.QueryContext(c.Request.Context(),
		`SELECT id, user FROM nodes`); err == nil {
		for rows.Next() {
			var id, user string
			if rows.Scan(&id, &user) == nil {
				users[id] = user
			}
		}
		rows.Close()
	}

	type blockedMatch struct {
		Node    string `json:"node"`
		Pattern string `json:"pattern"`
		Line    string `json:"line"`
	}
	var blocked []blockedMatch
	for _, nid := range nodeIDs {
		result, err := h.checker.CheckForExec(users[nid], checkCmd, req.DangerConfirmed)
		if err != nil {
			for _, m := range result.Matches {
				blocked = append(blocked, blockedMatch{Node: nid, Pattern: m.Pattern, Line: m.Line})
			}
		}
	}
	if len(blocked) > 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "危险命令已被黑名单拦截; 如确需执行请带 danger_confirmed=true 重新提交",
			"blocked": true,
			"matches": blocked,
		})
		return
	}
```

- [ ] **Step 4: playbook_engine.go 接入**

4a. webCommandExecutor 加字段与检查：

```go
type webCommandExecutor struct {
	ssh   *sshExecutor
	check *blacklist.Checker
	force bool
}

func (e *webCommandExecutor) ExecuteOnNode(nodeID string, cmd string, timeout time.Duration) (*task.TaskResult, error) {
	if e.check != nil {
		var user string
		if info, err := e.ssh.getNodeInfo(nodeID); err == nil {
			user = info.User
		}
		if _, err := e.check.CheckForExec(user, cmd, e.force); err != nil {
			now := time.Now()
			return &task.TaskResult{
				NodeID: nodeID, ExitCode: -1, Error: err,
				Output: err.Error(), StartTime: now, EndTime: now,
			}, err
		}
	}
	/* ...原有逻辑不变... */
```

4b. `executePlaybookRunV2`（`playbook_engine.go:170` 附近）构造改为：

```go
	cmdExec := &webCommandExecutor{
		ssh:   &sshExecutor{db: h.db},
		check: h.checker,
		force: run.DangerConfirmed,
	}
```

PlaybookHandler 加 `checker *blacklist.Checker` 字段，构造函数用 `blacklist.LoadConfig()` 初始化（模式同 ExecHandler；找到 NewPlaybookHandler 的构造函数位置照抄该模式）。

4c. model/playbook.go 的 PlaybookRun 加：

```go
	DangerConfirmed bool `json:"danger_confirmed,omitempty"`
```

playbook.go 的 runRequest 加同名字段；Run handler 中创建 run 后 `run.DangerConfirmed = req.DangerConfirmed`（若 `h.runs.Create` 不接受该字段，则在 Create 返回后通过现有 Update 方法或直接在 Create 调用前设置——先读 `store/playbook_run.go` 的 Create 签名决定最小改法；若 store 以 JSON 保存整个 PlaybookRun，仅需在保存前赋值）。

- [ ] **Step 5: aiexecutor.go 接入**

`ExecuteCommand`（:141-200 附近）与 `ExecuteScript`（:202-240 附近）在执行循环前各插入：

```go
	if e.checker != nil {
		for _, nodeID := range nodeIDs {
			var user string
			if info, err := (&sshExecutor{db: e.db}).getNodeInfo(nodeID); err == nil {
				user = info.User
			}
			if _, err := e.checker.CheckForExec(user, params.Command, false); err != nil {
				return &ai2.ExecResult{Text: err.Error()}, nil
			}
		}
	}
```

（ExecuteScript 处把 `params.Command` 换成实际执行的 `execCmd`。WebExecutor 结构体加 `checker *blacklist.Checker`，在构造处初始化；AI 场景不提供 force 放行——AI 自动生成的危险命令必须被拦。）

- [ ] **Step 6: 运行测试**

Run（workdir `cmd/plugins/serve`）: `go test ./...`
Expected: PASS（新增 3 个测试 + 既有全绿；playbook_test.go 若有未初始化 checker 的面板构造路径，checker 为 nil 时跳过检查的分支保证兼容）

CLI 门禁：Run（仓库根）: `go test ./cmd/cli/... ./internal/... ./pkg/...` → PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/plugins/serve/
git commit -m "feat(serve): enforce command blacklist on exec/playbook/AI endpoints"
```

---

### Task 8: 审计 Forced 标记（operations 表）

**Files:**
- Modify: `internal/history/history.go`（Operation +Forced）、`internal/history/db_sqlite3.go`（schema + RecordOperation）、duckdb 版若存在同构修改（`internal/history/db_duckdb.go`）
- Modify: `cmd/plugins/serve/store/history.go`（Operation +Forced、schema、RecordOperation）
- Modify: `cmd/plugins/serve/handler/exec.go`（danger_confirmed 放行时 op.Forced = true，:349 附近）
- Test: `internal/history/forced_migration_test.go`、serve store 测试

**约定：** 新列 `forced INTEGER DEFAULT 0`。sqlite 无 `ADD COLUMN IF NOT EXISTS`，用 `PRAGMA table_info(operations)` 探测后 ALTER。两侧 CREATE TABLE 必须逐字同步（store/history.go 头部注释的既有约束）。

- [ ] **Step 1: 写迁移失败测试**

```go
// internal/history/forced_migration_test.go
package history

import (
	"testing"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/require"
)

func TestForcedColumnMigration_LegacyDB(t *testing.T) {
	cfg := &Config{DBPath: t.TempDir() + "/owl.db"}
	db, err := NewDB(cfg)
	require.NoError(t, err)

	// 模拟旧库：直接删掉 forced 列（重建无该列的表）
	conn := db.backend.Connection()
	_, err = conn.Exec(`DROP TABLE operations`)
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE operations (
		id INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT, op_type TEXT,
		command TEXT, targets TEXT, status TEXT,
		execution_mode TEXT DEFAULT '', playbook_path TEXT DEFAULT '',
		current_task_index INTEGER DEFAULT 0, current_task_phase TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	require.NoError(t, db.EnsureForcedColumn())

	require.NoError(t, db.RecordOperation(&Operation{
		TaskID: "t1", OpType: "command", Command: "uptime",
		Targets: []string{"n1"}, Status: "running", Forced: true,
	}))
	var forced int
	require.NoError(t, conn.QueryRow(
		`SELECT forced FROM operations WHERE task_id = 't1'`).Scan(&forced))
	require.Equal(t, 1, forced)
	db.Close()
}
```

（若 `*DB` 无 `backend`/`Close`/`EnsureForcedColumn` 访问方式，按实际结构调整测试——先读 `internal/history/interface.go` 与 `db.go` 确认可用入口；语义不变。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/history/ -run TestForcedColumn`
Expected: FAIL（EnsureForcedColumn/Forced 未定义）

- [ ] **Step 3: 实现（internal/history）**

3a. `history.go` Operation 加 `Forced bool`。

3b. `db_sqlite3.go` CREATE TABLE operations 增加一行：`forced INTEGER DEFAULT 0,`（放在 current_task_phase 之后）。

3c. 新增迁移方法（db_sqlite3.go 内）：

```go
// EnsureForcedColumn 为存量库补充 operations.forced 列（幂等）。
func (s *SQLite3) EnsureForcedColumn() error {
	rows, err := s.conn.Query(`PRAGMA table_info(operations)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "forced" {
			return nil
		}
	}
	_, err = s.conn.Exec(`ALTER TABLE operations ADD COLUMN forced INTEGER DEFAULT 0`)
	return err
}
```

在 `InitSchema()` 末尾调用 `s.EnsureForcedColumn()`（忽略"表不存在"错误路径由 IF NOT EXISTS 语义保证）。

3d. RecordOperation insert 增加 forced 列（0/1）。

- [ ] **Step 4: serve 侧同步（store/history.go）**

store.Operation 加 `Forced bool \`json:"forced"\``；CREATE TABLE 加同名列；InitSchema 等价位置加同样的 PRAGMA+ALTER 逻辑；RecordOperation insert 加列。**两侧 schema 必须逐字一致**（文件头注释约束）。

- [ ] **Step 5: exec.go 记录 Forced**

`exec.go:349` 构造 op 处：

```go
		op := &store.Operation{TaskID: opID, OpType: opType, Command: command, Targets: opTargets, Status: "running", CreatedAt: time.Now().UTC(), Forced: req.DangerConfirmed}
```

- [ ] **Step 6: 运行测试**

Run（仓库根）: `go test ./internal/history/ ./cmd/cli/...`
Run（workdir `cmd/plugins/serve`）: `go test ./...`
Expected: PASS。注意 history 相关既有测试（如 CLI history list）不受影响。

- [ ] **Step 7: Commit**

```bash
git add internal/history/ cmd/plugins/serve/
git commit -m "feat(history): record forced flag when blacklist override is used"
```

---

### Task 9: 清理假测试 + 全量验证

**Files:**
- Delete: `tests/unit/command_test.go`、`tests/unit/variable_test.go`
- Modify: `tests/README.md`（如有指向上面两个文件的映射描述，同步删除）

- [ ] **Step 1: 确认两个文件不测生产代码后删除**

Run: `head -30 tests/unit/command_test.go tests/unit/variable_test.go`
Expected: 确认其内部自定义 parseNodeList/truncateString 等副本（不 import 生产包），与 spec 结论一致后删除：

```bash
git rm tests/unit/command_test.go tests/unit/variable_test.go
```

- [ ] **Step 2: CLI 全量门禁（CLI 优先）**

Run: `go vet ./... && go build ./... && go test ./cmd/cli/... ./internal/... ./pkg/...`
Expected: 全部 PASS

- [ ] **Step 3: serve 全量**

Run（workdir `cmd/plugins/serve`）: `go vet ./... && go build ./... && go test ./...`
Expected: 全部 PASS

- [ ] **Step 4: owl-serve 模块编译确认**

Run（仓库根，go.work 内）: `go build ./...`
Expected: PASS（三模块全编译）

- [ ] **Step 5: Commit**

```bash
git add -A tests/unit/ tests/README.md
git commit -m "test: remove placeholder unit tests that cover no production code"
```

---

## Self-Review 记录

- **Spec 覆盖**：§1 选择 → Task 1-3；§2 拨号 → Task 4-5；§3 黑名单 → Task 6-7；§3 审计 Forced → Task 8；§4 错误处理 → Task 3/5（错误整体化返回、ConnectionError）+ Global Constraints 记录 code/message 偏离；§5 测试策略 → 各 Task TDD + Task 9 删除假测试；§6 范围外 → 未包含 ✓
- **类型一致性**：NodeRow/SelectOptions/Selector（Task 1 定义，Task 2/3 消费一致）；Dial/DialOptions/Client（Task 4 定义，Task 5 消费一致）；CheckForExec/BlockedError（Task 6 定义，Task 7 消费一致）；Forced（Task 8 双侧同名）✓
- **偏离记录**：danger_confirmed 替代 spec 的 force（Global Constraints 已述原因）；错误响应沿用 code/message ✓
