package playbook

import (
	"fmt"

	pb "github.com/cangyunye/go-owl/pkg/playbook"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/cangyunye/go-owl/internal/i18n"
)

func NewPlaybookTemplateInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: i18n.T("playbook.template.info.short"),
		Args:  cobra.ExactArgs(1),
		Run:   runTemplateInfo,
	}
}

func runTemplateInfo(cmd *cobra.Command, args []string) {
	name := args[0]
	entry, err := pb.GetTemplate(name, "")
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s", i18n.T("playbook.template.info.err_get", err))
		return
	}

	out := cmd.OutOrStdout()

	fmt.Fprintf(out, "%s", i18n.T("playbook.template.info.name", entry.Name))
	fmt.Fprintf(out, "%s", i18n.T("playbook.template.info.path", entry.Path))
	if entry.Meta != nil {
		fmt.Fprintf(out, "%s", i18n.T("playbook.template.info.desc", entry.Meta.Description))
		if len(entry.Meta.Tags) > 0 {
			fmt.Fprintf(out, "%s", i18n.T("playbook.template.info.tags", entry.Meta.Tags))
		}
		if len(entry.Meta.Parameters) > 0 {
			fmt.Fprintln(out, i18n.T("playbook.template.info.params"))
			for _, p := range entry.Meta.Parameters {
				fmt.Fprintf(out, "%s", i18n.T("playbook.template.info.param_item", p.Name, p.Type, p.Description, p.Default))
			}
		}
	}

	var tpl pb.TemplatePlaybook
	if err := yaml.Unmarshal(entry.Content, &tpl); err == nil && len(tpl.Tasks) > 0 {
		fmt.Fprintln(out, i18n.T("playbook.template.info.tasks"))
		for i, task := range tpl.Tasks {
			fmt.Fprintf(out, "%s", i18n.T("playbook.template.info.task_item", i18n.F(i+1), task.Name, task.Action))
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, i18n.T("playbook.template.info.yaml_title"))
	fmt.Fprintln(out, string(entry.Content))
}
