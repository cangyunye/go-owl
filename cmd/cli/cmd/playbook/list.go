package playbook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

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

	// 查找所有 YAML 文件，并解析元数据
	var playbooks []PlaybookInfo
	err := filepath.Walk(library, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
			meta := ReadPlaybookMeta(path)
			playbooks = append(playbooks, meta)
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
	fmt.Println(`  $ owl playbook run deploy.yml`)
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
	fmt.Printf("%s %s %s %s %s\n",
		common.PadRight("Name", 25), common.PadRight("Description", 35), common.PadRight("Tasks", 8), common.PadRight("Path", 50), common.PadRight("Size", 10))
	fmt.Println(strings.Repeat("-", 130))
	for _, pb := range playbooks {
		size := formatSize(pb.Size)
		desc := pb.Description
		if desc == "" {
			desc = "-"
		}
		tasks := fmt.Sprintf("%d", pb.TasksCount)
		if pb.TasksCount == 0 {
			tasks = "-"
		}
		fmt.Printf("%s %s %s %s %s\n",
			common.PadRight(common.TruncateByWidth(pb.Name, 25), 25),
			common.PadRight(common.TruncateByWidth(desc, 35), 35),
			common.PadRight(tasks, 8),
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
		desc := pb.Description
		fmt.Printf("    {\"name\": \"%s\", \"description\": \"%s\", \"tasks\": %d, \"path\": \"%s\", \"size\": %d}%s\n",
			pb.Name, desc, pb.TasksCount, pb.Path, pb.Size, comma)
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
		fmt.Printf("    description: \"%s\"\n", pb.Description)
		fmt.Printf("    tasks: %d\n", pb.TasksCount)
		fmt.Printf("    path: %s\n", pb.Path)
		fmt.Printf("    size: %d\n", pb.Size)
	}
}

// PlaybookInfo 剧本信息
type PlaybookInfo struct {
	Name        string
	Description string
	TasksCount  int
	Path        string
	Size        int64
}

// playbookMeta 轻量级 YAML 元数据结构，用于 list 命令解析
// 只读取顶层字段，不做完整验证
type playbookMeta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Tasks       []any  `yaml:"tasks"`
}

// ReadPlaybookMeta 轻量读取 playbook 元数据（name, description, tasks 数量）
func ReadPlaybookMeta(path string) PlaybookInfo {
	info := PlaybookInfo{
		Name: filepath.Base(path),
		Path: path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return info
	}

	var meta playbookMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return info
	}

	info.Description = meta.Description
	info.TasksCount = len(meta.Tasks)

	// 获取文件大小
	if fi, err := os.Stat(path); err == nil {
		info.Size = fi.Size()
	}

	return info
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
