package node

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

var pingTimeout time.Duration
var pingAll bool

// NewPingCmd 创建节点 Ping 命令
func NewPingCmd() *cobra.Command {
	pingCmd := &cobra.Command{
		Use:   "ping",
		Short: "Ping 节点检查可达性",
		Long: `通过 ping 命令检查节点的可达性

可以 ping 单个或多个节点，返回节点是否可达以及响应时间。

示例：
  owl node ping node1
  owl node ping node1 node2 node3
  owl node ping --all
  owl node ping --timeout 5s --all`,
		Run: func(cmd *cobra.Command, args []string) {
			runPing(args)
		},
	}

	pingCmd.Flags().DurationVarP(&pingTimeout, "timeout", "t", 3*time.Second, "Ping 超时时间")
	pingCmd.Flags().BoolVar(&pingAll, "all", false, "Ping 所有节点")

	return pingCmd
}

func runPing(args []string) {
	store := common.GetNodeStore()

	var nodes []*common.NodeInfo
	var err error

	if pingAll {
		// Ping 所有节点
		nodes, err = store.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
	} else if len(args) > 0 {
		// Ping 指定的节点
		for _, nodeID := range args {
			node, err := store.Get(nodeID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "警告: 节点 '%s' 不存在，跳过\n", nodeID)
				continue
			}
			nodes = append(nodes, node)
		}
	} else {
		fmt.Fprintln(os.Stderr, "错误: 请指定要 ping 的节点或使用 --all")
		fmt.Fprintln(os.Stderr, "使用 'owl node ping --help' 查看帮助")
		os.Exit(1)
	}

	if len(nodes) == 0 {
		fmt.Println("没有可 ping 的节点")
		return
	}

	fmt.Printf("开始 Ping %d 个节点 (超时: %s)...\n\n", len(nodes), pingTimeout)

	reachable := 0
	unreachable := 0

	for _, node := range nodes {
		address := node.Address

		// 如果地址包含端口，去掉端口部分用于 ping
		if host, _, err := net.SplitHostPort(address); err == nil {
			address = host
		}

		start := time.Now()
		conn, err := net.DialTimeout("ip4:icmp", address, pingTimeout)
		latency := time.Since(start)

		if err != nil {
			fmt.Printf("✗ %s (%s): 不可达 - %v\n", node.ID, node.Address, err)
			unreachable++
		} else {
			conn.Close()
			fmt.Printf("✓ %s (%s): 可达 - %v\n", node.ID, node.Address, latency.Round(time.Millisecond))
			reachable++
		}
	}

	fmt.Printf("\n统计: %d 可达, %d 不可达, 总计 %d\n", reachable, unreachable, len(nodes))
}
