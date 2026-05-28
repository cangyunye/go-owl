package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const sepLine = "──────────────────────────────────────────────────────────────────────"

func TestNewNodeLogWriter_DefaultPath(t *testing.T) {
	origHomeDirFunc := homeDirFunc
	homeDirFunc = func() (string, error) {
		return "/tmp/testhome", nil
	}
	defer func() { homeDirFunc = origHomeDirFunc }()

	w := NewNodeLogWriter("")

	if !strings.Contains(w.logDir, ".owl/logs/nodes") {
		t.Errorf("logDir should contain .owl/logs/nodes, got: %s", w.logDir)
	}

	expected := filepath.Join("/tmp/testhome", ".owl", "logs", "nodes")
	if w.logDir != expected {
		t.Errorf("expected logDir=%s, got %s", expected, w.logDir)
	}
}

func TestNewNodeLogWriter_EnvVar(t *testing.T) {
	tempDir := t.TempDir()
	envDir := filepath.Join(tempDir, "custom-logs")

	t.Setenv("OWL_LOG_DIR", envDir)

	w := NewNodeLogWriter("")

	if w.logDir != envDir {
		t.Errorf("expected logDir=%s, got %s", envDir, w.logDir)
	}
}

func TestAppendEntry_SingleRecord(t *testing.T) {
	logDir := t.TempDir()
	w := NewNodeLogWriter(logDir)

	dur := 2 * time.Second
	err := w.AppendEntry("node1", "task-001", "echo hello", 0, "hello world", "", dur)
	if err != nil {
		t.Fatalf("AppendEntry failed: %v", err)
	}

	logPath := filepath.Join(logDir, "node1.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)

	if !strings.Contains(content, sepLine) {
		t.Error("log entry should contain separator lines")
	}

	sepCount := strings.Count(content, sepLine)
	if sepCount < 2 {
		t.Errorf("expected at least 2 separator lines, got %d", sepCount)
	}

	if !strings.Contains(content, " TASK: task-001") {
		t.Error("log entry should contain TASK field")
	}

	if !strings.Contains(content, "COMMAND: echo hello") {
		t.Error("log entry should contain COMMAND field")
	}

	if !strings.Contains(content, "EXIT CODE: 0") {
		t.Error("log entry should contain EXIT CODE field")
	}

	if !strings.Contains(content, "DURATION: 2.00s") {
		t.Error("log entry should contain DURATION field")
	}

	if !strings.Contains(content, "OUTPUT:") {
		t.Error("log entry should contain OUTPUT section")
	}

	if !strings.Contains(content, "hello world") {
		t.Error("log entry should contain the output text")
	}

	if !strings.Contains(content, "[") && !strings.Contains(content, "]") {
		t.Error("log entry should contain timestamp in brackets")
	}
}

func TestAppendEntry_MultipleRecords(t *testing.T) {
	logDir := t.TempDir()
	w := NewNodeLogWriter(logDir)

	dur := 100 * time.Millisecond

	for i := 0; i < 3; i++ {
		err := w.AppendEntry("node1", fmt.Sprintf("task-%03d", i), fmt.Sprintf("cmd-%d", i), 0, fmt.Sprintf("output-%d", i), "", dur)
		if err != nil {
			t.Fatalf("AppendEntry %d failed: %v", i, err)
		}
	}

	logPath := filepath.Join(logDir, "node1.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)

	sepCount := strings.Count(content, sepLine)
	if sepCount != 6 {
		t.Errorf("expected 6 separator lines (2 per entry × 3 entries), got %d", sepCount)
	}

	for i := 0; i < 3; i++ {
		if !strings.Contains(content, fmt.Sprintf("cmd-%d", i)) {
			t.Errorf("entry %d missing from log file", i)
		}
		if !strings.Contains(content, fmt.Sprintf("output-%d", i)) {
			t.Errorf("output for entry %d missing from log file", i)
		}
	}
}

func TestAppendEntry_AutoCreateDir(t *testing.T) {
	baseDir := t.TempDir()
	logDir := filepath.Join(baseDir, "nonexistent", "subdir")
	w := NewNodeLogWriter(logDir)

	err := w.AppendEntry("node1", "task-001", "echo test", 0, "auto created", "", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("AppendEntry failed: %v", err)
	}

	logPath := filepath.Join(logDir, "node1.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log file was not created: %v", err)
	}

	content := string(data)

	if !strings.Contains(content, "echo test") {
		t.Error("log file should contain the command")
	}
	if !strings.Contains(content, "auto created") {
		t.Error("log file should contain the output")
	}
}

func TestAppendEntry_ConcurrentWrites(t *testing.T) {
	logDir := t.TempDir()
	w := NewNodeLogWriter(logDir)

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			cmd := fmt.Sprintf("concurrent-cmd-%02d", idx)
			err := w.AppendEntry("node1", fmt.Sprintf("task-%02d", idx), cmd, 0, fmt.Sprintf("output-%02d", idx), "", 10*time.Millisecond)
			if err != nil {
				t.Errorf("concurrent AppendEntry %d failed: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	logPath := filepath.Join(logDir, "node1.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)

	sepCount := strings.Count(content, sepLine)
	if sepCount != numGoroutines*2 {
		t.Errorf("expected %d separator lines, got %d", numGoroutines*2, sepCount)
	}

	entries := strings.Split(content, sepLine)
	var nonEmpty int
	for _, e := range entries {
		if strings.TrimSpace(e) != "" {
			nonEmpty++
		}
	}
	if nonEmpty != numGoroutines {
		t.Errorf("expected %d non-empty entries, got %d", numGoroutines, nonEmpty)
	}

	for i := 0; i < numGoroutines; i++ {
		cmd := fmt.Sprintf("concurrent-cmd-%02d", i)
		if !strings.Contains(content, cmd) {
			t.Errorf("concurrent command %q not found in log file", cmd)
		}
	}
}

func TestAppendEntry_FailedExecution(t *testing.T) {
	logDir := t.TempDir()
	w := NewNodeLogWriter(logDir)

	err := w.AppendEntry("node1", "task-err", "bad-command", 1, "some output", "connection refused", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("AppendEntry failed: %v", err)
	}

	logPath := filepath.Join(logDir, "node1.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)

	if !strings.Contains(content, "EXIT CODE: 1") {
		t.Error("log entry should contain EXIT CODE: 1")
	}

	if !strings.Contains(content, "ERROR: connection refused") {
		t.Error("log entry should contain ERROR field with error message")
	}
}

func TestAppendEntry_EmptyOutput(t *testing.T) {
	logDir := t.TempDir()
	w := NewNodeLogWriter(logDir)

	err := w.AppendEntry("node1", "task-empty", "no-output-cmd", 0, "", "", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("AppendEntry failed: %v", err)
	}

	logPath := filepath.Join(logDir, "node1.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)

	if strings.Contains(content, "OUTPUT:") {
		t.Error("log entry should NOT contain OUTPUT section when output is empty")
	}

	if !strings.Contains(content, "TASK: task-empty") {
		t.Error("log entry should contain TASK field")
	}

	if !strings.Contains(content, "COMMAND: no-output-cmd") {
		t.Error("log entry should contain COMMAND field")
	}

	sepCount := strings.Count(content, sepLine)
	if sepCount != 2 {
		t.Errorf("expected 2 separator lines, got %d", sepCount)
	}
}
