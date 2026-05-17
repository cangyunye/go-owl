package node

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"golang.org/x/crypto/ssh"
)

var checkTimeout time.Duration
var checkWorkers int
var checkUpdateStatus bool
var checkAll bool

type checkResult struct {
	node     *common.NodeInfo
	reachable bool
	err      error
}

// NewCheckCmd 创建节点 Check 命令
func NewCheckCmd() *cobra.Command {
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "检查节点 SSH 连接状态",
		Long: `通过 SSH 连接测试节点是否可达，并可选择性地更新节点状态

会尝试 SSH 连接每个节点，连接成功则更新 Status 为 'online'，
连接失败则更新为 'offline'。

示例：
  owl node check node1
  owl node check node1 node2 node3
  owl node check --all
  owl node check --all --update`,
		Run: func(cmd *cobra.Command, args []string) {
			runCheck(args)
		},
	}

	checkCmd.Flags().DurationVarP(&checkTimeout, "timeout", "t", 10*time.Second, "SSH 连接超时时间")
	checkCmd.Flags().BoolVarP(&checkUpdateStatus, "update", "u", false, "更新节点状态")
	checkCmd.Flags().BoolVar(&checkAll, "all", false, "检查所有节点")
	checkCmd.Flags().IntVarP(&checkWorkers, "workers", "w", 5, "并发检查的工作协程数")

	return checkCmd
}

func runCheck(args []string) {
	store := common.GetNodeStore()

	var nodes []*common.NodeInfo
	var err error

	if checkAll {
		nodes, err = store.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
	} else if len(args) > 0 {
		for _, nodeID := range args {
			node, err := store.Get(nodeID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "警告: 节点 '%s' 不存在，跳过\n", nodeID)
				continue
			}
			nodes = append(nodes, node)
		}
	} else {
		fmt.Fprintln(os.Stderr, "错误: 请指定要检查的节点或使用 --all")
		fmt.Fprintln(os.Stderr, "使用 'owl node check --help' 查看帮助")
		os.Exit(1)
	}

	if len(nodes) == 0 {
		fmt.Println("没有可检查的节点")
		return
	}

	if checkUpdateStatus {
		fmt.Printf("开始 SSH 连接检查 %d 个节点 (超时: %s, 并发: %d)...\n\n", len(nodes), checkTimeout, checkWorkers)
	} else {
		fmt.Printf("开始 SSH 连接检查 %d 个节点 (超时: %s, 并发: %d, 不更新状态)...\n\n", len(nodes), checkTimeout, checkWorkers)
	}

	resultChan := make(chan checkResult, len(nodes))
	var wg sync.WaitGroup

	// 控制并发数
	semaphore := make(chan struct{}, checkWorkers)

	for _, node := range nodes {
		wg.Add(1)
		go func(n *common.NodeInfo) {
			defer wg.Done()
			semaphore <- struct{}{}        // 获取信号量
			defer func() { <-semaphore }() // 释放信号量

			result := checkSSHConnection(n)
			resultChan <- result
		}(node)
	}

	// 等待所有 goroutine 完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	online := 0
	offline := 0

	for result := range resultChan {
		if result.reachable {
			fmt.Printf("✓ %s (%s:%d): 在线\n", result.node.ID, result.node.Address, result.node.Port)
			online++
		} else {
			fmt.Printf("✗ %s (%s:%d): 离线 - %v\n", result.node.ID, result.node.Address, result.node.Port, result.err)
			offline++
		}

		// 如果需要更新状态
		if checkUpdateStatus {
			status := "offline"
			if result.reachable {
				status = "online"
			}

			updatedNode := *result.node
			updatedNode.Status = status

			if err := store.Update(&updatedNode); err != nil {
				fmt.Fprintf(os.Stderr, "  更新节点 %s 状态失败: %v\n", result.node.ID, err)
			} else {
				fmt.Printf("  → 状态已更新为: %s\n", status)
			}
		}
	}

	fmt.Printf("\n统计: %d 在线, %d 离线, 总计 %d\n", online, offline, len(nodes))

	if checkUpdateStatus && (online > 0 || offline > 0) {
		// 保存更改
		if inMemStore, ok := store.(*common.InMemoryNodeStore); ok {
			if err := inMemStore.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "保存节点状态失败: %v\n", err)
			} else {
				fmt.Println("节点状态已保存")
			}
		}
	}

	if offline > 0 {
		os.Exit(1)
	}
}

func checkSSHConnection(node *common.NodeInfo) checkResult {
	result := checkResult{node: node}

	// 构建 SSH 客户端配置
	config := &ssh.ClientConfig{
		User:            node.User,
		Timeout:         checkTimeout,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// 根据认证方式配置
	if node.Password != "" {
		config.Auth = append(config.Auth, ssh.Password(node.Password))
	}
	if node.SSHKey != "" {
		signer, err := getSSHKeySigner(node.SSHKey)
		if err == nil {
			config.Auth = append(config.Auth, ssh.PublicKeys(signer))
		}
	}

	// 尝试连接
	addr := fmt.Sprintf("%s:%d", node.Address, node.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		result.reachable = false
		result.err = err
		return result
	}
	defer client.Close()

	result.reachable = true
	return result
}

func getSSHKeySigner(keyPath string) (ssh.Signer, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	return signer, nil
}
