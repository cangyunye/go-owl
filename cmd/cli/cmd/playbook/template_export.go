package playbook

import (
	"fmt"
	"os"
	"path/filepath"

	pb "github.com/cangyunye/go-owl/pkg/playbook"
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
)

var templateExportTo string

func NewPlaybookTemplateExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <name>",
		Short: i18n.T("playbook.template.export.short"),
		Args:  cobra.ExactArgs(1),
		Run:   runTemplateExport,
	}

	cmd.Flags().StringVar(&templateExportTo, "to", "", i18n.T("playbook.template.export.flag_to"))

	return cmd
}

func runTemplateExport(cmd *cobra.Command, args []string) {
	name := args[0]

	toDir := templateExportTo
	if toDir == "" {
		toDir = pb.DefaultUserTemplatePath()
	}

	outPath := filepath.Join(toDir, name+".yaml")

	var content []byte
	entry, err := pb.GetTemplate(name, pb.DefaultUserTemplatePath())
	if err != nil {
		// 规范骨架不在模板体系中，直接导出共享生成器内容，
		// 保证与 scaffold / new / template create 产出逐字节一致。
		if name != CanonicalSkeletonName {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s", i18n.T("playbook.template.export.err_get", err))
			return
		}
		content, err = GeneratePlaybookSkeleton(SkeletonOptions{})
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s", i18n.T("playbook.template.export.err_get", err))
			return
		}
	} else {
		content = entry.Content
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s", i18n.T("playbook.template.export.err_mkdir", err))
		return
	}

	if err := os.WriteFile(outPath, content, 0644); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s", i18n.T("playbook.template.export.err_write", err))
		return
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s", i18n.T("playbook.template.export.ok", outPath))
}
