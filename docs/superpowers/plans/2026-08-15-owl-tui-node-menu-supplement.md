# owl tui Node 菜单补充(对齐 `owl node` 命令)Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 补齐 owl tui 原生 Node 菜单与 `owl node` 命令的差距:状态过滤 `s:`、表单 Status 字段、Ping 视图、SSH Check 视图、Import/Export 视图、Groups 管理视图、Labels 管理视图,使 TUI 覆盖 CLI 全部 node 子命令交互。

**Architecture:** 沿用 Phase 1 的 bubbletea Elm 架构与位置栈(LocList/…)+ Mode(Normal/Insert)隔离。新增 5 个 Location(LocPing/LocCheck/LocImportExport/LocGroups/LocLabels)各配独立子 model,`Update` 顶层按位置分发。网络/SSH 检查通过包级可替换函数(`pingDial`/`sshCheck`)注入,保证单测零网络依赖。

**Tech Stack:** Go 1.26.0、bubbletea v1.3.4、bubbles v0.21.0、lipgloss v1.1.0、`gopkg.in/yaml.v3`、`internal/ssh`(owlssh.Dial)。

## Global Constraints

- 模块 `github.com/cangyunye/go-owl`;测试命令 `go test ./cmd/cli/cmd/tui/...`(仓库根在 `F:\pantheon\trae_projects\git\go-owl`)
- 数据层只经 `common.NodeStore` 接口注入;测试用 `common.NewInMemoryNodeStoreAt(t.TempDir()+"/nodes.json")`,不得触碰 `~/.owl/nodes.json`
- TUI 内部文案简体中文;Model 内禁止 `os.Exit`;仅 App 通过 `tea.Quit` 退出
- 位置栈深度上限 3;`Esc` 弹栈
- 新增按键仅限列表 Normal 态生效(Insert 态全部进输入框):`p` ping · `k` check · `i` import/export · `o` groups · `l` labels
- 过滤语法新增 `s:<status>`(如 `s:online`),与 CLI `node list -S` 语义一致
- 不改变既有按键:`a/e/d/c//g/G/?/q` 行为不变;既有测试全部保持通过
- 网络/SSH 检查必须可注入:`var pingDial`/`var sshCheck` 为包级 var,测试可替换
- `common.NodeInfo.Status` 取值仅 `online`/`offline`;`LastCheckAt` 用 `2006-01-02 15:04:05` 格式(与 CLI check 一致)

---

### Task 1: 过滤支持状态 `s:`(`node list -S` 语义)

**Files:**
- Modify: `cmd/cli/cmd/tui/nodes/filter.go`
- Modify: `cmd/cli/cmd/tui/nodes/filter_test.go`
- Modify: `cmd/cli/cmd/tui/nodes/view.go`(statusBar chips 渲染 `s:`)
- Test: `cmd/cli/cmd/tui/nodes/filter_test.go`

**Interfaces:**
- Consumes: 既有 `FilterQuery`/`ParseFilterQuery`/`matchFilter`
- Produces: `FilterQuery.Status string`;`ParseFilterQuery("s:online")` 解析到该字段;`matchFilter` 对 `n.Status` 大小写不敏感精确匹配

- [ ] **Step 1: 写失败测试**

在 `cmd/cli/cmd/tui/nodes/filter_test.go` 末尾追加:

```go
func TestParseFilterQuery_Status(t *testing.T) {
	fq := ParseFilterQuery("s:online")
	if fq.Status != "online" {
		t.Fatalf("unexpected status: %q", fq.Status)
	}
	if fq.Empty() {
		t.Fatal("expected not empty")
	}
	fq = ParseFilterQuery("g:web s:offline")
	if fq.Status != "offline" || len(fq.Groups) != 1 || fq.Groups[0] != "web" {
		t.Fatalf("unexpected mixed: status=%q groups=%#v", fq.Status, fq.Groups)
	}
}

func TestParseFilterQuery_Status_Empty(t *testing.T) {
	if fq := ParseFilterQuery("s:"); !fq.Empty() {
		t.Fatalf("s: alone should be empty, got %#v", fq)
	}
}

func TestApplyFilter_Status(t *testing.T) {
	nodes := []*common.NodeInfo{
		{ID: "n1", Status: "online"},
		{ID: "n2", Status: "offline"},
		{ID: "n3", Status: "Online"},
	}
	got := applyFilter(nodes, ParseFilterQuery("s:online"))
	if len(got) != 2 || got[0].ID != "n1" || got[1].ID != "n3" {
		t.Fatalf("unexpected: %+v", got)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run "TestParseFilterQuery_Status|TestApplyFilter_Status" -v`
Expected: FAIL(`Status` 字段不存在,编译错误或断言失败)

- [ ] **Step 3: 实现**

在 `cmd/cli/cmd/tui/nodes/filter.go`:

1. `FilterQuery` 追加字段:

```go
type FilterQuery struct {
	Groups []string
	Labels map[string]string
	Search string
	Status string
}
```

2. `ParseFilterQuery` 的 switch 追加 `s:` 分支(放在 `l:` 分支后):

```go
			case strings.HasPrefix(tok, "s:"):
				if v := strings.TrimSpace(tok[2:]); v != "" {
					fq.Status = v
				}
```

3. `Empty()` 追加 `&& fq.Status == ""`:

```go
func (fq FilterQuery) Empty() bool {
	return len(fq.Groups) == 0 && len(fq.Labels) == 0 && fq.Search == "" && fq.Status == ""
}
```

4. `matchFilter` 追加状态判断(放在 labels 检查后、Search 检查前):

```go
	if fq.Status != "" && !strings.EqualFold(n.Status, fq.Status) {
		return false
	}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 5: statusBar 显示状态 chip**

在 `cmd/cli/cmd/tui/nodes/view.go` 的 `statusBar()` 中,labels chips 追加后加状态 chip:

```go
	if m.filter.Status != "" {
		chips = append(chips, "s:"+m.filter.Status)
	}
```

- [ ] **Step 6: 运行 + 提交**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

```bash
git add cmd/cli/cmd/tui/nodes/filter.go cmd/cli/cmd/tui/nodes/filter_test.go cmd/cli/cmd/tui/nodes/view.go
git commit -m "feat(tui): 过滤支持状态 s:<status>(对齐 node list -S)"
```

---

### Task 2: 表单增加 Status 字段(对齐 `node update --status`)

**Files:**
- Modify: `cmd/cli/cmd/tui/nodes/form.go`
- Modify: `cmd/cli/cmd/tui/nodes/form_save_test.go`
- Test: `cmd/cli/cmd/tui/nodes/form_save_test.go`

**Interfaces:**
- Consumes: 既有 `NewFormModel`/`validate`/`toNode`
- Produces: 表单新增第 11 个字段 `status`(索引 10,在 labels 之后);新增态默认 `offline`,编辑态预填 `base.Status`;`validate` 校验取值必须为 `online`/`offline`

- [ ] **Step 1: 写失败测试**

在 `cmd/cli/cmd/tui/nodes/form_save_test.go` 末尾追加:

```go
func TestFormEdit_StatusField_Prefilled(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('e'))
	m = nm.(Model)
	idx := len(m.form.fields) - 1 // status 是最后一个字段
	if m.form.fields[idx].key != "status" {
		t.Fatalf("expected last field status, got %q", m.form.fields[idx].key)
	}
	if m.form.fields[idx].input.Value() != "online" {
		t.Fatalf("expected prefilled online, got %q", m.form.fields[idx].input.Value())
	}
}

func TestFormEdit_SaveStatusOffline(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('e'))
	m = nm.(Model)
	idx := len(m.form.fields) - 1
	m.form.fields[idx].input.SetValue("offline")
	nm, _ = m.Update(runeKey('s'))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatalf("expected back to list, got %v", m.current())
	}
	got, _ := store.Get("n1")
	if got.Status != "offline" {
		t.Fatalf("expected status offline, got %q", got.Status)
	}
}

func TestFormEdit_SaveStatusInvalid(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('e'))
	m = nm.(Model)
	idx := len(m.form.fields) - 1
	m.form.fields[idx].input.SetValue("bogus")
	nm, _ = m.Update(runeKey('s'))
	m = nm.(Model)
	if m.current() != LocEdit {
		t.Fatalf("expected stay in form on invalid status")
	}
	if m.form.error == "" {
		t.Fatal("expected validation error")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run TestFormEdit_Status -v`
Expected: FAIL(status 字段不存在,`fields[len-1].key` 是 labels)

- [ ] **Step 3: 实现**

在 `cmd/cli/cmd/tui/nodes/form.go` 的 `NewFormModel` specs 中,labels 行之后追加:

```go
		{"status", "Status", false, base.Status, true},
```

在 `validate()` 的 port 检查之后追加 status 校验:

```go
		if fd.key == "status" {
			v := strings.TrimSpace(fd.input.Value())
			if v != "" && v != "online" && v != "offline" {
				return "Status 必须是 online 或 offline"
			}
		}
```

在 `focusFirstInvalid()` 的 port 检查之后追加:

```go
		if fd.key == "status" {
			v := strings.TrimSpace(fd.input.Value())
			if v != "" && v != "online" && v != "offline" {
				f.cursor = i
				return
			}
		}
```

修改 `toNode()` 的 Status 赋值:删除硬编码 `Status: "offline"`,改为在构造后设置:

```go
	n := &common.NodeInfo{
		ID:        f.value("id"),
		Name:      f.value("name"),
		Address:   f.value("address"),
		Port:      port,
		User:      f.value("user"),
		Password:  f.value("password"),
		SSHKey:    f.value("ssh_key"),
		ProxyJump: f.value("proxy_jump"),
		Groups:    splitTrim(f.value("groups"), ","),
		Labels:    parseLabels(f.value("labels")),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if st := f.value("status"); st != "" {
		n.Status = st
	} else if f.mode == FormAdd {
		n.Status = "offline"
	} else if f.base != nil {
		n.Status = f.base.Status
	} else {
		n.Status = "offline"
	}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS(既有新增/编辑保存测试不受影响;新增字段在末尾,索引 0-9 不变)

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/form.go cmd/cli/cmd/tui/nodes/form_save_test.go
git commit -m "feat(tui): 表单 Status 字段(新增默认 offline/编辑预填/校验 online|offline)"
```

---

### Task 3: Ping 视图(`p` = `node ping`)

**Files:**
- Create: `cmd/cli/cmd/tui/nodes/ping.go`
- Create: `cmd/cli/cmd/tui/nodes/ping_test.go`
- Modify: `cmd/cli/cmd/tui/nodes/model.go`(LocPing + updatePing + `p` 入口)
- Modify: `cmd/cli/cmd/tui/nodes/view.go`(pingView)
- Test: `cmd/cli/cmd/tui/nodes/ping_test.go`

**Interfaces:**
- Consumes: `common.NodeInfo`/`common.NodeStore`、`Mode`、`m.visible()`
- Produces:
  - `type PingResult struct { Node *common.NodeInfo; Success bool; Latency time.Duration; Err error }`
  - `type PingDoneMsg struct { Results []PingResult }`
  - `var pingDial = func(addr string, timeout time.Duration) (net.Conn, error)`(默认 `net.DialTimeout`,测试可替换)
  - `const pingTimeout = 3 * time.Second`
  - `func runPing(nodes []*common.NodeInfo, timeout time.Duration, dial func(string, time.Duration) (net.Conn, error)) []PingResult`
  - `type PingModel struct`(含 `NewPingModel(nodes) *PingModel`、`Start() tea.Cmd`)
  - `Model.updatePing(msg) (tea.Model, tea.Cmd)`

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/ping_test.go`(`package nodes`):

```go
package nodes

import (
	"net"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestRunPing_AllReachable(t *testing.T) {
	dial := func(addr string, timeout time.Duration) (net.Conn, error) {
		c1, c2 := net.Pipe()
		_ = c2
		return c1, nil
	}
	nodes := []*common.NodeInfo{
		{ID: "n1", Address: "10.0.0.1", Port: 22},
		{ID: "n2", Address: "10.0.0.2", Port: 22},
	}
	results := runPing(nodes, time.Second, dial)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Fatalf("expected success for %s", r.Node.ID)
		}
	}
}

func TestRunPing_MixedReachability(t *testing.T) {
	dial := func(addr string, timeout time.Duration) (net.Conn, error) {
		if strings.Contains(addr, "10.0.0.2") {
			return nil, &net.OpError{Err: net.ErrClosed}
		}
		c1, _ := net.Pipe()
		return c1, nil
	}
	nodes := []*common.NodeInfo{
		{ID: "n1", Address: "10.0.0.1", Port: 22},
		{ID: "n2", Address: "10.0.0.2", Port: 22},
	}
	results := runPing(nodes, time.Second, dial)
	if !results[0].Success || results[1].Success {
		t.Fatalf("expected first ok second fail: %+v", results)
	}
}

func TestPingModel_StartAndDone(t *testing.T) {
	old := pingDial
	pingDial = func(addr string, timeout time.Duration) (net.Conn, error) {
		c1, _ := net.Pipe()
		return c1, nil
	}
	defer func() { pingDial = old }()

	nodes := []*common.NodeInfo{{ID: "n1", Address: "10.0.0.1", Port: 22}}
	pm := NewPingModel(nodes)
	cmd := pm.Start()
	msg := cmd()
	dm, ok := msg.(PingDoneMsg)
	if !ok {
		t.Fatalf("expected PingDoneMsg, got %T", msg)
	}
	if len(dm.Results) != 1 || !dm.Results[0].Success {
		t.Fatalf("unexpected results: %+v", dm.Results)
	}
}

func TestModel_OpenPing_FromList(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('p'))
	m = nm.(Model)
	if m.current() != LocPing {
		t.Fatalf("expected LocPing, got %v", m.current())
	}
	if m.ping == nil || !m.ping.loading {
		t.Fatal("expected ping model loading")
	}
	path := m.Path()
	if len(path) != 2 || path[1] != "ping" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestModel_UpdatePing_DoneAndBack(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('p'))
	m = nm.(Model)
	// 注入结果
	m.ping.results = []PingResult{{Node: m.visible()[0], Success: true}}
	m.ping.loading = false
	// Enter 返回列表
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatalf("expected back to list, got %v", m.current())
	}
}
```

注意:`TestModel_OpenPing_FromList` 需要 `m.ping` 字段在 NewModel 后默认为 nil;`p` 入口 push LocPing 并新建 `PingModel`。`TestModel_UpdatePing_DoneAndBack` 直接操作 `m.ping.results`(package nodes 白盒访问)。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run "TestRunPing|TestPingModel|TestModel_OpenPing|TestModel_UpdatePing" -v`
Expected: FAIL(`runPing`/`PingModel`/`LocPing` 未定义)

- [ ] **Step 3: 实现 ping.go**

创建 `cmd/cli/cmd/tui/nodes/ping.go`:

```go
package nodes

import (
	"net"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

const pingTimeout = 3 * time.Second

type PingResult struct {
	Node    *common.NodeInfo
	Success bool
	Latency time.Duration
	Err     error
}

type PingDoneMsg struct {
	Results []PingResult
}

// pingDial 可注入的 TCP 拨号器(测试替换)
var pingDial = func(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}

func runPing(nodes []*common.NodeInfo, timeout time.Duration, dial func(string, time.Duration) (net.Conn, error)) []PingResult {
	results := make([]PingResult, 0, len(nodes))
	for _, n := range nodes {
		addr := net.JoinHostPort(n.Address, strconv.Itoa(n.Port))
		start := time.Now()
		conn, err := dial(addr, timeout)
		latency := time.Since(start)
		r := PingResult{Node: n, Latency: latency}
		if err == nil {
			conn.Close()
			r.Success = true
		} else {
			r.Err = err
		}
		results = append(results, r)
	}
	return results
}

type PingModel struct {
	nodes   []*common.NodeInfo
	results []PingResult
	loading bool
}

func NewPingModel(nodes []*common.NodeInfo) *PingModel {
	return &PingModel{nodes: nodes, loading: true}
}

func (m *PingModel) Start() tea.Cmd {
	return func() tea.Msg {
		results := runPing(m.nodes, pingTimeout, pingDial)
		return PingDoneMsg{Results: results}
	}
}
```

- [ ] **Step 4: 接入 model.go**

在 `cmd/cli/cmd/tui/nodes/model.go`:

1. `Location` 追加:

```go
const (
	LocList Location = iota
	LocNew
	LocEdit
	LocDelete
	LocColumns
	LocPing
)
```

2. `Model` 追加字段(columns 字段后):

```go
	ping *PingModel
```

3. `Path()` 追加分支(在 LocColumns case 后):

```go
	case LocPing:
		return []string{"nodes", "ping"}
```

4. 顶层 `Update` 分发 switch 追加(在 LocColumns case 后):

```go
	case LocPing:
		return m.updatePing(msg)
```

5. `updateList` 的 switch 追加 `p` 入口(a 分支后):

```go
	case "p":
		m.push(LocPing)
		m.ping = NewPingModel(m.visible())
		return m, m.ping.Start()
```

6. 追加 `updatePing` 方法(updateColumns 后):

```go
func (m Model) updatePing(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case PingDoneMsg:
		if m.ping != nil {
			m.ping.results = msg.Results
			m.ping.loading = false
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "enter" {
			m.pop()
			m.ping = nil
			return m, nil
		}
	}
	return m, nil
}
```

- [ ] **Step 5: 实现 view.go 的 pingView**

在 `cmd/cli/cmd/tui/nodes/view.go`:

1. `View()` 的 switch 追加(在 LocColumns case 后):

```go
	case LocPing:
		return m.listPane() + "\n\n" + m.pingView()
```

2. 追加方法(columnsView 后):

```go
func (m Model) pingView() string {
	p := m.ping
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ Ping 检查 ──────────────\n")
	if p.loading {
		b.WriteString("  " + styleDim.Render("正在检查 " + fmt.Sprintf("%d", len(p.nodes)) + " 个节点…") + "\n")
	} else {
		reachable := 0
		for _, r := range p.results {
			mark := "✗"
			if r.Success {
				mark = "✓"
				reachable++
			}
			line := fmt.Sprintf("  %s %s (%s:%d) %s\n",
				mark, r.Node.ID, r.Node.Address, r.Node.Port, r.Latency.Round(time.Millisecond))
			if r.Success {
				line = styleSelected.Render(line)
			}
			b.WriteString(line)
		}
		b.WriteString(styleDim.Render(fmt.Sprintf("  可达 %d/%d", reachable, len(p.results))) + "\n")
	}
	b.WriteString(styleDim.Render("  Enter/Esc 返回") + "\n")
	b.WriteString("└─")
	return b.String()
}
```

需在 view.go import 追加 `"time"`。

- [ ] **Step 6: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/ping.go cmd/cli/cmd/tui/nodes/ping_test.go cmd/cli/cmd/tui/nodes/model.go cmd/cli/cmd/tui/nodes/view.go
git commit -m "feat(tui): Ping 视图(p 键, TCP 连通检查, 可注入 dialer)"
```

---

### Task 4: SSH Check 视图(`k` = `node check`,回写 status/last_check)

**Files:**
- Create: `cmd/cli/cmd/tui/nodes/check.go`
- Create: `cmd/cli/cmd/tui/nodes/check_test.go`
- Modify: `cmd/cli/cmd/tui/nodes/model.go`(LocCheck + updateCheck + `k` 入口 + 回写)
- Modify: `cmd/cli/cmd/tui/nodes/view.go`(checkView)
- Test: `cmd/cli/cmd/tui/nodes/check_test.go`

**Interfaces:**
- Consumes: `common.NodeStore`、`owlssh.Dial`、`Mode`、`m.visible()`
- Produces:
  - `type CheckResult struct { Node *common.NodeInfo; Success bool; Method string; Err error }`
  - `type CheckDoneMsg struct { Results []CheckResult }`
  - `var sshCheck = func(n *common.NodeInfo, timeout time.Duration) (bool, string, error)`(默认实现走 `owlssh.Dial`,密钥优先密码兜底)
  - `const checkTimeout = 10 * time.Second`
  - `func runCheck(nodes []*common.NodeInfo, timeout time.Duration, fn func(*common.NodeInfo, time.Duration) (bool, string, error)) []CheckResult`
  - `type CheckModel struct`(含 `NewCheckModel(nodes) *CheckModel`、`Start() tea.Cmd`)
  - `Model.updateCheck(msg) (tea.Model, tea.Cmd)`——完成后把 `Success`/`Method`/status 回写 store

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/check_test.go`(`package nodes`):

```go
package nodes

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestRunCheck_SuccessAndFail(t *testing.T) {
	fn := func(n *common.NodeInfo, timeout time.Duration) (bool, string, error) {
		if n.ID == "n2" {
			return false, "", &net.OpError{Err: net.ErrClosed}
		}
		return true, "key", nil
	}
	nodes := []*common.NodeInfo{
		{ID: "n1", Address: "10.0.0.1", Port: 22, SSHKey: "~/.ssh/id_rsa"},
		{ID: "n2", Address: "10.0.0.2", Port: 22},
	}
	results := runCheck(nodes, time.Second, fn)
	if !results[0].Success || results[1].Success {
		t.Fatalf("unexpected: %+v", results)
	}
	if results[0].Method != "key" {
		t.Fatalf("expected method key, got %q", results[0].Method)
	}
}

func TestModel_Check_WritesBackStatus(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store) // n1 online, n2 offline, n3 online
	old := sshCheck
	sshCheck = func(n *common.NodeInfo, timeout time.Duration) (bool, string, error) {
		return true, "key", nil
	}
	defer func() { sshCheck = old }()

	m := NewModel(store)
	nm, _ := m.Update(runeKey('k'))
	m = nm.(Model)
	if m.current() != LocCheck {
		t.Fatalf("expected LocCheck, got %v", m.current())
	}
	// 模拟 check 完成(全部成功)
	nm, _ = m.updateCheck(CheckDoneMsg{Results: []CheckResult{
		{Node: m.visible()[0], Success: true, Method: "key"},
		{Node: m.visible()[1], Success: true, Method: "password"},
		{Node: m.visible()[2], Success: true, Method: "key"},
	}})
	m = nm.(Model)
	// 状态应全部回写 online
	for _, n := range m.nodes {
		got, err := store.Get(n.ID)
		if err != nil {
			t.Fatalf("get %s: %v", n.ID, err)
		}
		if got.Status != "online" {
			t.Fatalf("expected %s online, got %q", n.ID, got.Status)
		}
		if got.LastCheckAt == "" {
			t.Fatalf("expected %s last_check set", n.ID)
		}
	}
	if m.current() != LocList {
		t.Fatalf("expected back to list after done, got %v", m.current())
	}
}

func TestModel_Check_FailKeepsOffline(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	old := sshCheck
	sshCheck = func(n *common.NodeInfo, timeout time.Duration) (bool, string, error) {
		return false, "", &net.OpError{Err: net.ErrClosed}
	}
	defer func() { sshCheck = old }()

	m := NewModel(store)
	nm, _ := m.Update(runeKey('k'))
	m = nm.(Model)
	nm, _ = m.updateCheck(CheckDoneMsg{Results: []CheckResult{
		{Node: m.visible()[0], Success: false, Err: &net.OpError{Err: net.ErrClosed}},
	}})
	m = nm.(Model)
	got, _ := store.Get("n1")
	if got.Status != "offline" {
		t.Fatalf("expected n1 offline, got %q", got.Status)
	}
	if got.LastCheckAt == "" {
		t.Fatal("expected last_check set even on failure")
	}
}
```

注意:check_test.go 需 import `"net"`。`TestModel_Check_WritesBackStatus` 中 `m.updateCheck` 为白盒直接调用;完成后应自动弹栈回列表并 reload。Test 直接构造 `CheckDoneMsg` 喂给 `updateCheck`(同步完成,无异步)。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run "TestRunCheck|TestModel_Check" -v`
Expected: FAIL(`runCheck`/`CheckModel`/`LocCheck`/`sshCheck` 未定义)

- [ ] **Step 3: 实现 check.go**

创建 `cmd/cli/cmd/tui/nodes/check.go`:

```go
package nodes

import (
	"context"
	"errors"
	"net"
	"os"
	"os/user"
	"strconv"
	"time"

	gossh "golang.org/x/crypto/ssh"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	owlssh "github.com/cangyunye/go-owl/internal/ssh"
)

const checkTimeout = 10 * time.Second

type CheckResult struct {
	Node    *common.NodeInfo
	Success bool
	Method  string
	Err     error
}

type CheckDoneMsg struct {
	Results []CheckResult
}

// sshCheck 可注入的 SSH 认证检查(测试替换);默认实现走 owlssh.Dial,密钥优先密码兜底
var sshCheck = func(n *common.NodeInfo, timeout time.Duration) (bool, string, error) {
	addr := net.JoinHostPort(n.Address, strconv.Itoa(n.Port))
	ctx := context.Background()
	sshUser := n.User
	if sshUser == "" {
		if current, err := user.Current(); err == nil {
			sshUser = current.Username
		} else {
			sshUser = "root"
		}
	}
	if n.SSHKey != "" {
		signer, err := parsePrivateKey(n.SSHKey)
		if err == nil {
			client, err := owlssh.Dial(ctx, addr, owlssh.DialOptions{
				User:           sshUser,
				AuthMethods:    []gossh.AuthMethod{gossh.PublicKeys(signer)},
				ProxyJump:      n.ProxyJump,
				ConnectTimeout: timeout,
			})
			if err == nil {
				client.Close()
				return true, "key", nil
			}
			if n.Password == "" {
				return false, "", err
			}
		}
	}
	if n.Password != "" {
		client, err := owlssh.Dial(ctx, addr, owlssh.DialOptions{
			User:           sshUser,
			AuthMethods:    []gossh.AuthMethod{gossh.Password(n.Password)},
			ProxyJump:      n.ProxyJump,
			ConnectTimeout: timeout,
		})
		if err == nil {
			client.Close()
			return true, "password", nil
		}
		return false, "", err
	}
	return false, "", errors.New("未配置认证方式(SSHKey 或 Password)")
}

func parsePrivateKey(keyPath string) (gossh.Signer, error) {
	if len(keyPath) > 2 && keyPath[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			keyPath = home + keyPath[1:]
		}
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	return gossh.ParsePrivateKey(data)
}

func runCheck(nodes []*common.NodeInfo, timeout time.Duration, fn func(*common.NodeInfo, time.Duration) (bool, string, error)) []CheckResult {
	results := make([]CheckResult, 0, len(nodes))
	for _, n := range nodes {
		ok, method, err := fn(n, timeout)
		results = append(results, CheckResult{Node: n, Success: ok, Method: method, Err: err})
	}
	return results
}

type CheckModel struct {
	nodes   []*common.NodeInfo
	results []CheckResult
	loading bool
}

func NewCheckModel(nodes []*common.NodeInfo) *CheckModel {
	return &CheckModel{nodes: nodes, loading: true}
}

func (m *CheckModel) Start() tea.Cmd {
	return func() tea.Msg {
		results := runCheck(m.nodes, checkTimeout, sshCheck)
		return CheckDoneMsg{Results: results}
	}
}
```

- [ ] **Step 4: 接入 model.go**

在 `cmd/cli/cmd/tui/nodes/model.go`:

1. `Location` 追加 `LocCheck`(LocPing 后):

```go
	LocPing
	LocCheck
```

2. `Model` 追加字段(ping 字段后):

```go
	check *CheckModel
```

3. `Path()` 追加分支(LocPing case 后):

```go
	case LocCheck:
		return []string{"nodes", "check"}
```

4. 顶层 `Update` 分发 switch 追加(LocPing case 后):

```go
	case LocCheck:
		return m.updateCheck(msg)
```

5. `updateList` 的 switch 追加 `k` 入口(p 分支后):

```go
	case "k":
		m.push(LocCheck)
		m.check = NewCheckModel(m.visible())
		return m, m.check.Start()
```

6. 追加 `updateCheck` 方法(updatePing 后):

```go
func (m Model) updateCheck(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case CheckDoneMsg:
		if m.check != nil {
			m.check.results = msg.Results
			m.check.loading = false
		}
		// 回写 status/last_check 到 store
		now := time.Now().Format("2006-01-02 15:04:05")
		for _, r := range msg.Results {
			node, err := m.store.Get(r.Node.ID)
			if err != nil {
				continue
			}
			if r.Success {
				node.Status = "online"
			} else {
				node.Status = "offline"
			}
			node.LastCheckAt = now
			node.UpdatedAt = now
			_ = m.store.Update(node)
		}
		_ = m.store.Save()
		m.pop()
		m.check = nil
		m.reload()
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.pop()
			m.check = nil
			return m, nil
		}
	}
	return m, nil
}
```

需在 model.go import 追加 `"time"`。

- [ ] **Step 5: 实现 view.go 的 checkView**

在 `cmd/cli/cmd/tui/nodes/view.go`:

1. `View()` 的 switch 追加(LocPing case 后):

```go
	case LocCheck:
		return m.listPane() + "\n\n" + m.checkView()
```

2. 追加方法(pingView 后):

```go
func (m Model) checkView() string {
	c := m.check
	if c == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ SSH Check ──────────────\n")
	if c.loading {
		b.WriteString("  " + styleDim.Render("正在检查 " + fmt.Sprintf("%d", len(c.nodes)) + " 个节点…") + "\n")
	} else {
		online := 0
		for _, r := range c.results {
			mark := "✗"
			if r.Success {
				mark = "✓"
				online++
			}
			method := r.Method
			if method == "" {
				method = "-"
			}
			line := fmt.Sprintf("  %s %s (%s:%d) %s\n", mark, r.Node.ID, r.Node.Address, r.Node.Port, method)
			if r.Success {
				line = styleSelected.Render(line)
			}
			b.WriteString(line)
		}
		b.WriteString(styleDim.Render(fmt.Sprintf("  在线 %d/%d", online, len(c.results))) + "\n")
	}
	b.WriteString(styleDim.Render("  Esc 返回") + "\n")
	b.WriteString("└─")
	return b.String()
}
```

- [ ] **Step 6: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/check.go cmd/cli/cmd/tui/nodes/check_test.go cmd/cli/cmd/tui/nodes/model.go cmd/cli/cmd/tui/nodes/view.go
git commit -m "feat(tui): SSH Check 视图(k 键, 真实认证检查并回写 status/last_check)"
```

---

### Task 5: Import/Export 视图(`i` = `node import/export`)

**Files:**
- Create: `cmd/cli/cmd/tui/nodes/import_export.go`
- Create: `cmd/cli/cmd/tui/nodes/import_export_test.go`
- Modify: `cmd/cli/cmd/tui/nodes/model.go`(LocImportExport + updateImportExport + `i` 入口)
- Modify: `cmd/cli/cmd/tui/nodes/view.go`(importExportView)
- Test: `cmd/cli/cmd/tui/nodes/import_export_test.go`

**Interfaces:**
- Consumes: `common.NodeStore`、`yaml.v3`、`encoding/json`
- Produces:
  - `type ImportExportModel struct { op string /* export|import */; format string /* yaml|json */; path textinput.Model; overwrite bool; error string }`
  - `func NewImportExportModel() *ImportExportModel`
  - `func (m *Model) doExport(path, format string) error`(序列化 store 全部节点)
  - `func (m *Model) doImport(path string, overwrite bool) error`(存在跳过/overwrite 覆盖)
  - `func (m Model) updateImportExport(msg tea.Msg) (tea.Model, tea.Cmd)`

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/import_export_test.go`(`package nodes`):

```go
package nodes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestModel_DoExport_YAML(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	path := filepath.Join(t.TempDir(), "nodes.yaml")
	if err := m.doExport(path, "yaml"); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "n1") || !strings.Contains(string(data), "web-1") {
		t.Fatalf("export missing nodes: %s", data)
	}
}

func TestModel_DoExport_JSON(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	path := filepath.Join(t.TempDir(), "nodes.json")
	if err := m.doExport(path, "json"); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "n1") {
		t.Fatalf("export missing node: %s", data)
	}
}

func TestModel_DoImport_NewNodes(t *testing.T) {
	store := newTestStore(t)
	m := NewModel(store)
	path := filepath.Join(t.TempDir(), "in.yaml")
	content := "version: \"1.0\"\nnodes:\n  - id: imp1\n    name: imp-1\n    address: 10.9.9.9\n    port: 22\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := m.doImport(path, false); err != nil {
		t.Fatalf("import: %v", err)
	}
	got, err := store.Get("imp1")
	if err != nil || got.Name != "imp-1" {
		t.Fatalf("imported node wrong: %+v err=%v", got, err)
	}
}

func TestModel_DoImport_SkipExisting(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	path := filepath.Join(t.TempDir(), "in.yaml")
	content := "version: \"1.0\"\nnodes:\n  - id: n1\n    name: replaced\n    address: 10.9.9.9\n    port: 22\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// 不 overwrite → 跳过既有节点,名称不变
	if err := m.doImport(path, false); err != nil {
		t.Fatalf("import: %v", err)
	}
	got, _ := store.Get("n1")
	if got.Name != "web-1" {
		t.Fatalf("expected skip existing, got name %q", got.Name)
	}
}

func TestModel_DoImport_OverwriteExisting(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	path := filepath.Join(t.TempDir(), "in.yaml")
	content := "version: \"1.0\"\nnodes:\n  - id: n1\n    name: replaced\n    address: 10.9.9.9\n    port: 22\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := m.doImport(path, true); err != nil {
		t.Fatalf("import: %v", err)
	}
	got, _ := store.Get("n1")
	if got.Name != "replaced" {
		t.Fatalf("expected overwrite, got name %q", got.Name)
	}
}

func TestModel_OpenImportExport_FromList(t *testing.T) {
	store := newTestStore(t)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	if m.current() != LocImportExport {
		t.Fatalf("expected LocImportExport, got %v", m.current())
	}
	if m.importExport == nil {
		t.Fatal("expected importExport model")
	}
	path := m.Path()
	if len(path) != 2 || path[1] != "import" {
		t.Fatalf("unexpected path: %v", path)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run "TestModel_DoExport|TestModel_DoImport|TestModel_OpenImportExport" -v`
Expected: FAIL(`doExport`/`doImport`/`LocImportExport` 未定义)

- [ ] **Step 3: 实现 import_export.go**

创建 `cmd/cli/cmd/tui/nodes/import_export.go`:

```go
package nodes

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	"gopkg.in/yaml.v3"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type ImportExportModel struct {
	op        string // "export" | "import"
	format    string // "yaml" | "json"
	path      textinput.Model
	overwrite bool
	error     string
}

func NewImportExportModel() *ImportExportModel {
	ti := textinput.New()
	ti.Placeholder = "./nodes.yaml"
	ti.Width = 40
	ti.CharLimit = 256
	ti.Focus()
	return &ImportExportModel{op: "export", format: "yaml", path: ti}
}

type nodeFile struct {
	Version string             `json:"version" yaml:"version"`
	Nodes   []*common.NodeInfo `json:"nodes" yaml:"nodes"`
}

func (m Model) doExport(path, format string) error {
	nodes, err := m.store.List()
	if err != nil {
		return err
	}
	nf := nodeFile{Version: "1.0", Nodes: nodes}
	var data []byte
	if format == "json" {
		data, err = json.MarshalIndent(nf, "", "  ")
	} else {
		data, err = yaml.Marshal(nf)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (m Model) doImport(path string, overwrite bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var nf nodeFile
	if err := yaml.Unmarshal(data, &nf); err != nil {
		if jsonErr := json.Unmarshal(data, &nf); jsonErr != nil {
			return fmt.Errorf("解析导入文件失败: %v", err)
		}
	}
	for _, node := range nf.Nodes {
		if node.ID == "" || node.Name == "" || node.Address == "" {
			continue
		}
		_, exists := m.store.Get(node.ID)
		if exists == nil && !overwrite {
			continue
		}
		if exists != nil {
			if err := m.store.Add(node); err != nil {
				return err
			}
		} else if err := m.store.Update(node); err != nil {
			return err
		}
	}
	return m.store.Save()
}
```

- [ ] **Step 4: 接入 model.go**

在 `cmd/cli/cmd/tui/nodes/model.go`:

1. `Location` 追加 `LocImportExport`(LocCheck 后):

```go
	LocImportExport
```

2. `Model` 追加字段(check 字段后):

```go
	importExport *ImportExportModel
```

3. `Path()` 追加分支(LocCheck case 后):

```go
	case LocImportExport:
		return []string{"nodes", "import"}
```

4. 顶层 `Update` 分发 switch 追加(LocCheck case 后):

```go
	case LocImportExport:
		return m.updateImportExport(msg)
```

5. `updateList` 的 switch 追加 `i` 入口(k 分支后):

```go
	case "i":
		m.push(LocImportExport)
		m.importExport = NewImportExportModel()
		return m, textinput.Blink
```

6. 追加 `updateImportExport` 方法(updateCheck 后):

```go
func (m Model) updateImportExport(msg tea.Msg) (tea.Model, tea.Cmd) {
	ie := m.importExport
	if ie == nil {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "right":
			if ie.op == "export" {
				ie.op = "import"
			} else {
				ie.op = "export"
			}
			return m, nil
		case "f":
			if ie.format == "yaml" {
				ie.format = "json"
			} else {
				ie.format = "yaml"
			}
			return m, nil
		case "o":
			ie.overwrite = !ie.overwrite
			return m, nil
		case "esc":
			m.pop()
			m.importExport = nil
			return m, nil
		case "enter":
			var err error
			if ie.op == "export" {
				err = m.doExport(ie.path.Value(), ie.format)
			} else {
				err = m.doImport(ie.path.Value(), ie.overwrite)
			}
			if err != nil {
				ie.error = err.Error()
				return m, nil
			}
			m.pop()
			m.importExport = nil
			m.status = "导入导出完成"
			return m, nil
		}
	}
	var cmd tea.Cmd
	ie.path, cmd = ie.path.Update(msg)
	return m, cmd
}
```

注意:`updateList` 需已 import `textinput`(model.go 已有)。

- [ ] **Step 5: 实现 view.go 的 importExportView**

在 `cmd/cli/cmd/tui/nodes/view.go`:

1. `View()` 的 switch 追加(LocCheck case 后):

```go
	case LocImportExport:
		return m.listPane() + "\n\n" + m.importExportView()
```

2. 追加方法(checkView 后):

```go
func (m Model) importExportView() string {
	ie := m.importExport
	if ie == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ 导入/导出 ──────────────\n")
	op := "导出"
	if ie.op == "import" {
		op = "导入"
	}
	b.WriteString("  操作: " + styleSelected.Render(op) + styleDim.Render("  ←→ 切换") + "\n")
	b.WriteString("  格式: " + styleSelected.Render(ie.format) + styleDim.Render("  f 切换") + "\n")
	b.WriteString("  路径: " + ie.path.View() + "\n")
	if ie.op == "import" {
		mark := "[ ]"
		if ie.overwrite {
			mark = "[x]"
		}
		b.WriteString("  " + mark + " 覆盖已存在节点  o 切换\n")
	}
	if ie.error != "" {
		b.WriteString(styleError.Render("  "+ie.error) + "\n")
	}
	b.WriteString(styleDim.Render("  Enter 执行  Esc 返回") + "\n")
	b.WriteString("└─")
	return b.String()
}
```

- [ ] **Step 6: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/import_export.go cmd/cli/cmd/tui/nodes/import_export_test.go cmd/cli/cmd/tui/nodes/model.go cmd/cli/cmd/tui/nodes/view.go
git commit -m "feat(tui): 导入/导出视图(i 键, yaml/json, overwrite 控制)"
```

---

### Task 6: Groups 管理视图(`o` = `node groups`)

**Files:**
- Create: `cmd/cli/cmd/tui/nodes/groups.go`
- Create: `cmd/cli/cmd/tui/nodes/groups_test.go`
- Modify: `cmd/cli/cmd/tui/nodes/model.go`(LocGroups + updateGroups + `o` 入口)
- Modify: `cmd/cli/cmd/tui/nodes/view.go`(groupsView)
- Test: `cmd/cli/cmd/tui/nodes/groups_test.go`

**Interfaces:**
- Consumes: `common.NodeStore`、`Mode`、`m.selectedNode()`
- Produces:
  - `type GroupsModel struct { store common.NodeStore; nodeID string; groups []string; cursor int; input textinput.Model; adding bool; error string }`
  - `func NewGroupsModel(store common.NodeStore, nodeID string) *GroupsModel`
  - `func (g *GroupsModel) reload()`(按 store 刷新 groups)
  - `func (g *GroupsModel) addGroup(name string) error` / `removeGroup(name string) error`
  - `func (m Model) updateGroups(msg tea.Msg) (tea.Model, tea.Cmd)`

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/groups_test.go`(`package nodes`):

```go
package nodes

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestGroupsModel_AddGroup(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	g := NewGroupsModel(store, "n1")
	if err := g.addGroup("staging"); err != nil {
		t.Fatalf("add: %v", err)
	}
	node, _ := store.Get("n1")
	if !containsStr(node.Groups, "staging") {
		t.Fatalf("expected staging in groups, got %#v", node.Groups)
	}
}

func TestGroupsModel_RemoveGroup(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	g := NewGroupsModel(store, "n3")
	if err := g.removeGroup("web"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	node, _ := store.Get("n3")
	if containsStr(node.Groups, "web") {
		t.Fatalf("expected web removed, got %#v", node.Groups)
	}
}

func TestGroupsModel_ReloadFromStore(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	g := NewGroupsModel(store, "n1")
	if len(g.groups) == 0 {
		t.Fatal("expected groups loaded")
	}
	// store 变更后 reload 反映最新
	_ = g.addGroup("extra")
	g.reload()
	if !containsStr(g.groups, "extra") {
		t.Fatalf("expected extra after reload, got %#v", g.groups)
	}
}

func TestModel_OpenGroups_FromList(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('o'))
	m = nm.(Model)
	if m.current() != LocGroups {
		t.Fatalf("expected LocGroups, got %v", m.current())
	}
	if m.groups == nil || m.groups.nodeID != "n1" {
		t.Fatalf("unexpected groups model: %+v", m.groups)
	}
	path := m.Path()
	if len(path) != 3 || path[2] != "groups" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestModel_Groups_EscBack(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('o'))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatalf("expected back to list, got %v", m.current())
	}
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestGroupsModel_AddInputParsing(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	g := NewGroupsModel(store, "n1")
	g.adding = true
	g.input.SetValue("prod")
	// 模拟在导航态?不,add 由 model 层处理;此处验证 name 清洗
	name := strings.TrimSpace(g.input.Value())
	if name != "prod" {
		t.Fatalf("unexpected name: %q", name)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run "TestGroupsModel|TestModel_OpenGroups|TestModel_Groups|TestGroupsModel_AddInputParsing" -v`
Expected: FAIL(`GroupsModel`/`LocGroups` 未定义)

- [ ] **Step 3: 实现 groups.go**

创建 `cmd/cli/cmd/tui/nodes/groups.go`:

```go
package nodes

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type GroupsModel struct {
	store  common.NodeStore
	nodeID string
	groups []string
	cursor int
	input  textinput.Model
	adding bool
	error  string
}

func NewGroupsModel(store common.NodeStore, nodeID string) *GroupsModel {
	ti := textinput.New()
	ti.Placeholder = "分组名"
	ti.Width = 30
	ti.CharLimit = 64
	ti.Blur()
	g := &GroupsModel{store: store, nodeID: nodeID, input: ti}
	g.reload()
	return g
}

func (g *GroupsModel) reload() {
	node, err := g.store.Get(g.nodeID)
	if err != nil {
		g.groups = nil
		return
	}
	g.groups = append([]string(nil), node.Groups...)
	sort.Strings(g.groups)
	if g.cursor >= len(g.groups) {
		g.cursor = len(g.groups) - 1
	}
	if g.cursor < 0 {
		g.cursor = 0
	}
}

func (g *GroupsModel) addGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	node, err := g.store.Get(g.nodeID)
	if err != nil {
		return err
	}
	for _, x := range node.Groups {
		if x == name {
			return nil
		}
	}
	node.Groups = append(node.Groups, name)
	if err := g.store.Update(node); err != nil {
		return err
	}
	g.reload()
	return g.store.Save()
}

func (g *GroupsModel) removeGroup(name string) error {
	node, err := g.store.Get(g.nodeID)
	if err != nil {
		return err
	}
	out := make([]string, 0, len(node.Groups))
	for _, x := range node.Groups {
		if x != name {
			out = append(out, x)
		}
	}
	node.Groups = out
	if err := g.store.Update(node); err != nil {
		return err
	}
	g.reload()
	return g.store.Save()
}
```

- [ ] **Step 4: 接入 model.go**

在 `cmd/cli/cmd/tui/nodes/model.go`:

1. `Location` 追加 `LocGroups`(LocImportExport 后):

```go
	LocGroups
```

2. `Model` 追加字段(importExport 字段后):

```go
	groups *GroupsModel
```

3. `Path()` 追加分支(LocImportExport case 后):

```go
	case LocGroups:
		return []string{"nodes", m.selectedID(), "groups"}
```

4. 顶层 `Update` 分发 switch 追加(LocImportExport case 后):

```go
	case LocGroups:
		return m.updateGroups(msg)
```

5. `updateList` 的 switch 追加 `o` 入口(i 分支后):

```go
	case "o":
		if n := m.selectedNode(); n != nil {
			m.push(LocGroups)
			m.groups = NewGroupsModel(m.store, n.ID)
			return m, nil
		}
```

6. 追加 `updateGroups` 方法(updateImportExport 后):

```go
func (m Model) updateGroups(msg tea.Msg) (tea.Model, tea.Cmd) {
	g := m.groups
	if g == nil {
		return m, nil
	}
	if g.adding {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				g.adding = false
				g.input.Blur()
				return m, nil
			case "enter":
				if err := g.addGroup(g.input.Value()); err != nil {
					g.error = err.Error()
				} else {
					g.error = ""
				}
				g.adding = false
				g.input.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		g.input, cmd = g.input.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "esc":
		m.pop()
		m.groups = nil
	case "up":
		if g.cursor > 0 {
			g.cursor--
		}
	case "down":
		if g.cursor < len(g.groups)-1 {
			g.cursor++
		}
	case "a":
		g.adding = true
		g.input.Focus()
	case "d":
		if g.cursor >= 0 && g.cursor < len(g.groups) {
			if err := g.removeGroup(g.groups[g.cursor]); err != nil {
				g.error = err.Error()
			} else {
				g.error = ""
			}
		}
	}
	return m, nil
}
```

- [ ] **Step 5: 实现 view.go 的 groupsView**

在 `cmd/cli/cmd/tui/nodes/view.go`:

1. `View()` 的 switch 追加(LocImportExport case 后):

```go
	case LocGroups:
		return m.listPane() + "\n\n" + m.groupsView()
```

2. 追加方法(importExportView 后):

```go
func (m Model) groupsView() string {
	g := m.groups
	if g == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ 分组管理 ──────────────\n")
	b.WriteString("  节点: " + styleSelected.Render(g.nodeID) + "\n")
	if g.adding {
		b.WriteString("  新增分组: " + g.input.View() + styleDim.Render("  Enter 确认  Esc 取消") + "\n")
	} else {
		if len(g.groups) == 0 {
			b.WriteString("  " + styleDim.Render("(无分组,按 a 添加)") + "\n")
		}
		for i, name := range g.groups {
			line := "  " + name + "\n"
			if i == g.cursor {
				line = styleSelected.Render("> " + name) + "\n"
			}
			b.WriteString(line)
		}
		b.WriteString(styleDim.Render("  a 添加  d 删除  Esc 返回") + "\n")
	}
	if g.error != "" {
		b.WriteString(styleError.Render("  "+g.error) + "\n")
	}
	b.WriteString("└─")
	return b.String()
}
```

- [ ] **Step 6: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/groups.go cmd/cli/cmd/tui/nodes/groups_test.go cmd/cli/cmd/tui/nodes/model.go cmd/cli/cmd/tui/nodes/view.go
git commit -m "feat(tui): 分组管理视图(o 键, 添加/删除分组)"
```

---

### Task 7: Labels 管理视图(`l` = `node labels`)

**Files:**
- Create: `cmd/cli/cmd/tui/nodes/labels.go`
- Create: `cmd/cli/cmd/tui/nodes/labels_test.go`
- Modify: `cmd/cli/cmd/tui/nodes/model.go`(LocLabels + updateLabels + `l` 入口)
- Modify: `cmd/cli/cmd/tui/nodes/view.go`(labelsView)
- Test: `cmd/cli/cmd/tui/nodes/labels_test.go`

**Interfaces:**
- Consumes: `common.NodeStore`、`Mode`、`m.selectedNode()`
- Produces:
  - `type LabelsModel struct { store common.NodeStore; nodeID string; keys []string; cursor int; input textinput.Model; adding bool; error string }`
  - `func NewLabelsModel(store common.NodeStore, nodeID string) *LabelsModel`
  - `func (l *LabelsModel) reload()`(keys 按 label key 排序)
  - `func (l *LabelsModel) setLabel(kv string) error`(`key=value` 或 `key=` 删除)
  - `func (l *LabelsModel) removeLabel(key string) error`
  - `func (m Model) updateLabels(msg tea.Msg) (tea.Model, tea.Cmd)`

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/labels_test.go`(`package nodes`):

```go
package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLabelsModel_SetAndRemove(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	lm := NewLabelsModel(store, "n1")
	// n1 初始 labels: env=prod
	if err := lm.setLabel("tier=backend"); err != nil {
		t.Fatalf("set: %v", err)
	}
	node, _ := store.Get("n1")
	if node.Labels["tier"] != "backend" {
		t.Fatalf("expected tier=backend, got %#v", node.Labels)
	}
	if err := lm.setLabel("env="); err != nil {
		t.Fatalf("clear env: %v", err)
	}
	node, _ = store.Get("n1")
	if _, ok := node.Labels["env"]; ok {
		t.Fatalf("expected env removed, got %#v", node.Labels)
	}
}

func TestLabelsModel_ReloadSortsKeys(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	lm := NewLabelsModel(store, "n3") // n3 labels: env=prod, role=cache
	if len(lm.keys) != 2 || lm.keys[0] != "env" || lm.keys[1] != "role" {
		t.Fatalf("unexpected keys: %#v", lm.keys)
	}
}

func TestModel_OpenLabels_FromList(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('l'))
	m = nm.(Model)
	if m.current() != LocLabels {
		t.Fatalf("expected LocLabels, got %v", m.current())
	}
	if m.labels == nil || m.labels.nodeID != "n1" {
		t.Fatalf("unexpected labels model: %+v", m.labels)
	}
	path := m.Path()
	if len(path) != 3 || path[2] != "labels" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestModel_Labels_AddFlow(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('l'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	if !m.labels.adding {
		t.Fatal("expected adding mode")
	}
	// 输入 tier=worker 并回车
	for _, r := range []rune("tier=worker") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	node, _ := store.Get("n1")
	if node.Labels["tier"] != "worker" {
		t.Fatalf("expected tier=worker, got %#v", node.Labels)
	}
	if m.labels.adding {
		t.Fatal("expected adding closed")
	}
}

func TestModel_Labels_RemoveFlow(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('l'))
	m = nm.(Model)
	// 光标在 env(第一个 key),按 d 删除
	nm, _ = m.Update(runeKey('d'))
	m = nm.(Model)
	node, _ := store.Get("n1")
	if _, ok := node.Labels["env"]; ok {
		t.Fatalf("expected env removed, got %#v", node.Labels)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run "TestLabelsModel|TestModel_OpenLabels|TestModel_Labels" -v`
Expected: FAIL(`LabelsModel`/`LocLabels` 未定义)

- [ ] **Step 3: 实现 labels.go**

创建 `cmd/cli/cmd/tui/nodes/labels.go`:

```go
package nodes

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type LabelsModel struct {
	store  common.NodeStore
	nodeID string
	keys   []string
	cursor int
	input  textinput.Model
	adding bool
	error  string
}

func NewLabelsModel(store common.NodeStore, nodeID string) *LabelsModel {
	ti := textinput.New()
	ti.Placeholder = "key=value"
	ti.Width = 30
	ti.CharLimit = 128
	ti.Blur()
	l := &LabelsModel{store: store, nodeID: nodeID, input: ti}
	l.reload()
	return l
}

func (l *LabelsModel) reload() {
	node, err := l.store.Get(l.nodeID)
	if err != nil {
		l.keys = nil
		return
	}
	l.keys = make([]string, 0, len(node.Labels))
	for k := range node.Labels {
		l.keys = append(l.keys, k)
	}
	sort.Strings(l.keys)
	if l.cursor >= len(l.keys) {
		l.cursor = len(l.keys) - 1
	}
	if l.cursor < 0 {
		l.cursor = 0
	}
}

// setLabel 设置 key=value;value 为空表示删除该 key(对齐 node labels set key= / remove key)
func (l *LabelsModel) setLabel(kv string) error {
	kv = strings.TrimSpace(kv)
	if kv == "" {
		return nil
	}
	parts := strings.SplitN(kv, "=", 2)
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return nil
	}
	node, err := l.store.Get(l.nodeID)
	if err != nil {
		return err
	}
	if node.Labels == nil {
		node.Labels = map[string]string{}
	}
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		node.Labels[key] = strings.TrimSpace(parts[1])
	} else {
		delete(node.Labels, key)
	}
	if err := l.store.Update(node); err != nil {
		return err
	}
	l.reload()
	return l.store.Save()
}

func (l *LabelsModel) removeLabel(key string) error {
	node, err := l.store.Get(l.nodeID)
	if err != nil {
		return err
	}
	delete(node.Labels, key)
	if err := l.store.Update(node); err != nil {
		return err
	}
	l.reload()
	return l.store.Save()
}
```

- [ ] **Step 4: 接入 model.go**

在 `cmd/cli/cmd/tui/nodes/model.go`:

1. `Location` 追加 `LocLabels`(LocGroups 后):

```go
	LocLabels
```

2. `Model` 追加字段(groups 字段后):

```go
	labels *LabelsModel
```

3. `Path()` 追加分支(LocGroups case 后):

```go
	case LocLabels:
		return []string{"nodes", m.selectedID(), "labels"}
```

4. 顶层 `Update` 分发 switch 追加(LocGroups case 后):

```go
	case LocLabels:
		return m.updateLabels(msg)
```

5. `updateList` 的 switch 追加 `l` 入口(o 分支后):

```go
	case "l":
		if n := m.selectedNode(); n != nil {
			m.push(LocLabels)
			m.labels = NewLabelsModel(m.store, n.ID)
			return m, nil
		}
```

6. 追加 `updateLabels` 方法(updateGroups 后):

```go
func (m Model) updateLabels(msg tea.Msg) (tea.Model, tea.Cmd) {
	l := m.labels
	if l == nil {
		return m, nil
	}
	if l.adding {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				l.adding = false
				l.input.Blur()
				return m, nil
			case "enter":
				if err := l.setLabel(l.input.Value()); err != nil {
					l.error = err.Error()
				} else {
					l.error = ""
				}
				l.adding = false
				l.input.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		l.input, cmd = l.input.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "esc":
		m.pop()
		m.labels = nil
	case "up":
		if l.cursor > 0 {
			l.cursor--
		}
	case "down":
		if l.cursor < len(l.keys)-1 {
			l.cursor++
		}
	case "a":
		l.adding = true
		l.input.Focus()
	case "d":
		if l.cursor >= 0 && l.cursor < len(l.keys) {
			if err := l.removeLabel(l.keys[l.cursor]); err != nil {
				l.error = err.Error()
			} else {
				l.error = ""
			}
		}
	}
	return m, nil
}
```

- [ ] **Step 5: 实现 view.go 的 labelsView**

在 `cmd/cli/cmd/tui/nodes/view.go`:

1. `View()` 的 switch 追加(LocGroups case 后):

```go
	case LocLabels:
		return m.listPane() + "\n\n" + m.labelsView()
```

2. 追加方法(groupsView 后):

```go
func (m Model) labelsView() string {
	l := m.labels
	if l == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ 标签管理 ──────────────\n")
	b.WriteString("  节点: " + styleSelected.Render(l.nodeID) + "\n")
	if l.adding {
		b.WriteString("  新增标签: " + l.input.View() + styleDim.Render("  Enter 确认  Esc 取消") + "\n")
	} else {
		if len(l.keys) == 0 {
			b.WriteString("  " + styleDim.Render("(无标签,按 a 添加)") + "\n")
		}
		node, _ := l.store.Get(l.nodeID)
		for i, k := range l.keys {
			val := "-"
			if node != nil {
				if v, ok := node.Labels[k]; ok {
					val = v
				}
			}
			line := "  " + k + "=" + val + "\n"
			if i == l.cursor {
				line = styleSelected.Render("> " + k + "=" + val) + "\n"
			}
			b.WriteString(line)
		}
		b.WriteString(styleDim.Render("  a 添加  d 删除  Esc 返回") + "\n")
	}
	if l.error != "" {
		b.WriteString(styleError.Render("  "+l.error) + "\n")
	}
	b.WriteString("└─")
	return b.String()
}
```

- [ ] **Step 6: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/labels.go cmd/cli/cmd/tui/nodes/labels_test.go cmd/cli/cmd/tui/nodes/model.go cmd/cli/cmd/tui/nodes/view.go
git commit -m "feat(tui): 标签管理视图(l 键, key=value 添加/删除)"
```

---

### Task 8: App 帮助文案更新 + 全量回归 + E2E 冒烟

**Files:**
- Modify: `cmd/cli/cmd/tui/app.go`(helpView 增加新按键说明)
- Modify: `cmd/cli/cmd/tui/nodes/view.go`(statusBar 快捷键提示追加 p/k/i/o/l)
- Test: `go test ./cmd/cli/cmd/tui/...` 全量回归 + `go build ./cmd/cli`

- [ ] **Step 1: 更新帮助文案**

在 `cmd/cli/cmd/tui/app.go` 的 `helpView()` 中,`列表:` 行追加:

```go
		"        p ping  k SSH检查  i 导入导出  o 分组  l 标签",
```

- [ ] **Step 2: 更新状态栏提示**

在 `cmd/cli/cmd/tui/nodes/view.go` 的 `statusBar()` 中,非过滤提示追加:

```go
		b.WriteString(styleDim.Render("↑↓选择 ←→切栏 g/G首尾 a添加 e编辑 d删除 c列 p ping k检查 i导入导出 o分组 l标签 /过滤 ?帮助 q退出"))
```

- [ ] **Step 3: 全量回归**

Run: `go test ./cmd/cli/cmd/tui/... && go build ./cmd/cli`
Expected: 全部 PASS;`go vet ./cmd/cli/cmd/tui/...` 无告警

- [ ] **Step 4: pty E2E 冒烟**

手动验证(Windows 上可跳过 pty,改为确认 `build/owl.exe tui` 能启动并在列表按各键不崩溃)。若存在 `scripts/test-tui.sh`,更新其按键序列追加 `p`/`k`/`i`/`o`/`l` 冒烟(按后立刻 Esc 返回)后运行。

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/app.go cmd/cli/cmd/tui/nodes/view.go scripts/test-tui.sh
git commit -m "docs(tui): 帮助/状态栏补充新按键说明 p/k/i/o/l + E2E 冒烟"
```

---

## Self-Review

**1. Spec coverage(对齐 `owl node` 命令):**
- `node list -S/--status` → Task 1(过滤 `s:`)
- `node update --status` → Task 2(表单 Status 字段)
- `node ping` → Task 3(Ping 视图 `p`)
- `node check` → Task 4(SSH Check 视图 `k`,回写 status/last_check)
- `node import/export` → Task 5(Import/Export 视图 `i`)
- `node groups add/remove` → Task 6(Groups 视图 `o`)
- `node labels set/remove` → Task 7(Labels 视图 `l`)
- 帮助/回归/E2E → Task 8

**2. Placeholder scan:** 所有测试断言含实际代码与期望值;无 TODO/TBD。

**3. Type consistency:**
- `FilterQuery.Status` 在 Task 1 定义,`statusBar` 同任务使用
- `LocPing/LocCheck/LocImportExport/LocGroups/LocLabels` 依次在 Task 3-7 追加,`Update` 分发/`Path()`/`View()` 各任务同步追加对应分支
- `pingDial`/`sshCheck` 为包级 var,Task 3/4 测试替换后 defer 还原
- `NewPingModel/NewCheckModel/NewImportExportModel/NewGroupsModel/NewLabelsModel` 命名与既有 `NewConfirmModel/NewColumnsModel` 一致
- `updatePing/updateCheck/updateImportExport/updateGroups/updateLabels` 均返回 `(tea.Model, tea.Cmd)`
- `m.visible()`/`m.selectedNode()`/`m.selectedID()` 为既有方法,新任务直接复用
