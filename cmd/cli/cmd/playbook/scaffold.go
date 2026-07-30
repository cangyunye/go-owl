package playbook

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var pbScaffoldType string

const scaffoldBasic = `# description: "TODO: 描述此 Playbook 的用途"
# tags: []
#
# parameters:
#   - name: app_version
#     description: "应用版本号"
#     default: "latest"

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
		Short: "输出 Playbook 骨架",
		Long: `输出带注释的 Playbook 骨架到标准输出。

示例：
  owl playbook scaffold
  owl playbook scaffold --type basic > ./playbooks/my-playbook.yaml`,
		Run: runPlaybookScaffold,
	}

	cmd.Flags().StringVar(&pbScaffoldType, "type", "basic", "骨架类型")

	return cmd
}

func runPlaybookScaffold(cmd *cobra.Command, args []string) {
	switch pbScaffoldType {
	case "basic":
		fmt.Fprint(cmd.OutOrStdout(), scaffoldBasic)
	default:
		fmt.Fprintf(os.Stderr, "错误: 未知的骨架类型: %s\n", pbScaffoldType)
		os.Exit(1)
	}
}
