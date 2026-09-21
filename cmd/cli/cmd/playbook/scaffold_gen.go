package playbook

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	pb "github.com/cangyunye/go-owl/pkg/playbook"
	"gopkg.in/yaml.v3"
)

// CanonicalSkeletonName 是规范骨架模板的保留名称。
// `owl playbook new --from skeleton` 与 `owl playbook template export skeleton`
// 在模板体系中找不到该名称时，会直接走本文件的共享生成器，
// 从而与 `owl playbook scaffold`、`owl playbook template create` 产出逐字节一致的内容。
const CanonicalSkeletonName = "skeleton"

const (
	skeletonDescriptionPlaceholder = "TODO: 描述此 Playbook 的用途"
	skeletonTaskNamePlaceholder    = "TODO: 步骤名称"
	skeletonDefaultVersion         = "1.0"
)

// canonicalSkeletonHeader 是规范骨架的固定文件头，说明内容的唯一来源。
const canonicalSkeletonHeader = `# owl playbook 骨架 —— 由 owl playbook scaffold / new / template create / template export 共享生成
# 运行: owl playbook run <本文件> --nodes <节点> --dry-run
`

// canonicalSkeletonParameters 是规范骨架自带的示例参数块（仅作占位演示，
// 内容中不包含 {{ }} 引用点，因此不会影响各入口默认输出的字节一致性）。
const canonicalSkeletonParameters = `parameters:
  - name: app_version
    description: "应用版本号"
    default: "latest"
    type: string
    required: false
`

// canonicalBasicTasks 是规范骨架（basic 类型）的示例任务块。
const canonicalBasicTasks = `tasks:
  - name: "TODO: 步骤名称"
    action: command
    args:
      cmd: echo "hello"
    # timeout: 300
    # retries: 3
`

// SkeletonOptions 描述骨架生成器的全部输入。
// 零值即规范骨架（与 `owl playbook scaffold` 的默认输出一致）。
type SkeletonOptions struct {
	// ActionType 选择示例任务风格："basic"（默认）或 action 模板名
	// （command/script/upload/download/include，见 pb.GetActionTemplates）。
	ActionType string
	// Description 剧本描述；为空时使用规范占位符。
	Description string
	// Version 版本号；为空时使用 "1.0"。
	Version string
	// ExecutionMode 执行模式：""（默认 fail_continue）、"pipeline" 或 "fail_continue"。
	ExecutionMode string
	// Default 可选的 default 配置块。
	Default *pb.TemplateDefaultConfig
	// Vars 可选的 vars 块（按键名排序渲染，保证输出确定性）。
	Vars map[string]interface{}
	// Tasks 任务列表；为空时使用 ActionType 对应的规范示例任务。
	Tasks []pb.TemplateTask
}

// GeneratePlaybookSkeleton 是 Playbook 骨架的唯一事实源：
// scaffold / new / template create / template export 四个入口都必须经由它产出内容，
// 相同输入保证逐字节一致的输出。生成物可被 pbexec Parser 解析并通过语义校验。
func GeneratePlaybookSkeleton(opts SkeletonOptions) ([]byte, error) {
	switch opts.ExecutionMode {
	case "", "pipeline", "fail_continue":
	default:
		return nil, fmt.Errorf("无效的 execution_mode: %s（应为 pipeline 或 fail_continue）", opts.ExecutionMode)
	}

	var b strings.Builder
	b.WriteString(canonicalSkeletonHeader)
	b.WriteString(skeletonDescriptionLine(opts.Description))
	b.WriteString(skeletonVersionLine(opts.Version))
	b.WriteString("tags: []\n")
	b.WriteString("\n")
	b.WriteString(canonicalSkeletonParameters)

	if opts.ExecutionMode != "" {
		b.WriteString("\nexecution_mode: " + opts.ExecutionMode + "\n")
	}
	if block := renderDefaultBlock(opts.Default); block != "" {
		b.WriteString("\n" + block)
	}
	if block := renderVarsBlock(opts.Vars); block != "" {
		b.WriteString("\n" + block)
	}

	tasksBlock, err := renderTasksBlock(opts)
	if err != nil {
		return nil, err
	}
	b.WriteString("\n")
	b.WriteString(tasksBlock)

	return []byte(b.String()), nil
}

func skeletonDescriptionLine(desc string) string {
	if desc == "" {
		return `description: "TODO: 描述此 Playbook 的用途"` + "\n"
	}
	return "description: " + skeletonScalar(desc) + "\n"
}

func skeletonVersionLine(version string) string {
	if version == "" {
		return `version: "1.0"` + "\n"
	}
	return "version: " + skeletonScalar(version) + "\n"
}

// skeletonScalar 将标量渲染为安全的 YAML 值（由 yaml.v3 决定引号风格）。
func skeletonScalar(v interface{}) string {
	out, err := yaml.Marshal(v)
	if err != nil {
		return strconv.Quote(fmt.Sprintf("%v", v))
	}
	return strings.TrimRight(string(out), "\n")
}

func skeletonList(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, skeletonScalar(item))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func renderDefaultBlock(defaultCfg *pb.TemplateDefaultConfig) string {
	if defaultCfg == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("default:\n")
	if len(defaultCfg.Groups) > 0 {
		b.WriteString("  groups: " + skeletonList(defaultCfg.Groups) + "\n")
	}
	if len(defaultCfg.Tags) > 0 {
		b.WriteString("  tags: " + skeletonList(defaultCfg.Tags) + "\n")
	}
	if len(defaultCfg.SkipTags) > 0 {
		b.WriteString("  skip_tags: " + skeletonList(defaultCfg.SkipTags) + "\n")
	}
	if len(defaultCfg.Groups) == 0 && len(defaultCfg.Tags) == 0 && len(defaultCfg.SkipTags) == 0 {
		b.WriteString("  {}\n")
	}
	return b.String()
}

// renderVarsBlock 按键名排序渲染 vars 块，保证同一输入的输出逐字节一致。
func renderVarsBlock(vars map[string]interface{}) string {
	if len(vars) == 0 {
		return ""
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("vars:\n")
	for _, k := range keys {
		b.WriteString("  " + skeletonScalar(k) + ": " + skeletonScalar(vars[k]) + "\n")
	}
	return b.String()
}

// renderTasksBlock 渲染任务区：显式提供的任务优先，
// 否则按 ActionType 回退到内置的规范示例任务（basic 或具体 action 模板）。
func renderTasksBlock(opts SkeletonOptions) (string, error) {
	if len(opts.Tasks) > 0 {
		var b strings.Builder
		b.WriteString("tasks:\n")
		for _, task := range opts.Tasks {
			if err := writeSkeletonTask(&b, task, false); err != nil {
				return "", err
			}
		}
		return b.String(), nil
	}

	if opts.ActionType == "" || opts.ActionType == "basic" {
		return canonicalBasicTasks, nil
	}

	for _, tpl := range pb.GetActionTemplates() {
		if tpl.Name != opts.ActionType {
			continue
		}
		var b strings.Builder
		b.WriteString("tasks:\n")
		err := writeSkeletonTask(&b, pb.TemplateTask{
			Name:   skeletonTaskNamePlaceholder,
			Action: tpl.Name,
			Args:   cloneActionArgs(tpl.Template),
		}, true)
		if err != nil {
			return "", err
		}
		return b.String(), nil
	}

	return "", fmt.Errorf("未知的骨架类型: %s（可用: basic, %s）",
		opts.ActionType, strings.Join(actionTypeNames(), ", "))
}

// writeSkeletonTask 渲染单个任务；withHints 控制是否附带示例性的
// timeout/retries 注释提示（仅内置示例任务携带，用户显式任务不携带）。
func writeSkeletonTask(b *strings.Builder, t pb.TemplateTask, withHints bool) error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("任务缺少 name")
	}
	if strings.TrimSpace(t.Action) == "" {
		return fmt.Errorf("任务 %s 缺少 action", t.Name)
	}

	b.WriteString("  - name: " + skeletonScalar(t.Name) + "\n")
	b.WriteString("    action: " + t.Action + "\n")

	argsYAML, err := renderArgsYAML(t.Args)
	if err != nil {
		return err
	}
	b.WriteString("    args:\n")
	b.WriteString(argsYAML)

	if withHints {
		b.WriteString("    # timeout: 300\n")
		b.WriteString("    # retries: 3\n")
	}
	return nil
}

func cloneActionArgs(args map[string]interface{}) map[string]interface{} {
	copied := make(map[string]interface{}, len(args))
	for k, v := range args {
		copied[k] = v
	}
	return copied
}

// actionTypeNames 返回全部可用 action 模板名（含 basic 之外的类型）。
func actionTypeNames() []string {
	templates := pb.GetActionTemplates()
	names := make([]string, 0, len(templates))
	for _, t := range templates {
		names = append(names, t.Name)
	}
	return names
}

// renderArgsYAML 将任务参数渲染为任务内缩进（6 空格）的 YAML 行，键名有序。
func renderArgsYAML(args map[string]interface{}) (string, error) {
	if len(args) == 0 {
		return "      {}\n", nil
	}

	var b strings.Builder
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		line := fmt.Sprintf("      %s: %s\n", k, formatArgValue(args[k]))
		b.WriteString(line)
	}
	return b.String(), nil
}

func formatArgValue(v interface{}) string {
	switch val := v.(type) {
	case bool:
		return strconv.FormatBool(val)
	case string:
		return strconv.Quote(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// SubstituteSkeletonVars 将生成物中的 {{ var }} 占位符按字面量替换。
// 供 new 入口在骨架生成后应用 --var 参数；规范骨架不含占位符，
// 因此默认输入下替换是无操作，保持四入口输出逐字节一致。
func SubstituteSkeletonVars(content []byte, vars map[string]interface{}) []byte {
	if len(vars) == 0 {
		return content
	}
	s := string(content)
	for k, v := range vars {
		re := regexp.MustCompile(`\{\{\s*` + regexp.QuoteMeta(k) + `\s*\}\}`)
		s = re.ReplaceAllLiteralString(s, fmt.Sprintf("%v", v))
	}
	return []byte(s)
}
