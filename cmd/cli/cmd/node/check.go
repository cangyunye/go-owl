package node

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

var checkAll bool
var checkTimeout time.Duration
var checkWorkers int

// NewCheckCmd 创建 check 命令
func NewCheckCmd() *cobra.Command {
	checkCmd := &cobra.Command{
		Use:   "check [node_id...]",
		Short: "检查节点 SSH 连通性（真实 SSH 认证）",
		Long: `通过真实的 SSH 握手检查节点连通性，支持密钥和密码两种认证方式。
自动尝试密钥认证，失败后回退到密码认证。

注意：这与 "owl node ping" 不同，ping 只检查 TCP 端口是否开放，
而 check 会完成完整的 SSH 认证流程。
`,
		Example: `# 检查单个节点
  owl node check node1

  # 检查多个节点
  owl node check node1 node2 node3

  # 检查所有节点
  owl node check --all

  # 调整并发数和超时时间
  owl node check --all --workers 10 --timeout 30s`,
		Run: func(cmd *cobra.Command, args []string) {
			runCheck(args)
		},
	}

	checkCmd.Flags().BoolVar(&checkAll, "all", false, "检查所有节点")
	checkCmd.Flags().DurationVarP(&checkTimeout, "timeout", "t", 10*time.Second, "每个检查的超时时间")
	checkCmd.Flags().IntVarP(&checkWorkers, "workers", "w", 5, "并发工作协程数")

	return checkCmd
}

type checkResult struct {
	node    *common.NodeInfo
	success bool
	method  string // "key", "password", or ""
	err     error
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

	fmt.Printf("正在检查 %d 个节点的 SSH 连通性... (超时: %s, 并发: %d)\n\n",
		len(nodes), checkTimeout, checkWorkers)

	resultChan := make(chan checkResult, len(nodes))
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, checkWorkers)

	for _, n := range nodes {
		wg.Add(1)
		go func(n *common.NodeInfo) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			r := checkNodeSSH(n)
			resultChan <- r
		}(n)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	online := 0
	offline := 0

	currentTime := time.Now().Format("2006-01-02 15:04:05")

	for r := range resultChan {
		if r.success {
			authMethod := "密钥"
			if r.method == "password" {
				authMethod = "密码"
			}
			fmt.Printf("  ✓ %s (%s:%d) - 在线 [%s认证]", r.node.ID, r.node.Address, r.node.Port, authMethod)
			online++
			r.node.Status = "online"
			r.node.LastCheckAt = currentTime
			r.node.UpdatedAt = currentTime
			if err := store.Update(r.node); err != nil {
				fmt.Printf(" [更新失败: %v]", err)
			} else {
				fmt.Printf(" [状态已更新]")
			}
			fmt.Println()
		} else {
			fmt.Printf("  ✗ %s (%s:%d) - SSH 不可达\n", r.node.ID, r.node.Address, r.node.Port)
			offline++
			r.node.Status = "offline"
			r.node.LastCheckAt = currentTime
			r.node.UpdatedAt = currentTime
			if err := store.Update(r.node); err != nil {
				fmt.Printf("    [状态更新失败: %v]\n", err)
			} else {
				fmt.Println("    [状态已更新为 offline]")
			}
			if r.err != nil {
				fmt.Printf("    原因: %v\n", r.err)
			}
		}
	}

	fmt.Printf("\n总结: %d 在线, %d 离线, 共 %d\n", online, offline, len(nodes))

	if err := store.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "保存节点状态失败: %v\n", err)
	} else {
		fmt.Println("节点状态保存成功")
	}
}

func checkNodeSSH(n *common.NodeInfo) checkResult {
	addr := fmt.Sprintf("%s:%d", n.Address, n.Port)

	sshUser := n.User
	if sshUser == "" {
		current, err := user.Current()
		if err == nil {
			sshUser = current.Username
		} else {
			sshUser = "root"
		}
	}

	// 先尝试密钥认证
	if n.SSHKey != "" {
		signer, err := parsePrivateKey(n.SSHKey)
		if err == nil {
			config := &ssh.ClientConfig{
				User:            sshUser,
				Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				Timeout:         checkTimeout,
			}

			client, err := ssh.Dial("tcp", addr, config)
			if err == nil {
				client.Close()
				return checkResult{node: n, success: true, method: "key"}
			}

			return checkResult{
				node:    n,
				success: false,
				err:     fmt.Errorf("密钥认证失败: %w", err),
			}
		}

		// 密钥文件解析失败（文件不存在等情况），记录下来但继续尝试密码
		if n.Password == "" {
			return checkResult{
				node:    n,
				success: false,
				err:     fmt.Errorf("密钥文件无效: %v（且未配置密码）", err),
			}
		}
	}

	// 密钥认证失败或无密钥，尝试密码认证
	if n.Password != "" {
		config := &ssh.ClientConfig{
			User:            sshUser,
			Auth:            []ssh.AuthMethod{ssh.Password(n.Password)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         checkTimeout,
		}

		client, err := ssh.Dial("tcp", addr, config)
		if err == nil {
			client.Close()
			return checkResult{node: n, success: true, method: "password"}
		}

		return checkResult{
			node:    n,
			success: false,
			err:     fmt.Errorf("密钥和密码认证均失败（密码: %w）", err),
		}
	}

	// 既没有密钥也没有密码
	return checkResult{
		node:    n,
		success: false,
		err:     fmt.Errorf("节点未配置 SSH 密钥或密码，无法认证"),
	}
}

func parsePrivateKey(keyPath string) (ssh.Signer, error) {
	expandedPath := keyPath
	if len(keyPath) > 2 && keyPath[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("无法解析用户目录: %w", err)
		}
		expandedPath = filepath.Join(home, keyPath[2:])
	}

	keyData, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("读取密钥文件失败: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("解析密钥失败: %w", err)
	}

	return signer, nil
}
