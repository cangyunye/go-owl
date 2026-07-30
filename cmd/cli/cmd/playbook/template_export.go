package playbook

import (
	"fmt"
	"os"
	"path/filepath"

	pb "github.com/cangyunye/go-owl/pkg/playbook"
	"github.com/spf13/cobra"
)

var templateExportTo string

func NewPlaybookTemplateExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <name>",
		Short: "导出模板到本地",
		Args:  cobra.ExactArgs(1),
		Run:   runTemplateExport,
	}

	cmd.Flags().StringVar(&templateExportTo, "to", "", "导出目标目录（默认: ~/.owl/templates/）")

	return cmd
}

func runTemplateExport(cmd *cobra.Command, args []string) {
	name := args[0]
	entry, err := pb.GetTemplate(name, "")
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "获取模板失败: %v\n", err)
		return
	}

	toDir := templateExportTo
	if toDir == "" {
		toDir = pb.DefaultUserTemplatePath()
	}

	outPath := filepath.Join(toDir, name+".yaml")
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "创建目录失败: %v\n", err)
		return
	}

	if err := os.WriteFile(outPath, entry.Content, 0644); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "写入文件失败: %v\n", err)
		return
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✅ 模板已导出: %s\n", outPath)
}
