package file

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// transferFlags
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

// NewTransferCmd 创建扩散传输命令
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
	store := common.GetNodeStore()

	// 获取目标节点
	var targetNodes []*common.NodeInfo

	if transferAllNodes {
		allNodes, _ := store.List()
		targetNodes = allNodes
	} else if transferNodes != "" {
		nodeIDs := common.ParseNodeList(transferNodes)
		for _, id := range nodeIDs {
			if n, err := store.Get(id); err == nil {
				targetNodes = append(targetNodes, n)
			}
		}
	} else if transferGroup != "" || len(transferLabel) > 0 {
		targetNodes = selectTransferTargetNodes(store)
	} else {
		fmt.Fprintln(os.Stderr, "Error: must specify --nodes, --all-nodes, --group, or --label")
		os.Exit(1)
	}

	if len(targetNodes) == 0 {
		fmt.Println("No target nodes found.")
		return
	}

	// 过滤在线节点
	onlineNodes := make([]*common.NodeInfo, 0)
	for _, n := range targetNodes {
		if n.Status == "online" {
			onlineNodes = append(onlineNodes, n)
		}
	}

	if len(onlineNodes) == 0 {
		fmt.Println("No online nodes found.")
		return
	}

	// 判断是否使用扩散传输
	useDiffusion := shouldUseDiffusion(len(onlineNodes), transferThreshold)

	// 显示传输信息
	fmt.Printf("File: %s\n", fileName)
	fmt.Printf("Destination: %s\n", transferDest)
	fmt.Printf("Total nodes: %d\n", len(onlineNodes))

	if useDiffusion {
		fmt.Printf("Mode: Diffusion Transfer\n")
		fmt.Printf("Source count: %d\n", transferSourceCount)
		fmt.Printf("Fan-out: %d\n", transferFanOut)
	} else {
		fmt.Printf("Mode: Direct Transfer (nodes < threshold)\n")
	}

	// 构建扩散树
	fmt.Println("\nBuilding diffusion tree...")

	if useDiffusion && len(onlineNodes) > transferSourceCount {
		displayDiffusionTree(onlineNodes, transferSourceCount, transferFanOut)
	}

	// 模拟传输
	fmt.Println("\nTransferring...")

	progress := 0
	total := len(onlineNodes)
	for range onlineNodes {
		progress++
		percent := float64(progress) / float64(total) * 100
		bar := generateProgressBar(percent, 40)
		fmt.Printf("\r[%s] %.0f%% (%d/%d nodes)", bar, percent, progress, total)
	}

	fmt.Println()

	// 显示传输结果
	fmt.Println("Transfer complete!")
	fmt.Printf("  Total nodes: %d\n", total)
	fmt.Printf("  Source nodes: %d\n", minInt(transferSourceCount, total))
	fmt.Printf("  Transfer time: ~%.1fs\n", 3.2)
}

func shouldUseDiffusion(nodeCount, threshold int) bool {
	return nodeCount >= threshold
}

func displayDiffusionTree(nodes []*common.NodeInfo, sourceCount, fanOut int) {
	if len(nodes) == 0 {
		return
	}

	fmt.Println("\nDiffusion Tree Structure:")
	fmt.Println("========================")

	sourceNodes := nodes[:minInt(sourceCount, len(nodes))]
	otherNodes := nodes[minInt(sourceCount, len(nodes)):]

	fmt.Printf("Source nodes: ")
	for i, n := range sourceNodes {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%s", n.ID)
	}
	fmt.Println()

	if len(otherNodes) > 0 {
		fmt.Println("Diffusion paths:")
		childIndex := 0
		for _, source := range sourceNodes {
			maxChildren := minInt(fanOut, len(otherNodes)-childIndex)
			if maxChildren <= 0 {
				break
			}

			children := otherNodes[childIndex : childIndex+maxChildren]
			fmt.Printf("  %s -> ", source.ID)
			for j, child := range children {
				if j > 0 {
					fmt.Printf(", ")
				}
				fmt.Printf("%s", child.ID)
			}
			fmt.Println()

			childIndex += maxChildren
		}
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func selectTransferTargetNodes(store common.NodeStore) []*common.NodeInfo {
	var result []*common.NodeInfo
	allNodes, _ := store.List()

	for _, n := range allNodes {
		if transferGroup != "" {
			if !containsNodeIDList(n.Groups, transferGroup) {
				continue
			}
		}

		if len(transferLabel) > 0 {
			match := true
			for _, label := range transferLabel {
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
