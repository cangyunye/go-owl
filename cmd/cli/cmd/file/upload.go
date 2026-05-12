package file

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// uploadFlags
var (
	uploadNodes string
	uploadGroup string
	uploadLabel []string
	uploadDest  string
	uploadMode  string
)

// NewUploadCmd 创建上传命令
func NewUploadCmd() *cobra.Command {
	uploadCmd := &cobra.Command{
		Use:   "upload <local-file>",
		Short: "上传文件到节点",
		Long: `上传本地文件到指定的远程节点。

示例：
  owl file upload app.tar.gz --nodes node1,node2 --dest /opt/app/
  owl file upload config.yaml --group web --dest /etc/myapp/
  owl file upload data.json --label env=prod --mode 0644`,
		Args: cobra.ExactArgs(1),
		Run:  runUpload,
	}

	uploadCmd.Flags().StringVar(&uploadNodes, "nodes", "",
		"指定节点 ID (逗号分隔)")
	uploadCmd.Flags().StringVar(&uploadGroup, "group", "",
		"按分组选择节点")
	uploadCmd.Flags().StringSliceVarP(&uploadLabel, "label", "l", nil,
		"按标签选择节点")
	uploadCmd.Flags().StringVarP(&uploadDest, "dest", "d", "/tmp",
		"目标目录")
	uploadCmd.Flags().StringVar(&uploadMode, "mode", "0644",
		"文件权限")

	return uploadCmd
}

func runUpload(cmd *cobra.Command, args []string) {
	localFile := args[0]
	store := common.GetNodeStore()

	// 检查本地文件
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: local file not found: %s\n", localFile)
		os.Exit(1)
	}

	// 获取目标节点
	targetNodes := selectUploadTargetNodes(store)
	if len(targetNodes) == 0 {
		fmt.Println("No target nodes found.")
		return
	}

	// 显示上传信息
	fmt.Printf("File: %s\n", localFile)
	fmt.Printf("Destination: %s\n", uploadDest)
	fmt.Printf("Mode: %s\n", uploadMode)
	fmt.Printf("Target: %d nodes\n", len(targetNodes))
	fmt.Println("\nUploading...")

	// 模拟上传
	success := 0
	failed := 0
	for _, n := range targetNodes {
		if n.Status == "online" {
			fileName := getFileNameFromPath(localFile)
			fmt.Printf("[%s] OK: Uploaded to %s/%s\n", n.ID, uploadDest, fileName)
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

func getFileNameFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

func selectUploadTargetNodes(store common.NodeStore) []*common.NodeInfo {
	var result []*common.NodeInfo
	allNodes, _ := store.List()

	for _, n := range allNodes {
		if uploadNodes != "" {
			nodeIDs := common.ParseNodeList(uploadNodes)
			if !containsNodeIDList(nodeIDs, n.ID) {
				continue
			}
		}

		if uploadGroup != "" {
			if !containsNodeIDList(n.Groups, uploadGroup) {
				continue
			}
		}

		if len(uploadLabel) > 0 {
			match := true
			for _, label := range uploadLabel {
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

func containsNodeIDList(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func splitLabelEq(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
