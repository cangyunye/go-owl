package node

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

var checkAll bool
var checkTimeout time.Duration
var checkWorkers int
var checkUpdateStatus bool

// NewCheckCmd 创建 check 命令
func NewCheckCmd() *cobra.Command {
	checkCmd := &cobra.Command{
		Use:   "check [node_id...]",
		Short: "检查节点 SSH 连通性",
		Long:  `检查一个或多个节点的 SSH 连通性，并可选择性地更新状态。`,
		Example: `# 检查单个节点
  owl node check node1

  # 检查多个节点
  owl node check node1 node2 node3

  # 检查所有节点
  owl node check --all

  # 检查并更新节点状态
  owl node check --all --update

  # 调整并发数和超时时间
  owl node check --all --update --workers 10 --timeout 30s`,
		Run: func(cmd *cobra.Command, args []string) {
			runCheck(args)
		},
	}

	checkCmd.Flags().BoolVar(&checkAll, "all", false, "检查所有节点")
	checkCmd.Flags().DurationVarP(&checkTimeout, "timeout", "t", 10*time.Second, "每个检查的超时时间")
	checkCmd.Flags().IntVarP(&checkWorkers, "workers", "w", 5, "并发工作协程数")
	checkCmd.Flags().BoolVarP(&checkUpdateStatus, "update", "u", false, "检查后更新节点状态")

	return checkCmd
}

func runCheck(nodeIDs []string) {
	store := common.GetNodeStore()

	var nodes []*common.NodeInfo
	var err error

	if checkAll {
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
		fmt.Fprintln(os.Stderr, "使用 'owl node check --help' 获取更多信息")
		os.Exit(1)
	}

	if len(nodes) == 0 {
		fmt.Println("没有节点需要检查")
		return
	}

	statusText := ""
	if checkUpdateStatus {
		statusText = " (将更新状态)"
	}
	fmt.Printf("正在检查 %d 个节点... (超时: %s, 并发: %d)%s\n\n", 
		len(nodes), checkTimeout, checkWorkers, statusText)

	type result struct {
		node    *common.NodeInfo
		success bool
		err     error
	}

	resultChan := make(chan result, len(nodes))
	var wg sync.WaitGroup

	// 使用信号量控制并发数
	semaphore := make(chan struct{}, checkWorkers)

	for _, node := range nodes {
		wg.Add(1)
		go func(n *common.NodeInfo) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 尝试 TCP 连接到 SSH 端口
			address := fmt.Sprintf("%s:%d", n.Address, n.Port)
			conn, err := net.DialTimeout("tcp", address, checkTimeout)

			r := result{node: n}
			if err == nil {
				conn.Close()
				r.success = true
			} else {
				r.success = false
				r.err = err
			}

			resultChan <- r
		}(node)
	}

	// 等待所有 goroutine 完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	online := 0
	offline := 0

	for r := range resultChan {
		if r.success {
			fmt.Printf("  ✓ %s (%s:%d) - 在线", r.node.ID, r.node.Address, r.node.Port)
			online++
			if checkUpdateStatus {
				r.node.Status = "online"
				if err := store.Update(r.node); err != nil {
					fmt.Printf(" [更新失败: %v]", err)
				} else {
					fmt.Printf(" [状态已更新]")
				}
			}
			fmt.Println()
		} else {
			fmt.Printf("  ✗ %s (%s:%d) - 离线", r.node.ID, r.node.Address, r.node.Port)
			offline++
			if checkUpdateStatus {
				r.node.Status = "offline"
				if err := store.Update(r.node); err != nil {
					fmt.Printf(" [更新失败: %v]", err)
				} else {
					fmt.Printf(" [状态已更新]")
				}
			}
			fmt.Printf(" - %v\n", r.err)
		}
	}

	fmt.Printf("\n总结: %d 在线, %d 离线, 共 %d\n", online, offline, len(nodes))

	if checkUpdateStatus {
		// 保存更改
		if inMemStore, ok := store.(*common.InMemoryNodeStore); ok {
			if err := inMemStore.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "保存节点状态失败: %v\n", err)
			} else {
				fmt.Println("节点状态保存成功")
			}
		}
	}
}
