package playbook

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pb "github.com/cangyunye/go-owl/pkg/playbook"
	"github.com/spf13/cobra"
)

var (
	pbNewFrom   string
	pbNewVars   []string
	pbNewOutput string
)

func NewPlaybookNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "从模板创建 Playbook",
		Long: `从模板创建 Playbook 文件。

示例：
  owl playbook new --from=utility/healthcheck/http --var url=http://example.com
  owl playbook new --from=utility/healthcheck/http -o ./my-check.yaml`,
		Run: runPlaybookNew,
	}

	cmd.Flags().StringVar(&pbNewFrom, "from", "", "模板名称（必填）")
	cmd.Flags().StringArrayVar(&pbNewVars, "var", nil, "参数值 (key=value)，可多次指定")
	cmd.Flags().StringVarP(&pbNewOutput, "output", "o", "", "输出文件路径（默认: ./playbooks/<模板名>.yaml）")

	_ = cmd.MarkFlagRequired("from")

	return cmd
}

func runPlaybookNew(cmd *cobra.Command, args []string) {
	entry, err := pb.GetTemplate(pbNewFrom, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	meta, err := pb.ParseTemplateMeta(entry.Content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	provided := parseVarFlags(pbNewVars)

	promptForMissingParams(meta.Parameters, provided)

	validated, err := pb.ValidateParams(meta.Parameters, provided)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	rendered, err := pb.Instantiate(entry.Content, validated)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	outputPath := determineNewOutputPath(pbNewFrom, pbNewOutput)

	if err := savePlaybookFile(outputPath, rendered); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Playbook 已创建: %s\n", outputPath)
	fmt.Println("💡 执行命令:")
	fmt.Printf("   owl playbook run %s --nodes <节点> --dry-run\n", outputPath)
}

func parseVarFlags(vars []string) map[string]interface{} {
	provided := make(map[string]interface{})
	for _, v := range vars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			fmt.Fprintf(os.Stderr, "错误: 无效的参数格式: %q（应为 key=value）\n", v)
			os.Exit(1)
		}
		provided[parts[0]] = parts[1]
	}
	return provided
}

func promptForMissingParams(params []pb.TemplateParameter, provided map[string]interface{}) {
	reader := bufio.NewReader(os.Stdin)
	for _, p := range params {
		if _, ok := provided[p.Name]; ok {
			continue
		}

		prompt := fmt.Sprintf("参数 %s", p.Name)
		if p.Description != "" {
			prompt += fmt.Sprintf("（%s）", p.Description)
		}
		if p.Default != nil {
			prompt += fmt.Sprintf(" [默认: %v]", p.Default)
		}
		fmt.Printf("%s: ", prompt)

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
			os.Exit(1)
		}
		if val := strings.TrimSpace(input); val != "" {
			provided[p.Name] = val
		}
	}
}

func determineNewOutputPath(from, specifiedPath string) string {
	if specifiedPath != "" {
		return specifiedPath
	}
	name := from
	if i := strings.LastIndex(from, "/"); i >= 0 {
		name = from[i+1:]
	}
	return filepath.Join("./playbooks", name+".yaml")
}
