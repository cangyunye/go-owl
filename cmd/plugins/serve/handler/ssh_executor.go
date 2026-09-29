package handler

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/secrets"
	owlssh "github.com/cangyunye/go-owl/internal/ssh"
	gossh "golang.org/x/crypto/ssh"
)

const sshConnectTimeout = 10 * time.Second

// maxOutputLineBytes 单行输出上限。bufio.Scanner 默认 64KB，压缩成一行的
// JSON/base64/无换行大段输出会触发 ErrTooLong 并静默丢弃该行及其后全部输出。
const maxOutputLineBytes = 4 * 1024 * 1024

// readLines 逐行读取输出并交给 emit；emit 返回 false 时立即停止（用于 ctx 取消）。
// 返回扫描错误（如超过 maxOutputLineBytes 的行），由调用方决定如何呈现——
// 绝不能静默截断，那会表现为"打印到一半就没有后面的内容了"。
func readLines(r io.Reader, emit func(line string) bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxOutputLineBytes)
	for scanner.Scan() {
		if !emit(scanner.Text()) {
			return nil
		}
	}
	return scanner.Err()
}

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

// Execute 执行远端命令并等待其主进程退出。
//
// 关键点：gossh 的 session.Wait/CombinedOutput 都等到 SSH 通道关闭才返回，
// 而 start.sh 这类启动后台守护进程的脚本会继承 stdout/stderr 管道且不退出
// ——EOF 永远不来，步骤永久 running（用户实测卡死 8 小时）。因此用
// 「哨兵标记」：把命令包一层子 shell，主命令退出后 echo 一个随机标记，
// 客户端读到标记即返回并按标记携带的退出码收尾，绝不傻等 EOF。
// 标记未出现时（连接中断、被超时杀等）由 Wait/ctx watchdog 兜底返回。
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

	// Wait/pipe 读取均不响应 ctx，ctx 取消/超时时必须主动断开连接，
	// 否则命令超时设置无法真正终止远端命令，交互式命令会把任务永久挂住。
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
		return "", -1, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return "", -1, fmt.Errorf("stderr pipe: %w", err)
	}

	// 随机 nonce 防止命令输出撞上标记串
	nonce := strconv.FormatInt(time.Now().UnixNano(), 36) + strconv.Itoa(rand.IntN(1e9))
	sentinel := "__OWLDONE_" + nonce + "_"
	// ${__owl_rc} 必须显式定界：$__owl_rc__ 会被 shell 当作变量 __owl_rc__（未定义，输出空）
	wrapped := fmt.Sprintf("( %s ); __owl_rc=$?; echo %s${__owl_rc}__; exit $__owl_rc", command, sentinel)
	sentinelPrefix := []byte(sentinel)
	sentinelTail := []byte("__")

	var bufMu sync.Mutex
	var out bytes.Buffer
	// sentinelDone 携带主命令退出码；仅在 stdout 流中检出标记时发送
	sentinelDone := make(chan int, 1)
	collect := func(r io.Reader, detect bool) <-chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			chunk := make([]byte, 32*1024)
			var tail []byte // 保留跨 chunk 的标记前缀
			for {
				n, rerr := r.Read(chunk)
				if n > 0 {
					bufMu.Lock()
					out.Write(chunk[:n])
					bufMu.Unlock()
					if detect {
						tail = append(tail, chunk[:n]...)
						if idx := bytes.Index(tail, sentinelPrefix); idx >= 0 {
							rest := tail[idx+len(sentinelPrefix):]
							if end := bytes.Index(rest, sentinelTail); end >= 0 {
								if rc, perr := strconv.Atoi(string(rest[:end])); perr == nil {
									sentinelDone <- rc
									return
								}
							}
						}
						// 只保留可能容纳半截标记的尾部，防无限增长
						if len(tail) > len(sentinel)+16 {
							tail = tail[len(tail)-len(sentinel)-16:]
						}
					}
				}
				if rerr != nil {
					return
				}
			}
		}()
		return done
	}
	stdoutDone := collect(stdout, true)
	stderrDone := collect(stderr, false)

	if err := session.Start(wrapped); err != nil {
		return "", -1, fmt.Errorf("ssh start: %w", err)
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- session.Wait() }()

	select {
	case rc := <-sentinelDone:
		// 主命令已退出：立即收尾，不等守护进程关闭通道。
		// 输出截掉标记及其后内容。
		bufMu.Lock()
		full := out.String()
		bufMu.Unlock()
		if idx := strings.LastIndex(full, sentinel); idx >= 0 {
			full = full[:idx]
		}
		_ = session.Close()
		return full, rc, nil
	case waitErr := <-waitDone:
		// 通道关闭且未见标记（老服务端/异常路径）：EOF 已到，沿用原语义
		<-stdoutDone
		<-stderrDone
		bufMu.Lock()
		output := out.String()
		bufMu.Unlock()
		exitCode := 0
		if waitErr != nil {
			if exitErr, ok := waitErr.(*gossh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else if ctx.Err() != nil {
				return output, -1, fmt.Errorf("ssh exec: %w（ctx: %v）", waitErr, ctx.Err())
			} else {
				return output, -1, fmt.Errorf("ssh exec: %w", waitErr)
			}
		}
		return output, exitCode, nil
	case <-ctx.Done():
		return "", -1, fmt.Errorf("ssh exec: %w", ctx.Err())
	}
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
	// streamReader 逐行上报一路输出；扫描异常（如超长行）作为 stderr 行上报，
	// 不再静默丢尾。
	streamReader := func(stream io.Reader, lineType string) {
		defer func() { done <- struct{}{} }()
		err := readLines(stream, func(line string) bool {
			select {
			case outputCh <- OutputLine{NodeID: nodeID, Line: line, Type: lineType}:
				return true
			case <-ctx.Done():
				return false
			}
		})
		if err != nil {
			select {
			case outputCh <- OutputLine{NodeID: nodeID, Line: fmt.Sprintf("[%s 采集中断: %v]", lineType, err), Type: "stderr"}:
			case <-ctx.Done():
			}
		}
	}
	go streamReader(stdout, "stdout")
	go streamReader(stderr, "stderr")

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
