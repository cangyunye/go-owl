package playbook

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	playbook "github.com/cangyunye/go-owl/pkg/playbook"
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
)

var playbookTemplateOutput string

func NewPlaybookTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: i18n.T("playbook.template.cmd.short"),
		Long:  i18n.T("playbook.template.cmd.long"),
	}
	cmd.AddCommand(NewPlaybookTemplateCreateCmd())
	cmd.AddCommand(NewPlaybookTemplateListCmd())
	cmd.AddCommand(NewPlaybookTemplateInfoCmd())
	cmd.AddCommand(NewPlaybookTemplateExportCmd())
	return cmd
}

func NewPlaybookTemplateCreateCmd() *cobra.Command {
	templateCmd := &cobra.Command{
		Use:   "create",
		Short: i18n.T("playbook.template.create.short"),
		Long:  i18n.T("playbook.template.create.long"),
		Run:   runPlaybookTemplate,
	}

	templateCmd.Flags().StringVarP(&playbookTemplateOutput, "output", "o", "",
		i18n.T("playbook.template.create.flag_output"))

	return templateCmd
}

func runPlaybookTemplate(cmd *cobra.Command, args []string) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println(i18n.T("playbook.template.wizard_title"))
	fmt.Println(i18n.T("playbook.template.wizard_sep"))
	fmt.Println()

	name := promptForName(reader)
	description := promptForDescription(reader)
	version := promptForVersion(reader)
	vars := promptForVars(reader)
	mode := promptForExecutionMode(reader)
	defaultConfig := promptForDefaultConfig(reader)
	tasks := promptForTasks(reader)

	tpl := playbook.TemplatePlaybook{
		Name:          name,
		Description:   description,
		Version:       version,
		Hosts:         []string{},
		ExecutionMode: mode,
		Default:       defaultConfig,
		Vars:          vars,
		PreTasks:      []playbook.TemplateTask{},
		Tasks:         tasks,
		PostTasks:     []playbook.TemplateTask{},
	}

	playbookYAML, err := playbook.RenderTemplateYAML(&tpl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_yaml", err))
		os.Exit(1)
	}

	outputPath := determineOutputPath(name, playbookTemplateOutput)

	if err := savePlaybookFile(outputPath, playbookYAML); err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_save", err))
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println(i18n.T("playbook.template.created"))
	fmt.Printf("%s", i18n.T("playbook.template.file_path", outputPath))
	fmt.Println()
	fmt.Println(i18n.T("playbook.template.next_step"))
	fmt.Println(i18n.T("playbook.template.next_1"))
	fmt.Println(i18n.T("playbook.template.next_2"))
	fmt.Println(i18n.T("playbook.template.next_3"))
}

func promptForName(reader *bufio.Reader) string {
	for {
		fmt.Print(i18n.T("playbook.template.prompt_name"))
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
			os.Exit(1)
		}
		name := strings.TrimSpace(input)
		if name != "" {
			return name
		}
		fmt.Println(i18n.T("playbook.template.err_name_empty"))
	}
}

func promptForDescription(reader *bufio.Reader) string {
	fmt.Print(i18n.T("playbook.template.prompt_desc"))
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
		os.Exit(1)
	}
	return strings.TrimSpace(input)
}

func promptForVersion(reader *bufio.Reader) string {
	fmt.Print(i18n.T("playbook.template.prompt_version"))
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
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

	fmt.Print(i18n.T("playbook.template.prompt_vars"))
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
		os.Exit(1)
	}
	choice := strings.ToLower(strings.TrimSpace(input))

	if choice != "y" && choice != "yes" {
		return vars
	}

	for {
		fmt.Print(i18n.T("playbook.template.prompt_var_name"))
		varNameInput, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
			os.Exit(1)
		}
		varName := strings.TrimSpace(varNameInput)
		if varName == "" {
			break
		}

		fmt.Printf("%s", i18n.T("playbook.template.prompt_var_value", varName))
		varValueInput, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
			os.Exit(1)
		}
		varValue := strings.TrimSpace(varValueInput)
		vars[varName] = varValue
	}

	return vars
}

func displayActionChoices() {
	fmt.Println()
	fmt.Println(i18n.T("playbook.template.choose_task"))
	fmt.Println(i18n.T("playbook.template.choose_sep"))
	for i, t := range playbook.GetActionTemplates() {
		fmt.Printf("%d. %s  - %s\n", i+1, t.Name, t.Description)
	}
	fmt.Println()
}

func promptForExecutionMode(reader *bufio.Reader) string {
	fmt.Print(i18n.T("playbook.template.prompt_mode"))
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
		os.Exit(1)
	}
	mode := strings.ToLower(strings.TrimSpace(input))
	if mode == "pipeline" || mode == "p" {
		fmt.Println(i18n.T("playbook.template.mode_pipeline"))
		return "pipeline"
	}
	return ""
}

func promptForDefaultConfig(reader *bufio.Reader) *playbook.TemplateDefaultConfig {
	fmt.Print(i18n.T("playbook.template.prompt_default"))
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
		os.Exit(1)
	}
	choice := strings.ToLower(strings.TrimSpace(input))
	if choice != "y" && choice != "yes" {
		return nil
	}

	cfg := &playbook.TemplateDefaultConfig{}

	fmt.Print(i18n.T("playbook.template.prompt_group"))
	input, err = reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
		os.Exit(1)
	}
	groups := strings.TrimSpace(input)
	if groups != "" {
		for _, g := range strings.Split(groups, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				cfg.Groups = append(cfg.Groups, g)
			}
		}
	}

	fmt.Print(i18n.T("playbook.template.prompt_tags_input"))
	input, err = reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
		os.Exit(1)
	}
	tags := strings.TrimSpace(input)
	if tags != "" {
		for _, t := range strings.Split(tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				cfg.Tags = append(cfg.Tags, t)
			}
		}
	}

	fmt.Print(i18n.T("playbook.template.prompt_skip_tags"))
	input, err = reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
		os.Exit(1)
	}
	skipTags := strings.TrimSpace(input)
	if skipTags != "" {
		for _, t := range strings.Split(skipTags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				cfg.SkipTags = append(cfg.SkipTags, t)
			}
		}
	}

	return cfg
}

func promptForTasks(reader *bufio.Reader) []playbook.TemplateTask {
	tasks := []playbook.TemplateTask{}
	taskIndex := 1

	for {
		displayActionChoices()

		actionTemplates := playbook.GetActionTemplates()
		fmt.Printf("%s", i18n.T("playbook.template.choose_action", i18n.F(len(actionTemplates))))
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.template.err_read", err))
			os.Exit(1)
		}

		choiceStr := strings.ToLower(strings.TrimSpace(input))
		if choiceStr == "q" || choiceStr == "quit" || choiceStr == "exit" {
			break
		}

		choice, err := strconv.Atoi(choiceStr)
		if err != nil || choice < 1 || choice > len(actionTemplates) {
			fmt.Printf("%s", i18n.T("playbook.template.invalid_choice", i18n.F(len(actionTemplates))))
			continue
		}

		selectedTemplate := actionTemplates[choice-1]

		argsCopy := make(map[string]interface{})
		for k, v := range selectedTemplate.Template {
			argsCopy[k] = v
		}

		task := playbook.TemplateTask{
			Name:   i18n.T("playbook.template.task_name", i18n.F(taskIndex)),
			Action: selectedTemplate.Name,
			Args:   argsCopy,
		}

		tasks = append(tasks, task)
		taskIndex++

		fmt.Printf("%s", i18n.T("playbook.template.task_added", task.Name, selectedTemplate.Name))
		fmt.Println()
	}

	return tasks
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
			return fmt.Errorf(i18n.Raw("playbook.template.err_save_mkdir"), err)
		}
	}

	return os.WriteFile(path, content, 0644)
}