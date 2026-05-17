package node

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// NewSampleCmd 创建示例节点生成命令
func NewSampleCmd() *cobra.Command {
	sampleCmd := &cobra.Command{
		Use:   "sample",
		Short: "生成示例节点配置文件",
		Long: `生成示例节点配置文件到 ~/.owl/sample_nodes.json

该命令会在 ~/.owl 目录下创建 sample_nodes.json 文件，
包含一些示例节点配置，方便用户参考或快速开始使用。

示例：
  owl node sample

生成的文件可以直接编辑以添加自定义节点。`,
		Run: func(cmd *cobra.Command, args []string) {
			runSample()
		},
	}

	return sampleCmd
}

func runSample() {
	configFile := common.GetSampleConfigFile()

	// 检查文件是否已存在
	if _, err := os.Stat(configFile); err == nil {
		fmt.Printf("示例配置文件已存在: %s\n", configFile)
		fmt.Println("如需重新生成，请先删除该文件。")
		return
	}

	// 确保目录存在
	configDir := common.GetConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "创建配置目录失败: %v\n", err)
		os.Exit(1)
	}

	// 生成示例节点
	sampleNodes := getDefaultSampleNodes()

	// 写入文件
	data, err := json.MarshalIndent(sampleNodes, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "序列化示例节点失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入配置文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 已生成示例节点配置文件: %s\n", configFile)
	fmt.Println("\n文件包含以下示例节点:")
	for _, node := range sampleNodes {
		fmt.Printf("  - %s (%s:%d) [%s]\n", node.Name, node.Address, node.Port, node.Groups)
	}
	fmt.Println("\n您可以编辑该文件以添加自定义节点。")
}

func getDefaultSampleNodes() []*common.NodeInfo {
	return []*common.NodeInfo{
		{
			ID:      "node1",
			Name:    "web-server-1",
			Address: "192.168.1.10",
			Port:    22,
			User:    "root",
			Status:  "online",
			Groups:  []string{"web", "production"},
			Labels:  map[string]string{"env": "prod", "region": "us-east"},
		},
		{
			ID:      "node2",
			Name:    "web-server-2",
			Address: "192.168.1.11",
			Port:    22,
			User:    "root",
			Status:  "online",
			Groups:  []string{"web", "production"},
			Labels:  map[string]string{"env": "prod", "region": "us-west"},
		},
		{
			ID:      "node3",
			Name:    "db-server-1",
			Address: "192.168.1.20",
			Port:    22,
			User:    "root",
			Status:  "online",
			Groups:  []string{"database"},
			Labels:  map[string]string{"env": "prod", "type": "mysql"},
		},
		{
			ID:      "node4",
			Name:    "cache-server-1",
			Address: "192.168.1.30",
			Port:    22,
			User:    "root",
			Status:  "offline",
			Groups:  []string{"cache"},
			Labels:  map[string]string{"env": "staging"},
		},
	}
}
