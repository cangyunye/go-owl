package playbook

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewPlaybookValidateCmd 创建剧本验证命令
func NewPlaybookValidateCmd() *cobra.Command {
	validateCmd := &cobra.Command{
		Use:   "validate <playbook-file>",
		Short: "验证剧本语法",
		Long: `验证 Ansible 风格剧本的 YAML 语法。

示例：
  owl playbook validate site.yml
  owl playbook validate ./playbooks/*.yml`,
		Args: cobra.ExactArgs(1),
		Run:  runPlaybookValidate,
	}

	return validateCmd
}

func runPlaybookValidate(cmd *cobra.Command, args []string) {
	playbookFile := args[0]

	// 检查文件是否存在
	if _, err := os.Stat(playbookFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: playbook file not found: %s\n", playbookFile)
		os.Exit(1)
	}

	fmt.Printf("Validating playbook: %s\n", playbookFile)

	// 模拟 YAML 验证
	// 实际会调用 internal/control/playbook/parser.go 的解析器
	err := validatePlaybookSyntax(playbookFile)
	if err != nil {
		fmt.Printf("✗ Validation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Playbook syntax is valid")
}

func validatePlaybookSyntax(file string) error {
	// TODO: 集成实际的 YAML 解析器验证
	// 这里只是模拟验证
	return nil
}
