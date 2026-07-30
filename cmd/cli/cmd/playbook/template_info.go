package playbook

import (
	"fmt"

	pb "github.com/cangyunye/go-owl/pkg/playbook"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewPlaybookTemplateInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "查看模板详情",
		Args:  cobra.ExactArgs(1),
		Run:   runTemplateInfo,
	}
}

func runTemplateInfo(cmd *cobra.Command, args []string) {
	name := args[0]
	entry, err := pb.GetTemplate(name, "")
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "获取模板失败: %v\n", err)
		return
	}

	out := cmd.OutOrStdout()

	fmt.Fprintf(out, "模板: %s\n", entry.Name)
	fmt.Fprintf(out, "路径: %s\n", entry.Path)
	if entry.Meta != nil {
		fmt.Fprintf(out, "描述: %s\n", entry.Meta.Description)
		if len(entry.Meta.Tags) > 0 {
			fmt.Fprintf(out, "标签: %v\n", entry.Meta.Tags)
		}
		if len(entry.Meta.Parameters) > 0 {
			fmt.Fprintln(out, "参数:")
			for _, p := range entry.Meta.Parameters {
				fmt.Fprintf(out, "  - %s (%s): %s [默认: %v]\n", p.Name, p.Type, p.Description, p.Default)
			}
		}
	}

	var tpl pb.TemplatePlaybook
	if err := yaml.Unmarshal(entry.Content, &tpl); err == nil && len(tpl.Tasks) > 0 {
		fmt.Fprintln(out, "任务:")
		for i, task := range tpl.Tasks {
			fmt.Fprintf(out, "  %d. %s (%s)\n", i+1, task.Name, task.Action)
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- YAML 内容 ---")
	fmt.Fprintln(out, string(entry.Content))
}
