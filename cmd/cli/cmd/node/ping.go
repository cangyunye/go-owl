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

  # 设置超时时间
  owl node ping --all --timeout 5s`,
		Run: func(cmd *cobra.Command, args []string) {
			runPing(args)
		},
	}

	pingCmd.Flags().BoolVar(&pingAll, "all", false, "检查所有节点")
	pingCmd.Flags().DurationVarP(&pingTimeout, "timeout", "t", 3*time.Second, "每个 ping 的超时时间")

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

	fmt.Printf("正在检查 %d 个节点... (超时: %s)\n\n", len(nodes), pingTimeout)

	reachable := 0
	unreachable := 0

	for _, node := range nodes {
		addr := node.Address
		// 如果有端口，先分离出来
		if host, _, err := net.SplitHostPort(addr); err == nil {
			addr = host
		}

		start := time.Now()
		// 使用 TCP 连接模拟 ping
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", addr, node.Port), pingTimeout)
		latency := time.Since(start)

		if err != nil {
			fmt.Printf("  ✗ %s (%s) - 不可达\n    %v\n", node.ID, node.Address, err)
			unreachable++
		} else {
			conn.Close()
			fmt.Printf("  ✓ %s (%s) - 可达 (%v)\n", node.ID, node.Address, latency.Round(time.Millisecond))
			reachable++
		}
	}

	fmt.Printf("\n总结: %d 可达, %d 不可达, 共 %d\n", reachable, unreachable, len(nodes))
}
