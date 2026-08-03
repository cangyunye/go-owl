package playbook

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	pb "github.com/cangyunye/go-owl/pkg/playbook"
	"github.com/cangyunye/go-owl/internal/i18n"
)

var pbScaffoldType string

const scaffoldHeader = `# description: "TODO: 描述此 Playbook 的用途"
# tags: []
#
# parameters:
#   - name: app_version
#     description: "应用版本号"
#     default: "latest"
`

const scaffoldBasic = scaffoldHeader + `
tasks:
  - name: "TODO: 步骤名称"
    action: command
    args:
      cmd: echo "hello"
    # timeout: 300
    # retries: 3
`

func NewPlaybookScaffoldCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: i18n.T("playbook.scaffold.short"),
		Long:  i18n.T("playbook.scaffold.long") + scaffoldTypeHelp(),
		Run:   runPlaybookScaffold,
	}

	cmd.Flags().StringVar(&pbScaffoldType, "type", "basic", i18n.T("playbook.scaffold.flag_type")+scaffoldTypeHelp())

	return cmd
}

func scaffoldTypeHelp() string {
	var b strings.Builder
	for _, t := range pb.GetActionTemplates() {
		b.WriteString(fmt.Sprintf("  %-10s - %s\n", t.Name, t.Description))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func runPlaybookScaffold(cmd *cobra.Command, args []string) {
	content, err := renderScaffold(pbScaffoldType)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s", i18n.T("playbook.scaffold.err", err))
		return
	}
	fmt.Fprint(cmd.OutOrStdout(), content)
}

func renderScaffold(typ string) (string, error) {
	if typ == "basic" || typ == "" {
		return scaffoldBasic, nil
	}

	for _, t := range pb.GetActionTemplates() {
		if t.Name == typ {
			return renderActionScaffold(&t)
		}
	}

	return "", errors.New(i18n.T("playbook.scaffold.err_unknown_type",
		typ, strings.Join(actionTypeNames(), ", ")))
}

func actionTypeNames() []string {
	templates := pb.GetActionTemplates()
	names := make([]string, len(templates))
	for i, t := range templates {
		names[i] = t.Name
	}
	return names
}

func renderActionScaffold(t *pb.ActionTemplate) (string, error) {
	argsYAML, err := renderArgsYAML(t.Template)
	if err != nil {
		return "", err
	}

	return scaffoldHeader + `
tasks:
  - name: "任务 1"
    action: ` + t.Name + `
    args:
` + argsYAML + `
    # timeout: 300
    # retries: 3
`, nil
}

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
