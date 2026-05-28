package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var homeDirFunc = os.UserHomeDir

func resolveLogDir(dir string) string {
	if dir != "" {
		return dir
	}
	if envDir := os.Getenv("OWL_LOG_DIR"); envDir != "" {
		return envDir
	}
	home, err := homeDirFunc()
	if err != nil {
		return filepath.Join(".owl", "logs", "nodes")
	}
	return filepath.Join(home, ".owl", "logs", "nodes")
}

type NodeLogWriter struct {
	logDir string
	mu     sync.Mutex
	locks  map[string]*sync.Mutex
}

func NewNodeLogWriter(logDir string) *NodeLogWriter {
	return &NodeLogWriter{
		logDir: resolveLogDir(logDir),
		locks:  make(map[string]*sync.Mutex),
	}
}

func (w *NodeLogWriter) lockNode(nodeID string) *sync.Mutex {
	w.mu.Lock()
	defer w.mu.Unlock()
	if mu, ok := w.locks[nodeID]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	w.locks[nodeID] = mu
	return mu
}

func (w *NodeLogWriter) AppendEntry(nodeID, taskID, command string, exitCode int, output string, errMsg string, duration time.Duration) error {
	nodeMu := w.lockNode(nodeID)
	nodeMu.Lock()
	defer nodeMu.Unlock()

	logPath := filepath.Join(w.logDir, nodeID+".log")

	if err := os.MkdirAll(w.logDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}
	defer f.Close()

	var entry string
	entry += "──────────────────────────────────────────────────────────────────────\n"
	entry += fmt.Sprintf("[%s] TASK: %s\n", time.Now().Format("2006-01-02 15:04:05"), taskID)
	entry += fmt.Sprintf("COMMAND: %s\n", command)
	entry += fmt.Sprintf("EXIT CODE: %d\n", exitCode)
	entry += fmt.Sprintf("DURATION: %s\n", formatDuration(duration))
	if errMsg != "" {
		entry += fmt.Sprintf("ERROR: %s\n", errMsg)
	}
	if output != "" {
		entry += "OUTPUT:\n"
		entry += output
		if output[len(output)-1] != '\n' {
			entry += "\n"
		}
	}
	entry += "──────────────────────────────────────────────────────────────────────\n"

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("写入日志失败: %w", err)
	}

	return nil
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}
