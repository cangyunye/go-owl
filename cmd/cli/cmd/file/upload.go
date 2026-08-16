package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/google/uuid"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/settings"
	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/node"
)

var (
	uploadNodes       string
	uploadGroup       []string
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
		Short: i18n.T("file.upload.cmd.short"),
		Long:  i18n.T("file.upload.cmd.long"),
		Args:  cobra.ExactArgs(1),
		Run:   runUpload,
	}

	uploadCmd.Flags().StringVarP(&uploadNodes, "nodes", "N", "",
		i18n.T("file.common.flag_nodes"))
	uploadCmd.Flags().StringSliceVarP(&uploadGroup, "groups", "g", nil, i18n.T("file.common.flag_groups"))
	uploadCmd.Flags().StringSliceVar(&uploadGroup, "group", nil, i18n.T("file.common.flag_group_deprecated"))
	uploadCmd.Flags().MarkHidden("group")
	uploadCmd.Flags().StringSliceVarP(&uploadLabel, "label", "l", nil,
		i18n.T("file.common.flag_label"))
	uploadCmd.Flags().StringVarP(&uploadDest, "dest", "d", "/tmp",
		i18n.T("file.common.flag_dest"))
	uploadCmd.Flags().StringVar(&uploadMode, "mode", "0644",
		i18n.T("file.upload.flag_mode"))
	uploadCmd.Flags().BoolVar(&uploadParallel, "parallel", true,
		i18n.T("file.upload.flag_parallel"))
	uploadCmd.Flags().BoolVarP(&uploadOverwrite, "overwrite", "O", false,
		i18n.T("file.upload.flag_overwrite"))
	uploadCmd.Flags().BoolVar(&uploadNoOverwrite, "no-overwrite", false,
		i18n.T("file.upload.flag_no_overwrite"))
	uploadCmd.Flags().BoolVarP(&uploadResume, "resume", "r", true,
		i18n.T("file.common.flag_resume"))

	return uploadCmd
}

func runUpload(cmd *cobra.Command, args []string) {
	localFile := args[0]

	// 从 owl settings 加载未显式指定的 flag 默认值
	applyUploadSettingsFallback(cmd)

	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("file.upload.err_local_file_not_found", localFile))
		os.Exit(1)
	}

	logger.Init(nil)
	defer logger.Sync()
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("file.common.warn_history_db", err))
	}

	common.CheckNodeConflictsBeforeExec()

	nodeResolver := node.NewNodeResolver()

	var targetNodeIDs []string

	if uploadNodes != "" {
		targetNodeIDs = parseNodeList(uploadNodes)
	} else if len(uploadGroup) > 0 {
		nodes, err := node.ListNodesByGroups(nodeResolver, uploadGroup)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("file.common.err_list_nodes", err))
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
			fmt.Fprintf(os.Stderr, "%s", i18n.T("file.common.err_list_nodes", err))
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	} else {
		fmt.Fprintln(os.Stderr, i18n.T("file.upload.err_no_selector"))
		os.Exit(1)
	}

	if len(targetNodeIDs) == 0 {
		fmt.Println(i18n.T("file.common.no_nodes"))
		return
	}

	fmt.Printf("%s", i18n.T("file.upload.info_file", localFile))
	fmt.Printf("%s", i18n.T("file.upload.info_dest", uploadDest))
	fmt.Printf("%s", i18n.T("file.common.info_nodes", i18n.F(len(targetNodeIDs))))
	if uploadParallel {
		fmt.Println(i18n.T("file.upload.mode_parallel"))
	} else {
		fmt.Println(i18n.T("file.upload.mode_serial"))
	}
	if uploadResume {
		fmt.Println(i18n.T("file.common.resume_enabled"))
	} else {
		fmt.Println(i18n.T("file.common.resume_disabled"))
	}
	fmt.Println(i18n.T("file.upload.uploading"))

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

	taskID := uuid.New().String()
	startTime := time.Now()
	meta, _ := json.Marshal(map[string]string{
		"local_path":  localFile,
		"remote_path": remotePath,
	})
	history.RecordOperation(&history.Operation{
		TaskID:    taskID,
		OpType:    "file_transfer",
		Command:   string(meta),
		Targets:   targetNodeIDs,
		Status:    "running",
		CreatedAt: startTime,
	})

	results := manager.Upload(ctx, targetNodeIDs, localFile, remotePath, opts)

	fileInfo, _ := os.Stat(localFile)
	fileSize := int64(0)
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}

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
			FileName:     getFileNameFromPath(localFile),
			FileSize:     fileSize,
			TransferType: result.Method,
			Status:       status,
			Progress:     100.0,
			Error:        errMsg,
			CreatedAt:    time.Now(),
		})

		if result.Error != nil {
			fmt.Printf("%s", i18n.T("file.common.result_failed", result.NodeID, result.Error))
			failed++
		} else {
			method := "scp"
			if result.Method == "rsync" {
				method = "rsync"
				if result.Speed != "" && result.Speed != "N/A" {
					fmt.Printf("%s", i18n.T("file.common.result_success_speed", result.NodeID, method, result.Speed, result.Path))
					success++
					continue
				}
			}
			fmt.Printf("%s", i18n.T("file.common.result_success", result.NodeID, method, result.Path))
			success++
		}
	}

	fmt.Printf("%s", i18n.T("file.common.summary", i18n.F(success), i18n.F(failed)))

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

func getFileNameFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// applyUploadSettingsFallback 从 owl settings 加载未显式指定的 upload flag 默认值
func applyUploadSettingsFallback(cmd *cobra.Command) {
	s := settings.GetCurrentSettings()

	// --groups: 如果用户未指定，使用 settings 中的 default.group 或 target.groups
	if !cmd.Flags().Changed("groups") {
		group := s.Default.Group
		if group == "" {
			group = s.Target.Groups
		}
		if group != "" {
			uploadGroup = strings.Split(group, ",")
		}
	}

	// --label: 如果用户未指定，使用 settings 中的 default.labels
	if !cmd.Flags().Changed("label") {
		for k, v := range s.Default.Labels {
			uploadLabel = append(uploadLabel, k+"="+v)
		}
	}

	// --parallel: 如果用户未指定，使用 settings 中的 default.parallel
	if !cmd.Flags().Changed("parallel") {
		uploadParallel = s.Default.Parallel
	}
}
