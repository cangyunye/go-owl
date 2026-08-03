package node

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
)

// NewSampleCmd 创建示例节点生成命令
func NewSampleCmd() *cobra.Command {
	sampleCmd := &cobra.Command{
		Use:   "sample",
		Short: i18n.T("node.sample.short"),
		Long:  i18n.T("node.sample.long"),
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
		fmt.Printf("%s\n", i18n.T("node.sample.exists", configFile))
		fmt.Println(i18n.T("node.sample.exists_hint"))
		return
	}

	// 确保目录存在
	configDir := common.GetConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.sample.err_mkdir", err))
		os.Exit(1)
	}

	// 生成示例节点
	sampleNodes := getDefaultSampleNodes()

	// 写入文件
	data, err := json.MarshalIndent(sampleNodes, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.sample.err_serialize", err))
		os.Exit(1)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.sample.err_write", err))
		os.Exit(1)
	}

	fmt.Printf("%s\n", i18n.T("node.sample.ok", configFile))
	fmt.Println(i18n.T("node.sample.list_title"))
	for _, node := range sampleNodes {
		fmt.Printf("%s\n", i18n.T("node.sample.list_item", node.Name, node.Address, node.Port, node.Groups))
	}
	fmt.Println(i18n.T("node.sample.edit_hint"))
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
