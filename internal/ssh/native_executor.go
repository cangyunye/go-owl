package ssh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// NativeNodeExecutor 基于 crypto/ssh 的原生 SSH 执行器
type NativeNodeExecutor struct {
	connInfo *ConnectionInfo
}

// NewNativeNodeExecutor 创建原生 SSH 执行器
func NewNativeNodeExecutor(connInfo *ConnectionInfo) *NativeNodeExecutor {
	return &NativeNodeExecutor{
		connInfo: connInfo,
	}
}

func (e *NativeNodeExecutor) Execute(command string, timeout time.Duration) (int, string, error) {
	return e.execute(command, timeout, timeout)
}

// WriteFile 通过 SSH 将本地文件写入远程路径（基于 crypto/ssh）
func (e *NativeNodeExecutor) WriteFile(localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("读取本地文件 %s 失败: %w", localPath, err)
	}

	addr := fmt.Sprintf("%s:%d", e.connInfo.Address, e.connInfo.Port)

	client, err := Dial(context.Background(), addr, DialOptions{
		User:           e.connInfo.GetUser(),
		Password:       e.connInfo.Password,
		KeyFile:        e.connInfo.KeyFile,
		KeyContent:     e.connInfo.KeyContent,
		ProxyJump:      e.connInfo.ProxyJump,
		ConnectTimeout: 30 * time.Second,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stdin = bytes.NewReader(data)
	session.Stderr = &stderr

	cmd := fmt.Sprintf("mkdir -p '%s' && cat > '%s'", filepath.Dir(remotePath), remotePath)
	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("文件传输失败: %w\n%s", err, stderr.String())
	}

	return nil
}

func (e *NativeNodeExecutor) ExecuteWithConfig(command string, config *TimeoutConfig) (int, string, error) {
	if config == nil {
		config = &TimeoutConfig{
			ConnectTimeout: 10 * time.Second,
			CommandTimeout: 30 * time.Second,
		}
	}
	totalTimeout := config.ConnectTimeout + config.CommandTimeout
	return e.execute(command, totalTimeout, config.CommandTimeout)
}

func (e *NativeNodeExecutor) execute(command string, dialTimeout, commandTimeout time.Duration) (int, string, error) {
	addr := fmt.Sprintf("%s:%d", e.connInfo.Address, e.connInfo.Port)

	client, err := Dial(context.Background(), addr, DialOptions{
		User:           e.connInfo.GetUser(),
		Password:       e.connInfo.Password,
		KeyFile:        e.connInfo.KeyFile,
		KeyContent:     e.connInfo.KeyContent,
		ProxyJump:      e.connInfo.ProxyJump,
		ConnectTimeout: dialTimeout,
	})
	if err != nil {
		return -1, "", err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return -1, "", fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case err := <-done:
		output := stdout.String()
		if stderr.Len() > 0 {
			output += "\n" + stderr.String()
		}
		if err != nil {
			if exitErr, ok := err.(*gossh.ExitError); ok {
				return exitErr.ExitStatus(), output, nil
			}
			return -1, output, err
		}
		return 0, output, nil
	case <-ctx.Done():
		session.Signal(gossh.SIGTERM)
		return -1, "", fmt.Errorf("命令执行超时")
	}
}

func expandPath(path string) string {
	if len(path) > 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	if len(path) > 0 && path[0] == '~' {
		u, err := user.Current()
		if err == nil {
			return filepath.Join(u.HomeDir, path[1:])
		}
	}
	return path
}

func containsAnySSH(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(substr) <= len(s) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i] == substr[0] || s[i] == substr[0]-32 || s[i] == substr[0]+32 {
					match := true
					for j := 0; j < len(substr); j++ {
						sc := s[i+j]
						tc := substr[j]
						if sc != tc && sc != tc-32 && sc != tc+32 {
							match = false
							break
						}
					}
					if match {
						return true
					}
				}
			}
		}
	}
	return false
}

// ConnectionError SSH 连接错误
type ConnectionError struct {
	NodeID    string
	ErrorType ErrorType
	Stderr    string
	Cause     error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("SSH 连接失败 on node %s: %s", e.NodeID, stringsTrimSpace(e.Stderr))
}

func (e *ConnectionError) Unwrap() error {
	return e.Cause
}

// ErrorType 连接错误类型
type ErrorType int

const (
	ErrorTypeUnknown ErrorType = iota
	ErrorTypeConnection
	ErrorTypeAuth
)

func stringsTrimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	if start >= end {
		return ""
	}
	return s[start:end]
}
