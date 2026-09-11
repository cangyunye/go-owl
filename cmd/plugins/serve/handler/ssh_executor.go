package handler

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/cangyunye/go-owl/internal/secrets"
	owlssh "github.com/cangyunye/go-owl/internal/ssh"
	gossh "golang.org/x/crypto/ssh"
)

const sshConnectTimeout = 10 * time.Second

type sshExecutor struct {
	db             *sql.DB
	connectTimeout time.Duration // 请求级连接超时；0 表示使用 sshConnectTimeout
}

type nodeSSHInfo struct {
	Address   string
	Port      int
	User      string
	Password  string
	SSHKey    string
	ProxyJump string
}

func (e *sshExecutor) getNodeInfo(nodeID string) (*nodeSSHInfo, error) {
	var info nodeSSHInfo
	var pw, key, jump sql.NullString
	err := e.db.QueryRow(
		`SELECT COALESCE(address, ''), port, user, password, ssh_key, COALESCE(proxy_jump, '') FROM nodes WHERE id = ?`, nodeID,
	).Scan(&info.Address, &info.Port, &info.User, &pw, &key, &jump)
	if err != nil {
		return nil, err
	}
	if pw.Valid {
		info.Password = pw.String
	}
	if key.Valid {
		info.SSHKey = key.String
	}
	info.ProxyJump = jump.String
	if err := decryptNodeSSHInfo(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

// decryptNodeSSHInfo 就地解密节点凭据(存储层经 internal/secrets 加密,
// 无前缀的存量明文原样通过)。所有从 nodes 表读凭据的路径都必须调用。
func decryptNodeSSHInfo(info *nodeSSHInfo) error {
	pw, err := secrets.Decrypt(info.Password)
	if err != nil {
		return fmt.Errorf("node %s: %w", info.Address, err)
	}
	key, err := secrets.Decrypt(info.SSHKey)
	if err != nil {
		return fmt.Errorf("node %s: %w", info.Address, err)
	}
	info.Password = pw
	info.SSHKey = key
	return nil
}

func (e *sshExecutor) dialNode(ctx context.Context, nodeID string) (*owlssh.Client, error) {
	info, err := e.getNodeInfo(nodeID)
	if err != nil {
		return nil, fmt.Errorf("resolve node: %w", err)
	}
	addr := net.JoinHostPort(info.Address, strconv.Itoa(info.Port))
	timeout := e.connectTimeout
	if timeout <= 0 {
		timeout = sshConnectTimeout
	}
	return owlssh.Dial(ctx, addr, owlssh.DialOptions{
		User:           info.User,
		Password:       info.Password,
		KeyContent:     info.SSHKey,
		ProxyJump:      info.ProxyJump,
		ConnectTimeout: timeout,
	})
}

func (e *sshExecutor) Execute(ctx context.Context, nodeID, command string) (string, int, error) {
	client, err := e.dialNode(ctx, nodeID)
	if err != nil {
		return "", -1, err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", -1, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*gossh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return "", -1, fmt.Errorf("ssh exec: %w", err)
		}
	}
	return string(output), exitCode, nil
}

func (e *sshExecutor) ExecuteStream(ctx context.Context, nodeID, command string, outputCh chan<- OutputLine) (int, error) {
	client, err := e.dialNode(ctx, nodeID)
	if err != nil {
		return -1, err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return -1, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	// session.Wait() 不响应 ctx；ctx 取消/超时时必须主动断开连接，
	// 否则命令超时设置无法真正终止远端命令，任务会一直挂住。
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
			_ = client.Close()
		case <-watchDone:
		}
	}()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return -1, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return -1, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := session.Start(command); err != nil {
		return -1, fmt.Errorf("ssh start: %w", err)
	}

	done := make(chan struct{}, 2)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case outputCh <- OutputLine{NodeID: nodeID, Line: scanner.Text(), Type: "stdout"}:
			case <-ctx.Done():
				done <- struct{}{}
				return
			}
		}
		done <- struct{}{}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			select {
			case outputCh <- OutputLine{NodeID: nodeID, Line: scanner.Text(), Type: "stderr"}:
			case <-ctx.Done():
				done <- struct{}{}
				return
			}
		}
		done <- struct{}{}
	}()

	err = session.Wait()
	<-done
	<-done

	exitCode := 0
	if exitErr, ok := err.(*gossh.ExitError); ok {
		exitCode = exitErr.ExitStatus()
		err = nil
	}
	return exitCode, err
}
