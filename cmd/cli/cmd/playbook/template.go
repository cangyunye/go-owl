package playbook

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var playbookTemplateOutput string

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
		Template: map[string]interface{}{
			"cmd": "<命令内容>",
		},
	},
	{
		Name:        "script",
		Description: "执行脚本文件",
		Template: map[string]interface{}{
			"script": "<脚本路径>",
			"dest":   "/tmp/",
			"args":   "",
		},
	},
	{
		Name:        "upload",
		Description: "上传文件到节点",
		Template: map[string]interface{}{
			"src":       "<本地路径>",
			"dest":      "<远程路径>",
			"overwrite": true,
		},
	},
	{
		Name:        "download",
		Description: "从节点下载文件",
		Template: map[string]interface{}{
			"src":    "<远程路径>",
			"dest":   "<本地路径>",
			"subdir": true,
		},
	},
	{
		Name:        "include",
		Description: "包含其他剧本",
		Template: map[string]interface{}{
			"playbook": "<剧本路径>",
		},
	},
}

type TemplatePlaybook struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description,omitempty"`
	Version     string                 `yaml:"version,omitempty"`
	Hosts       []string               `yaml:"hosts"`
	Vars        map[string]interface{} `yaml:"vars,omitempty"`
	PreTasks    []TemplateTask         `yaml:"pre_tasks"`
	Tasks       []TemplateTask         `yaml:"tasks"`
	PostTasks   []TemplateTask         `yaml:"post_tasks"`
}

type TemplateTask struct {
	Name   string                 `yaml:"name"`
	Action string                 `yaml:"action"`
	Args   map[string]interface{} `yaml:"args"`
}

func NewPlaybookTemplateCmd() *cobra.Command {
	templateCmd := &cobra.Command{
		Use:   "template",
		Short: "交互式创建剧本模板",
		Long: `通过会话式问答创建剧本模板。

示例：
  owl playbook template
  owl playbook template --output ./my-playbook.yaml`,
		Run: runPlaybookTemplate,
	}

	templateCmd.Flags().StringVarP(&playbookTemplateOutput, "output", "o", "",
		"输出文件路径（默认: ./playbooks/<name>.yaml）")

	return templateCmd
}

func runPlaybookTemplate(cmd *cobra.Command, args []string) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("📝 剧本模板创建向导")
	fmt.Println("====================")
	fmt.Println()

	name := promptForName(reader)
	description := promptForDescription(reader)
	version := promptForVersion(reader)
	vars := promptForVars(reader)

	tasks := promptForTasks(reader)

	playbook := TemplatePlaybook{
		Name:        name,
		Description: description,
		Version:     version,
		Hosts:       []string{},
		Vars:        vars,
		PreTasks:    []TemplateTask{},
		Tasks:       tasks,
		PostTasks:   []TemplateTask{},
	}

	playbookYAML, err := yaml.Marshal(&playbook)
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成 YAML 失败: %v\n", err)
		os.Exit(1)
	}

	outputPath := determineOutputPath(name, playbookTemplateOutput)

	if err := savePlaybookFile(outputPath, playbookYAML); err != nil {
		fmt.Fprintf(os.Stderr, "保存文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("✅ 剧本模板已创建!")
	fmt.Printf("📄 文件路径: %s\n", outputPath)
	fmt.Println()
	fmt.Println("💡 下一步:")
	fmt.Println("   1. 编辑文件，填充占位符内容")
	fmt.Println("   2. 使用 owl playbook validate 验证语法")
	fmt.Println("   3. 使用 owl playbook run 执行剧本")
}

func promptForName(reader *bufio.Reader) string {
	for {
		fmt.Print("任务名 (name): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
			os.Exit(1)
		}
		name := strings.TrimSpace(input)
		if name != "" {
			return name
		}
		fmt.Println("❌ 任务名不能为空，请重新输入")
	}
}

func promptForDescription(reader *bufio.Reader) string {
	fmt.Print("描述 (description，可选): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
		os.Exit(1)
	}
	return strings.TrimSpace(input)
}

func promptForVersion(reader *bufio.Reader) string {
	fmt.Print("版本 (version，默认 1.0): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
		os.Exit(1)
	}
	version := strings.TrimSpace(input)
	if version == "" {
		return "1.0"
	}
	return version
}

func promptForVars(reader *bufio.Reader) map[string]interface{} {
	vars := make(map[string]interface{})

	fmt.Print("是否添加变量？(y/n，默认 n): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
		os.Exit(1)
	}
	choice := strings.ToLower(strings.TrimSpace(input))

	if choice != "y" && choice != "yes" {
		return vars
	}

	for {
		fmt.Print("变量名 (留空结束): ")
		varNameInput, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
			os.Exit(1)
		}
		varName := strings.TrimSpace(varNameInput)
		if varName == "" {
			break
		}

		fmt.Printf("变量 '%s' 的值: ", varName)
		varValueInput, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
			os.Exit(1)
		}
		varValue := strings.TrimSpace(varValueInput)
		vars[varName] = varValue
	}

	return vars
}

func displayActionChoices() {
	fmt.Println()
	fmt.Println("请选择任务类型:")
	fmt.Println("----------------")
	for i, t := range actionTemplates {
		fmt.Printf("%d. %s  - %s\n", i+1, t.Name, t.Description)
	}
	fmt.Println()
}

func promptForTasks(reader *bufio.Reader) []TemplateTask {
	tasks := []TemplateTask{}
	taskIndex := 1

	for {
		displayActionChoices()

		fmt.Printf("选择任务类型 (1-%d): ", len(actionTemplates))
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
			os.Exit(1)
		}

		choiceStr := strings.TrimSpace(input)
		choice, err := strconv.Atoi(choiceStr)
		if err != nil || choice < 1 || choice > len(actionTemplates) {
			fmt.Printf("❌ 请输入有效序号 (1-%d)\n", len(actionTemplates))
			continue
		}

		selectedTemplate := actionTemplates[choice-1]

		task := TemplateTask{
			Name:   fmt.Sprintf("任务 %d", taskIndex),
			Action: selectedTemplate.Name,
			Args:   selectedTemplate.Template,
		}

		tasks = append(tasks, task)
		taskIndex++

		fmt.Printf("✅ 已添加任务: %s (%s)\n", task.Name, selectedTemplate.Name)
		fmt.Println()

		if !promptForContinue(reader) {
			break
		}
	}

	return tasks
}

func promptForContinue(reader *bufio.Reader) bool {
	fmt.Print("是否继续添加任务？(y/n): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
		os.Exit(1)
	}
	choice := strings.ToLower(strings.TrimSpace(input))
	return choice == "y" || choice == "yes"
}

func determineOutputPath(name, specifiedPath string) string {
	if specifiedPath != "" {
		return specifiedPath
	}
	return filepath.Join("./playbooks", name+".yaml")
}

func savePlaybookFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
	}

	return os.WriteFile(path, content, 0644)
}