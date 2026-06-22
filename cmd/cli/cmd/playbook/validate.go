package playbook

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	pbexec "github.com/cangyunye/go-owl/internal/control/playbook"
)

// ValidationResult 表示一个文件的验证结果
type ValidationResult struct {
	File  string
	Valid bool
	Error error
}

// NewPlaybookValidateCmd 创建剧本验证命令
func NewPlaybookValidateCmd() *cobra.Command {
	validateCmd := &cobra.Command{
		Use:   "validate [playbook-files...]",
		Short: "验证剧本语法",
		Long: `验证 Ansible 风格剧本的 YAML 语法。

示例：
  owl playbook validate site.yml
  owl playbook validate ./playbooks/*.yml
  owl playbook validate a.yml b.yml`,
		Args: cobra.ArbitraryArgs,
		Run:  runPlaybookValidate,
	}

	return validateCmd
}

func runPlaybookValidate(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		fmt.Println("用法: owl playbook validate <playbook-file> [files...]")
		fmt.Println()
		fmt.Println("请指定要验证的 playbook 文件路径")
		fmt.Println("示例:")
		fmt.Println("  owl playbook validate site.yml")
		fmt.Println("  owl playbook validate ./playbooks/*.yml")
		fmt.Println("  owl playbook validate a.yml b.yml")
		return
	}

	// 展开 glob 参数，收集所有文件
	var files []string
	for _, arg := range args {
		matches, err := filepath.Glob(arg)
		if err != nil {
			// glob 语法错误，视为普通文件路径
			files = append(files, arg)
			continue
		}
		if len(matches) == 0 {
			// 没有匹配到任何文件，视为普通文件路径
			files = append(files, arg)
		} else {
			files = append(files, matches...)
		}
	}

	results := ValidatePlaybookFiles(files)

	hasError := false
	for _, r := range results {
		if r.Valid {
			fmt.Printf("  ✅ %s: 有效\n", r.File)
		} else {
			fmt.Printf("  ❌ %s: %v\n", r.File, r.Error)
			hasError = true
		}
	}

	if hasError {
		os.Exit(1)
	}
}

// ValidatePlaybookFiles 验证一组 playbook 文件，返回每个文件的结果
func ValidatePlaybookFiles(files []string) []ValidationResult {
	if len(files) == 0 {
		return nil
	}

	results := make([]ValidationResult, 0, len(files))
	parser := pbexec.NewParser()

	for _, file := range files {
		result := ValidationResult{File: file}

		// 检查文件是否存在
		if _, err := os.Stat(file); os.IsNotExist(err) {
			result.Error = fmt.Errorf("file not found")
			results = append(results, result)
			continue
		}

		// 使用真实 parser 解析和验证
		parsed, err := parser.ParseFromFile(file)
		if err != nil {
			result.Error = err
			results = append(results, result)
			continue
		}

		// 额外的语义验证
		validationErrors := parser.Validate(parsed)
		if len(validationErrors) > 0 {
			errMsg := ""
			for i, verr := range validationErrors {
				if i > 0 {
					errMsg += "; "
				}
				errMsg += verr.Error()
			}
			result.Error = fmt.Errorf("%s", errMsg)
			results = append(results, result)
			continue
		}

		result.Valid = true
		results = append(results, result)
	}

	return results
}
