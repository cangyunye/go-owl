# Playbook Create Wizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) for syntax checking.

**Goal:** Make the "新建" button on the `/playbooks` page open a web wizard matching `owl playbook template` interactive flow, producing identical YAML.

**Architecture:** Frontend multi-step modal → `POST /api/v1/playbook/template` → backend generates YAML using same structs as CLI → saves to playbook library path → auto-refreshes list. Template structs extracted to `pkg/playbook/template.go` for CLI and server to share.

**Tech Stack:** Go (Gin + yaml.v3), vanilla JS SPA

## Global Constraints

- No new Go dependencies beyond stdlib + yaml.v3 + gin
- No new npm/node dependencies — all frontend is vanilla JS modules
- YAML output must be byte-identical to `owl playbook template` output (same struct, same yaml.Marshal)
- All frontend changes in `cmd/plugins/serve/web/js/pages/playbooks.js` + `api.js`
- Template structs shared via `pkg/playbook/template.go`
- Playbook library path: configured via `playbook_library_path` setting, fallback `~/.owl/playbooks`
- Admin RBAC for the create endpoint (same as refresh)

---

### Task 1: Extract shared template types and rendering to `pkg/playbook/template.go`

**Files:**
- Create: `pkg/playbook/template.go`
- Modify: `cmd/cli/cmd/playbook/template.go` — import from `pkg/playbook` instead of local types

**Interfaces:**
- Consumes: `yaml.v3` (already in go.sum)
- Produces: Package `pkg/playbook` with `TemplatePlaybook`, `TemplateTask`, `TemplateDefaultConfig`, `ActionTemplate`, `GetActionTemplates()`, `RenderTemplateYAML(tpl *TemplatePlaybook) ([]byte, error)`

- [ ] **Step 1: Create `pkg/playbook/template.go`**

```go
package playbook

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ActionTemplate struct {
	Name        string
	Description string
	Template    map[string]interface{}
}

func GetActionTemplates() []ActionTemplate {
	return actionTemplates
}

var actionTemplates = []ActionTemplate{
	{
		Name:        "command",
		Description: "执行 Shell 命令",
		Template:    map[string]interface{}{"cmd": "<命令内容>"},
	},
	{
		Name:        "script",
		Description: "执行脚本文件",
		Template:    map[string]interface{}{"script": "<脚本路径>", "dest": "/tmp/", "args": ""},
	},
	{
		Name:        "upload",
		Description: "上传文件到节点",
		Template:    map[string]interface{}{"src": "<本地路径>", "dest": "<远程路径>", "overwrite": true},
	},
	{
		Name:        "download",
		Description: "从节点下载文件",
		Template:    map[string]interface{}{"src": "<远程路径>", "dest": "<本地路径>", "subdir": true},
	},
	{
		Name:        "include",
		Description: "包含其他剧本",
		Template:    map[string]interface{}{"playbook": "<剧本路径>"},
	},
}

type TemplateDefaultConfig struct {
	Groups   []string `yaml:"groups,omitempty"`
	Tags     []string `yaml:"tags,omitempty"`
	SkipTags []string `yaml:"skip_tags,omitempty"`
}

type TemplatePlaybook struct {
	Name          string                 `yaml:"name"`
	Description   string                 `yaml:"description,omitempty"`
	Version       string                 `yaml:"version,omitempty"`
	Hosts         []string               `yaml:"hosts"`
	ExecutionMode string                 `yaml:"execution_mode,omitempty"`
	Default       *TemplateDefaultConfig `yaml:"default,omitempty"`
	Vars          map[string]interface{} `yaml:"vars,omitempty"`
	PreTasks      []TemplateTask         `yaml:"pre_tasks"`
	Tasks         []TemplateTask         `yaml:"tasks"`
	PostTasks     []TemplateTask         `yaml:"post_tasks"`
}

type TemplateTask struct {
	Name   string                 `yaml:"name"`
	Action string                 `yaml:"action"`
	Args   map[string]interface{} `yaml:"args"`
}

func RenderTemplateYAML(tpl *TemplatePlaybook) ([]byte, error) {
	return yaml.Marshal(tpl)
}
```

- [ ] **Step 2: Update CLI `template.go` to import from `pkg/playbook`**

Remove local type definitions from `cmd/cli/cmd/playbook/template.go` (lines 17-94: `ActionTemplate`, `actionTemplates`, `TemplateDefaultConfig`, `TemplatePlaybook`, `TemplateTask`, `GetActionTemplates`). Update import to add `"github.com/cangyunye/go-owl/pkg/playbook"`.

In `runPlaybookTemplate`, replace `yaml.Marshal(&playbook)` with `playbook.RenderTemplateYAML(&playbook)` — but note the variable `playbook` shadows the package. Rename the local variable to `tpl`.

Run: `go build ./cmd/cli/...` — must compile clean.

- [ ] **Step 3: Run tests to verify CLI still works**

Run: `go test ./cmd/cli/cmd/playbook/... -v` — all tests must pass, especially the template test checking 5 action types.

---

### Task 2: Add `POST /api/v1/playbook/template` backend endpoint

**Files:**
- Modify: `cmd/plugins/serve/handler/playbook.go` — add `Create` handler
- Modify: `cmd/plugins/serve/server.go` — register new route
- Modify: `cmd/plugins/serve/handler/playbook.go` — import `pkg/playbook`

**Interfaces:**
- Consumes: `POST /api/v1/playbook/template` with JSON body matching the frontend wizard output
- Produces: Saves YAML file to library path, triggers SyncFromDir, returns playbook entry

- [ ] **Step 1: Add `Create` handler method**

In `cmd/plugins/serve/handler/playbook.go`, add:

```go
import (
	// ... existing imports
	pb "github.com/cangyunye/go-owl/pkg/playbook"
)

type createTemplateRequest struct {
	Name          string                     `json:"name"`
	Description   string                     `json:"description,omitempty"`
	Version       string                     `json:"version,omitempty"`
	ExecutionMode string                     `json:"execution_mode,omitempty"`
	Vars          map[string]interface{}     `json:"vars,omitempty"`
	DefaultGroups []string                   `json:"default_groups,omitempty"`
	DefaultTags   []string                   `json:"default_tags,omitempty"`
	DefaultSkipTags []string                 `json:"default_skip_tags,omitempty"`
	Tasks         []createTemplateTask       `json:"tasks"`
}

type createTemplateTask struct {
	Name   string                 `json:"name"`
	Action string                 `json:"action"`
	Args   map[string]interface{} `json:"args"`
}

func (h *PlaybookHandler) Create(c *gin.Context) {
	var req createTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name is required"})
		return
	}
	if req.Version == "" {
		req.Version = "1.0"
	}

	// Build template
	var defaultCfg *pb.TemplateDefaultConfig
	if len(req.DefaultGroups) > 0 || len(req.DefaultTags) > 0 || len(req.DefaultSkipTags) > 0 {
		defaultCfg = &pb.TemplateDefaultConfig{
			Groups:   req.DefaultGroups,
			Tags:     req.DefaultTags,
			SkipTags: req.DefaultSkipTags,
		}
	}

	tasks := make([]pb.TemplateTask, len(req.Tasks))
	for i, t := range req.Tasks {
		tasks[i] = pb.TemplateTask{
			Name:   t.Name,
			Action: t.Action,
			Args:   t.Args,
		}
	}

	tpl := &pb.TemplatePlaybook{
		Name:          req.Name,
		Description:   req.Description,
		Version:       req.Version,
		Hosts:         []string{},
		ExecutionMode: req.ExecutionMode,
		Default:       defaultCfg,
		Vars:          req.Vars,
		PreTasks:      []pb.TemplateTask{},
		Tasks:         tasks,
		PostTasks:     []pb.TemplateTask{},
	}

	yamlData, err := pb.RenderTemplateYAML(tpl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "generate yaml failed"})
		return
	}

	// Determine output path
	libraryPath := h.getPlaybookLibraryPath()
	outputPath := filepath.Join(libraryPath, req.Name+".yaml")

	if err := os.MkdirAll(libraryPath, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create directory failed"})
		return
	}
	if err := os.WriteFile(outputPath, yamlData, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "write file failed"})
		return
	}

	// Auto-refresh to register the new playbook
	_, _, err = h.playbooks.SyncFromDir(c.Request.Context(), libraryPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "sync failed: " + err.Error()})
		return
	}

	// Return the new playbook entry
	all, _ := h.playbooks.List(c.Request.Context())
	var created *model.Playbook
	for i := range all {
		if all[i].Name == req.Name {
			created = &all[i]
			break
		}
	}

	c.JSON(http.StatusCreated, gin.H{"data": created, "file_path": outputPath})
}

// helper — reads library path from settings
func (h *PlaybookHandler) getPlaybookLibraryPath() string {
	var path string
	err := h.db.QueryRow(`SELECT value FROM settings WHERE key = 'playbook_library_path'`).Scan(&path)
	if err != nil || path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".owl", "playbooks")
	}
	return path
}
```

- [ ] **Step 2: Register route in `server.go`**

In the admin group block:
```go
admin.POST("/playbook/template", s.playbookHandler.Create)
```

Add to `operator` group so admin+operator can both use it, or keep in admin. CLI `owl playbook template` has no auth, and the web endpoint writes to filesystem, so admin-level makes sense. Add to admin group.

- [ ] **Step 3: Run to verify compilation**

Run: `go build ./cmd/plugins/serve/...` — must compile clean.

---

### Task 3: Frontend API client — add `api.createPlaybookTemplate()`

**Files:**
- Modify: `cmd/plugins/serve/web/js/api.js`

- [ ] **Step 1: Add API method**

```javascript
createPlaybookTemplate: (data) =>
  request('POST', '/playbook/template', data),
```

Add after the `playbookSettingsPath` method (around line 184).

---

### Task 4: Frontend wizard modal — wire up "新建" button with multi-step form

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/playbooks.js`

**Architecture:** The existing template string in `render()` will get a new modal overlay added (similar to `#run-playbook-modal` pattern). The modal has multiple "pages" (divs), shown/hidden by JS state. Steps:

1. **Step 1 — Basic Info**: name (required), description, version (default "1.0")
2. **Step 2 — Variables**: dynamic key-value rows (add/remove)
3. **Step 3 — Execution Mode + Default Config**: mode (fail_continue/pipeline), groups, tags, skip_tags
4. **Step 4 — Tasks**: add tasks with action type selection (5 types), remove tasks, preview list
5. **Step 5 — Confirm**: summary of all settings, "Save" button calls API

- [ ] **Step 1: Add modal HTML to the render template**

Append before the closing backtick of the template string, after the existing `#playbook-detail-modal` div:

```javascript
<div class="modal-overlay" id="create-playbook-modal">
  <div class="modal modal-lg">
    <div class="modal-header">
      <h3>📝 创建剧本</h3>
      <button class="btn btn-ghost btn-sm" id="create-pb-close-btn" style="margin-left:auto;background:none;border:none;color:var(--muted);cursor:pointer;font-size:18px">&times;</button>
    </div>
    <div class="modal-body" id="create-pb-body" style="max-height:70vh;overflow-y:auto">
      <!-- Step indicator -->
      <div style="display:flex;gap:8px;margin-bottom:16px" id="create-pb-steps">
        <span class="step-dot active" data-step="1">1</span><span style="color:var(--muted);font-size:12px">基本信息</span>
        <span style="color:var(--muted);padding:0 4px">→</span>
        <span class="step-dot" data-step="2">2</span><span style="color:var(--muted);font-size:12px">变量</span>
        <span style="color:var(--muted);padding:0 4px">→</span>
        <span class="step-dot" data-step="3">3</span><span style="color:var(--muted);font-size:12px">执行配置</span>
        <span style="color:var(--muted);padding:0 4px">→</span>
        <span class="step-dot" data-step="4">4</span><span style="color:var(--muted);font-size:12px">任务</span>
        <span style="color:var(--muted);padding:0 4px">→</span>
        <span class="step-dot" data-step="5">5</span><span style="color:var(--muted);font-size:12px">确认</span>
      </div>

      <!-- Step 1: Basic Info -->
      <div class="create-pb-page" data-page="1">
        <div class="form-row"><label>剧本名称 *</label><input id="cp-name" placeholder="my-playbook" style="width:100%"></div>
        <div class="form-row"><label>描述</label><textarea id="cp-desc" placeholder="可选" style="width:100%;resize:vertical" rows="2"></textarea></div>
        <div class="form-row"><label>版本</label><input id="cp-version" value="1.0" style="width:100%"></div>
      </div>

      <!-- Step 2: Variables -->
      <div class="create-pb-page" data-page="2" style="display:none">
        <p style="font-size:13px;color:var(--muted);margin-bottom:12px">添加变量（可选）</p>
        <div id="cp-vars-list"></div>
        <button class="btn btn-secondary btn-sm" id="cp-add-var"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 添加变量</button>
        <div style="margin-top:8px"><label class="checkbox-label"><input type="checkbox" id="cp-skip-vars"> 跳过（不添加变量）</label></div>
      </div>

      <!-- Step 3: Execution Mode + Default Config -->
      <div class="create-pb-page" data-page="3" style="display:none">
        <div class="form-row">
          <label>执行模式</label>
          <select id="cp-mode" style="width:100%">
            <option value="">fail_continue（失败继续）</option>
            <option value="pipeline">pipeline（失败终止）</option>
          </select>
        </div>
        <h4 style="font-size:13px;color:var(--muted);margin:16px 0 8px">默认配置（可选）</h4>
        <div class="form-row"><label>目标分组</label><input id="cp-groups" placeholder="web, db (逗号分隔)" style="width:100%"></div>
        <div class="form-row"><label>执行标签</label><input id="cp-tags" placeholder="tag1, tag2 (逗号分隔)" style="width:100%"></div>
        <div class="form-row"><label>跳过标签</label><input id="cp-skip-tags" placeholder="skip-me (逗号分隔)" style="width:100%"></div>
      </div>

      <!-- Step 4: Tasks -->
      <div class="create-pb-page" data-page="4" style="display:none">
        <p style="font-size:13px;color:var(--muted);margin-bottom:12px">添加任务项</p>
        <div class="form-row" style="display:flex;gap:8px;align-items:end">
          <div style="flex:1">
            <label>任务类型</label>
            <select id="cp-task-action" style="width:100%">
              <option value="command">command — 执行 Shell 命令</option>
              <option value="script">script — 执行脚本文件</option>
              <option value="upload">upload — 上传文件到节点</option>
              <option value="download">download — 从节点下载文件</option>
              <option value="include">include — 包含其他剧本</option>
            </select>
          </div>
          <button class="btn btn-primary btn-sm" id="cp-add-task" style="white-space:nowrap">+ 添加</button>
        </div>
        <div id="cp-tasks-list" style="margin-top:12px">
          <p class="empty-state" style="font-size:13px;padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>
        </div>
      </div>

      <!-- Step 5: Confirm -->
      <div class="create-pb-page" data-page="5" style="display:none">
        <div id="cp-summary" style="font-size:13px;margin-bottom:12px"></div>
        <h4 style="font-size:13px;color:var(--muted);margin-bottom:8px">YAML 预览</h4>
        <pre id="cp-preview" style="background:var(--code-bg);border:1px solid var(--border);border-radius:var(--radius);padding:12px;font:12px/1.6 var(--font-mono);overflow-x:auto;white-space:pre;max-height:300px"></pre>
      </div>
    </div>
    <div class="modal-actions">
      <button class="btn-cancel" id="cp-prev-btn" style="display:none">上一步</button>
      <button class="btn-primary" id="cp-next-btn">下一步</button>
      <button class="btn-primary" id="cp-save-btn" style="display:none">保存剧本</button>
    </div>
    <p class="error-msg" id="cp-error"></p>
  </div>
</div>
```

- [ ] **Step 2: Wire up event handlers in the `afterRender` callback**

Add after the existing event handler registrations (before `loadAll()`):

```javascript
// Create playbook wizard state
let cpState = {
  step: 1,
  totalSteps: 5,
  vars: [],
  tasks: [],
};
let cpTaskCounter = 0;

document.getElementById('add-playbook-btn').addEventListener('click', () => {
  cpState = { step: 1, totalSteps: 5, vars: [], tasks: [] };
  cpTaskCounter = 0;
  document.getElementById('cp-name').value = '';
  document.getElementById('cp-desc').value = '';
  document.getElementById('cp-version').value = '1.0';
  document.getElementById('cp-mode').value = '';
  document.getElementById('cp-groups').value = '';
  document.getElementById('cp-tags').value = '';
  document.getElementById('cp-skip-tags').value = '';
  document.getElementById('cp-vars-list').innerHTML = '';
  document.getElementById('cp-skip-vars').checked = false;
  document.getElementById('cp-tasks-list').innerHTML = '<p class="empty-state" style="font-size:13px;padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>';
  document.getElementById('cp-error').textContent = '';
  showCpStep(1);
  document.getElementById('create-playbook-modal').classList.add('open');
});

document.getElementById('create-pb-close-btn').addEventListener('click', () => {
  document.getElementById('create-playbook-modal').classList.remove('open');
});
document.getElementById('create-playbook-modal').addEventListener('click', (e) => {
  if (e.target === e.currentTarget) document.getElementById('create-playbook-modal').classList.remove('open');
});

function showCpStep(n) {
  cpState.step = n;
  document.querySelectorAll('.create-pb-page').forEach(el => el.style.display = 'none');
  document.querySelector(`.create-pb-page[data-page="${n}"]`).style.display = 'block';
  document.querySelectorAll('.step-dot').forEach(el => el.classList.toggle('active', parseInt(el.dataset.step) <= n));
  document.getElementById('cp-prev-btn').style.display = n > 1 ? '' : 'none';
  document.getElementById('cp-next-btn').style.display = n < cpState.totalSteps ? '' : 'none';
  document.getElementById('cp-save-btn').style.display = n === cpState.totalSteps ? '' : 'none';
  if (n === cpState.totalSteps) buildCpSummary();
  document.getElementById('cp-error').textContent = '';
}

document.getElementById('cp-prev-btn').addEventListener('click', () => {
  if (cpState.step > 1) showCpStep(cpState.step - 1);
});

document.getElementById('cp-next-btn').addEventListener('click', () => {
  const err = document.getElementById('cp-error');
  if (cpState.step === 1) {
    if (!document.getElementById('cp-name').value.trim()) {
      err.textContent = '剧本名称不能为空';
      return;
    }
  }
  if (cpState.step < cpState.totalSteps) showCpStep(cpState.step + 1);
});

// Variables
document.getElementById('cp-add-var').addEventListener('click', () => {
  const idx = cpState.vars.length;
  cpState.vars.push({ key: '', value: '' });
  const row = document.createElement('div');
  row.className = 'form-row';
  row.style.cssText = 'display:flex;gap:8px;align-items:end';
  row.innerHTML = `
    <div style="flex:1"><input class="cp-var-key" data-idx="${idx}" placeholder="变量名" style="width:100%"></div>
    <div style="flex:1"><input class="cp-var-value" data-idx="${idx}" placeholder="值" style="width:100%"></div>
    <button class="cp-var-remove" data-idx="${idx}" style="background:none;border:1px solid var(--danger);color:var(--danger);border-radius:var(--radius);cursor:pointer;padding:4px 8px;font-size:12px">删除</button>
  `;
  document.getElementById('cp-vars-list').appendChild(row);
  row.querySelector('.cp-var-key').addEventListener('input', (e) => { cpState.vars[idx].key = e.target.value; });
  row.querySelector('.cp-var-value').addEventListener('input', (e) => { cpState.vars[idx].value = e.target.value; });
  row.querySelector('.cp-var-remove').addEventListener('click', () => {
    cpState.vars.splice(idx, 1);
    row.remove();
  });
});

// Tasks
document.getElementById('cp-add-task').addEventListener('click', () => {
  const action = document.getElementById('cp-task-action').value;
  const actionLabels = { command: 'command', script: 'script', upload: 'upload', download: 'download', include: 'include' };
  cpTaskCounter++;
  const task = { name: `任务 ${cpTaskCounter}`, action, args: getActionArgs(action) };
  cpState.tasks.push(task);
  renderCpTasks();
});

function getActionArgs(action) {
  const templates = {
    command: { cmd: '<命令内容>' },
    script: { script: '<脚本路径>', dest: '/tmp/', args: '' },
    upload: { src: '<本地路径>', dest: '<远程路径>', overwrite: true },
    download: { src: '<远程路径>', dest: '<本地路径>', subdir: true },
    include: { playbook: '<剧本路径>' },
  };
  return JSON.parse(JSON.stringify(templates[action] || {}));
}

function renderCpTasks() {
  const list = document.getElementById('cp-tasks-list');
  if (cpState.tasks.length === 0) {
    list.innerHTML = '<p class="empty-state" style="font-size:13px;padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>';
    return;
  }
  list.innerHTML = cpState.tasks.map((t, i) =>
    `<div style="display:flex;align-items:center;gap:8px;padding:8px;border:1px solid var(--border);border-radius:var(--radius);margin-bottom:6px">
      <span style="font-weight:500;font-size:13px;flex:1">${esc(t.name)}</span>
      <span class="tag">${esc(t.action)}</span>
      <button class="cp-task-remove" data-idx="${i}" style="background:none;border:1px solid var(--danger);color:var(--danger);border-radius:var(--radius);cursor:pointer;padding:2px 8px;font-size:11px">删除</button>
    </div>`
  ).join('');
  document.querySelectorAll('.cp-task-remove').forEach(btn => {
    btn.addEventListener('click', () => {
      cpState.tasks.splice(parseInt(btn.dataset.idx), 1);
      renderCpTasks();
    });
  });
}

function buildCpSummary() {
  const name = document.getElementById('cp-name').value.trim();
  const desc = document.getElementById('cp-desc').value.trim();
  const version = document.getElementById('cp-version').value.trim() || '1.0';
  const mode = document.getElementById('cp-mode').value || 'fail_continue';

  const html = `
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px">
      <div><strong>名称:</strong> ${esc(name)}</div>
      <div><strong>版本:</strong> ${esc(version)}</div>
      <div><strong>描述:</strong> ${esc(desc || '-')}</div>
      <div><strong>执行模式:</strong> ${mode}</div>
    </div>
  `;
  document.getElementById('cp-summary').innerHTML = html;

  // Build preview YAML (for visual only — the actual YAML is generated server-side)
  let preview = `name: ${name}\n`;
  if (desc) preview += `description: ${desc}\n`;
  preview += `version: "${version}"\nhosts: []\n`;
  if (mode) preview += `execution_mode: ${mode}\n`;
  preview += `pre_tasks: []\n`;
  preview += `tasks:\n`;
  for (const t of cpState.tasks) {
    preview += `  - name: ${t.name}\n    action: ${t.action}\n    args:\n`;
    for (const [k, v] of Object.entries(t.args)) {
      const val = typeof v === 'string' ? v : JSON.stringify(v);
      preview += `      ${k}: ${val}\n`;
    }
  }
  preview += `post_tasks: []\n`;
  document.getElementById('cp-preview').textContent = preview;
}

// Save
document.getElementById('cp-save-btn').addEventListener('click', async () => {
  const err = document.getElementById('cp-error');
  const name = document.getElementById('cp-name').value.trim();
  if (!name) { err.textContent = '剧本名称不能为空'; return; }

  // Collect vars
  const vars = {};
  if (!document.getElementById('cp-skip-vars').checked) {
    for (const v of cpState.vars) {
      if (v.key.trim()) vars[v.key.trim()] = v.value;
    }
  }

  const data = {
    name,
    description: document.getElementById('cp-desc').value.trim() || undefined,
    version: document.getElementById('cp-version').value.trim() || '1.0',
    execution_mode: document.getElementById('cp-mode').value || undefined,
    vars: Object.keys(vars).length > 0 ? vars : undefined,
    default_groups: document.getElementById('cp-groups').value.trim() ? document.getElementById('cp-groups').value.split(',').map(s => s.trim()).filter(Boolean) : undefined,
    default_tags: document.getElementById('cp-tags').value.trim() ? document.getElementById('cp-tags').value.split(',').map(s => s.trim()).filter(Boolean) : undefined,
    default_skip_tags: document.getElementById('cp-skip-tags').value.trim() ? document.getElementById('cp-skip-tags').value.split(',').map(s => s.trim()).filter(Boolean) : undefined,
    tasks: cpState.tasks,
  };

  try {
    document.getElementById('cp-save-btn').textContent = '保存中…';
    document.getElementById('cp-save-btn').disabled = true;
    await api.createPlaybookTemplate(data);
    document.getElementById('create-playbook-modal').classList.remove('open');
    loadAll();
    loadRuns();
  } catch (e) {
    err.textContent = e.message;
  } finally {
    document.getElementById('cp-save-btn').textContent = '保存剧本';
    document.getElementById('cp-save-btn').disabled = false;
  }
});
```

- [ ] **Step 3: Add minimal CSS for step dots and wizard**

In `app.css`, add:

```css
.step-dot {
  display:inline-flex;align-items:center;justify-content:center;
  width:24px;height:24px;border-radius:50%;
  background:var(--surface-2);color:var(--muted);
  font-size:12px;font-weight:600;transition:all var(--transition);
}
.step-dot.active { background:var(--accent);color:#fff; }

.create-pb-page { min-height:200px; }
```

---

### Task 5: Verify end-to-end

- [ ] **Step 1: Build and start server**

```bash
go build ./cmd/plugins/serve/... -o ./build/owl-serve
./build/owl-serve --reset-admin --port 8080
```

- [ ] **Step 2: Login and navigate to /playbooks**

Check that "新建" button exists in the filter bar. Click it — modal opens at step 1.

- [ ] **Step 3: Walk through all 5 steps**

Fill in name, description, version → add vars → set mode and default config → add tasks of each type → review summary and YAML preview → click save.

- [ ] **Step 4: Verify playbook appears in list**

After saving, the playbook list should refresh and show the new playbook. The YAML file should exist in the library path.

- [ ] **Step 5: Verify CLI consistency**

Run: `go build ./cmd/cli/... -o ./build/owl` then `./build/owl playbook list` — should show the same playbook.

Run: `cat ~/.owl/playbooks/<name>.yaml` — YAML should be structurally identical to what `owl playbook template` produces (same fields, same host/empty array handling).
