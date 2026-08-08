package file

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/google/uuid"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/settings"
	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/node"
)

var (
	transferNodes       string
	transferAllNodes    bool
	transferGroup       []string
	transferLabel       []string
	transferDest        string
	transferSourceCount int
	transferFanOut      int
	transferThreshold   int
)

func NewTransferCmd() *cobra.Command {
	transferCmd := &cobra.Command{
		Use:   "transfer <file>",
		Short: i18n.T("file.transfer.cmd.short"),
		Long:  i18n.T("file.transfer.cmd.long"),
		Args:  cobra.ExactArgs(1),
		Run:   runTransfer,
	}

	transferCmd.Flags().StringVarP(&transferNodes, "nodes", "N", "",
		i18n.T("file.transfer.flag_nodes"))
	transferCmd.Flags().BoolVar(&transferAllNodes, "all-nodes", false,
		i18n.T("file.transfer.flag_all_nodes"))
	transferCmd.Flags().StringSliceVarP(&transferGroup, "groups", "g", nil, i18n.T("file.common.flag_groups"))
	transferCmd.Flags().StringSliceVar(&transferGroup, "group", nil, i18n.T("file.common.flag_group_deprecated"))
	transferCmd.Flags().MarkHidden("group")
	transferCmd.Flags().StringSliceVarP(&transferLabel, "label", "l", nil,
		i18n.T("file.common.flag_label"))
	transferCmd.Flags().StringVarP(&transferDest, "dest", "d", "/tmp",
		i18n.T("file.common.flag_dest"))
	transferCmd.Flags().IntVar(&transferSourceCount, "source-count", 2,
		i18n.T("file.transfer.flag_source_count"))
	transferCmd.Flags().IntVar(&transferFanOut, "fan-out", 3,
		i18n.T("file.transfer.flag_fan_out"))
	transferCmd.Flags().IntVar(&transferThreshold, "threshold", 5,
		i18n.T("file.transfer.flag_threshold"))

	return transferCmd
}

func runTransfer(cmd *cobra.Command, args []string) {
	fileName := args[0]

	// 从 owl settings 加载未显式指定的 flag 默认值
	applyTransferSettingsFallback(cmd)

	fileInfo, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("file.transfer.err_file_not_found", fileName))
		os.Exit(1)
	}
	fileSize := fileInfo.Size()

	logger.Init(nil)
	defer logger.Sync()
	_, err = history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("file.common.warn_history_db", err))
	}

	common.CheckNodeConflictsBeforeExec()

	nodeResolver := node.NewNodeResolver()

	var resolvedNodes []*node.ResolvedNode

	if transferNodes != "" {
		nodeIDs := parseNodeList(transferNodes)
		resolvedNodes, err = nodeResolver.ResolveMultiple(nodeIDs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("file.transfer.err_resolve_nodes", err))
			os.Exit(1)
		}
	} else if transferAllNodes {
		resolvedNodes, err = nodeResolver.ListNodes(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("file.common.err_list_nodes", err))
			os.Exit(1)
		}
	} else if len(transferGroup) > 0 {
		resolvedNodes, err = node.ListNodesByGroups(nodeResolver, transferGroup)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("file.common.err_list_nodes", err))
			os.Exit(1)
		}
	} else if len(transferLabel) > 0 {
		resolvedNodes, err = nodeResolver.ListNodes(&node.ListOptions{
			Label: transferLabel[0],
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("file.common.err_list_nodes", err))
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, i18n.T("file.transfer.err_no_selector"))
		os.Exit(1)
	}

	if len(resolvedNodes) == 0 {
		fmt.Println(i18n.T("file.common.no_nodes"))
		return
	}

	useDiffusion := len(resolvedNodes) >= transferThreshold

	taskID := uuid.New().String()
	startTime := time.Now()
	nodeIDs := make([]string, len(resolvedNodes))
	for i, n := range resolvedNodes {
		nodeIDs[i] = n.ID
	}

	history.RecordOperation(&history.Operation{
		TaskID:    taskID,
		OpType:    "file_transfer",
		Command:   fileName,
		Targets:   nodeIDs,
		Status:    "running",
		CreatedAt: startTime,
	})

	remotePath := transferDest
	if remotePath[len(remotePath)-1] != '/' {
		remotePath += "/"
	}
	remotePath += getFileNameFromPath(fileName)

	ctx := context.Background()

	fmt.Printf("%s", i18n.T("file.transfer.info_file", fileName, fmt.Sprintf("%.2f", float64(fileSize)/1024/1024)))
	fmt.Printf("%s", i18n.T("file.transfer.info_dest", remotePath))
	fmt.Printf("%s", i18n.T("file.transfer.info_nodes", i18n.F(len(resolvedNodes))))

	if useDiffusion {
		fmt.Printf("%s", i18n.T("file.transfer.info_mode_diffusion", i18n.F(transferFanOut), i18n.F(transferThreshold)))
	} else {
		fmt.Printf("%s", i18n.T("file.transfer.info_mode_direct", i18n.F(transferThreshold)))
	}

	var successCount, failCount int

	if useDiffusion {
		successCount, failCount = runDiffusionTransfer(ctx, nodeResolver, taskID, fileName, fileSize, remotePath, resolvedNodes)
	} else {
		successCount, failCount = runDirectTransfer(ctx, nodeResolver, taskID, fileName, fileSize, remotePath, nodeIDs)
	}

	finalStatus := "completed"
	if failCount > 0 {
		if successCount == 0 {
			finalStatus = "failed"
		} else {
			finalStatus = "partial_failure"
		}
	}

	history.RecordOperation(&history.Operation{
		TaskID:    taskID,
		OpType:    "file_transfer",
		Command:   fileName,
		Targets:   nodeIDs,
		Status:    finalStatus,
		CreatedAt: startTime,
	})

	fmt.Println()
	fmt.Printf("%s", i18n.T("file.transfer.summary", i18n.F(successCount), i18n.F(failCount)))
	if failCount > 0 {
		os.Exit(1)
	}
}

func runDirectTransfer(ctx context.Context, nodeResolver *node.NodeResolver, taskID, fileName string, fileSize int64, remotePath string, nodeIDs []string) (int, int) {
	manager := transfer.NewTransferManager(nodeResolver)
	defer manager.Close()

	opts := &transfer.UploadOptions{
		Parallel: true,
		Resume:   true,
	}

	fmt.Println(i18n.T("file.transfer.transferring"))
	results := manager.Upload(ctx, nodeIDs, fileName, remotePath, opts)

	successCount := 0
	failCount := 0

	for _, result := range results {
		method := result.Method
		if method == "" {
			method = "scp"
		}

		status := "completed"
		errMsg := ""
		if result.Error != nil {
			status = "failed"
			errMsg = result.Error.Error()
			fmt.Printf("%s", i18n.T("file.transfer.node_failed", result.NodeID, method, result.Error))
			failCount++
		} else {
			speedInfo := ""
			if result.Speed != "" && result.Speed != "N/A" {
				speedInfo = ", " + result.Speed
			}
			fmt.Printf("%s", i18n.T("file.transfer.node_success", result.NodeID, method, speedInfo))
			successCount++
		}

		history.RecordFileTransfer(&history.FileTransfer{
			TaskID:       taskID,
			NodeID:       result.NodeID,
			FileName:     fileName,
			FileSize:     fileSize,
			TransferType: method,
			Status:       status,
			Progress:     100,
			Error:        errMsg,
			CreatedAt:    time.Now(),
		})
	}

	return successCount, failCount
}

func runDiffusionTransfer(ctx context.Context, nodeResolver *node.NodeResolver, taskID, fileName string, fileSize int64, remotePath string, resolvedNodes []*node.ResolvedNode) (int, int) {
	fmt.Println(i18n.T("file.transfer.building_tree"))

	modelNodes := resolvedToModelNodes(resolvedNodes)
	treeBuilder := transfer.NewTreeBuilder(transferFanOut, 10, transferThreshold)
	tree := treeBuilder.Build(modelNodes)

	diffTransfer := transfer.NewDiffusionTransfer(taskID, getFileNameFromPath(fileName), fileName, remotePath, fileSize, "", tree)
	diffTransfer.InitializeStatuses()

	displayDiffusionTree(tree, resolvedNodes)

	fmt.Println(i18n.T("file.transfer.transferring"))

	manager := transfer.NewTransferManager(nodeResolver)
	defer manager.Close()

	ctx = context.Background()
	opts := &transfer.UploadOptions{
		Parallel: true,
		Resume:   true,
	}

	successCount := 0
	failCount := 0

	queue := make([]string, 0)
	queue = append(queue, tree.Nodes["control"].Children...)

	progress := 0
	total := len(resolvedNodes)

	firstBatch := true

	for len(queue) > 0 {
		currentLevel := make([]string, len(queue))
		copy(currentLevel, queue)
		queue = nil

		levelNodeIDs := make([]string, 0)
		for _, nodeID := range currentLevel {
			if _, ok := tree.Nodes[nodeID]; ok {
				levelNodeIDs = append(levelNodeIDs, nodeID)
			}
		}

		if len(levelNodeIDs) == 0 {
			continue
		}

		results := manager.Upload(ctx, levelNodeIDs, fileName, remotePath, opts)

		resultMap := make(map[string]transfer.TransferResult)
		for _, r := range results {
			resultMap[r.NodeID] = r
		}

		for _, nodeID := range levelNodeIDs {
			result, ok := resultMap[nodeID]
			if !ok {
				continue
			}

			method := result.Method
			if method == "" {
				method = "scp"
			}

			status := "completed"
			errMsg := ""
			if result.Error != nil {
				status = "failed"
				errMsg = result.Error.Error()
				fmt.Printf("%s", i18n.T("file.transfer.node_failed", nodeID, method, result.Error))
				failCount++
				diffTransfer.UpdateNodeStatus(nodeID, transfer.DiffusionStatusFailed, 100, errMsg)
			} else {
				speedInfo := ""
				if result.Speed != "" && result.Speed != "N/A" {
					speedInfo = ", " + result.Speed
				}
				fmt.Printf("%s", i18n.T("file.transfer.node_success", nodeID, method, speedInfo))
				successCount++
				diffTransfer.UpdateNodeStatus(nodeID, transfer.DiffusionStatusCompleted, 100, "")
			}

			history.RecordFileTransfer(&history.FileTransfer{
				TaskID:       taskID,
				NodeID:       nodeID,
				FileName:     fileName,
				FileSize:     fileSize,
				TransferType: method,
				Status:       status,
				Progress:     100,
				Error:        errMsg,
				CreatedAt:    time.Now(),
			})

			progress++
			percent := float64(progress) / float64(total) * 100
			bar := generateProgressBar(percent, 40)
			fmt.Printf("%s", i18n.T("file.transfer.progress", bar, fmt.Sprintf("%.0f", percent), i18n.F(progress), i18n.F(total)))

			if result.Error == nil {
				treeNode := tree.Nodes[nodeID]
				if treeNode != nil && len(treeNode.Children) > 0 {
					queue = append(queue, treeNode.Children...)
				}
			}
		}

		if firstBatch {
			firstBatch = false
			break
		}
	}

	resolvedMap := make(map[string]*node.ResolvedNode)
	for _, rn := range resolvedNodes {
		resolvedMap[rn.ID] = rn
	}

	completedSources := make([]string, 0)
	for _, childID := range tree.Nodes["control"].Children {
		if st, ok := diffTransfer.NodeStatuses[childID]; ok && st.Status == transfer.DiffusionStatusCompleted {
			completedSources = append(completedSources, childID)
		}
	}

	if len(queue) > 0 {
		type relayTarget struct {
			nodeID   string
			host     string
			password string
		}

		var relayTargets []relayTarget
		var directNodeIDs []string

		for _, nodeID := range queue {
			resolved, ok := resolvedMap[nodeID]
			if !ok {
				continue
			}
			if resolved.SSHPassword != "" {
				relayTargets = append(relayTargets, relayTarget{
					nodeID:   nodeID,
					host:     fmt.Sprintf("%s@%s:%s", resolved.User, resolved.Address, remotePath),
					password: resolved.SSHPassword,
				})
			} else {
				directNodeIDs = append(directNodeIDs, nodeID)
			}
		}

		if len(directNodeIDs) > 0 {
			results := manager.Upload(ctx, directNodeIDs, fileName, remotePath, opts)
			for _, result := range results {
				method := result.Method
				if method == "" {
					method = "scp"
				}

				status := "completed"
				errMsg := ""
				if result.Error != nil {
					status = "failed"
					errMsg = result.Error.Error()
					fmt.Printf("%s", i18n.T("file.transfer.node_failed", result.NodeID, method, result.Error))
					failCount++
					diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusFailed, 100, errMsg)
				} else {
					speedInfo := ""
					if result.Speed != "" && result.Speed != "N/A" {
						speedInfo = ", " + result.Speed
					}
					fmt.Printf("%s", i18n.T("file.transfer.node_success", result.NodeID, method, speedInfo))
					successCount++
					diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusCompleted, 100, "")
				}

				history.RecordFileTransfer(&history.FileTransfer{
					TaskID:       taskID,
					NodeID:       result.NodeID,
					FileName:     fileName,
					FileSize:     fileSize,
					TransferType: method,
					Status:       status,
					Progress:     100,
					Error:        errMsg,
					CreatedAt:    time.Now(),
				})

				progress++
				percent := float64(progress) / float64(total) * 100
				bar := generateProgressBar(percent, 40)
				fmt.Printf("%s", i18n.T("file.transfer.progress", bar, fmt.Sprintf("%.0f", percent), i18n.F(progress), i18n.F(total)))
			}
		}

		if len(relayTargets) > 0 && len(completedSources) > 0 {
			relayExecutor := transfer.NewRelayExecutor(nodeResolver)

			var deployedSources []string
			for _, sourceID := range completedSources {
				fmt.Printf("%s", i18n.T("file.transfer.deploy_relay_tool", sourceID))
				if err := relayExecutor.DeployRelay(ctx, sourceID); err != nil {
					fmt.Printf("%s", i18n.T("file.transfer.deploy_relay_failed", sourceID, err))
					continue
				}
				deployedSources = append(deployedSources, sourceID)
			}

			if len(deployedSources) == 0 {
				fmt.Println(i18n.T("file.transfer.deploy_all_failed"))
				fallbackIDs := make([]string, len(relayTargets))
				for j, t := range relayTargets {
					fallbackIDs[j] = t.nodeID
				}
				results := manager.Upload(ctx, fallbackIDs, fileName, remotePath, opts)
				for _, result := range results {
					method := result.Method
					if method == "" {
						method = "scp"
					}
					status := "completed"
					errMsg := ""
					if result.Error != nil {
						status = "failed"
						errMsg = result.Error.Error()
						fmt.Printf("%s", i18n.T("file.transfer.fallback_direct_failed", result.NodeID, method, result.Error))
						failCount++
						diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusFailed, 100, errMsg)
					} else {
						speedInfo := ""
						if result.Speed != "" && result.Speed != "N/A" {
							speedInfo = ", " + result.Speed
						}
						fmt.Printf("%s", i18n.T("file.transfer.fallback_direct_success", result.NodeID, method, speedInfo))
						successCount++
						diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusCompleted, 100, "")
					}
					history.RecordFileTransfer(&history.FileTransfer{
						TaskID:       taskID,
						NodeID:       result.NodeID,
						FileName:     fileName,
						FileSize:     fileSize,
						TransferType: method,
						Status:       status,
						Progress:     100,
						Error:        errMsg,
						CreatedAt:    time.Now(),
					})
					progress++
					percent := float64(progress) / float64(total) * 100
					bar := generateProgressBar(percent, 40)
					fmt.Printf("%s", i18n.T("file.transfer.progress", bar, fmt.Sprintf("%.0f", percent), i18n.F(progress), i18n.F(total)))
				}
			} else {
				dist := make(map[string][]relayTarget)
				for i, rt := range relayTargets {
					sourceIdx := i % len(deployedSources)
					dist[deployedSources[sourceIdx]] = append(dist[deployedSources[sourceIdx]], rt)
				}

				for _, sourceID := range deployedSources {
					targets, ok := dist[sourceID]
					if !ok || len(targets) == 0 {
						continue
					}

					relayTargetObjs := make([]transfer.RelayTarget, len(targets))
					targetNodeIDs := make([]string, len(targets))
					for j, t := range targets {
						relayTargetObjs[j] = transfer.RelayTarget{Host: t.host, Password: t.password}
						targetNodeIDs[j] = t.nodeID
					}

					subTask := &transfer.RelaySubTask{
						SourceNodeID: sourceID,
						Targets:      relayTargetObjs,
						SourceFile:   remotePath,
						TimeoutSec:   300,
					}

					targetNames := make([]string, len(targetNodeIDs))
					for j, id := range targetNodeIDs {
						name := id
						if rn, ok := resolvedMap[id]; ok && rn.Name != "" {
							name = rn.Name
						}
						targetNames[j] = name
					}
					fmt.Printf("%s", i18n.T("file.transfer.relay_to", sourceID, strings.Join(targetNames, ", ")))

					relayResults, relayErr := relayExecutor.ExecuteRelay(ctx, sourceID, subTask)
					if relayErr != nil {
						fmt.Printf("%s", i18n.T("file.transfer.relay_warning", sourceID, relayErr))
					}

					hostToNodeID := make(map[string]string)
					for _, t := range targets {
						hostToNodeID[t.host] = t.nodeID
					}

					var failedRelayTargets []relayTarget
					for _, rr := range relayResults {
						nodeID := hostToNodeID[rr.Target]
						if nodeID == "" {
							nodeID = rr.Target
						}

						name := nodeID
						if rn, ok := resolvedMap[nodeID]; ok && rn.Name != "" {
							name = rn.Name
						}

						if rr.Status == "success" {
							fmt.Printf("%s", i18n.T("file.transfer.relay_success", name, i18n.F(rr.DurationMs)))
							successCount++
							diffTransfer.UpdateNodeStatus(nodeID, transfer.DiffusionStatusCompleted, 100, "")

							history.RecordFileTransfer(&history.FileTransfer{
								TaskID:       taskID,
								NodeID:       nodeID,
								FileName:     fileName,
								FileSize:     fileSize,
								TransferType: "relay",
								Status:       "completed",
								Progress:     100,
								Error:        "",
								CreatedAt:    time.Now(),
							})
						} else {
							fmt.Printf("%s", i18n.T("file.transfer.relay_failed_fallback", name, rr.Error))
							failedRelayTargets = append(failedRelayTargets, relayTarget{
								nodeID:   nodeID,
								host:     rr.Target,
								password: "",
							})
						}

						progress++
						percent := float64(progress) / float64(total) * 100
						bar := generateProgressBar(percent, 40)
						fmt.Printf("%s", i18n.T("file.transfer.progress", bar, fmt.Sprintf("%.0f", percent), i18n.F(progress), i18n.F(total)))
					}

					if len(failedRelayTargets) > 0 {
						fallbackIDs := make([]string, len(failedRelayTargets))
						for j, t := range failedRelayTargets {
							fallbackIDs[j] = t.nodeID
						}
						fmt.Printf("%s", i18n.T("file.transfer.relay_nodes_failed", sourceID, i18n.F(len(fallbackIDs)), fallbackIDs))
						results := manager.Upload(ctx, fallbackIDs, fileName, remotePath, opts)
						for _, result := range results {
							method := result.Method
							if method == "" {
								method = "scp"
							}
							status := "completed"
							errMsg := ""
							if result.Error != nil {
								status = "failed"
								errMsg = result.Error.Error()
								fmt.Printf("%s", i18n.T("file.transfer.fallback_direct_failed", result.NodeID, method, result.Error))
								failCount++
								diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusFailed, 100, errMsg)
							} else {
								speedInfo := ""
								if result.Speed != "" && result.Speed != "N/A" {
									speedInfo = ", " + result.Speed
								}
								fmt.Printf("%s", i18n.T("file.transfer.fallback_direct_success", result.NodeID, method, speedInfo))
								successCount++
								diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusCompleted, 100, "")
							}
							history.RecordFileTransfer(&history.FileTransfer{
								TaskID:       taskID,
								NodeID:       result.NodeID,
								FileName:     fileName,
								FileSize:     fileSize,
								TransferType: method,
								Status:       status,
								Progress:     100,
								Error:        errMsg,
								CreatedAt:    time.Now(),
							})
							progress++
							percent := float64(progress) / float64(total) * 100
							bar := generateProgressBar(percent, 40)
							fmt.Printf("%s", i18n.T("file.transfer.progress", bar, fmt.Sprintf("%.0f", percent), i18n.F(progress), i18n.F(total)))
						}
					}
				}
			}
		} else if len(relayTargets) > 0 && len(completedSources) == 0 {
			fmt.Println(i18n.T("file.transfer.no_relay_source"))
			for _, rt := range relayTargets {
				directNodeIDs = append(directNodeIDs, rt.nodeID)
			}
			if len(directNodeIDs) > 0 {
				results := manager.Upload(ctx, directNodeIDs, fileName, remotePath, opts)
				for _, result := range results {
					method := result.Method
					if method == "" {
						method = "scp"
					}
					status := "completed"
					errMsg := ""
					if result.Error != nil {
						status = "failed"
						errMsg = result.Error.Error()
						fmt.Printf("%s", i18n.T("file.transfer.norelay_downgrade_failed", result.NodeID, method, result.Error))
						failCount++
						diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusFailed, 100, errMsg)
					} else {
						speedInfo := ""
						if result.Speed != "" && result.Speed != "N/A" {
							speedInfo = ", " + result.Speed
						}
						fmt.Printf("%s", i18n.T("file.transfer.norelay_downgrade_success", result.NodeID, method, speedInfo))
						successCount++
						diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusCompleted, 100, "")
					}
					history.RecordFileTransfer(&history.FileTransfer{
						TaskID:       taskID,
						NodeID:       result.NodeID,
						FileName:     fileName,
						FileSize:     fileSize,
						TransferType: method,
						Status:       status,
						Progress:     100,
						Error:        errMsg,
						CreatedAt:    time.Now(),
					})
					progress++
					percent := float64(progress) / float64(total) * 100
					bar := generateProgressBar(percent, 40)
					fmt.Printf("%s", i18n.T("file.transfer.progress", bar, fmt.Sprintf("%.0f", percent), i18n.F(progress), i18n.F(total)))
				}
			}
		}
	}

	fmt.Println()

	return successCount, failCount
}

func resolvedToModelNodes(resolved []*node.ResolvedNode) []*model.Node {
	nodes := make([]*model.Node, len(resolved))
	for i, r := range resolved {
		labels := make(map[string]string)
		for k, v := range r.Labels {
			labels[k] = v
		}
		groups := make([]string, len(r.Groups))
		copy(groups, r.Groups)

		nodes[i] = &model.Node{
			ID:      r.ID,
			Name:    r.Name,
			Address: r.Address,
			Port:    r.Port,
			User:    r.User,
			Status:  model.NodeStatusOnline,
			Groups:  groups,
			Labels:  labels,
		}
	}
	return nodes
}

func displayDiffusionTree(tree *transfer.DiffusionTree, resolvedNodes []*node.ResolvedNode) {
	nodeNameMap := make(map[string]string)
	for _, n := range resolvedNodes {
		name := n.ID
		if n.Name != "" {
			name = n.Name
		}
		nodeNameMap[n.ID] = name
	}

	fmt.Println(i18n.T("file.transfer.tree_title"))
	fmt.Println("========================")

	controlNode := tree.Nodes["control"]
	sourceNodes := controlNode.Children

	fmt.Print(i18n.T("file.transfer.source_nodes_label"))
	for i, id := range sourceNodes {
		if i > 0 {
			fmt.Print(", ")
		}
		name, ok := nodeNameMap[id]
		if ok {
			fmt.Print(name)
		} else {
			fmt.Print(id)
		}
	}
	fmt.Println()

	childIndex := 0
	for _, sourceID := range sourceNodes {
		sourceNode := tree.Nodes[sourceID]
		if sourceNode == nil || len(sourceNode.Children) == 0 {
			continue
		}

		sourceName, ok := nodeNameMap[sourceID]
		if !ok {
			sourceName = sourceID
		}
		fmt.Printf("  %s -> ", sourceName)

		for j, childID := range sourceNode.Children {
			if j > 0 {
				fmt.Print(", ")
			}
			childName, ok := nodeNameMap[childID]
			if ok {
				fmt.Print(childName)
			} else {
				fmt.Print(childID)
			}
			childIndex++
		}
		fmt.Println()
	}

	remainingCount := len(resolvedNodes) - len(sourceNodes) - childIndex
	if remainingCount > 0 {
		fmt.Printf("%s", i18n.T("file.transfer.more_nodes_deeper", i18n.F(remainingCount)))
	}
}

func generateProgressBar(percent float64, width int) string {
	filled := int(float64(width) * percent / 100)
	empty := width - filled

	result := "["
	for i := 0; i < filled; i++ {
		result += "="
	}
	for i := 0; i < empty; i++ {
		result += "-"
	}
	result += "]"

	return result
}

// applyTransferSettingsFallback 从 owl settings 加载未显式指定的 transfer flag 默认值
func applyTransferSettingsFallback(cmd *cobra.Command) {
	s := settings.GetCurrentSettings()

	// --groups: 如果用户未指定，使用 settings 中的 default.group 或 target.groups
	if !cmd.Flags().Changed("groups") {
		group := s.Default.Group
		if group == "" {
			group = s.Target.Groups
		}
		if group != "" {
			transferGroup = strings.Split(group, ",")
		}
	}

	// --label: 如果用户未指定，使用 settings 中的 default.labels
	if !cmd.Flags().Changed("label") {
		for k, v := range s.Default.Labels {
			transferLabel = append(transferLabel, k+"="+v)
		}
	}
}