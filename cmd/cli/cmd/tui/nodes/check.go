package nodes

import (
	"context"
	"errors"
	"net"
	"os"
	"os/user"
	"strconv"
	"time"

	gossh "golang.org/x/crypto/ssh"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	owlssh "github.com/cangyunye/go-owl/internal/ssh"
)

const checkTimeout = 10 * time.Second

type CheckResult struct {
	Node    *common.NodeInfo
	Success bool
	Method  string
	Err     error
}

type CheckDoneMsg struct {
	Results []CheckResult
}

// sshCheck 可注入的 SSH 认证检查(测试替换);默认实现走 owlssh.Dial,密钥优先密码兜底
var sshCheck = func(n *common.NodeInfo, timeout time.Duration) (bool, string, error) {
	addr := net.JoinHostPort(n.Address, strconv.Itoa(n.Port))
	ctx := context.Background()
	sshUser := n.User
	if sshUser == "" {
		if current, err := user.Current(); err == nil {
			sshUser = current.Username
		} else {
			sshUser = "root"
		}
	}
	if n.SSHKey != "" {
		signer, err := parsePrivateKey(n.SSHKey)
		if err == nil {
			client, err := owlssh.Dial(ctx, addr, owlssh.DialOptions{
				User:           sshUser,
				AuthMethods:    []gossh.AuthMethod{gossh.PublicKeys(signer)},
				ProxyJump:      n.ProxyJump,
				ConnectTimeout: timeout,
			})
			if err == nil {
				client.Close()
				return true, "key", nil
			}
			if n.Password == "" {
				return false, "", err
			}
		}
	}
	if n.Password != "" {
		client, err := owlssh.Dial(ctx, addr, owlssh.DialOptions{
			User:           sshUser,
			AuthMethods:    []gossh.AuthMethod{gossh.Password(n.Password)},
			ProxyJump:      n.ProxyJump,
			ConnectTimeout: timeout,
		})
		if err == nil {
			client.Close()
			return true, "password", nil
		}
		return false, "", err
	}
	return false, "", errors.New("未配置认证方式(SSHKey 或 Password)")
}

func parsePrivateKey(keyPath string) (gossh.Signer, error) {
	if len(keyPath) > 2 && keyPath[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			keyPath = home + keyPath[1:]
		}
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	return gossh.ParsePrivateKey(data)
}

func runCheck(nodes []*common.NodeInfo, timeout time.Duration, fn func(*common.NodeInfo, time.Duration) (bool, string, error)) []CheckResult {
	results := make([]CheckResult, 0, len(nodes))
	for _, n := range nodes {
		ok, method, err := fn(n, timeout)
		results = append(results, CheckResult{Node: n, Success: ok, Method: method, Err: err})
	}
	return results
}

type CheckModel struct {
	nodes   []*common.NodeInfo
	results []CheckResult
	loading bool
}

func NewCheckModel(nodes []*common.NodeInfo) *CheckModel {
	return &CheckModel{nodes: nodes, loading: true}
}

func (m *CheckModel) Start() tea.Cmd {
	return func() tea.Msg {
		results := runCheck(m.nodes, checkTimeout, sshCheck)
		return CheckDoneMsg{Results: results}
	}
}
