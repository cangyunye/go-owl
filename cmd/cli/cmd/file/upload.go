package file

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/node"
)

var (
	uploadNodes       string
	uploadGroup       string
	uploadLabel       []string
	uploadDest        string
	uploadMode        string
	uploadParallel    bool
	uploadOverwrite   bool
	uploadNoOverwrite bool
	uploadResume      bool
)

func NewUploadCmd() *cobra.Command {
	uploadCmd := &cobra.Command{
		Use:   "upload <local-file>",
		Short: "上传文件到节点",
		Long: `上传本地文件到指定的远程节点，支持断点续传。

支持断点续传：
- 自动检测远程节点是否支持 rsync
- 支持则使用 rsync（支持断点续传）
- 不支持则回退到 scp

示例：
  owl file upload app.tar.gz --nodes node1,node2 --dest /opt/app/
  owl file upload config.yaml --group web --dest /etc/myapp/
  owl file upload data.json --label env=prod --no-resume`,
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
	uploadCmd.Flags().BoolVar(&uploadParallel, "parallel", true,
		"并行上传到多个节点")
	uploadCmd.Flags().BoolVar(&uploadOverwrite, "overwrite", false,
		"覆盖已存在的文件")
	uploadCmd.Flags().BoolVar(&uploadNoOverwrite, "no-overwrite", false,
		"如果文件已存在则跳过上传")
	uploadCmd.Flags().BoolVar(&uploadResume, "resume", true,
		"启用断点续传（rsync 优先）")

	return uploadCmd
}

func runUpload(cmd *cobra.Command, args []string) {
	localFile := args[0]

	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "错误: 本地文件不存在: %s\n", localFile)
		os.Exit(1)
	}

	nodeResolver := node.NewNodeResolver()

	var targetNodeIDs []string

	if uploadNodes != "" {
		targetNodeIDs = parseNodeList(uploadNodes)
	} else if uploadGroup != "" {
		nodes, err := nodeResolver.ListNodes(&node.ListOptions{
			Group: uploadGroup,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	} else if len(uploadLabel) > 0 {
		nodes, err := nodeResolver.ListNodes(&node.ListOptions{
			Label: uploadLabel[0],
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	} else {
		fmt.Fprintln(os.Stderr, "错误: 请指定 --nodes, --group 或 --label")
		os.Exit(1)
	}

	if len(targetNodeIDs) == 0 {
		fmt.Println("未找到目标节点")
		return
	}

	fmt.Printf("📤 文件: %s\n", localFile)
	fmt.Printf("📍 目标: %s\n", uploadDest)
	fmt.Printf("🎯 节点: %d 个\n", len(targetNodeIDs))
	if uploadParallel {
		fmt.Println("⚡ 模式: 并行上传")
	} else {
		fmt.Println("⚡ 模式: 串行上传")
	}
	if uploadResume {
		fmt.Println("🔄 断点续传: 已启用")
	} else {
		fmt.Println("🔄 断点续传: 已禁用")
	}
	fmt.Println("\n正在上传...")

	manager := transfer.NewTransferManager(nodeResolver)
	defer manager.Close()

	ctx := context.Background()
	opts := &transfer.UploadOptions{
		Parallel:    uploadParallel,
		Overwrite:   uploadOverwrite,
		NoOverwrite: uploadNoOverwrite,
		Resume:      uploadResume,
	}

	remotePath := uploadDest
	if remotePath[len(remotePath)-1] != '/' {
		remotePath += "/"
	}
	remotePath += getFileNameFromPath(localFile)

	results := manager.Upload(ctx, targetNodeIDs, localFile, remotePath, opts)

	success := 0
	failed := 0

	for _, result := range results {
		if result.Error != nil {
			fmt.Printf("❌ [%s] 失败: %v\n", result.NodeID, result.Error)
			failed++
		} else {
			method := "scp"
			if result.Method == "rsync" {
				method = "rsync"
				if result.Speed != "" && result.Speed != "N/A" {
					fmt.Printf("✅ [%s] 成功 [%s, %s]: %s\n", result.NodeID, method, result.Speed, result.Path)
					success++
					continue
				}
			}
			fmt.Printf("✅ [%s] 成功 [%s]: %s\n", result.NodeID, method, result.Path)
			success++
		}
	}

	fmt.Printf("\n📊 总结: %d 成功, %d 失败\n", success, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func parseNodeList(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func parseLabels(labels []string) map[string]string {
	result := make(map[string]string)
	for _, label := range labels {
		for i := 0; i < len(label); i++ {
			if label[i] == '=' {
				result[label[:i]] = label[i+1:]
				break
			}
		}
	}
	return result
}

func getFileNameFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
