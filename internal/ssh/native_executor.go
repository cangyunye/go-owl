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

	config := &gossh.ClientConfig{
		User:            e.connInfo.GetUser(),
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         dialTimeout,
	}

	auths := e.buildAuthMethods()
	if len(auths) == 0 {
		return -1, "", &SSHAuthError{
			ExitCode: -1,
			NodeID:   e.connInfo.Address,
			Stderr:   "没有可用的认证方式：请配置 SSH 密钥或密码",
			Cause:    fmt.Errorf("no authentication methods available"),
		}
	}
	config.Auth = auths

	client, err := gossh.Dial("tcp", addr, config)
	if err != nil {
		errMsg := err.Error()

		stderrStr := errMsg
		errType := ErrorTypeConnection
		if containsAnySSH(errMsg, "auth", "password", "key", "permission", "authentication") {
			errType = ErrorTypeAuth
		}
		if containsAnySSH(errMsg, "timeout", "timed out", "refused") {
			errType = ErrorTypeConnection
		}

		return -1, "", &ConnectionError{
			NodeID:    e.connInfo.Address,
			ErrorType: errType,
			Stderr:    stderrStr,
			Cause:     err,
		}
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

// buildAuthMethods 构建认证方法列表，密钥优先，密码兜底
func (e *NativeNodeExecutor) buildAuthMethods() []gossh.AuthMethod {
	var auths []gossh.AuthMethod

	// 1. 尝试节点配置的密钥
	if e.connInfo.KeyFile != "" {
		if signers, err := e.loadKeyFile(e.connInfo.KeyFile); err == nil && len(signers) > 0 {
			auths = append(auths, gossh.PublicKeys(signers...))
		}
	}

	// 2. 尝试密码
	if e.connInfo.Password != "" {
		auths = append(auths, gossh.Password(e.connInfo.Password))
		auths = append(auths, gossh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = e.connInfo.Password
			}
			return answers, nil
		}))
	}

	// 3. 如果两者都没有，尝试默认密钥
	if len(auths) == 0 {
		signers := e.tryDefaultKeys()
		if len(signers) > 0 {
			auths = append(auths, gossh.PublicKeys(signers...))
		}
	}

	return auths
}

func (e *NativeNodeExecutor) loadKeyFile(keyPath string) ([]gossh.Signer, error) {
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

func (e *NativeNodeExecutor) tryDefaultKeys() []gossh.Signer {
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
		signer, err := e.loadKeyFile(keyPath)
		if err == nil {
			signers = append(signers, signer...)
		}
	}
	return signers
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
	ErrorTypeUnknown    ErrorType = iota
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
