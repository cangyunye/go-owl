package logfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	executionsSubDir = "executions"
	manifestName     = "manifest.json"
	sepLine          = "──────────────────────────────────────────────────────────────────────"
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

// resolveExecutionsDir 返回按执行批次+节点保存日志的根目录。
func resolveExecutionsDir() string {
	if envDir := os.Getenv("OWL_LOG_DIR"); envDir != "" {
		return filepath.Join(envDir, executionsSubDir)
	}
	home, err := homeDirFunc()
	if err != nil {
		return filepath.Join(".owl", "logs", executionsSubDir)
	}
	return filepath.Join(home, ".owl", "logs", executionsSubDir)
}

// ExecutionsDir 返回批次日志根目录(供 serve 下载/清理使用)。
func ExecutionsDir() string {
	return resolveExecutionsDir()
}

// SanitizeNodeID 将节点 ID 净化为安全的文件名(去掉路径分隔符/空白/控制字符)。
func SanitizeNodeID(nodeID string) string {
	s := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, nodeID)
	s = strings.Trim(s, "_")
	if s == "" || s == "." || s == ".." {
		return "node"
	}
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func sanitizeID(id string) string {
	s := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '_'
	}, id)
	s = strings.Trim(s, "-_")
	if s == "" {
		return "op"
	}
	if len(s) > 64 {
		s = s[:64]
	}
	return s
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
	return w.lockKey(nodeID)
}

func (w *NodeLogWriter) lockKey(key string) *sync.Mutex {
	w.mu.Lock()
	defer w.mu.Unlock()
	if mu, ok := w.locks[key]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	w.locks[key] = mu
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

type nodeLogMeta struct {
	NodeID    string    `json:"node_id"`
	File      string    `json:"file"`
	TaskID    string    `json:"task_id"`
	Command   string    `json:"command"`
	ExitCode  int       `json:"exit_code"`
	Success   bool      `json:"success"`
	CreatedAt time.Time `json:"created_at"`
}

type executionManifest struct {
	OpID  string                 `json:"op_id"`
	Nodes map[string]nodeLogMeta `json:"nodes"`
}

// WriteExecutionLog 将单次执行(单节点)的日志按批次+节点写入本地:
// ~/.owl/logs/executions/<opID>/<sanitized-nodeID>.log + manifest.json。
// 同一 opID 的并发写入以 per-op mutex 串行化,文件与 manifest 均原子写。
func (w *NodeLogWriter) WriteExecutionLog(opID, nodeID, taskID, command string, exitCode int, output, errMsg string, duration time.Duration) (string, error) {
	dir := filepath.Join(resolveExecutionsDir(), sanitizeID(opID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建执行日志目录失败: %w", err)
	}

	filename := SanitizeNodeID(nodeID) + ".log"
	path := filepath.Join(dir, filename)
	content := buildExecutionLog(opID, nodeID, taskID, command, exitCode, output, errMsg, duration)

	opMu := w.lockKey(opID)
	opMu.Lock()
	defer opMu.Unlock()

	if err := atomicWriteFile(path, content); err != nil {
		return "", err
	}
	if err := w.updateManifest(dir, opID, filename, nodeID, taskID, command, exitCode, errMsg == ""); err != nil {
		return "", err
	}
	return path, nil
}

func buildExecutionLog(opID, nodeID, taskID, command string, exitCode int, output, errMsg string, duration time.Duration) string {
	var b strings.Builder
	b.WriteString(sepLine + "\n")
	fmt.Fprintf(&b, "[%s] TASK: %s\n", time.Now().Format("2006-01-02 15:04:05"), taskID)
	fmt.Fprintf(&b, "OP: %s\n", opID)
	fmt.Fprintf(&b, "NODE: %s\n", nodeID)
	fmt.Fprintf(&b, "COMMAND: %s\n", command)
	fmt.Fprintf(&b, "EXIT CODE: %d\n", exitCode)
	fmt.Fprintf(&b, "DURATION: %s\n", formatDuration(duration))
	if errMsg != "" {
		fmt.Fprintf(&b, "ERROR: %s\n", errMsg)
	}
	if output != "" {
		b.WriteString("OUTPUT:\n")
		b.WriteString(output)
		if output[len(output)-1] != '\n' {
			b.WriteString("\n")
		}
	}
	b.WriteString(sepLine + "\n")
	return b.String()
}

func atomicWriteFile(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入日志失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("落盘日志失败: %w", err)
	}
	return nil
}

func (w *NodeLogWriter) updateManifest(dir, opID, filename, nodeID, taskID, command string, exitCode int, success bool) error {
	manifestPath := filepath.Join(dir, manifestName)
	m := loadManifest(manifestPath)
	if m.OpID == "" {
		m.OpID = opID
	}
	if m.Nodes == nil {
		m.Nodes = map[string]nodeLogMeta{}
	}
	m.Nodes[filename] = nodeLogMeta{
		NodeID: nodeID, File: filename, TaskID: taskID, Command: command,
		ExitCode: exitCode, Success: success, CreatedAt: time.Now().UTC(),
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(manifestPath, string(data))
}

func loadManifest(manifestPath string) executionManifest {
	m := executionManifest{OpID: "", Nodes: map[string]nodeLogMeta{}}
	if data, err := os.ReadFile(manifestPath); err == nil {
		json.Unmarshal(data, &m)
	}
	if m.Nodes == nil {
		m.Nodes = map[string]nodeLogMeta{}
	}
	return m
}

// ExecutionLogInfo 描述批次目录内的一个节点日志文件。
type ExecutionLogInfo struct {
	NodeID  string    `json:"node_id"`
	File    string    `json:"file"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// ListExecutionLogs 列出指定批次(opID)目录下的节点日志;目录不存在返回 os.ErrNotExist。
func ListExecutionLogs(opID string) ([]ExecutionLogInfo, error) {
	dir := filepath.Join(resolveExecutionsDir(), sanitizeID(opID))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	m := loadManifest(filepath.Join(dir, manifestName))
	infos := make([]ExecutionLogInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || e.Name() == manifestName || filepath.Ext(e.Name()) != ".log" {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		nodeID := strings.TrimSuffix(e.Name(), ".log")
		if meta, ok := m.Nodes[e.Name()]; ok && meta.NodeID != "" {
			nodeID = meta.NodeID
		}
		infos = append(infos, ExecutionLogInfo{
			NodeID: nodeID, File: e.Name(), Size: fi.Size(), ModTime: fi.ModTime(),
		})
	}
	return infos, nil
}
