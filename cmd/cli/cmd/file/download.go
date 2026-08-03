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
	downloadNodes       string
	downloadGroup       []string
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
		Short: i18n.T("file.download.cmd.short"),
		Long:  i18n.T("file.download.cmd.long"),
		Args:  cobra.ExactArgs(1),
		Run:   runDownload,
	}

	downloadCmd.Flags().StringVarP(&downloadNodes, "nodes", "N", "",
		i18n.T("file.common.flag_nodes"))
	downloadCmd.Flags().StringSliceVarP(&downloadGroup, "groups", "g", nil, i18n.T("file.common.flag_groups"))
	downloadCmd.Flags().StringSliceVar(&downloadGroup, "group", nil, i18n.T("file.common.flag_group_deprecated"))
	downloadCmd.Flags().MarkHidden("group")
	downloadCmd.Flags().StringSliceVarP(&downloadLabel, "label", "l", nil,
		i18n.T("file.common.flag_label"))
	downloadCmd.Flags().StringVarP(&downloadDest, "dest", "d", ".",
		i18n.T("file.download.flag_dest"))
	downloadCmd.Flags().StringVar(&downloadSource, "node", "",
		i18n.T("file.download.flag_node"))
	downloadCmd.Flags().BoolVar(&downloadParallel, "parallel", true,
		i18n.T("file.download.flag_parallel"))
	downloadCmd.Flags().BoolVar(&downloadSubdir, "subdir", false,
		i18n.T("file.download.flag_subdir"))
	downloadCmd.Flags().StringVar(&downloadNameFormat, "name-format", "",
		i18n.T("file.download.flag_name_format"))
	downloadCmd.Flags().BoolVarP(&downloadResume, "resume", "r", true,
		i18n.T("file.common.flag_resume"))

	return downloadCmd
}

func runDownload(cmd *cobra.Command, args []string) {
	remoteFile := args[0]

	// 从 owl settings 加载未显式指定的 flag 默认值
	applyDownloadSettingsFallback(cmd)

	logger.Init(nil)
	defer logger.Sync()
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("file.common.warn_history_db", err))
	}

	common.CheckNodeConflictsBeforeExec()

	nodeResolver := node.NewNodeResolver()

	var targetNodeIDs []string

	if downloadSource != "" {
		targetNodeIDs = []string{downloadSource}
	} else if downloadNodes != "" {
		targetNodeIDs = parseNodeList(downloadNodes)
	} else if len(downloadGroup) > 0 {
		nodes, err := node.ListNodesByGroups(nodeResolver, downloadGroup)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("file.common.err_list_nodes", err))
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
			fmt.Fprintf(os.Stderr, "%s", i18n.T("file.common.err_list_nodes", err))
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	}

	if len(targetNodeIDs) == 0 {
		fmt.Fprintln(os.Stderr, i18n.T("file.download.err_no_selector"))
		os.Exit(1)
	}

	if err := os.MkdirAll(downloadDest, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("file.download.err_mkdir", err))
		os.Exit(1)
	}

	fmt.Printf("%s", i18n.T("file.download.info_source", remoteFile))
	fmt.Printf("%s", i18n.T("file.common.info_nodes", i18n.F(len(targetNodeIDs))))
	fmt.Printf("%s", i18n.T("file.download.info_save_to", downloadDest))
	if downloadParallel {
		fmt.Println(i18n.T("file.download.mode_parallel"))
	} else {
		fmt.Println(i18n.T("file.download.mode_serial"))
	}
	if downloadSubdir {
		fmt.Println(i18n.T("file.download.subdir_mode"))
	}
	if downloadNameFormat != "" {
		fmt.Printf("%s", i18n.T("file.download.info_name_format", downloadNameFormat))
	}
	if downloadResume {
		fmt.Println(i18n.T("file.common.resume_enabled"))
	} else {
		fmt.Println(i18n.T("file.common.resume_disabled"))
	}
	fmt.Println(i18n.T("file.download.downloading"))

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

// applyDownloadSettingsFallback 从 owl settings 加载未显式指定的 download flag 默认值
func applyDownloadSettingsFallback(cmd *cobra.Command) {
	s := settings.GetCurrentSettings()

	// --groups: 如果用户未指定，使用 settings 中的 default.group 或 target.groups
	if !cmd.Flags().Changed("groups") {
		group := s.Default.Group
		if group == "" {
			group = s.Target.Groups
		}
		if group != "" {
			downloadGroup = strings.Split(group, ",")
		}
	}

	// --label: 如果用户未指定，使用 settings 中的 default.labels
	if !cmd.Flags().Changed("label") {
		for k, v := range s.Default.Labels {
			downloadLabel = append(downloadLabel, k+"="+v)
		}
	}

	// --parallel: 如果用户未指定，使用 settings 中的 default.parallel
	if !cmd.Flags().Changed("parallel") {
		downloadParallel = s.Default.Parallel
	}
}
