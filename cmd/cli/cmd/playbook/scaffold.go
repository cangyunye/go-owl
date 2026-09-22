package playbook

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
	pb "github.com/cangyunye/go-owl/pkg/playbook"
)

var pbScaffoldType string

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

// runPlaybookScaffold 将共享生成器的骨架写到标准输出，
// 与 new / template create / template export 产出同一份规范内容。
func runPlaybookScaffold(cmd *cobra.Command, args []string) {
	content, err := GeneratePlaybookSkeleton(SkeletonOptions{ActionType: pbScaffoldType})
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s", i18n.T("playbook.scaffold.err", err))
		return
	}
	if _, err := cmd.OutOrStdout().Write(content); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s", i18n.T("playbook.scaffold.err", err))
	}
}
