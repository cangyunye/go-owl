package node

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

var pingAll bool
var pingTimeout time.Duration
var pingCount int

// NewPingCmd 创建 ping 命令
func NewPingCmd() *cobra.Command {
	pingCmd := &cobra.Command{
		Use:   "ping [node_id...]",
		Short: "检查节点是否可达",
		Long:  `Ping 一个或多个节点，检查是否可达。可使用 --all 检查所有节点。`,
		Example: `# Ping 单个节点
  owl node ping node1

  # Ping 多个节点
  owl node ping node1 node2 node3

  # Ping 所有节点
  owl node ping --all

  # 设置超时时间和次数
  owl node ping --all --timeout 5s --count 3

  # Ping 3次取平均值
  owl node ping node1 -n 3`,
		Run: func(cmd *cobra.Command, args []string) {
			runPing(args)
		},
	}

	pingCmd.Flags().BoolVar(&pingAll, "all", false, "检查所有节点")
	pingCmd.Flags().DurationVarP(&pingTimeout, "timeout", "t", 3*time.Second, "每个 ping 的超时时间")
	pingCmd.Flags().IntVarP(&pingCount, "count", "n", 1, "ping 次数（支持浮点数表示间隔时间）")

	return pingCmd
}

func runPing(nodeIDs []string) {
	store := common.GetNodeStore()

	var nodes []*common.NodeInfo
	var err error

	if pingAll {
		nodes, err = store.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
	} else if len(nodeIDs) > 0 {
		for _, id := range nodeIDs {
			node, err := store.Get(id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "警告: 节点 '%s' 未找到，跳过\n", id)
				continue
			}
			nodes = append(nodes, node)
		}
	} else {
		fmt.Fprintln(os.Stderr, "错误: 请指定节点 ID 或使用 --all")
		fmt.Fprintln(os.Stderr, "使用 'owl node ping --help' 获取更多信息")
		os.Exit(1)
	}

	if len(nodes) == 0 {
		fmt.Println("没有节点需要检查")
		return
	}

	fmt.Printf("正在检查 %d 个节点... (超时: %s, 次数: %d)\n\n", len(nodes), pingTimeout, pingCount)

	reachable := 0
	unreachable := 0

	for _, node := range nodes {
		addr := node.Address
		if host, _, err := net.SplitHostPort(addr); err == nil {
			addr = host
		}

		var latencies []time.Duration
		success := false

		for i := 0; i < pingCount; i++ {
			start := time.Now()
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", addr, node.Port), pingTimeout)
			latency := time.Since(start)

			if err == nil {
				conn.Close()
				latencies = append(latencies, latency)
				success = true
			}

			if i < pingCount-1 && success {
				time.Sleep(100 * time.Millisecond)
			}
		}

		if len(latencies) > 0 {
			var total time.Duration
			for _, lat := range latencies {
				total += lat
			}
			avgLatency := total / time.Duration(len(latencies))
			minLatency := latencies[0]
			maxLatency := latencies[0]
			for _, lat := range latencies[1:] {
				if lat < minLatency {
					minLatency = lat
				}
				if lat > maxLatency {
					maxLatency = lat
				}
			}

			if pingCount > 1 {
				fmt.Printf("  ✓ %s (%s) - 可达\n", node.ID, node.Address)
				fmt.Printf("    次数: %d, 平均: %v, 最小: %v, 最大: %v\n",
					len(latencies), avgLatency.Round(time.Millisecond),
					minLatency.Round(time.Millisecond), maxLatency.Round(time.Millisecond))
			} else {
				fmt.Printf("  ✓ %s (%s) - 可达 (%v)\n", node.ID, node.Address, avgLatency.Round(time.Millisecond))
			}
			reachable++
		} else {
			fmt.Printf("  ✗ %s (%s) - 不可达\n", node.ID, node.Address)
			unreachable++
		}
	}

	fmt.Printf("\n总结: %d 可达, %d 不可达, 共 %d\n", reachable, unreachable, len(nodes))
}
