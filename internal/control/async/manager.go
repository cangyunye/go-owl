package async

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AsyncTaskManager 异步任务管理器
type AsyncTaskManager struct {
	mu            sync.RWMutex
	tasks         map[string]*AsyncTask
	remoteBaseDir string
	maxConcurrent int
	cleanupAfter  time.Duration
}

// NewAsyncTaskManager 创建异步任务管理器
func NewAsyncTaskManager(opts *AsyncOptions) *AsyncTaskManager {
	if opts == nil {
		opts = &AsyncOptions{}
	}

	if opts.RemoteBaseDir == "" {
		opts.RemoteBaseDir = "/tmp/owl"
	}

	return &AsyncTaskManager{
		tasks:          make(map[string]*AsyncTask),
		remoteBaseDir:  opts.RemoteBaseDir,
		maxConcurrent:  100,
		cleanupAfter:   24 * time.Hour,
	}
}

// StartAsync 启动异步任务
func (m *AsyncTaskManager) StartAsync(ctx context.Context, nodeID, command string, opts *AsyncOptions) (*AsyncTask, error) {
	if opts == nil {
		opts = &AsyncOptions{}
	}

	task := &AsyncTask{
		ID:            uuid.New().String(),
		NodeID:        nodeID,
		Command:       command,
		StartTime:     time.Now(),
		Status:        AsyncTaskStatusPending,
		AsyncTimeout:  opts.Timeout,
		RemoteBaseDir: opts.RemoteBaseDir,
	}

	m.mu.Lock()
	if len(m.tasks) >= m.maxConcurrent {
		m.mu.Unlock()
		return nil, fmt.Errorf("max concurrent async tasks exceeded: %d", m.maxConcurrent)
	}
	m.tasks[task.ID] = task
	m.mu.Unlock()

	go m.runTask(ctx, task, opts)

	return task, nil
}

func (m *AsyncTaskManager) runTask(ctx context.Context, task *AsyncTask, opts *AsyncOptions) {
	task.Status = AsyncTaskStatusRunning

	pid, outputFile, err := m.startBackgroundTask(task.NodeID, task.Command, task.RemoteBaseDir)
	if err != nil {
		task.Status = AsyncTaskStatusFailed
		task.Error = err
		task.EndTime = time.Now()
		return
	}

	task.Pid = pid
	task.OutputFile = outputFile

	if opts.PollInterval == 0 {
		return
	}

	completed, err := m.PollTaskStatus(ctx, task, opts.PollInterval, opts.MaxPollCount)
	if err != nil {
		task.Error = err
	}
	if completed != nil {
		*task = *completed
	}
}

// startBackgroundTask 启动后台任务
func (m *AsyncTaskManager) startBackgroundTask(nodeID, command, remoteBaseDir string) (int, string, error) {
	if remoteBaseDir == "" {
		remoteBaseDir = "/tmp/owl"
	}

	cmd := fmt.Sprintf(`mkdir -p %s && chmod 1777 %s && cd %s && unset TMOUT && setsid %s > %s/output.log 2>&1 & echo $!`,
		remoteBaseDir, remoteBaseDir, remoteBaseDir, command, remoteBaseDir)

	output, err := executeLocalCommand(cmd)
	if err != nil {
		return 0, "", fmt.Errorf("failed to start background task: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 0, "", fmt.Errorf("failed to parse PID: %w", err)
	}

	outputFile := fmt.Sprintf("%s/output.log", remoteBaseDir)
	return pid, outputFile, nil
}

// PollTaskStatus 轮询任务状态
func (m *AsyncTaskManager) PollTaskStatus(ctx context.Context, task *AsyncTask, pollInterval time.Duration, maxPollCount int) (*AsyncTask, error) {
	pollCount := 0
	consecutiveFailures := 0

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return m.GetTask(task.ID), nil

		case <-ticker.C:
			pollCount++

			if maxPollCount > 0 && pollCount >= maxPollCount {
				m.forceTerminate(task)
				task.Status = AsyncTaskStatusTimeout
				task.Error = fmt.Errorf("max poll count exceeded: %d", maxPollCount)
				return task, nil
			}

			if task.AsyncTimeout > 0 && time.Since(task.StartTime) > task.AsyncTimeout {
				m.forceTerminate(task)
				task.Status = AsyncTaskStatusTimeout
				task.Error = fmt.Errorf("task timeout after %v", task.AsyncTimeout)
				return task, nil
			}

			running, err := m.isProcessRunning(task.NodeID, task.Pid)
			if err != nil {
				consecutiveFailures++
				if consecutiveFailures >= 3 {
					task.Status = AsyncTaskStatusFailed
					task.Error = fmt.Errorf("poll failed %d times: %w", consecutiveFailures, err)
					return task, nil
				}
				continue
			}
			consecutiveFailures = 0

			if !running {
				m.collectResults(task)
				return task, nil
			}
		}
	}
}

// isProcessRunning 检查进程是否在运行
func (m *AsyncTaskManager) isProcessRunning(nodeID string, pid int) (bool, error) {
	cmd := fmt.Sprintf("kill -0 %d 2>/dev/null && echo running || echo stopped", pid)
	output, err := executeLocalCommand(cmd)
	if err != nil {
		return false, err
	}
	return strings.Contains(output, "running"), nil
}

// collectResults 收集任务结果
func (m *AsyncTaskManager) collectResults(task *AsyncTask) {
	if task.OutputFile != "" {
		output, _ := executeLocalCommand(fmt.Sprintf("cat %s", task.OutputFile))
		task.OutputFile = output
	}

	task.ExitCode = m.getExitCode(task.NodeID, task.Pid)
	task.EndTime = time.Now()

	if task.ExitCode == 0 {
		task.Status = AsyncTaskStatusSuccess
	} else {
		task.Status = AsyncTaskStatusFailed
	}
}

// getExitCode 获取进程退出码
func (m *AsyncTaskManager) getExitCode(nodeID string, pid int) int {
	cmd := fmt.Sprintf("wait %d 2>/dev/null; echo $?", pid)
	output, err := executeLocalCommand(cmd)
	if err != nil {
		return -1
	}
	code, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return -1
	}
	return code
}

// forceTerminate 强制终止任务
func (m *AsyncTaskManager) forceTerminate(task *AsyncTask) error {
	killCmd := fmt.Sprintf("kill -TERM %d 2>/dev/null || true", task.Pid)
	executeLocalCommand(killCmd)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		running, _ := m.isProcessRunning(task.NodeID, task.Pid)
		if !running {
			task.ExitCode = m.getExitCode(task.NodeID, task.Pid)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	killCmd = fmt.Sprintf("kill -KILL %d 2>/dev/null || true", task.Pid)
	executeLocalCommand(killCmd)

	time.Sleep(500 * time.Millisecond)

	task.Status = AsyncTaskStatusCanceled
	task.ExitCode = -1
	task.Error = fmt.Errorf("task force terminated")

	return nil
}

// GetTask 获取任务
func (m *AsyncTaskManager) GetTask(taskID string) *AsyncTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[taskID]
}

// CancelTask 取消任务
func (m *AsyncTaskManager) CancelTask(taskID string) error {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}
	m.mu.Unlock()

	return m.forceTerminate(task)
}

// ListTasks 列出所有任务
func (m *AsyncTaskManager) ListTasks() []*AsyncTask {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*AsyncTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// StartAndForget 启动异步任务并立即返回
func (m *AsyncTaskManager) StartAndForget(ctx context.Context, nodeID, command string, opts *AsyncOptions) (string, error) {
	if opts == nil {
		opts = &AsyncOptions{}
	}

	opts.PollInterval = 0

	task, err := m.StartAsync(ctx, nodeID, command, opts)
	if err != nil {
		return "", err
	}

	return task.ID, nil
}

// WaitForAll 等待所有指定任务完成
func (m *AsyncTaskManager) WaitForAll(ctx context.Context, taskIDs []string, pollInterval time.Duration) []AsyncTask {
	results := make([]AsyncTask, len(taskIDs))

	var wg sync.WaitGroup
	wg.Add(len(taskIDs))

	for i, taskID := range taskIDs {
		go func(idx int, id string) {
			defer wg.Done()
			task := m.GetTask(id)
			if task == nil {
				return
			}

			completed, _ := m.PollTaskStatus(ctx, task, pollInterval, 0)
			if completed != nil {
				results[idx] = *completed
			}
		}(i, taskID)
	}

	wg.Wait()
	return results
}

// CleanupCompletedTasks 清理已完成的任务
func (m *AsyncTaskManager) CleanupCompletedTasks() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, task := range m.tasks {
		if task.IsCompleted() && time.Since(task.EndTime) > m.cleanupAfter {
			delete(m.tasks, id)
		}
	}
}

// executeLocalCommand 执行本地命令
func executeLocalCommand(cmd string) (string, error) {
	out, err := exec.Command("/bin/sh", "-c", cmd).CombinedOutput()
	return string(out), err
}