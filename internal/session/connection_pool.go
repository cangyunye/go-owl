package session

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHConnection SSH 连接
type SSHConnection struct {
	NodeID     string
	Address    string
	Port       int
	User       string
	client     *ssh.Client
	connected  bool
	lastActive time.Time
	mu         sync.RWMutex
}

// SSHConnectionPool SSH 连接池
type SSHConnectionPool struct {
	connections map[string]*SSHConnection
	config      *PoolConfig
	mu          sync.RWMutex
}

// PoolConfig 连接池配置
type PoolConfig struct {
	MaxConnections int
	ConnectTimeout time.Duration
	IdleTimeout    time.Duration
}

// DefaultPoolConfig 默认配置
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxConnections: 50,
		ConnectTimeout: 30 * time.Second,
		IdleTimeout:    30 * time.Minute,
	}
}

// NewSSHConnectionPool 创建连接池
func NewSSHConnectionPool(config *PoolConfig) *SSHConnectionPool {
	if config == nil {
		config = DefaultPoolConfig()
	}
	return &SSHConnectionPool{
		connections: make(map[string]*SSHConnection),
		config:      config,
	}
}

// Connect 建立连接
func (p *SSHConnectionPool) Connect(nodeID, address string, port int, user string, authMethods []ssh.AuthMethod) (*SSHConnection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 检查是否已存在连接
	if conn, ok := p.connections[nodeID]; ok && conn.IsConnected() {
		return conn, nil
	}

	// 创建新连接
	conn, err := p.createConnection(nodeID, address, port, user, authMethods)
	if err != nil {
		return nil, err
	}

	p.connections[nodeID] = conn
	return conn, nil
}

// createConnection 创建新连接
func (p *SSHConnectionPool) createConnection(nodeID, address string, port int, user string, authMethods []ssh.AuthMethod) (*SSHConnection, error) {
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         p.config.ConnectTimeout,
	}

	addr := fmt.Sprintf("%s:%d", address, port)

	ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectTimeout)
	defer cancel()

	// 使用 DialContext 支持超时
	var client *ssh.Client
	var err error

	done := make(chan struct{})
	go func() {
		client, err = ssh.Dial("tcp", addr, config)
		close(done)
	}()

	select {
	case <-done:
		if err != nil {
			return nil, fmt.Errorf("SSH 连接失败: %w", err)
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("SSH 连接超时")
	}

	conn := &SSHConnection{
		NodeID:     nodeID,
		Address:    address,
		Port:       port,
		User:       user,
		client:     client,
		connected:  true,
		lastActive: time.Now(),
	}

	return conn, nil
}

// GetConnection 获取连接
func (p *SSHConnectionPool) GetConnection(nodeID string) (*SSHConnection, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	conn, ok := p.connections[nodeID]
	return conn, ok
}

// CloseConnection 关闭指定连接
func (p *SSHConnectionPool) CloseConnection(nodeID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	conn, ok := p.connections[nodeID]
	if !ok {
		return nil
	}

	err := conn.Close()
	delete(p.connections, nodeID)
	return err
}

// CloseAll 关闭所有连接
func (p *SSHConnectionPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.connections {
		_ = conn.Close()
	}
	p.connections = make(map[string]*SSHConnection)
}

// IsConnected 检查连接状态
func (c *SSHConnection) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Execute 执行命令
func (c *SSHConnection) Execute(command string, timeout time.Duration) (int, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return -1, "", fmt.Errorf("连接已断开")
	}

	c.lastActive = time.Now()

	// 创建会话
	session, err := c.client.NewSession()
	if err != nil {
		c.connected = false
		return -1, "", fmt.Errorf("创建会话失败: %w", err)
	}
	defer session.Close()

	// 设置输出
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// 执行命令
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
			if exitErr, ok := err.(*ssh.ExitError); ok {
				return exitErr.ExitStatus(), output, nil
			}
			return -1, output, err
		}
		return 0, output, nil

	case <-ctx.Done():
		return -1, "", fmt.Errorf("命令执行超时")
	}
}

// Close 关闭连接
func (c *SSHConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		err := c.client.Close()
		c.connected = false
		return err
	}
	return nil
}

// SendHeartbeat 发送心跳检测
func (c *SSHConnection) SendHeartbeat() bool {
	_, _, err := c.Execute("echo 1", 5*time.Second)
	return err == nil
}

// GetLastActive 获取最后活动时间
func (c *SSHConnection) GetLastActive() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastActive
}

// ConnectionStats 连接统计
type ConnectionStats struct {
	TotalConnections  int
	ActiveConnections int
	NodeIDs           []string
}

// GetStats 获取连接池统计
func (p *SSHConnectionPool) GetStats() *ConnectionStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := &ConnectionStats{
		TotalConnections: len(p.connections),
		NodeIDs:          make([]string, 0, len(p.connections)),
	}

	for nodeID, conn := range p.connections {
		if conn.IsConnected() {
			stats.ActiveConnections++
		}
		stats.NodeIDs = append(stats.NodeIDs, nodeID)
	}

	return stats
}
