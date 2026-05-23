package session

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/history"
	gossh "golang.org/x/crypto/ssh"
)

// SessionMode 会话模式
type SessionMode string

const (
	SessionModeSingle   SessionMode = "single"
	SessionModeMultiple SessionMode = "multiple"
)

// SessionStatus 会话状态
type SessionStatus string

const (
	SessionStatusActive  SessionStatus = "active"
	SessionStatusClosing SessionStatus = "closing"
	SessionStatusClosed  SessionStatus = "closed"
	SessionStatusTimeout SessionStatus = "timeout"
)

// Session 会话
type Session struct {
	ID           string
	Mode         SessionMode
	Nodes        []string
	Status       SessionStatus
	CreatedAt    time.Time
	ClosedAt     *time.Time
	CommandCount int
	SuccessCount int
	ErrorCount   int
	Timeout      time.Duration
	mu           sync.RWMutex
	pool         *SSHConnectionPool
	history      *CommandHistory
	lastActivity time.Time
	done         chan struct{}
}

// NodeConfig 节点配置
type NodeConfig struct {
	ID        string
	Address   string
	Port      int
	User      string
	Password  string
	SSHKey    string
	ProxyJump string
	Auth      []gossh.AuthMethod
}

// NewSession 创建新会话
func NewSession(mode SessionMode, nodes []string, timeout time.Duration) *Session {
	sessionID := generateSessionID()
	now := time.Now()

	sess := &Session{
		ID:           sessionID,
		Mode:         mode,
		Nodes:        nodes,
		Status:       SessionStatusActive,
		CreatedAt:    now,
		Timeout:      timeout,
		pool:         NewSSHConnectionPool(nil),
		history:      NewCommandHistory(100),
		lastActivity: now,
		done:         make(chan struct{}),
	}

	history.RecordSession(&history.Session{
		ID:           sess.ID,
		Mode:         string(sess.Mode),
		NodeIDs:      sess.Nodes,
		Status:       string(sess.Status),
		CreatedAt:    sess.CreatedAt,
		CommandCount: 0,
		SuccessCount: 0,
		ErrorCount:   0,
	})

	return sess
}

// Connect 连接到节点
func (s *Session) Connect(nodeConfigs []*NodeConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status != SessionStatusActive {
		return fmt.Errorf("会话已关闭")
	}

	successCount := 0
	for _, config := range nodeConfigs {
		_, err := s.pool.Connect(config.ID, config.Address, config.Port, config.User, config.Auth)
		if err != nil {
			fmt.Printf("连接节点 %s 失败: %v\n", config.ID, err)
			continue
		}
		successCount++
		fmt.Printf("✓ 已连接到 %s\n", config.ID)
	}

	if successCount == 0 {
		return fmt.Errorf("无法连接到任何节点")
	}

	return nil
}

// ExecuteCommand 执行命令（并发执行）
func (s *Session) ExecuteCommand(commandStr string, timeout time.Duration) []command.CommandResult {
	s.mu.Lock()
	s.lastActivity = time.Now()
	s.mu.Unlock()

	s.history.Add(commandStr)
	s.CommandCount++

	stats := s.pool.GetStats()
	nodeCount := len(stats.NodeIDs)

	// 使用channel收集结果，容量为节点数量
	resultChan := make(chan command.CommandResult, nodeCount)
	var wg sync.WaitGroup

	// 并发执行命令
	for _, nodeID := range stats.NodeIDs {
		wg.Add(1)
		go func(nid string) {
			defer wg.Done()

			conn, ok := s.pool.GetConnection(nid)
			if !ok {
				resultChan <- command.CommandResult{
					NodeID: nid,
					Error:  fmt.Errorf("连接不存在"),
				}
				return
			}

			start := time.Now()
			exitCode, output, err := conn.Execute(commandStr, timeout)
			duration := time.Since(start)

			resultChan <- command.CommandResult{
				NodeID:   nid,
				ExitCode: exitCode,
				Output:   output,
				Error:    err,
				Duration: duration,
			}
		}(nodeID)
	}

	// 等待所有goroutine完成后关闭channel
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	var results []command.CommandResult
	for result := range resultChan {
		results = append(results, result)

		// 更新统计
		if result.Error == nil && result.ExitCode == 0 {
			s.SuccessCount++
		} else {
			s.ErrorCount++
		}

		// 记录到数据库
		errMsg := ""
		if result.Error != nil {
			errMsg = result.Error.Error()
		}
		history.RecordSessionCommand(&history.SessionCommand{
			SessionID:  s.ID,
			Command:    commandStr,
			NodeID:     result.NodeID,
			ExitCode:   result.ExitCode,
			Output:     result.Output,
			Error:      errMsg,
			Duration:   result.Duration.Nanoseconds() / 1000000,
			ExecutedAt: time.Now(),
		})
	}

	return results
}

// CheckTimeout 检查超时
func (s *Session) CheckTimeout() error {
	s.mu.RLock()
	elapsed := time.Since(s.lastActivity)
	s.mu.RUnlock()

	if elapsed < s.Timeout {
		return nil
	}

	// 发送心跳检测
	stats := s.pool.GetStats()
	allFailed := true

	for _, nodeID := range stats.NodeIDs {
		conn, ok := s.pool.GetConnection(nodeID)
		if ok && conn.SendHeartbeat() {
			allFailed = false
		}
	}

	if allFailed {
		return fmt.Errorf("所有连接心跳检测失败")
	}

	// 重置活动时间
	s.mu.Lock()
	s.lastActivity = time.Now()
	s.mu.Unlock()

	return nil
}

// Close 关闭会话
func (s *Session) Close() {
	s.mu.Lock()
	if s.Status != SessionStatusActive {
		s.mu.Unlock()
		return
	}
	s.Status = SessionStatusClosing
	s.mu.Unlock()

	close(s.done)

	s.pool.CloseAll()

	now := time.Now()
	s.ClosedAt = &now
	s.Status = SessionStatusClosed

	history.UpdateSession(&history.Session{
		ID:           s.ID,
		Mode:         string(s.Mode),
		NodeIDs:      s.Nodes,
		Status:       string(s.Status),
		CreatedAt:    s.CreatedAt,
		ClosedAt:     s.ClosedAt,
		CommandCount: s.CommandCount,
		SuccessCount: s.SuccessCount,
		ErrorCount:   s.ErrorCount,
	})
}

// WaitForSignal 等待退出信号
func (s *Session) WaitForSignal() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan

	fmt.Println("\n正在关闭会话...")

	s.Close()
	s.PrintSummary()
}

// PrintSummary 打印会话摘要
func (s *Session) PrintSummary() {
	duration := time.Since(s.CreatedAt)
	if s.ClosedAt != nil {
		duration = s.ClosedAt.Sub(s.CreatedAt)
	}

	fmt.Println("─────────────────────────────────────")
	fmt.Println("会话摘要:")
	fmt.Printf("  会话时长: %s\n", formatDuration(duration))
	fmt.Printf("  执行命令: %d\n", s.CommandCount)
	if s.CommandCount > 0 {
		rate := float64(s.SuccessCount) / float64(s.CommandCount) * 100
		fmt.Printf("  成功率:  %.1f%% (%d/%d)\n", rate, s.SuccessCount, s.CommandCount)
	}
	fmt.Println("─────────────────────────────────────")
	fmt.Println("✓ 会话已关闭")
}

// GetHistory 获取命令历史
func (s *Session) GetHistory() []string {
	return s.history.GetAll()
}

// GetConnectionStats 获取连接统计
func (s *Session) GetConnectionStats() *ConnectionStats {
	return s.pool.GetStats()
}

func generateSessionID() string {
	return fmt.Sprintf("sess-%s-%03d",
		time.Now().Format("20060102-150405"),
		time.Now().UnixNano()%1000,
	)
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}