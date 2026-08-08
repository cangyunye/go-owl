# Playbook 模板系统 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现设计文档 §3-§4、§8.2-§8.6、§9 描述的模板系统：分层目录加载、参数化模板、template list/info/export 命令、new --from 实例化、scaffold 骨架生成、内置模板库。

**Architecture:** 模板加载器放在 `pkg/playbook/`（CLI 和 Web 共享），扫描 `~/.owl/templates/`（用户）和 `~/.owl/builtin-templates/`（内置，go:embed 回退）两级目录。模板文件是标准 Playbook YAML + 顶层 `parameters` 元数据。`owl playbook template` 从叶子命令重构为父命令，挂载 list/info/export 子命令；新增 `owl playbook new` 和 `owl playbook scaffold` 作为 playbook 直属子命令。

**Tech Stack:** Go 1.26, cobra, gopkg.in/yaml.v3, go:embed

## Global Constraints

- 模板 YAML 必须是合法 Playbook（可被 `pbexec.NewParser().ParseFromFile` 解析）
- 用户模板优先于内置模板（同名覆盖）
- `parameters` 中的 `default` 是提示性默认值，实例化时绑定
- 不破坏现有 `owl playbook template`（交互向导）功能——重构为 `template create` 子命令
- 所有 UI 文本使用中文（与现有命令一致）

---

## File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `pkg/playbook/template_loader.go` | 模板目录扫描、分层加载、名称解析 |
| Create | `pkg/playbook/template_types.go` | TemplateMeta 结构体（description/tags/parameters）、参数验证 |
| Create | `pkg/playbook/template_loader_test.go` | 加载器 + 参数验证测试 |
| Create | `pkg/playbook/builtin/embed.go` | go:embed 内置模板 YAML |
| Create | `pkg/playbook/builtin/webserver/nginx/deploy.yaml` | 内置模板：Nginx 部署 |
| Create | `pkg/playbook/builtin/utility/healthcheck/http.yaml` | 内置模板：HTTP 健康检查 |
| Create | `pkg/playbook/builtin/utility/backup/files.yaml` | 内置模板：文件备份 |
| Create | `cmd/cli/cmd/playbook/template_list.go` | `template list` 子命令 |
| Create | `cmd/cli/cmd/playbook/template_info.go` | `template info <name>` 子命令 |
| Create | `cmd/cli/cmd/playbook/template_export.go` | `template export <name>` 子命令 |
| Create | `cmd/cli/cmd/playbook/new.go` | `owl playbook new --from` 命令 |
| Create | `cmd/cli/cmd/playbook/scaffold.go` | `owl playbook scaffold` 命令 |
| Modify | `cmd/cli/cmd/playbook/template.go` | 重构为父命令，原向导移至 `template create` |
| Modify | `cmd/cli/cmd/playbook/playbook.go` | 注册 new、scaffold 子命令 |

---

### Task 1: 模板类型定义 + 参数验证

**Files:**
- Create: `pkg/playbook/template_types.go`
- Test: `pkg/playbook/template_loader_test.go`

**Interfaces:**
- Produces: `TemplateMeta`, `TemplateParameter`, `ValidateParams()`, `Instantiate()`

- [ ] **Step 1: Write failing tests for parameter validation**

```go
// pkg/playbook/template_loader_test.go
package playbook

import "testing"

func TestValidateParams_Defaults(t *testing.T) {
	params := []TemplateParameter{
		{Name: "version", Description: "版本号", Default: "1.0", Type: "string"},
		{Name: "port", Description: "端口", Default: 80, Type: "number"},
	}
	vals, err := ValidateParams(params, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if vals["version"] != "1.0" {
		t.Errorf("expected default version 1.0, got %v", vals["version"])
	}
}

func TestValidateParams_Required(t *testing.T) {
	params := []TemplateParameter{
		{Name: "app_name", Description: "应用名", Required: true},
	}
	_, err := ValidateParams(params, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
}

func TestValidateParams_Options(t *testing.T) {
	params := []TemplateParameter{
		{Name: "port", Description: "端口", Options: []interface{}{80, 443}},
	}
	_, err := ValidateParams(params, map[string]interface{}{"port": 8080})
	if err == nil {
		t.Fatal("expected error for invalid option")
	}
}

func TestValidateParams_Pattern(t *testing.T) {
	params := []TemplateParameter{
		{Name: "version", Description: "版本", Pattern: `^\d+\.\d+\.\d+$`},
	}
	_, err := ValidateParams(params, map[string]interface{}{"version": "abc"})
	if err == nil {
		t.Fatal("expected error for pattern mismatch")
	}
	vals, err := ValidateParams(params, map[string]interface{}{"version": "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if vals["version"] != "1.2.3" {
		t.Errorf("expected 1.2.3, got %v", vals["version"])
	}
}

func TestInstantiate(t *testing.T) {
	yamlContent := `description: test
parameters:
  - name: app_name
    description: "应用名"
    default: myapp
tasks:
  - name: 部署
    action: command
    args:
      cmd: echo {{ app_name }}
`
	result, err := Instantiate([]byte(yamlContent), map[string]interface{}{"app_name": "prod-app"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(result), "prod-app") {
		t.Errorf("expected instantiated content to contain prod-app")
	}
	if contains(string(result), "parameters:") {
		t.Errorf("instantiated content should not contain parameters block")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/playbook/ -run 'TestValidateParams|TestInstantiate' -v`
Expected: FAIL — `TemplateParameter`, `ValidateParams`, `Instantiate` undefined

- [ ] **Step 3: Implement template_types.go**

```go
// pkg/playbook/template_types.go
package playbook

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type TemplateParameter struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Type        string        `yaml:"type,omitempty"`
	Required    bool          `yaml:"required,omitempty"`
	Default     interface{}   `yaml:"default,omitempty"`
	Options     []interface{} `yaml:"options,omitempty"`
	Pattern     string        `yaml:"pattern,omitempty"`
}

type TemplateMeta struct {
	Description string              `yaml:"description,omitempty"`
	Tags        []string            `yaml:"tags,omitempty"`
	Parameters  []TemplateParameter `yaml:"parameters,omitempty"`
}

type templateFile struct {
	Meta TemplateMeta `yaml:",inline"`
	Rest yaml.Node    `yaml:",inline"`
}

func ParseTemplateMeta(data []byte) (*TemplateMeta, error) {
	var meta TemplateMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse template meta: %w", err)
	}
	return &meta, nil
}

func ValidateParams(params []TemplateParameter, provided map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, p := range params {
		val, ok := provided[p.Name]
		if !ok {
			if p.Default != nil {
				result[p.Name] = p.Default
				continue
			}
			if p.Required {
				return nil, fmt.Errorf("缺少必填参数: %s (%s)", p.Name, p.Description)
			}
			continue
		}
		result[p.Name] = val
	}

	for _, p := range params {
		val, ok := result[p.Name]
		if !ok {
			continue
		}
		if len(p.Options) > 0 {
			found := false
			for _, opt := range p.Options {
				if fmt.Sprintf("%v", opt) == fmt.Sprintf("%v", val) {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("参数 %s 的值 %v 不在可选项 %v 中", p.Name, val, p.Options)
			}
		}
		if p.Pattern != "" {
			re, err := regexp.Compile(p.Pattern)
			if err != nil {
				return nil, fmt.Errorf("参数 %s 的正则表达式无效: %w", p.Name, err)
			}
			if !re.MatchString(fmt.Sprintf("%v", val)) {
				return nil, fmt.Errorf("参数 %s 的值 %q 不匹配格式 %s", p.Name, val, p.Pattern)
			}
		}
	}

	return result, nil
}

func Instantiate(templateData []byte, vars map[string]interface{}) ([]byte, error) {
	var raw map[string]yaml.Node
	if err := yaml.Unmarshal(templateData, &raw); err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	delete(raw, "parameters")
	delete(raw, "description")
	delete(raw, "tags")

	cleaned, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal template: %w", err)
	}

	content := string(cleaned)
	for k, v := range vars {
		placeholder1 := fmt.Sprintf("{{ %s }}", k)
		placeholder2 := fmt.Sprintf("{{%s}}", k)
		replacement := fmt.Sprintf("%v", v)
		content = strings.ReplaceAll(content, placeholder1, replacement)
		content = strings.ReplaceAll(content, placeholder2, replacement)
	}

	return []byte(content), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/playbook/ -run 'TestValidateParams|TestInstantiate' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/playbook/template_types.go pkg/playbook/template_loader_test.go
git commit -m "feat(playbook): add template metadata types and parameter validation"
```

---

### Task 2: 模板目录加载器

**Files:**
- Create: `pkg/playbook/template_loader.go`
- Create: `pkg/playbook/builtin/embed.go`
- Create: `pkg/playbook/builtin/utility/healthcheck/http.yaml`
- Test: `pkg/playbook/template_loader_test.go` (append)

**Interfaces:**
- Consumes: `TemplateMeta`, `ParseTemplateMeta` from Task 1
- Produces: `TemplateEntry`, `LoadTemplates()`, `GetTemplate()`, `TemplatePaths()`

- [ ] **Step 1: Create a minimal builtin template**

```yaml
# pkg/playbook/builtin/utility/healthcheck/http.yaml
description: HTTP 健康检查模板，对目标节点执行 HTTP 探活
tags: [healthcheck, http, utility]

parameters:
  - name: url
    description: "检查的 URL 地址"
    default: "http://localhost:80/"
    required: false
  - name: expected_code
    description: "期望的 HTTP 状态码"
    default: 200
    type: number
    required: false
  - name: timeout
    description: "请求超时（秒）"
    default: 5
    type: number
    required: false

tasks:
  - name: HTTP 健康检查
    action: command
    args:
      cmd: |
        code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout {{ timeout }} {{ url }})
        if [ "$code" = "{{ expected_code }}" ]; then
          echo "OK: {{ url }} returned $code"
        else
          echo "FAIL: {{ url }} returned $code (expected {{ expected_code }})"
          exit 1
        fi
```

- [ ] **Step 2: Create embed.go**

```go
// pkg/playbook/builtin/embed.go
package builtin

import "embed"

//go:embed **/*.yaml
var Templates embed.FS
```

- [ ] **Step 3: Write failing tests for loader**

```go
// append to pkg/playbook/template_loader_test.go

func TestLoadTemplates_BuiltinOnly(t *testing.T) {
	entries, err := LoadTemplates("/nonexistent/user/path")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected builtin templates to be loaded")
	}
	found := false
	for _, e := range entries {
		if e.Name == "utility/healthcheck/http" {
			found = true
			if e.Source != "builtin" {
				t.Errorf("expected source builtin, got %s", e.Source)
			}
		}
	}
	if !found {
		t.Error("expected to find utility/healthcheck/http template")
	}
}

func TestGetTemplate(t *testing.T) {
	entry, err := GetTemplate("utility/healthcheck/http", "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Meta.Description == "" {
		t.Error("expected non-empty description")
	}
	if len(entry.Meta.Parameters) == 0 {
		t.Error("expected parameters")
	}
	if entry.Content == nil {
		t.Error("expected non-nil content")
	}
}

func TestGetTemplate_NotFound(t *testing.T) {
	_, err := GetTemplate("nonexistent/template", "/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./pkg/playbook/ -run 'TestLoadTemplates|TestGetTemplate' -v`
Expected: FAIL — `LoadTemplates`, `GetTemplate`, `TemplateEntry` undefined

- [ ] **Step 5: Implement template_loader.go**

```go
// pkg/playbook/template_loader.go
package playbook

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cangyunye/go-owl/pkg/playbook/builtin"
)

type TemplateEntry struct {
	Name     string
	Category string
	Source   string // "user" or "builtin"
	Path     string
	Meta     *TemplateMeta
	Content  []byte
}

func DefaultUserTemplatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".owl", "templates")
}

func LoadTemplates(userPath string) ([]*TemplateEntry, error) {
	entries := make(map[string]*TemplateEntry)

	loadFromFS(builtin.Templates, "builtin", entries)

	if userPath == "" {
		userPath = DefaultUserTemplatePath()
	}
	if info, err := os.Stat(userPath); err == nil && info.IsDir() {
		loadFromDir(userPath, entries)
	}

	result := make([]*TemplateEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, e)
	}
	return result, nil
}

func loadFromFS(fsys fs.FS, source string, entries map[string]*TemplateEntry) {
	fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !isYAML(path) {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil
		}
		name := templateNameFromPath(path)
		meta, _ := ParseTemplateMeta(data)
		if meta == nil {
			meta = &TemplateMeta{}
		}
		entries[name] = &TemplateEntry{
			Name:     name,
			Category: categoryFromPath(path),
			Source:   source,
			Path:     path,
			Meta:     meta,
			Content:  data,
		}
		return nil
	})
}

func loadFromDir(dir string, entries map[string]*TemplateEntry) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !isYAML(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(dir, path)
		name := templateNameFromPath(relPath)
		meta, _ := ParseTemplateMeta(data)
		if meta == nil {
			meta = &TemplateMeta{}
		}
		entries[name] = &TemplateEntry{
			Name:     name,
			Category: categoryFromPath(relPath),
			Source:   "user",
			Path:     path,
			Meta:     meta,
			Content:  data,
		}
		return nil
	})
}

func GetTemplate(name string, userPath string) (*TemplateEntry, error) {
	all, err := LoadTemplates(userPath)
	if err != nil {
		return nil, err
	}
	for _, e := range all {
		if e.Name == name {
			return e, nil
		}
	}
	return nil, fmt.Errorf("模板 %q 不存在", name)
}

func templateNameFromPath(path string) string {
	path = strings.TrimSuffix(path, ".yaml")
	path = strings.TrimSuffix(path, ".yml")
	return filepath.ToSlash(path)
}

func categoryFromPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./pkg/playbook/ -v`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/playbook/template_loader.go pkg/playbook/builtin/
git commit -m "feat(playbook): add template directory loader with builtin templates"
```

---

### Task 3: 重构 template 命令为父命令 + list/info/export 子命令

**Files:**
- Modify: `cmd/cli/cmd/playbook/template.go`
- Create: `cmd/cli/cmd/playbook/template_list.go`
- Create: `cmd/cli/cmd/playbook/template_info.go`
- Create: `cmd/cli/cmd/playbook/template_export.go`

**Interfaces:**
- Consumes: `LoadTemplates`, `GetTemplate`, `TemplateEntry` from Task 2
- Produces: `NewPlaybookTemplateCmd()` 返回带子命令的父命令

- [ ] **Step 1: Restructure template.go — rename existing wizard to `create` subcommand, make `template` a parent**

将 `NewPlaybookTemplateCmd` 改为父命令，原交互向导逻辑移至 `NewPlaybookTemplateCreateCmd`。

- [ ] **Step 2: Implement template_list.go**

`owl playbook template list` — 调用 `LoadTemplates`，按 source 分组输出。

- [ ] **Step 3: Implement template_info.go**

`owl playbook template info <name>` — 调用 `GetTemplate`，输出描述、参数、任务列表、完整 YAML。

- [ ] **Step 4: Implement template_export.go**

`owl playbook template export <name> --to <dir>` — 将内置模板内容写入用户目录。

- [ ] **Step 5: Run build + existing tests**

Run: `go build ./... && go test ./cmd/cli/cmd/playbook/... -v`
Expected: PASS（原有 `TestPlaybookTemplateCmd` 需适配新的命令结构）

- [ ] **Step 6: Commit**

```bash
git add cmd/cli/cmd/playbook/template*.go
git commit -m "feat(playbook): restructure template command with list/info/export subcommands"
```

---

### Task 4: `owl playbook new` + `owl playbook scaffold`

**Files:**
- Create: `cmd/cli/cmd/playbook/new.go`
- Create: `cmd/cli/cmd/playbook/scaffold.go`
- Modify: `cmd/cli/cmd/playbook/playbook.go`

**Interfaces:**
- Consumes: `GetTemplate`, `ValidateParams`, `Instantiate` from Task 1-2
- Produces: `NewPlaybookNewCmd()`, `NewPlaybookScaffoldCmd()`

- [ ] **Step 1: Implement new.go**

`owl playbook new --from=<template> [--var key=value ...] [--output file]`
- 无 `--var` 时进入交互式参数输入（提示默认值）
- 调用 `ValidateParams` 验证 → `Instantiate` 渲染 → 写文件

- [ ] **Step 2: Implement scaffold.go**

`owl playbook scaffold [--type basic]` — 输出带注释的 Playbook 骨架到 stdout。

- [ ] **Step 3: Register in playbook.go**

- [ ] **Step 4: Build + test**

- [ ] **Step 5: Commit**

```bash
git add cmd/cli/cmd/playbook/new.go cmd/cli/cmd/playbook/scaffold.go cmd/cli/cmd/playbook/playbook.go
git commit -m "feat(playbook): add new --from and scaffold commands"
```

---

### Task 5: 补充内置模板

**Files:**
- Create: `pkg/playbook/builtin/webserver/nginx/deploy.yaml`
- Create: `pkg/playbook/builtin/utility/backup/files.yaml`

- [ ] **Step 1: 编写 nginx/deploy 模板**（对应设计文档 §4.3 完整示例）
- [ ] **Step 2: 编写 backup/files 模板**
- [ ] **Step 3: 验证所有内置模板可被 ParseTemplateMeta 解析**

Run: `go test ./pkg/playbook/ -run TestLoadTemplates -v`

- [ ] **Step 4: Commit**

```bash
git add pkg/playbook/builtin/
git commit -m "feat(playbook): add nginx deploy and file backup builtin templates"
```
