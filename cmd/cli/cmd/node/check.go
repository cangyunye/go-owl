package node

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
	owlssh "github.com/cangyunye/go-owl/internal/ssh"
)

var checkAll bool
var checkTimeout time.Duration
var checkWorkers int
var checkGroups []string
var checkLabels []string
var checkOnlyFailed bool

// NewCheckCmd 创建 check 命令
func NewCheckCmd() *cobra.Command {
	checkCmd := &cobra.Command{
		Use:     "check [node_id...]",
		Short:   i18n.T("node.check.short"),
		Long:    i18n.T("node.check.long"),
		Example: i18n.T("node.check.example"),
		Run: func(cmd *cobra.Command, args []string) {
			runCheck(args)
		},
	}

	checkCmd.Flags().BoolVarP(&checkAll, "all", "a", false, i18n.T("node.check.flag_all"))
	checkCmd.Flags().DurationVarP(&checkTimeout, "timeout", "t", 10*time.Second, i18n.T("node.check.flag_timeout"))
	checkCmd.Flags().IntVarP(&checkWorkers, "workers", "w", 5, i18n.T("node.check.flag_workers"))
	checkCmd.Flags().StringSliceVarP(&checkGroups, "groups", "g", nil, i18n.T("node.check.flag_groups"))
	checkCmd.Flags().StringSliceVarP(&checkLabels, "label", "l", nil, i18n.T("node.check.flag_label"))
	checkCmd.Flags().BoolVarP(&checkOnlyFailed, "failed", "f", false, i18n.T("node.check.flag_failed"))

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

	switch {
	case checkAll || len(checkGroups) > 0 || len(checkLabels) > 0 || checkOnlyFailed:
		nodes, err = store.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, i18n.T("node.check.err_list", err))
			os.Exit(1)
		}
	case len(nodeIDs) > 0:
		for _, id := range nodeIDs {
			node, err := store.Get(id)
			if err != nil {
				fmt.Fprintln(os.Stderr, i18n.T("node.check.warn_not_found", id))
				continue
			}
			nodes = append(nodes, node)
		}
	default:
		fmt.Fprintln(os.Stderr, i18n.T("node.check.err_no_target"))
		fmt.Fprintln(os.Stderr, i18n.T("node.check.err_help"))
		os.Exit(1)
	}

	nodes = filterCheckNodes(nodes, checkGroups, checkLabels, checkOnlyFailed)

	if len(nodes) == 0 {
		fmt.Println(i18n.T("node.check.no_nodes"))
		return
	}

	fmt.Print(i18n.T("node.check.checking",
		i18n.F(len(nodes)), i18n.F(checkTimeout), i18n.F(checkWorkers)))

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
			authMethod := i18n.T("node.check.auth_key")
			if r.method == "password" {
				authMethod = i18n.T("node.check.auth_password")
			}
			fmt.Print(i18n.T("node.check.online", r.node.ID, r.node.Address, i18n.F(r.node.Port), authMethod))
			online++
			r.node.Status = "online"
			r.node.LastCheckAt = currentTime
			r.node.UpdatedAt = currentTime
			if err := store.Update(r.node); err != nil {
				fmt.Print(i18n.T("node.check.online_update_fail", err))
			} else {
				fmt.Print(i18n.T("node.check.online_updated"))
			}
			fmt.Println()
		} else {
			fmt.Print(i18n.T("node.check.offline", r.node.ID, r.node.Address, i18n.F(r.node.Port)))
			offline++
			r.node.Status = "offline"
			r.node.LastCheckAt = currentTime
			r.node.UpdatedAt = currentTime
			if err := store.Update(r.node); err != nil {
				fmt.Print(i18n.T("node.check.offline_update_fail", err))
			} else {
				fmt.Print(i18n.T("node.check.offline_updated"))
			}
			if r.err != nil {
				fmt.Print(i18n.T("node.check.reason", r.err))
			}
		}
	}

	fmt.Print(i18n.T("node.check.summary", i18n.F(online), i18n.F(offline), i18n.F(len(nodes))))

	if err := store.Save(); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.check.err_save", err))
	} else {
		fmt.Println(i18n.T("node.check.ok_save"))
	}
}

func filterCheckNodes(nodes []*common.NodeInfo, groups, labels []string, onlyFailed bool) []*common.NodeInfo {
	filtered := make([]*common.NodeInfo, 0)

	for _, n := range nodes {
		if len(groups) > 0 && !containsAnyGroup(n.Groups, groups) {
			continue
		}

		if len(labels) > 0 {
			match := true
			for _, label := range labels {
				parts := strings.Split(label, "=")
				if len(parts) != 2 {
					continue
				}
				key, value := parts[0], parts[1]
				if v, ok := n.Labels[key]; !ok || v != value {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		if onlyFailed && strings.ToLower(n.Status) != "offline" {
			continue
		}

		filtered = append(filtered, n)
	}

	return filtered
}

func checkNodeSSH(n *common.NodeInfo) checkResult {
	addr := fmt.Sprintf("%s:%d", n.Address, n.Port)
	ctx := context.Background()

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
			client, err := owlssh.Dial(ctx, addr, owlssh.DialOptions{
				User:           sshUser,
				AuthMethods:    []gossh.AuthMethod{gossh.PublicKeys(signer)},
				ProxyJump:      n.ProxyJump,
				ConnectTimeout: checkTimeout,
			})
			if err == nil {
				client.Close()
				return checkResult{node: n, success: true, method: "key"}
			}

			return checkResult{
				node:    n,
				success: false,
				err:     fmt.Errorf(i18n.Raw("node.check.auth_key_failed"), err),
			}
		}

		// 密钥文件解析失败（文件不存在等情况），记录下来但继续尝试密码
		if n.Password == "" {
			return checkResult{
				node:    n,
				success: false,
				err:     fmt.Errorf(i18n.Raw("node.check.auth_key_invalid"), err),
			}
		}
	}

	// 密钥认证失败或无密钥，尝试密码认证
	if n.Password != "" {
		client, err := owlssh.Dial(ctx, addr, owlssh.DialOptions{
			User:           sshUser,
			AuthMethods:    []gossh.AuthMethod{gossh.Password(n.Password)},
			ProxyJump:      n.ProxyJump,
			ConnectTimeout: checkTimeout,
		})
		if err == nil {
			client.Close()
			return checkResult{node: n, success: true, method: "password"}
		}

		return checkResult{
			node:    n,
			success: false,
			err:     fmt.Errorf(i18n.Raw("node.check.auth_both_failed"), err),
		}
	}

	// 既没有密钥也没有密码
	return checkResult{
		node:    n,
		success: false,
		err:     errors.New(i18n.Raw("node.check.no_auth_configured")),
	}
}

func parsePrivateKey(keyPath string) (gossh.Signer, error) {
	expandedPath := keyPath
	if len(keyPath) > 2 && keyPath[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf(i18n.Raw("node.check.err_home_dir"), err)
		}
		expandedPath = filepath.Join(home, keyPath[2:])
	}

	keyData, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf(i18n.Raw("node.check.err_read_key"), err)
	}

	signer, err := gossh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf(i18n.Raw("node.check.err_parse_key"), err)
	}

	return signer, nil
}
