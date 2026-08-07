package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cangyunye/go-owl/internal/control/node"
)

// ---------- playbook template list ----------

type PlaybookTemplateListTool struct {
	executor Executor
}

func NewPlaybookTemplateListTool(executor Executor) *PlaybookTemplateListTool {
	return &PlaybookTemplateListTool{executor: executor}
}

func (t *PlaybookTemplateListTool) Name() string        { return "playbook_template_list" }
func (t *PlaybookTemplateListTool) Description() string { return "List all playbook templates." }
func (t *PlaybookTemplateListTool) Parameters() string  { return `{"type":"object","properties":{}}` }
func (t *PlaybookTemplateListTool) Validate(p map[string]interface{}) error { return nil }
func (t *PlaybookTemplateListTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		result, err := t.executor.PlaybookTemplateList(ctx)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("playbook_template_list failed")
}

// ---------- playbook template info ----------

type PlaybookTemplateInfoTool struct {
	executor Executor
}

func NewPlaybookTemplateInfoTool(executor Executor) *PlaybookTemplateInfoTool {
	return &PlaybookTemplateInfoTool{executor: executor}
}

func (t *PlaybookTemplateInfoTool) Name() string        { return "playbook_template_info" }
func (t *PlaybookTemplateInfoTool) Description() string { return "Show details of a playbook template." }
func (t *PlaybookTemplateInfoTool) Parameters() string { return playbookTemplateInfoParamsSchema }
func (t *PlaybookTemplateInfoTool) Validate(p map[string]interface{}) error {
	if strings.TrimSpace(strOf(p["name"])) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

const playbookTemplateInfoParamsSchema = `{
	"type": "object",
	"properties": {"name": {"type": "string", "description": "Template name"}},
	"required": ["name"]
}`

func (t *PlaybookTemplateInfoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		result, err := t.executor.PlaybookTemplateInfo(ctx, PlaybookTemplateInfoParams{Name: strOf(params["name"])})
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("playbook_template_info failed")
}

// ---------- playbook template export ----------

type PlaybookTemplateExportTool struct {
	executor Executor
}

func NewPlaybookTemplateExportTool(executor Executor) *PlaybookTemplateExportTool {
	return &PlaybookTemplateExportTool{executor: executor}
}

func (t *PlaybookTemplateExportTool) Name() string        { return "playbook_template_export" }
func (t *PlaybookTemplateExportTool) Description() string { return "Export a playbook template to a YAML file." }
func (t *PlaybookTemplateExportTool) Parameters() string  { return playbookTemplateExportParamsSchema }
func (t *PlaybookTemplateExportTool) Validate(p map[string]interface{}) error {
	if strings.TrimSpace(strOf(p["name"])) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

const playbookTemplateExportParamsSchema = `{
	"type": "object",
	"properties": {
		"name": {"type": "string", "description": "Template name"},
		"to": {"type": "string", "description": "Output file path, default ./playbooks/<name>.yaml"}
	},
	"required": ["name"]
}`

func (t *PlaybookTemplateExportTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := PlaybookTemplateExportParams{Name: strOf(params["name"]), To: strOf(params["to"])}
		result, err := t.executor.PlaybookTemplateExport(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("playbook_template_export failed")
}

// ---------- playbook scaffold ----------

type PlaybookScaffoldTool struct {
	executor Executor
}

func NewPlaybookScaffoldTool(executor Executor) *PlaybookScaffoldTool {
	return &PlaybookScaffoldTool{executor: executor}
}

func (t *PlaybookScaffoldTool) Name() string        { return "playbook_scaffold" }
func (t *PlaybookScaffoldTool) Description() string { return "Generate a playbook scaffold (basic or a specific action type)." }
func (t *PlaybookScaffoldTool) Parameters() string  { return playbookScaffoldParamsSchema }
func (t *PlaybookScaffoldTool) Validate(p map[string]interface{}) error { return nil }

const playbookScaffoldParamsSchema = `{
	"type": "object",
	"properties": {
		"type": {"type": "string", "description": "Scaffold type: basic (default), command, script, file_transfer, ..."}
	}
}`

func (t *PlaybookScaffoldTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		result, err := t.executor.PlaybookScaffold(ctx, PlaybookScaffoldParams{Type: strOf(params["type"])})
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("playbook_scaffold failed")
}

// ---------- playbook state list / show ----------

type PlaybookStateListTool struct {
	executor Executor
}

func NewPlaybookStateListTool(executor Executor) *PlaybookStateListTool {
	return &PlaybookStateListTool{executor: executor}
}

func (t *PlaybookStateListTool) Name() string        { return "playbook_state_list" }
func (t *PlaybookStateListTool) Description() string { return "List playbook execution runs and their status." }
func (t *PlaybookStateListTool) Parameters() string  { return playbookStateListParamsSchema }
func (t *PlaybookStateListTool) Validate(p map[string]interface{}) error { return nil }

const playbookStateListParamsSchema = `{
	"type": "object",
	"properties": {
		"playbook": {"type": "string", "description": "Filter by playbook name"},
		"status": {"type": "string", "description": "Filter by run status"},
		"limit": {"type": "integer", "description": "Max rows, default 20"}
	}
}`

func (t *PlaybookStateListTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := PlaybookStateListParams{
			Playbook: strOf(params["playbook"]),
			Status:   strOf(params["status"]),
			Limit:    intOf(params["limit"]),
		}
		result, err := t.executor.PlaybookStateList(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("playbook_state_list failed")
}

type PlaybookStateShowTool struct {
	executor Executor
}

func NewPlaybookStateShowTool(executor Executor) *PlaybookStateShowTool {
	return &PlaybookStateShowTool{executor: executor}
}

func (t *PlaybookStateShowTool) Name() string        { return "playbook_state_show" }
func (t *PlaybookStateShowTool) Description() string { return "Show detailed results of a playbook run." }
func (t *PlaybookStateShowTool) Parameters() string  { return playbookStateShowParamsSchema }
func (t *PlaybookStateShowTool) Validate(p map[string]interface{}) error {
	if strings.TrimSpace(strOf(p["run_id"])) == "" {
		return fmt.Errorf("run_id is required")
	}
	return nil
}

const playbookStateShowParamsSchema = `{
	"type": "object",
	"properties": {
		"run_id": {"type": "string", "description": "Playbook run id"},
		"node": {"type": "string", "description": "Filter by node"}
	},
	"required": ["run_id"]
}`

func (t *PlaybookStateShowTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := PlaybookStateShowParams{RunID: strOf(params["run_id"]), Node: strOf(params["node"])}
		result, err := t.executor.PlaybookStateShow(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("playbook_state_show failed")
}

// ---------- playbook generate (本地生成 + 保存到 ~/.owl/playbooks) ----------

// playbookSaveDir 返回 playbook 保存目录：~/.owl/playbooks（本地 ./playbooks 存在时优先，与 CLI 约定一致）。
// 以变量形式暴露便于测试注入。
var playbookSaveDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./playbooks"
	}
	return filepath.Join(home, ".owl", "playbooks")
}

type PlaybookGenerateTool struct {
	nodeMgr node.Manager
}

func NewPlaybookGenerateTool(nodeMgr node.Manager) *PlaybookGenerateTool {
	return &PlaybookGenerateTool{nodeMgr: nodeMgr}
}

func (t *PlaybookGenerateTool) Name() string        { return "playbook_generate" }
func (t *PlaybookGenerateTool) Description() string { return "Generate a playbook YAML from a natural language requirement and save it under the playbook library." }
func (t *PlaybookGenerateTool) Parameters() string  { return playbookGenerateParamsSchema }
func (t *PlaybookGenerateTool) Validate(p map[string]interface{}) error {
	if strings.TrimSpace(strOf(p["requirement"])) == "" {
		return fmt.Errorf("requirement is required")
	}
	return nil
}

const playbookGenerateParamsSchema = `{
	"type": "object",
	"properties": {
		"requirement": {"type": "string", "description": "Natural language requirement, e.g. 'Install nginx on all web nodes and start it'"},
		"name": {"type": "string", "description": "Playbook file name (without .yaml), default derived from requirement"}
	},
	"required": ["requirement"]
}`

func (t *PlaybookGenerateTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	requirement := strOf(params["requirement"])
	name := strOf(params["name"])
	if name == "" {
		name = sanitizePlaybookName(requirement)
	}

	var hosts []string
	if t.nodeMgr != nil {
		for _, n := range t.nodeMgr.List() {
			hosts = append(hosts, n.Name)
		}
	}

	gen := &GeneratePlaybookTool{}
	content := gen.generatePlaybookFromRequirement(requirement, hosts)

	dir := playbookSaveDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create playbook dir: %w", err)
	}
	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("save playbook: %w", err)
	}

	return fmt.Sprintf("已生成并保存 playbook 到 %s\n\n```yaml\n%s\n```", path, content), nil
}

func sanitizePlaybookName(requirement string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(requirement) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else if r == ' ' {
			sb.WriteRune('-')
		}
	}
	name := strings.Trim(sb.String(), "-")
	if name == "" {
		return "playbook"
	}
	if len(name) > 40 {
		name = name[:40]
	}
	return name
}
