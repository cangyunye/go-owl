package file

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// downloadFlags
var (
	downloadNodes  string
	downloadGroup  string
	downloadLabel  []string
	downloadDest   string
	downloadSource string
)

// NewDownloadCmd 创建下载命令
func NewDownloadCmd() *cobra.Command {
	downloadCmd := &cobra.Command{
		Use:   "download <remote-file>",
		Short: "从节点下载文件",
		Long: `从远程节点下载文件到本地。

示例：
  owl file download /var/log/app.log --node node1 --dest ./logs/
  owl file download /tmp/*.log --nodes node1,node2 --dest ./logs/`,
		Args: cobra.ExactArgs(1),
		Run:  runDownload,
	}

	downloadCmd.Flags().StringVar(&downloadNodes, "nodes", "",
		"指定节点 ID (逗号分隔)")
	downloadCmd.Flags().StringVar(&downloadGroup, "group", "",
		"按分组选择节点")
	downloadCmd.Flags().StringSliceVarP(&downloadLabel, "label", "l", nil,
		"按标签选择节点")
	downloadCmd.Flags().StringVarP(&downloadDest, "dest", "d", ".",
		"本地目标目录")
	downloadCmd.Flags().StringVar(&downloadSource, "node", "",
		"指定源节点 (单节点下载)")

	return downloadCmd
}

func runDownload(cmd *cobra.Command, args []string) {
	remoteFile := args[0]
	store := common.GetNodeStore()

	// 如果指定了单节点下载
	if downloadSource != "" {
		nodeInfo, err := store.Get(downloadSource)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if nodeInfo.Status != "online" {
			fmt.Fprintf(os.Stderr, "Error: node %s is offline\n", downloadSource)
			os.Exit(1)
		}

		fileName := getFileNameFromPath(remoteFile)
		fmt.Printf("Downloading %s from %s...\n", remoteFile, downloadSource)
		fmt.Printf("Saving to %s/%s\n", downloadDest, fileName)

		// 模拟下载
		fmt.Printf("[%s] OK: Downloaded %s\n", downloadSource, remoteFile)
		return
	}

	// 多节点下载
	targetNodes := selectDownloadTargetNodes(store)
	if len(targetNodes) == 0 {
		fmt.Println("No target nodes found.")
		return
	}

	// 检查目标目录
	if err := os.MkdirAll(downloadDest, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	// 显示下载信息
	fmt.Printf("Remote file: %s\n", remoteFile)
	fmt.Printf("Local destination: %s\n", downloadDest)
	fmt.Printf("Source: %d nodes\n", len(targetNodes))
	fmt.Println("\nDownloading...")

	// 模拟下载
	success := 0
	failed := 0
	for _, n := range targetNodes {
		if n.Status == "online" {
			fileName := getFileNameFromPath(remoteFile)
			fmt.Printf("[%s] OK: Downloaded %s\n", n.ID, fileName)
			success++
		} else {
			fmt.Printf("[%s] FAIL: Node offline\n", n.ID)
			failed++
		}
	}

	fmt.Printf("\nSummary: %d succeeded, %d failed\n", success, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func selectDownloadTargetNodes(store common.NodeStore) []*common.NodeInfo {
	var result []*common.NodeInfo
	allNodes, _ := store.List()

	for _, n := range allNodes {
		if downloadNodes != "" {
			nodeIDs := common.ParseNodeList(downloadNodes)
			if !containsNodeIDList(nodeIDs, n.ID) {
				continue
			}
		}

		if downloadGroup != "" {
			if !containsNodeIDList(n.Groups, downloadGroup) {
				continue
			}
		}

		if len(downloadLabel) > 0 {
			match := true
			for _, label := range downloadLabel {
				parts := splitLabelEq(label)
				if len(parts) == 2 {
					key, value := parts[0], parts[1]
					if v, ok := n.Labels[key]; !ok || v != value {
						match = false
						break
					}
				}
			}
			if !match {
				continue
			}
		}

		result = append(result, n)
	}

	return result
}
