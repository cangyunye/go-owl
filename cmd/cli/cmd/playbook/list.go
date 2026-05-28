package playbook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	common "github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// playbookListFlags
var (
	playbookListLibrary string
	playbookListFormat  string
)

// NewPlaybookListCmd 创建剧本列表命令
func NewPlaybookListCmd() *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出剧本",
		Long: `列出所有可用的剧本文件。

示例：
  owl playbook list
  owl playbook list --library ./playbooks/
  owl playbook list --library /etc/owl/playbooks/ -o json`,
		Run: runPlaybookList,
	}

	listCmd.Flags().StringVar(&playbookListLibrary, "library", "./playbooks",
		"剧本库目录")
	listCmd.Flags().StringVarP(&playbookListFormat, "output", "o", "table",
		"输出格式: table, json, yaml")

	return listCmd
}

func runPlaybookList(cmd *cobra.Command, args []string) {
	library := playbookListLibrary

	// 检查目录是否存在
	if _, err := os.Stat(library); os.IsNotExist(err) {
		// 如果目录不存在，显示示例剧本
		displaySamplePlaybooks()
		return
	}

	// 查找所有 YAML 文件
	var playbooks []PlaybookInfo
	err := filepath.Walk(library, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
			playbooks = append(playbooks, PlaybookInfo{
				Name: info.Name(),
				Path: path,
				Size: info.Size(),
			})
		}
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning directory: %v\n", err)
		os.Exit(1)
	}

	if len(playbooks) == 0 {
		displaySamplePlaybooks()
		return
	}

	displayPlaybookList(playbooks)
}

func displaySamplePlaybooks() {
	fmt.Println("No playbooks found in library.")
	fmt.Println("\nSample playbooks (for reference):")
	fmt.Println("================================")
	fmt.Println()
	fmt.Println("1. deploy.yml  - Deploy application to web servers")
	fmt.Println("2. update.yml  - Update system packages")
	fmt.Println("3. backup.yml  - Backup database and files")
	fmt.Println()
	fmt.Println("Create a playbook to get started:")
	fmt.Println(`  $ owl playbook run --name deploy`)
	fmt.Println()
	fmt.Println(`  Or use the exec command:`)
	fmt.Println(`  $ owl exec playbook deploy.yml`)
}

func displayPlaybookList(playbooks []PlaybookInfo) {
	fmt.Printf("Library: %s\n", playbookListLibrary)
	fmt.Printf("Total: %d playbooks\n\n", len(playbooks))

	switch playbookListFormat {
	case "json":
		displayPlaybooksJSON(playbooks)
	case "yaml":
		displayPlaybooksYAML(playbooks)
	default:
		displayPlaybooksTable(playbooks)
	}
}

func displayPlaybooksTable(playbooks []PlaybookInfo) {
	fmt.Printf("%s %s %s\n",
		common.PadRight("Name", 30), common.PadRight("Path", 50), common.PadRight("Size", 10))
	fmt.Println(strings.Repeat("-", 93))
	for _, pb := range playbooks {
		size := formatSize(pb.Size)
		fmt.Printf("%s %s %s\n",
			common.PadRight(common.TruncateByWidth(pb.Name, 30), 30),
			common.PadRight(common.TruncateByWidth(pb.Path, 50), 50),
			common.PadRight(size, 10))
	}
}

func displayPlaybooksJSON(playbooks []PlaybookInfo) {
	fmt.Println("{")
	fmt.Printf("  \"library\": \"%s\",\n", playbookListLibrary)
	fmt.Printf("  \"total\": %d,\n", len(playbooks))
	fmt.Println("  \"playbooks\": [")
	for i, pb := range playbooks {
		comma := ","
		if i == len(playbooks)-1 {
			comma = ""
		}
		fmt.Printf("    {\"name\": \"%s\", \"path\": \"%s\", \"size\": %d}%s\n", pb.Name, pb.Path, pb.Size, comma)
	}
	fmt.Println("  ]")
	fmt.Println("}")
}

func displayPlaybooksYAML(playbooks []PlaybookInfo) {
	fmt.Printf("library: %s\n", playbookListLibrary)
	fmt.Printf("total: %d\n", len(playbooks))
	fmt.Println("playbooks:")
	for _, pb := range playbooks {
		fmt.Printf("  - name: %s\n", pb.Name)
		fmt.Printf("    path: %s\n", pb.Path)
		fmt.Printf("    size: %d\n", pb.Size)
	}
}

// PlaybookInfo 剧本信息
type PlaybookInfo struct {
	Name string
	Path string
	Size int64
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
}
