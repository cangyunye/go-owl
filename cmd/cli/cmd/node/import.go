package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type importOptions struct {
	filePath     string
	overwrite    bool
	skipExisting bool
	dryRun       bool
	outputFormat string
	template     bool
	filterNodes  []string
	filterGroups []string
	filterLabels []string
}

func NewImportCmd() *cobra.Command {
	opts := &importOptions{}

	importCmd := &cobra.Command{
		Use:   "import",
		Short: "从文件导入节点",
		Long: `从 YAML 或 JSON 文件导入节点配置。

示例：
  owl node import -f nodes.yaml
  owl node import -f nodes.json --overwrite
  owl node import -f nodes.yaml --skip-existing
  owl node import --template > nodes.yaml`,
		Run: func(cmd *cobra.Command, args []string) {
			if opts.template {
				generateTemplate(opts.outputFormat)
				return
			}

			if opts.filePath == "" {
				fmt.Fprintln(os.Stderr, "Error: 请指定文件路径 -f")
				os.Exit(1)
			}

			importNodes(opts)
		},
	}

	importCmd.Flags().StringVarP(&opts.filePath, "file", "f", "",
		"导入文件路径 (YAML/JSON)")
	importCmd.Flags().BoolVar(&opts.overwrite, "overwrite", false,
		"覆盖已存在的节点")
	importCmd.Flags().BoolVar(&opts.skipExisting, "skip-existing", false,
		"跳过已存在的节点")
	importCmd.Flags().BoolVar(&opts.dryRun, "dry-run", false,
		"预览导入结果，不实际导入")
	importCmd.Flags().BoolVar(&opts.template, "template", false,
		"生成模板文件")
	importCmd.Flags().StringVarP(&opts.outputFormat, "format", "o", "yaml",
		"输出格式 (yaml/json)")

	return importCmd
}

func NewExportCmd() *cobra.Command {
	opts := &importOptions{}

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "导出节点到文件",
		Long: `将节点配置导出到 YAML 或 JSON 文件，支持按节点、标签、分组筛选。

示例：
  owl node export -f nodes.yaml
  owl node export -f nodes.json
  owl node export --nodes node1,node2 > filtered.yaml
  owl node export --groups web,production
  owl node export --labels env=prod
  owl node export > nodes.yaml`,
		Run: func(cmd *cobra.Command, args []string) {
			exportNodes(opts)
		},
	}

	exportCmd.Flags().StringVarP(&opts.filePath, "file", "f", "",
		"导出文件路径")
	exportCmd.Flags().StringVarP(&opts.outputFormat, "format", "o", "yaml",
		"输出格式 (yaml/json)")
	exportCmd.Flags().StringSliceVar(&opts.filterNodes, "nodes", nil,
		"按节点 ID 筛选 (逗号分隔)")
	exportCmd.Flags().StringSliceVar(&opts.filterGroups, "groups", nil,
		"按分组筛选 (逗号分隔)")
	exportCmd.Flags().StringSliceVar(&opts.filterLabels, "labels", nil,
		"按标签筛选 (格式: key=value)")

	return exportCmd
}

type nodeFile struct {
	Version string             `json:"version" yaml:"version"`
	Nodes   []*common.NodeInfo `json:"nodes" yaml:"nodes"`
}

func importNodes(opts *importOptions) {
	data, err := os.ReadFile(opts.filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: 读取文件失败: %v\n", err)
		os.Exit(1)
	}

	var nf nodeFile
	ext := strings.ToLower(filepath.Ext(opts.filePath))

	if ext == ".json" {
		if err := json.Unmarshal(data, &nf); err != nil {
			fmt.Fprintf(os.Stderr, "Error: 解析 JSON 失败: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := yaml.Unmarshal(data, &nf); err != nil {
			fmt.Fprintf(os.Stderr, "Error: 解析 YAML 失败: %v\n", err)
			os.Exit(1)
		}
	}

	store := common.GetNodeStore()

	success := 0
	failed := 0
	skipped := 0

	for _, node := range nf.Nodes {
		if node.ID == "" {
			fmt.Printf("跳过: 节点 ID 为空\n")
			failed++
			continue
		}

		if node.Name == "" {
			fmt.Printf("跳过 %s: 节点名称为空\n", node.ID)
			failed++
			continue
		}

		if node.Address == "" {
			fmt.Printf("跳过 %s: 节点地址为空\n", node.ID)
			failed++
			continue
		}

		_, err := store.Get(node.ID)
		nodeExists := err == nil

		if nodeExists && !opts.overwrite && !opts.skipExisting {
			fmt.Printf("跳过 %s: 节点已存在 (使用 --overwrite 覆盖或 --skip-existing 跳过)\n", node.ID)
			skipped++
			continue
		}

		if nodeExists && opts.skipExisting {
			skipped++
			continue
		}

		if opts.dryRun {
			fmt.Printf("[预览] %s -> %s (%s:%d)\n", node.ID, node.Name, node.Address, node.Port)
			success++
			continue
		}

		now := time.Now().Format(time.RFC3339)
		node.CreatedAt = now
		node.UpdatedAt = now

		if nodeExists {
			if err := store.Update(node); err != nil {
				fmt.Printf("更新失败 %s: %v\n", node.ID, err)
				failed++
			} else {
				fmt.Printf("✓ 更新节点 %s\n", node.ID)
				success++
			}
		} else {
			if err := store.Add(node); err != nil {
				fmt.Printf("添加失败 %s: %v\n", node.ID, err)
				failed++
			} else {
				fmt.Printf("✓ 添加节点 %s\n", node.ID)
				success++
			}
		}
	}

	if !opts.dryRun && success > 0 {
		store.Save()
	}

	fmt.Printf("\n结果: 添加/更新 %d, 跳过 %d, 失败 %d\n", success, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func exportNodes(opts *importOptions) {
	store := common.GetNodeStore()
	allNodes, err := store.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: 获取节点列表失败: %v\n", err)
		os.Exit(1)
	}

	nodes := exportFilterNodes(allNodes, opts)

	if len(nodes) == 0 {
		fmt.Println("没有符合条件的节点")
		return
	}

	nf := nodeFile{
		Version: "1.0",
		Nodes:   nodes,
	}

	var data []byte
	var err2 error

	if opts.outputFormat == "json" {
		data, err2 = json.MarshalIndent(nf, "", "  ")
	} else {
		data, err2 = yaml.Marshal(nf)
	}

	if err2 != nil {
		fmt.Fprintf(os.Stderr, "Error: 序列化失败: %v\n", err2)
		os.Exit(1)
	}

	if opts.filePath != "" {
		if err := os.WriteFile(opts.filePath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: 写入文件失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("已导出 %d 个节点到 %s\n", len(nodes), opts.filePath)
	} else {
		fmt.Println(string(data))
	}
}

func exportFilterNodes(nodes []*common.NodeInfo, opts *importOptions) []*common.NodeInfo {
	if len(opts.filterNodes) == 0 && len(opts.filterGroups) == 0 && len(opts.filterLabels) == 0 {
		return nodes
	}

	filterNodeSet := make(map[string]bool)
	for _, n := range opts.filterNodes {
		filterNodeSet[n] = true
	}

	filterGroupSet := make(map[string]bool)
	for _, g := range opts.filterGroups {
		filterGroupSet[g] = true
	}

	filterLabelMap := make(map[string]string)
	for _, l := range opts.filterLabels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) == 2 {
			filterLabelMap[parts[0]] = parts[1]
		}
	}

	var filtered []*common.NodeInfo
	for _, node := range nodes {
		if len(filterNodeSet) > 0 {
			if !filterNodeSet[node.ID] {
				continue
			}
		}

		if len(filterGroupSet) > 0 {
			hasGroup := false
			for _, g := range node.Groups {
				if filterGroupSet[g] {
					hasGroup = true
					break
				}
			}
			if !hasGroup {
				continue
			}
		}

		if len(filterLabelMap) > 0 {
			hasLabel := true
			for k, v := range filterLabelMap {
				if nodeVal, ok := node.Labels[k]; !ok || (v != "" && nodeVal != v) {
					hasLabel = false
					break
				}
			}
			if !hasLabel {
				continue
			}
		}

		filtered = append(filtered, node)
	}

	return filtered
}

func generateTemplate(format string) {
	template := nodeFile{
		Version: "1.0",
		Nodes: []*common.NodeInfo{
			{
				ID:        "web-server-01",
				Name:      "Web Server 01",
				Address:   "192.168.1.10",
				Port:      22,
				User:      "root",
				Password:  "",
				SSHKey:    "~/.ssh/id_rsa",
				ProxyJump: "",
				Status:    "offline",
				Groups:    []string{"web", "production"},
				Labels:    map[string]string{"env": "prod", "region": "cn-east-1"},
			},
			{
				ID:        "db-server-01",
				Name:      "Database Server 01",
				Address:   "192.168.1.20",
				Port:      22,
				User:      "postgres",
				Password:  "",
				SSHKey:    "",
				ProxyJump: "bastion.example.com",
				Status:    "offline",
				Groups:    []string{"database"},
				Labels:    map[string]string{"env": "prod", "type": "postgresql"},
			},
			{
				ID:        "cache-server-01",
				Name:      "Cache Server 01",
				Address:   "192.168.1.30",
				Port:      22,
				User:      "redis",
				Password:  "secure-password",
				SSHKey:    "~/.ssh/cache_key.pem",
				ProxyJump: "",
				Status:    "online",
				Groups:    []string{"cache", "staging"},
				Labels:    map[string]string{"env": "staging"},
			},
		},
	}

	var data []byte
	var err error

	if format == "json" {
		data, err = json.MarshalIndent(template, "", "  ")
	} else {
		data, err = yaml.Marshal(template)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: 生成模板失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(data))
}
