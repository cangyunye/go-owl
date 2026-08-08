package logfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSanitizeNodeID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"web-01", "web-01"},
		{"node/with/slash", "node_with_slash"},
		{"..", "node"},
		{"sp ace", "sp_ace"},
		{"", "node"},
	}
	for _, c := range cases {
		if got := SanitizeNodeID(c.in); got != c.want {
			t.Errorf("SanitizeNodeID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExecutionsDir_EnvVar(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("OWL_LOG_DIR", tempDir)
	if got, want := ExecutionsDir(), filepath.Join(tempDir, "executions"); got != want {
		t.Errorf("ExecutionsDir() = %q, want %q", got, want)
	}
}

func TestExecutionsDir_Default(t *testing.T) {
	orig := homeDirFunc
	homeDirFunc = func() (string, error) { return "/tmp/exec-home", nil }
	defer func() { homeDirFunc = orig }()
	if got, want := ExecutionsDir(), filepath.Join("/tmp/exec-home", ".owl", "logs", "executions"); got != want {
		t.Errorf("ExecutionsDir() = %q, want %q", got, want)
	}
}

func TestWriteExecutionLog_Success(t *testing.T) {
	t.Setenv("OWL_LOG_DIR", t.TempDir())
	w := NewNodeLogWriter("")

	path, err := w.WriteExecutionLog("op-001", "web-01", "task-1", "uptime", 0, "load 1.00", "", 2*time.Second)
	if err != nil {
		t.Fatalf("WriteExecutionLog failed: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("executions", "op-001", "web-01.log")) {
		t.Errorf("unexpected path: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	content := string(data)
	for _, want := range []string{"TASK: task-1", "NODE: web-01", "COMMAND: uptime", "EXIT CODE: 0", "DURATION: 2.00s", "OUTPUT:", "load 1.00"} {
		if !strings.Contains(content, want) {
			t.Errorf("log missing %q\n%s", want, content)
		}
	}

	m := readManifest(t, filepath.Join(ExecutionsDir(), "op-001"))
	if m.OpID != "op-001" {
		t.Errorf("manifest op_id = %q, want op-001", m.OpID)
	}
	entry, ok := m.Nodes["web-01.log"]
	if !ok {
		t.Fatalf("manifest missing web-01.log: %+v", m.Nodes)
	}
	if entry.NodeID != "web-01" || entry.TaskID != "task-1" || !entry.Success {
		t.Errorf("manifest entry wrong: %+v", entry)
	}
}

func TestWriteExecutionLog_Failed(t *testing.T) {
	t.Setenv("OWL_LOG_DIR", t.TempDir())
	w := NewNodeLogWriter("")

	_, err := w.WriteExecutionLog("op-002", "db-01", "task-2", "bad-cmd", 1, "partial out", "connection refused", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("WriteExecutionLog failed: %v", err)
	}

	path := filepath.Join(ExecutionsDir(), "op-002", "db-01.log")
	data, _ := os.ReadFile(path)
	content := string(data)
	for _, want := range []string{"EXIT CODE: 1", "ERROR: connection refused", "partial out"} {
		if !strings.Contains(content, want) {
			t.Errorf("log missing %q\n%s", want, content)
		}
	}
	m := readManifest(t, filepath.Join(ExecutionsDir(), "op-002"))
	if m.Nodes["db-01.log"].Success {
		t.Error("failed execution should have Success=false")
	}
}

func TestWriteExecutionLog_SanitizedFilename(t *testing.T) {
	t.Setenv("OWL_LOG_DIR", t.TempDir())
	w := NewNodeLogWriter("")

	path, err := w.WriteExecutionLog("op-003", "node/a b", "task-3", "echo", 0, "x", "", 0)
	if err != nil {
		t.Fatalf("WriteExecutionLog failed: %v", err)
	}
	if !strings.HasSuffix(path, "node_a_b.log") {
		t.Errorf("expected sanitized filename, got %s", path)
	}
}

func TestWriteExecutionLog_ConcurrentSameOp(t *testing.T) {
	t.Setenv("OWL_LOG_DIR", t.TempDir())
	w := NewNodeLogWriter("")

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			nid := "node-" + strings.Repeat(string(rune('a'+i)), 3)
			if _, err := w.WriteExecutionLog("op-conc", nid, "t", "cmd", 0, "out", "", 0); err != nil {
				t.Errorf("write %d failed: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	dir := filepath.Join(ExecutionsDir(), "op-conc")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	m := readManifest(t, dir)
	if len(m.Nodes) != 5 {
		t.Errorf("manifest should have 5 nodes, got %d: %+v", len(m.Nodes), m.Nodes)
	}
	if len(entries) != 6 { // 5 logs + manifest
		t.Errorf("expected 6 entries, got %d", len(entries))
	}
}

func readManifest(t *testing.T, dir string) executionManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m executionManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	return m
}
