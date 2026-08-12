# 命令执行页脚本来源支持「中转站」复用 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 命令执行页脚本 Tab 新增「中转站」脚本来源：上传的脚本保存到中转站目录后，可直接从下拉列表选择执行，无需每次重新上传。

**Architecture:** 复用已有的中转站系统（`~/.owl/staging` 目录 + `/api/v1/staging/*` 接口）。前端脚本 Tab 增加第 4 个来源「中转站」，列出中转站内脚本文件，选中后在 exec payload 里传 `script_ref`；后端 `resolveScriptContent` 命中 `script_ref` 时从中转站目录读文件内容，走原有 base64→远端执行管线。黑名单校验天然覆盖（内容照常经过 checker）。

**Tech Stack:** Go (gin, sqlite), vanilla JS 前端（无框架），Go 测试用 testify。

## Global Constraints

- 沿用现有 `stagingDirFromDB`（`handler/staging.go:41`）读取中转站目录，不新增配置。
- 文件名白名单复用现有 `scriptNameRe`（`handler/exec.go:693`）：仅 `[A-Za-z0-9._-]`，天然禁止 `/`、`..`，防路径穿越。
- `resolveScriptContent` 签名扩展为 `(req execRequest, stagingDir string)`，保持纯函数可测。
- 前端无 JS 测试框架，前端行为通过 `cmd/plugins/serve/filesjs_test.go` 字符串断言验证（已有 `TestExecJS_*` 先例）。
- 测试命令：`go test ./cmd/plugins/serve/handler/ -run TestResolveScriptContent -v` 与 `go test ./cmd/plugins/serve/ -run TestExecJS_StagingScriptSource -v`。
- 不改动 `staging.go`、`server.go`、`api.js`（中转站接口已就绪）。

---

### Task 1: 后端 `resolveScriptContent` 支持 `script_ref`（TDD）

**Files:**
- Modify: `cmd/plugins/serve/handler/exec.go:66-102`（execRequest 加字段）、`exec.go:182-213`（resolveScriptContent）
- Modify: `cmd/plugins/serve/handler/exec.go:215-263`（Create 调用点传 stagingDir）
- Test: `cmd/plugins/serve/handler/exec_test.go:675-692`（更新 3 个调用 + 新增 4 个用例）

**Interfaces:**
- Consumes: `stagingDirFromDB(db *sql.DB) string`（同包，`handler/staging.go:41`）、`scriptNameRe`（`handler/exec.go:693`）
- Produces: `resolveScriptContent(req execRequest, stagingDir string) (content string, name string, err error)`；`resolveStagingScriptRef(dir, ref string) (string, string, error)`；`execRequest.ScriptRef string \`json:"script_ref"\``

- [ ] **Step 1: 写失败测试**

在 `cmd/plugins/serve/handler/exec_test.go` 中，更新 3 个既有 `resolveScriptContent` 调用（补第二个参数），并新增用例（放在 `TestResolveScriptContent_Missing` 之后）：

```go
func TestResolveScriptContent_StagingRef(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deploy.sh"), []byte("#!/bin/bash\necho staging"), 0644))

	content, name, err := resolveScriptContent(execRequest{ScriptRef: "deploy.sh"}, dir)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/bash\necho staging", content)
	assert.Equal(t, "deploy.sh", name)
}

func TestResolveScriptContent_StagingRef_NotFound(t *testing.T) {
	_, _, err := resolveScriptContent(execRequest{ScriptRef: "missing.sh"}, t.TempDir())
	assert.ErrorContains(t, err, "script not found in staging")
}

func TestResolveScriptContent_StagingRef_RejectTraversal(t *testing.T) {
	_, _, err := resolveScriptContent(execRequest{ScriptRef: "../secret.sh"}, t.TempDir())
	assert.Error(t, err)
}

func TestResolveScriptContent_StagingRef_RejectSubpath(t *testing.T) {
	_, _, err := resolveScriptContent(execRequest{ScriptRef: "sub/dir.sh"}, t.TempDir())
	assert.Error(t, err)
}
```

既有 3 个用例改为两参调用：
```go
resolveScriptContent(execRequest{ScriptContent: "echo hi", ScriptName: "a.sh"}, t.TempDir())
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/plugins/serve/handler/ -run TestResolveScriptContent -v`
Expected: 编译错误 `resolveScriptContent` 参数个数不匹配。

- [ ] **Step 3: 最小实现**

在 `cmd/plugins/serve/handler/exec.go`：

execRequest 加字段（`ScriptURL` 之后）：
```go
	ScriptRef       string            `json:"script_ref"`
```

`resolveScriptContent` 改为：
```go
func resolveScriptContent(req execRequest, stagingDir string) (content string, name string, err error) {
	if req.ScriptContent != "" {
		name = req.ScriptName
		if name == "" {
			name = "script.sh"
		}
		return req.ScriptContent, name, nil
	}
	if req.ScriptURL != "" {
		// …原 URL 分支不变…
	}
	if req.ScriptRef != "" {
		return resolveStagingScriptRef(stagingDir, req.ScriptRef)
	}
	return "", "", fmt.Errorf("script_content, script_url or script_ref is required")
}

// resolveStagingScriptRef 从中转站目录读取脚本内容；
// 文件名复用 scriptNameRe 白名单（仅 [A-Za-z0-9._-]），天然禁止 / 与 ..，防路径穿越。
func resolveStagingScriptRef(dir, ref string) (string, string, error) {
	if !scriptNameRe.MatchString(ref) || ref == "." || ref == ".." {
		return "", "", fmt.Errorf("invalid script_ref %q: only [A-Za-z0-9._-] allowed", ref)
	}
	path := filepath.Join(dir, ref)
	fi, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("script not found in staging: %s", ref)
	}
	if fi.IsDir() {
		return "", "", fmt.Errorf("script_ref %q is a directory", ref)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read staging script: %w", err)
	}
	if len(b) == 0 {
		return "", "", fmt.Errorf("staging script %q is empty", ref)
	}
	return string(b), ref, nil
}
```

Create 调用点（`exec.go:237`）改为：
```go
		content, name, err := resolveScriptContent(req, stagingDirFromDB(h.db))
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/plugins/serve/handler/ -run TestResolveScriptContent -v`
Expected: 全部 PASS。

- [ ] **Step 5: Commit**

```bash
git add cmd/plugins/serve/handler/exec.go cmd/plugins/serve/handler/exec_test.go
git commit -m "feat(exec): resolve script_content from staging dir via script_ref"
```

---

### Task 2: Create API 端到端支持 `script_ref`（TDD）

**Files:**
- Modify: `cmd/plugins/serve/handler/exec_test.go`
- Test: 新增 2 个端到端用例

**Interfaces:**
- Consumes: `execPOST(t, router, body)`（`exec_test.go:128`）、`execTestSetup`（`exec_test.go:65`）
- Produces: 无新接口

- [ ] **Step 1: 写失败测试**

在 `cmd/plugins/serve/handler/exec_test.go` 末尾追加：

```go
func TestExecCreate_ScriptMode_StagingRef(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deploy.sh"), []byte("#!/bin/bash\necho from-staging"), 0644))
	_, err := h.db.Exec(`INSERT INTO settings (key, value) VALUES ('staging_dir', ?)`, dir)
	require.NoError(t, err)

	w := execPOST(t, router, map[string]interface{}{
		"mode":       "script",
		"node_ids":   []string{"test-node"},
		"script_ref": "deploy.sh",
	})

	require.Equal(t, 202, w.Code)
	var resp execResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Tasks, 1)
	assert.Equal(t, "script: deploy.sh", resp.Tasks[0].Command)
}

func TestExecCreate_ScriptMode_StagingRef_MissingFile(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	_, err := h.db.Exec(`INSERT INTO settings (key, value) VALUES ('staging_dir', ?)`, t.TempDir())
	require.NoError(t, err)

	w := execPOST(t, router, map[string]interface{}{
		"mode":       "script",
		"node_ids":   []string{"test-node"},
		"script_ref": "ghost.sh",
	})
	assert.Equal(t, 400, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), "script not found in staging"))
}
```

注意：`execTestSetup` 的 sqlite 未建 `settings` 表。若 `INSERT` 报错，在 `Step 3` 中于测试内先建表：
```sql
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/plugins/serve/handler/ -run TestExecCreate_ScriptMode_StagingRef -v`
Expected: FAIL（`staging_dir` 指向临时空目录时 `resolveScriptContent` 尚未读 staging，或 settings 表缺失）。

- [ ] **Step 3: 最小实现/修正测试基建**

Task 1 已实现后端读 staging；若失败仅是 `settings` 表缺失，在测试内补建表（见 Step 1 note）。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/plugins/serve/handler/ -run TestExecCreate_ScriptMode_StagingRef -v`
Expected: 全部 PASS。

- [ ] **Step 5: Commit**

```bash
git add cmd/plugins/serve/handler/exec_test.go
git commit -m "test(exec): end-to-end script_ref via staging dir"
```

---

### Task 3: 前端脚本 Tab 新增「中转站」来源 + 上传可保存到中转站

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/exec.js`（状态、来源切换、payload、上传保存）
- Test: `cmd/plugins/serve/filesjs_test.go`（新增断言）

**Interfaces:**
- Consumes: `api.staging.files()`、`api.staging.upload(file)`（`web/js/api.js:293-324`，已存在）
- Produces: payload 字段 `script_ref`（后端 Task 1 已定义）

- [ ] **Step 1: 写失败测试**

在 `cmd/plugins/serve/filesjs_test.go` 追加：

```go
func TestExecJS_StagingScriptSource(t *testing.T) {
	src := readWebFile(t, "web/js/pages/exec.js")

	assert.True(t, strings.Contains(src, `data-script-src="staging"`),
		"exec.js must expose a staging script source")
	assert.True(t, strings.Contains(src, "script-staging-row"),
		"exec.js must render the staging picker row")
	assert.True(t, strings.Contains(src, "script-staging-select"),
		"exec.js must render a staging file select")
	assert.True(t, strings.Contains(src, "api.staging.files()"),
		"exec.js must load staging files via the staging api")
	assert.True(t, strings.Contains(src, "payload.script_ref"),
		"exec.js must send script_ref in the exec payload")
	assert.True(t, strings.Contains(src, "script-save-staging"),
		"exec.js must offer saving an uploaded script to staging")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/plugins/serve/ -run TestExecJS_StagingScriptSource -v`
Expected: FAIL（各断言不成立）。

- [ ] **Step 3: 最小实现 `exec.js`**

状态变量（`scriptInlineContent` 声明附近，`exec.js:23` 之后加）：
```js
  let scriptStagingName = '';
  let stagingScripts = [];
```

`hasScriptInput`（`exec.js:307`）末尾追加分支：
```js
    if (scriptInputMode === 'staging') return scriptStagingName !== '';
```

`switchScriptSource`（`exec.js:345`）改为：
```js
  function switchScriptSource(src) {
    const ta = document.getElementById('cmd-input');
    if (scriptInputMode === 'inline') scriptInlineContent = ta.value;
    scriptInputMode = src;
    document.getElementById('script-upload-row').style.display = src === 'upload' ? '' : 'none';
    document.getElementById('script-url-row').style.display = src === 'url' ? '' : 'none';
    document.getElementById('script-staging-row').style.display = src === 'staging' ? '' : 'none';
    if (src === 'staging') loadStagingScripts();
    if (src === 'inline') {
      ta.value = scriptInlineContent;
      ta.placeholder = '#!/bin/bash\n# 输入脚本内容\necho "hello world"';
      ta.style.display = '';
    } else {
      ta.value = '';
      ta.style.display = 'none';
    }
    updateExecButton();
  }
```

新增两个函数（放在 `switchScriptSource` 之后）：
```js
  async function loadStagingScripts() {
    try {
      const res = await api.staging.files();
      stagingScripts = (res.data || []).filter(f => /\.(sh|bash|py|pl|rb|go|expect|exp)$/i.test(f.name));
    } catch { stagingScripts = []; }
    renderStagingScriptSelect();
  }

  function renderStagingScriptSelect() {
    const sel = document.getElementById('script-staging-select');
    if (!sel) return;
    sel.innerHTML = stagingScripts.length === 0
      ? '<option value="">中转站无脚本文件，可先在「上传」中勾选保存</option>'
      : stagingScripts.map(f => `<option value="${esc(f.name)}">${esc(f.name)}</option>`).join('');
    scriptStagingName = sel.value;
    updateExecButton();
  }
```

`buildExecPayload` 脚本分支（`exec.js:528`）追加：
```js
      } else if (scriptInputMode === 'staging') {
        payload.script_ref = scriptStagingName;
      }
```

HTML 来源按钮（`exec.js:679`，URL 按钮后）：
```html
                <button data-script-src="staging">中转站</button>
```

URL row（`exec.js:689-692`）之后新增：
```html
            <div class="filter-row" id="script-staging-row" style="display:none">
              <label>中转站脚本</label>
              <select id="script-staging-select" class="exec-input"></select>
            </div>
```

上传行内加保存勾选（`exec.js:685-688` 的 label 改法——将上传行改为含 file input + 勾选）：
```html
            <div class="filter-row" id="script-upload-row" style="display:none">
              <label>脚本文件</label>
              <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
                <input type="file" id="script-file" class="exec-input">
                <label class="force-check">
                  <input type="checkbox" id="script-save-staging"> 保存到中转站
                </label>
              </div>
            </div>
```

事件绑定（`exec.js:893` 文件 change 处理器内，`reader.readAsText(file)` 之前追加；并在其后绑 select change）：
```js
      if (document.getElementById('script-save-staging')?.checked) {
        api.staging.upload(file).then(() => loadStagingScripts()).catch(() => {});
      }
```
并在 `scriptUrlEl` 监听之后：
```js
    const stagingSelect = document.getElementById('script-staging-select');
    if (stagingSelect) stagingSelect.addEventListener('change', function() {
      scriptStagingName = this.value;
      updateExecButton();
    });
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/plugins/serve/ -run TestExecJS_StagingScriptSource -v`
Expected: PASS。再跑 `go test ./cmd/plugins/serve/ -run TestExecJS` 确保既有断言不受影响。

- [ ] **Step 5: Commit**

```bash
git add cmd/plugins/serve/web/js/pages/exec.js cmd/plugins/serve/filesjs_test.go
git commit -m "feat(exec): pick staged scripts in script tab and save uploads to staging"
```

---

### Task 4: 全量回归 + E2E 验证 + 提交

- [ ] **Step 1: 全量测试**

Run: `go build ./... && go test ./cmd/plugins/serve/... ./internal/control/...`
Expected: 全部 PASS。

- [ ] **Step 2: E2E 手工验证**

1. 启动 server：`./build/owl-serve --reset-admin --port 8080`（`~/.owl` 已有数据，勿删 db）。
2. 登录拿 token；上传脚本到中转站：
   ```bash
   TOKEN="$(curl -s http://localhost:8080/api/v1/login -X POST -H 'Content-Type: application/json' -d '{"username":"admin","password":"<密码>"}' | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')"
   printf '#!/bin/bash\necho from-staging-ok\n' > /tmp/staging-demo.sh
   curl -s http://localhost:8080/api/v1/staging/upload -H "Authorization: Bearer $TOKEN" -F "file=@/tmp/staging-demo.sh"
   ```
3. 浏览器打开 `http://localhost:8080/exec` → 脚本 Tab → 「中转站」来源 → 应能看到 `staging-demo.sh`，选中后执行。
4. 验证任务输出包含 `from-staging-ok`。
5. 反向验证：清空中转站该文件后执行 `script_ref`，应返回 400「script not found in staging」。

- [ ] **Step 3: Commit 收尾（如需）**

如 E2E 发现遗漏修改，补齐后提交：
```bash
git status
```
提交尚未提交的改动（若 Task 1-3 已各自提交则无需此步）。

---

## Self-Review

- **Spec 覆盖**：上传→存中转站（Task 3 勾选保存）✓；直接选择执行（Task 1/2 后端 + Task 3 前端下拉）✓；复用中转站目录与接口（未动 staging.go/server.go/api.js）✓。
- **占位符扫描**：所有代码块均为最终内容，无 TBD/示例。
- **类型一致性**：`script_ref` 在 execRequest json tag、resolveScriptContent 参数、payload 字段三处统一为 `script_ref`；`script-staging-select`/`script-staging-row`/`script-save-staging` id 在 HTML 与 JS 绑定一致。
