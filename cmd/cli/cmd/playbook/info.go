package playbook

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewPlaybookInfoCmd 创建剧本信息命令
func NewPlaybookInfoCmd() *cobra.Command {
	infoCmd := &cobra.Command{
		Use:   "info <playbook-file>",
		Short: "显示剧本信息",
		Long: `显示剧本的详细信息，包括任务列表、变量、条件等。

示例：
  owl playbook info site.yml
  owl playbook info ./playbooks/deploy.yml`,
		Args: cobra.ExactArgs(1),
		Run:  runPlaybookInfo,
	}

	return infoCmd
}

func runPlaybookInfo(cmd *cobra.Command, args []string) {
	playbookFile := args[0]

	// 检查文件是否存在
	if _, err := os.Stat(playbookFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: playbook file not found: %s\n", playbookFile)
		os.Exit(1)
	}

	fmt.Printf("Playbook: %s\n", playbookFile)
	fmt.Println("========================================")

	// 解析并显示剧本信息
	displayPlaybookDetails(playbookFile)
}

func displayPlaybookDetails(file string) {
	// TODO: 集成实际的剧本解析器
	// 这里显示模拟信息
	fmt.Println()
	fmt.Println("Tasks: 5")
	fmt.Println("  1. pre_tasks:")
	fmt.Println("     - Check disk space")
	fmt.Println("  2. tasks:")
	fmt.Println("     - Upload application")
	fmt.Println("     - Extract application")
	fmt.Println("     - Start service")
	fmt.Println("  3. post_tasks:")
	fmt.Println("     - Verify deployment")
	fmt.Println()
	fmt.Println("Hosts: web (from inventory)")
	fmt.Println("Variables:")
	fmt.Println("  - app_version: \"1.0.0\"")
	fmt.Println("  - app_path: \"/opt/app\"")
	fmt.Println()
	fmt.Println("Handlers: 1")
	fmt.Println("  - Restart nginx")
	fmt.Println()
	fmt.Println("Estimated execution time: ~2 minutes")
}
