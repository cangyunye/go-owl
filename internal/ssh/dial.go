package ssh

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// DialOptions SSH 拨号选项
type DialOptions struct {
	User           string
	Password       string
	KeyFile        string
	KeyContent     string             // 内联 PEM 私钥
	ProxyJump      string             // 跳板机 "host" 或 "host:port"
	ConnectTimeout time.Duration      // 连接超时，<=0 时默认 10s
	AuthMethods    []gossh.AuthMethod // 非空时直接使用，跳过内建认证链；跳板连接同样使用
}

// Client 包装 gossh.Client；Close 会连带关闭经由 ProxyJump 建立的跳板连接。
type Client struct {
	*gossh.Client
	jump *gossh.Client
}

// Close 关闭目标连接，并关闭跳板连接（如有）
func (c *Client) Close() error {
	err := c.Client.Close()
	if c.jump != nil {
		if jerr := c.jump.Close(); err == nil {
			err = jerr
		}
	}
	return err
}

// Dial 建立 SSH 连接。认证链：密钥文件/内联密钥优先，密码兜底，
// 两者皆无时尝试默认密钥（~/.ssh/id_ed25519 等）。
// ProxyJump 非空时先连跳板机，再经跳板 direct-tcpip 转发到目标。
// 返回的错误为 *SSHAuthError 或 *ConnectionError。
func Dial(ctx context.Context, addr string, opts DialOptions) (*Client, error) {
	timeout := opts.ConnectTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	auths := opts.AuthMethods
	if len(auths) == 0 {
		auths = buildDialAuth(opts)
	}
	if len(auths) == 0 {
		return nil, &SSHAuthError{
			ExitCode: -1,
			NodeID:   addr,
			Stderr:   "没有可用的认证方式：请配置 SSH 密钥或密码",
			Cause:    fmt.Errorf("no authentication methods available"),
		}
	}

	config := &gossh.ClientConfig{
		User:            opts.User,
		Auth:            auths,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}

	if opts.ProxyJump != "" {
		jumpAddr := opts.ProxyJump
		if _, _, err := net.SplitHostPort(jumpAddr); err != nil {
			jumpAddr = net.JoinHostPort(jumpAddr, "22")
		}
		jump, err := Dial(ctx, jumpAddr, DialOptions{
			User:           opts.User,
			Password:       opts.Password,
			KeyFile:        opts.KeyFile,
			KeyContent:     opts.KeyContent,
			AuthMethods:    opts.AuthMethods,
			ConnectTimeout: opts.ConnectTimeout,
		})
		if err != nil {
			return nil, fmt.Errorf("跳板机 %s 连接失败: %w", jumpAddr, err)
		}
		forwarded, err := jump.Dial("tcp", addr)
		if err != nil {
			jump.Close()
			return nil, connErr(addr, fmt.Errorf("经跳板转发到 %s 失败: %w", addr, err))
		}
		target, err := newSSHClient(ctx, forwarded, addr, config, timeout)
		if err != nil {
			forwarded.Close()
			jump.Close()
			return nil, err
		}
		return &Client{Client: target, jump: jump.Client}, nil
	}

	netConn, err := dialTCP(ctx, addr, timeout)
	if err != nil {
		return nil, connErr(addr, err)
	}
	client, err := newSSHClient(ctx, netConn, addr, config, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{Client: client}, nil
}

// newSSHClient 在已建立的传输连接上执行 SSH 握手并装配客户端。
// NewClientConn 本身不受 gossh.ClientConfig.Timeout 约束（该字段只作用于
// gossh.Dial 的 TCP 拨号），因此在握手前对底层连接设置 deadline，将握手
// 限制在 timeout 与 ctx deadline 的更早者之内；握手结束后清除 deadline，
// 避免后续命令执行被误杀。
func newSSHClient(ctx context.Context, netConn net.Conn, addr string, config *gossh.ClientConfig, timeout time.Duration) (*gossh.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, connErr(addr, err)
	}

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	// 优先用连接 deadline 限制握手；部分连接（如跳板 direct-tcpip 通道）不支持
	// deadline，退化为到期后关闭连接来中止握手。
	hasDeadline := netConn.SetDeadline(deadline) == nil
	abort := func() {
		if hasDeadline {
			_ = netConn.SetDeadline(time.Unix(1, 0))
		} else {
			_ = netConn.Close()
		}
	}

	// 监听超时与 ctx 取消：一旦触发即中止阻塞中的握手读写。
	var mu sync.Mutex
	done := false
	watchDone := make(chan struct{})
	go func() {
		var timerCh <-chan time.Time
		if !hasDeadline {
			timer := time.NewTimer(time.Until(deadline))
			defer timer.Stop()
			timerCh = timer.C
		}
		select {
		case <-timerCh:
		case <-ctx.Done():
		case <-watchDone:
			return
		}
		mu.Lock()
		if !done {
			abort()
		}
		mu.Unlock()
	}()

	conn, chans, reqs, err := gossh.NewClientConn(netConn, addr, config)

	mu.Lock()
	done = true
	if hasDeadline {
		_ = netConn.SetDeadline(time.Time{}) // 成功路径必须清除，否则后续命令执行会被 deadline 误杀
	}
	mu.Unlock()
	close(watchDone)

	if err != nil {
		return nil, connErr(addr, err)
	}
	return gossh.NewClient(conn, chans, reqs), nil
}

func dialTCP(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, "tcp", addr)
}

// connErr 将底层错误分类为 *ConnectionError（认证类 / 连接类）
func connErr(addr string, cause error) *ConnectionError {
	errType := ErrorTypeConnection
	msg := cause.Error()
	if containsAnySSH(msg, "auth", "password", "key", "permission", "authentication",
		"no supported methods", "unable to authenticate") {
		errType = ErrorTypeAuth
	}
	if containsAnySSH(msg, "timeout", "timed out", "refused") {
		errType = ErrorTypeConnection
	}
	return &ConnectionError{NodeID: addr, ErrorType: errType, Stderr: msg, Cause: cause}
}

// buildDialAuth 构建认证方法列表：密钥文件 > 内联密钥 > 密码（含 keyboard-interactive）> 默认密钥
func buildDialAuth(opts DialOptions) []gossh.AuthMethod {
	var auths []gossh.AuthMethod

	if opts.KeyFile != "" {
		if signers, err := loadKeyFile(opts.KeyFile); err == nil && len(signers) > 0 {
			auths = append(auths, gossh.PublicKeys(signers...))
		}
	}

	if opts.KeyContent != "" {
		if signer, err := gossh.ParsePrivateKey([]byte(opts.KeyContent)); err == nil {
			auths = append(auths, gossh.PublicKeys(signer))
		}
	}

	if opts.Password != "" {
		auths = append(auths, gossh.Password(opts.Password))
		auths = append(auths, gossh.KeyboardInteractive(
			func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = opts.Password
				}
				return answers, nil
			}))
	}

	if len(auths) == 0 {
		if signers := tryDefaultKeys(); len(signers) > 0 {
			auths = append(auths, gossh.PublicKeys(signers...))
		}
	}

	return auths
}

// loadKeyFile 读取并解析 PEM 私钥文件（支持 ~ 展开）
func loadKeyFile(keyPath string) ([]gossh.Signer, error) {
	expandedPath := expandPath(keyPath)
	keyData, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("读取密钥文件 %s 失败: %w", expandedPath, err)
	}

	signer, err := gossh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("解析密钥 %s 失败: %w", expandedPath, err)
	}

	return []gossh.Signer{signer}, nil
}

// tryDefaultKeys 尝试加载 ~/.ssh 下的默认密钥
func tryDefaultKeys() []gossh.Signer {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	defaultKeys := []string{
		filepath.Join(homeDir, ".ssh", "id_ed25519"),
		filepath.Join(homeDir, ".ssh", "id_rsa"),
		filepath.Join(homeDir, ".ssh", "id_ecdsa"),
		filepath.Join(homeDir, ".ssh", "id_dsa"),
	}

	var signers []gossh.Signer
	for _, keyPath := range defaultKeys {
		signer, err := loadKeyFile(keyPath)
		if err == nil {
			signers = append(signers, signer...)
		}
	}
	return signers
}
