package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/google/uuid"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/node"
)

var (
	downloadNodes       string
	downloadGroup       string
	downloadLabel       []string
	downloadDest        string
	downloadSource      string
	downloadParallel    bool
	downloadSubdir      bool
	downloadNameFormat  string
	downloadResume      bool
)

func NewDownloadCmd() *cobra.Command {
	downloadCmd := &cobra.Command{
		Use:   "download <remote-file>",
		Short: "从节点下载文件",
		Long: `从远程节点下载文件到本地，支持断点续传。

支持断点续传：
- 自动检测远程节点是否支持 rsync
- 支持则使用 rsync（支持断点续传）
- 不支持则回退到 scp

示例：
  owl file download /var/log/app.log --nodes node1 --dest ./logs/
  owl file download /tmp/data.json --group web --dest ./data/
  owl file download /var/log/app.log --nodes node1,node2 --dest ./logs/ --subdir
  owl file download /var/log/app.log --nodes node1,node2 --dest ./logs/ --name-format "{node}-{file}"`,
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
	downloadCmd.Flags().BoolVar(&downloadParallel, "parallel", true,
		"并行从多个节点下载")
	downloadCmd.Flags().BoolVar(&downloadSubdir, "subdir", false,
		"为每个节点创建子目录")
	downloadCmd.Flags().StringVar(&downloadNameFormat, "name-format", "",
		"文件命名格式 (支持 {node} 和 {file} 占位符)")
	downloadCmd.Flags().BoolVar(&downloadResume, "resume", true,
		"启用断点续传（rsync 优先）")

	return downloadCmd
}

func runDownload(cmd *cobra.Command, args []string) {
	remoteFile := args[0]

	logger.Init(nil)
	defer logger.Sync()
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法初始化历史记录数据库: %v\n", err)
	}

	common.CheckNodeConflictsBeforeExec()

	nodeResolver := node.NewNodeResolver()

	var targetNodeIDs []string

	if downloadSource != "" {
		targetNodeIDs = []string{downloadSource}
	} else if downloadNodes != "" {
		targetNodeIDs = parseNodeList(downloadNodes)
	} else if downloadGroup != "" {
		nodes, err := nodeResolver.ListNodes(&node.ListOptions{
			Group: downloadGroup,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	} else if len(downloadLabel) > 0 {
		nodes, err := nodeResolver.ListNodes(&node.ListOptions{
			Label: downloadLabel[0],
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	}

	if len(targetNodeIDs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 请指定 --nodes, --group, --label 或 --node")
		os.Exit(1)
	}

	if err := os.MkdirAll(downloadDest, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建目录失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📥 源文件: %s\n", remoteFile)
	fmt.Printf("🎯 节点: %d 个\n", len(targetNodeIDs))
	fmt.Printf("💾 保存到: %s\n", downloadDest)
	if downloadParallel {
		fmt.Println("⚡ 模式: 并行下载")
	} else {
		fmt.Println("⚡ 模式: 串行下载")
	}
	if downloadSubdir {
		fmt.Println("📁 组织: 按节点创建子目录")
	}
	if downloadNameFormat != "" {
		fmt.Printf("📝 命名: %s\n", downloadNameFormat)
	}
	if downloadResume {
		fmt.Println("🔄 断点续传: 已启用")
	} else {
		fmt.Println("🔄 断点续传: 已禁用")
	}
	fmt.Println("\n正在下载...")

	manager := transfer.NewTransferManager(nodeResolver)
	defer manager.Close()

	ctx := context.Background()
	opts := &transfer.DownloadOptions{
		Parallel:   downloadParallel,
		Subdir:     downloadSubdir,
		NameFormat: downloadNameFormat,
		Resume:     downloadResume,
	}

	taskID := uuid.New().String()
	startTime := time.Now()
	meta, _ := json.Marshal(map[string]string{
		"remote_file": remoteFile,
		"local_path":  downloadDest,
	})
	history.RecordOperation(&history.Operation{
		TaskID:    taskID,
		OpType:    "file_transfer",
		Command:   string(meta),
		Targets:   targetNodeIDs,
		Status:    "running",
		CreatedAt: startTime,
	})

	results := manager.Download(ctx, targetNodeIDs, remoteFile, downloadDest, opts)

	success := 0
	failed := 0

	for _, result := range results {
		status := "completed"
		errMsg := ""
		if result.Error != nil {
			status = "failed"
			errMsg = result.Error.Error()
		}
		history.RecordFileTransfer(&history.FileTransfer{
			TaskID:       taskID,
			NodeID:       result.NodeID,
			FileName:     getFileNameFromPath(remoteFile),
			FileSize:     0,
			TransferType: result.Method,
			Status:       status,
			Progress:     100.0,
			Error:        errMsg,
			CreatedAt:    time.Now(),
		})

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

	finalStatus := "completed"
	if failed > 0 {
		if success == 0 {
			finalStatus = "failed"
		} else {
			finalStatus = "partial_failure"
		}
	}
	history.RecordOperation(&history.Operation{
		TaskID:    taskID,
		OpType:    "file_transfer",
		Command:   string(meta),
		Targets:   targetNodeIDs,
		Status:    finalStatus,
		CreatedAt: startTime,
	})

	if failed > 0 {
		os.Exit(1)
	}
}
