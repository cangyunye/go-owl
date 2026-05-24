package file

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/google/uuid"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/node"
)

var (
	transferNodes       string
	transferAllNodes    bool
	transferGroup       string
	transferLabel       []string
	transferDest        string
	transferSourceCount int
	transferFanOut      int
	transferThreshold   int
)

func NewTransferCmd() *cobra.Command {
	transferCmd := &cobra.Command{
		Use:   "transfer <file>",
		Short: "节点间扩散传输 (P2P 模式)",
		Long: `使用自扩散传输方案，将文件从源节点扩散到其他节点。

前 N 个节点将被选为源节点，然后继续将文件传输到其他节点。

示例：
  owl file transfer app.tar.gz --nodes node1,node2,node3,node4,node5 \
    --dest /opt/app/ --source-count 2
  owl file transfer data.zip --all-nodes --dest /data/ --fan-out 3
  owl file transfer db.tar.gz --group database --source-count 1`,
		Args: cobra.ExactArgs(1),
		Run:  runTransfer,
	}

	transferCmd.Flags().StringVar(&transferNodes, "nodes", "",
		"指定节点列表 (逗号分隔)")
	transferCmd.Flags().BoolVar(&transferAllNodes, "all-nodes", false,
		"选择所有节点")
	transferCmd.Flags().StringVar(&transferGroup, "group", "",
		"按分组选择节点")
	transferCmd.Flags().StringSliceVarP(&transferLabel, "label", "l", nil,
		"按标签选择节点")
	transferCmd.Flags().StringVarP(&transferDest, "dest", "d", "/tmp",
		"目标目录")
	transferCmd.Flags().IntVar(&transferSourceCount, "source-count", 2,
		"源节点数量 (前 N 个节点作为源)")
	transferCmd.Flags().IntVar(&transferFanOut, "fan-out", 3,
		"扇出系数 (每个节点可传给的最大子节点数)")
	transferCmd.Flags().IntVar(&transferThreshold, "threshold", 5,
		"阈值 (小于此数量的节点直接传输，不使用扩散)")

	return transferCmd
}

func runTransfer(cmd *cobra.Command, args []string) {
	fileName := args[0]

	fileInfo, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "错误: 文件不存在: %s\n", fileName)
		os.Exit(1)
	}
	fileSize := fileInfo.Size()

	logger.Init(nil)
	defer logger.Sync()
	_, err = history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法初始化历史记录数据库: %v\n", err)
	}

	nodeResolver := node.NewNodeResolver()

	var resolvedNodes []*node.ResolvedNode

	if transferNodes != "" {
		nodeIDs := parseNodeList(transferNodes)
		resolvedNodes, err = nodeResolver.ResolveMultiple(nodeIDs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 解析节点失败: %v\n", err)
			os.Exit(1)
		}
	} else if transferAllNodes {
		resolvedNodes, err = nodeResolver.ListNodes(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
	} else if transferGroup != "" {
		resolvedNodes, err = nodeResolver.ListNodes(&node.ListOptions{
			Group: transferGroup,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
	} else if len(transferLabel) > 0 {
		resolvedNodes, err = nodeResolver.ListNodes(&node.ListOptions{
			Label: transferLabel[0],
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "错误: 请指定 --nodes, --all-nodes, --group 或 --label")
		os.Exit(1)
	}

	if len(resolvedNodes) == 0 {
		fmt.Println("未找到目标节点")
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

	fmt.Printf("文件: %s (%.2f MB)\n", fileName, float64(fileSize)/1024/1024)
	fmt.Printf("目标: %s\n", remotePath)
	fmt.Printf("节点: %d 个\n", len(resolvedNodes))

	if useDiffusion {
		fmt.Printf("模式: 扩散传输 (fan-out=%d, threshold=%d)\n", transferFanOut, transferThreshold)
	} else {
		fmt.Printf("模式: 直接传输 (节点数 < threshold=%d)\n", transferThreshold)
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
	fmt.Printf("总结: %d 成功, %d 失败\n", successCount, failCount)
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

	fmt.Println("\n正在传输...")
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
			fmt.Printf("  [%s] 失败 [%s]: %v\n", result.NodeID, method, result.Error)
			failCount++
		} else {
			speedInfo := ""
			if result.Speed != "" && result.Speed != "N/A" {
				speedInfo = ", " + result.Speed
			}
			fmt.Printf("  [%s] 成功 [%s%s]\n", result.NodeID, method, speedInfo)
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
	fmt.Println("\n构建扩散树...")

	modelNodes := resolvedToModelNodes(resolvedNodes)
	treeBuilder := transfer.NewTreeBuilder(transferFanOut, 10, transferThreshold)
	tree := treeBuilder.Build(modelNodes)

	diffTransfer := transfer.NewDiffusionTransfer(taskID, getFileNameFromPath(fileName), fileName, remotePath, fileSize, "", tree)
	diffTransfer.InitializeStatuses()

	displayDiffusionTree(tree, resolvedNodes)

	fmt.Println("\n正在传输...")

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
				fmt.Printf("  [%s] 失败 [%s]: %v\n", nodeID, method, result.Error)
				failCount++
				diffTransfer.UpdateNodeStatus(nodeID, transfer.DiffusionStatusFailed, 100, errMsg)
			} else {
				speedInfo := ""
				if result.Speed != "" && result.Speed != "N/A" {
					speedInfo = ", " + result.Speed
				}
				fmt.Printf("  [%s] 成功 [%s%s]\n", nodeID, method, speedInfo)
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
			fmt.Printf("\r  进度: [%s] %.0f%% (%d/%d)", bar, percent, progress, total)

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
					fmt.Printf("  [%s] 失败 [%s]: %v\n", result.NodeID, method, result.Error)
					failCount++
					diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusFailed, 100, errMsg)
				} else {
					speedInfo := ""
					if result.Speed != "" && result.Speed != "N/A" {
						speedInfo = ", " + result.Speed
					}
					fmt.Printf("  [%s] 成功 [%s%s]\n", result.NodeID, method, speedInfo)
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
				fmt.Printf("\r  进度: [%s] %.0f%% (%d/%d)", bar, percent, progress, total)
			}
		}

		if len(relayTargets) > 0 && len(completedSources) > 0 {
			relayExecutor := transfer.NewRelayExecutor(nodeResolver)

			dist := make(map[string][]relayTarget)
			for i, rt := range relayTargets {
				sourceIdx := i % len(completedSources)
				dist[completedSources[sourceIdx]] = append(dist[completedSources[sourceIdx]], rt)
			}

			for _, sourceID := range completedSources {
				targets, ok := dist[sourceID]
				if !ok || len(targets) == 0 {
					continue
				}

				fmt.Printf("  正在部署中继脚本到 [%s]...\n", sourceID)
				if err := relayExecutor.DeployScript(ctx, sourceID); err != nil {
					fmt.Printf("  警告: 部署中继脚本到 [%s] 失败: %v, 降级为直接传输\n", sourceID, err)
					fallbackIDs := make([]string, len(targets))
					for j, t := range targets {
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
							fmt.Printf("  [%s] 失败 [%s]: %v\n", result.NodeID, method, result.Error)
							failCount++
							diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusFailed, 100, errMsg)
						} else {
							speedInfo := ""
							if result.Speed != "" && result.Speed != "N/A" {
								speedInfo = ", " + result.Speed
							}
							fmt.Printf("  [%s] 成功 [%s%s]\n", result.NodeID, method, speedInfo)
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
						fmt.Printf("\r  进度: [%s] %.0f%% (%d/%d)", bar, percent, progress, total)
					}
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
				fmt.Printf("  [%s] 正在向 [%s] 中继传输...\n", sourceID, strings.Join(targetNames, ", "))

				relayResults, err := relayExecutor.ExecuteRelay(ctx, sourceID, subTask)
				if err != nil {
					fmt.Printf("  警告: [%s] 中继传输失败: %v, 降级为直接传输\n", sourceID, err)
					fallbackIDs := make([]string, len(targets))
					for j, t := range targets {
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
							fmt.Printf("  [%s] 失败 [%s]: %v\n", result.NodeID, method, result.Error)
							failCount++
							diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusFailed, 100, errMsg)
						} else {
							speedInfo := ""
							if result.Speed != "" && result.Speed != "N/A" {
								speedInfo = ", " + result.Speed
							}
							fmt.Printf("  [%s] 成功 [%s%s]\n", result.NodeID, method, speedInfo)
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
						fmt.Printf("\r  进度: [%s] %.0f%% (%d/%d)", bar, percent, progress, total)
					}
					continue
				}

				hostToNodeID := make(map[string]string)
				for _, t := range targets {
					hostToNodeID[t.host] = t.nodeID
				}

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
						fmt.Printf("  [%s] 成功 [relay, %dms]\n", name, rr.DurationMs)
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
						fmt.Printf("  [%s] 失败 [relay]: %s\n", name, rr.Error)
						failCount++
						diffTransfer.UpdateNodeStatus(nodeID, transfer.DiffusionStatusFailed, 100, rr.Error)

						history.RecordFileTransfer(&history.FileTransfer{
							TaskID:       taskID,
							NodeID:       nodeID,
							FileName:     fileName,
							FileSize:     fileSize,
							TransferType: "relay",
							Status:       "failed",
							Progress:     100,
							Error:        rr.Error,
							CreatedAt:    time.Now(),
						})
					}

					progress++
					percent := float64(progress) / float64(total) * 100
					bar := generateProgressBar(percent, 40)
					fmt.Printf("\r  进度: [%s] %.0f%% (%d/%d)", bar, percent, progress, total)
				}
			}
		} else if len(relayTargets) > 0 && len(completedSources) == 0 {
			fmt.Println("  无可用中继源节点，全部降级为直接传输")
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
						fmt.Printf("  [%s] 失败 [%s]: %v\n", result.NodeID, method, result.Error)
						failCount++
						diffTransfer.UpdateNodeStatus(result.NodeID, transfer.DiffusionStatusFailed, 100, errMsg)
					} else {
						speedInfo := ""
						if result.Speed != "" && result.Speed != "N/A" {
							speedInfo = ", " + result.Speed
						}
						fmt.Printf("  [%s] 成功 [%s%s]\n", result.NodeID, method, speedInfo)
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
					fmt.Printf("\r  进度: [%s] %.0f%% (%d/%d)", bar, percent, progress, total)
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

	fmt.Println("\n扩散树结构:")
	fmt.Println("========================")

	controlNode := tree.Nodes["control"]
	sourceNodes := controlNode.Children

	fmt.Print("源节点: ")
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
		fmt.Printf("  ... 还有 %d 个节点在更深层级\n", remainingCount)
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




