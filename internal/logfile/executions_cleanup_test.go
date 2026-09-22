package logfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeBatch 用生产写入路径造一个批次目录（opID + 两个节点日志）。
func writeBatch(t *testing.T, opID string, files int) {
	t.Helper()
	w := NewNodeLogWriter("")
	for i := 0; i < files; i++ {
		_, err := w.WriteExecutionLog(opID, "node-"+string(rune('a'+i)), "task-1", "uptime", 0, "ok", "", time.Second)
		if err != nil {
			t.Fatalf("WriteExecutionLog: %v", err)
		}
	}
}

func withExecDir(t *testing.T) {
	t.Helper()
	t.Setenv("OWL_LOG_DIR", t.TempDir())
}

// SummarizeExecutions 汇总所有批次：批次数、每批文件数与字节、总量。
func TestExecutionsSummary(t *testing.T) {
	withExecDir(t)
	writeBatch(t, "op-old-1", 2)
	writeBatch(t, "op-new-2", 3)

	sum, err := SummarizeExecutions()
	if err != nil {
		t.Fatalf("ExecutionsSummary: %v", err)
	}
	if sum.TotalBatches != 2 {
		t.Fatalf("TotalBatches = %d, want 2", sum.TotalBatches)
	}
	if sum.TotalSizeBytes <= 0 {
		t.Fatalf("TotalSizeBytes = %d, want > 0", sum.TotalSizeBytes)
	}
	byOp := map[string]ExecutionsBatch{}
	for _, b := range sum.Batches {
		byOp[b.OpID] = b
	}
	if b := byOp["op-old-1"]; b.Files != 2 || b.SizeBytes <= 0 {
		t.Fatalf("batch op-old-1 = %+v, want files=2 size>0", b)
	}
	if b := byOp["op-new-2"]; b.Files != 3 {
		t.Fatalf("batch op-new-2 = %+v, want files=3", b)
	}
}

// CleanupExecutions 只删除最后修改时间早于保留期的批次目录，
// 保留期内的批次不动；非批次命名的目录不触碰。
func TestCleanupExecutions_RemovesOnlyExpired(t *testing.T) {
	withExecDir(t)
	writeBatch(t, "op-keep", 2)
	writeBatch(t, "op-drop", 2)

	// 手工把 op-drop 目录时间拨到 40 天前
	dropDir := filepath.Join(ExecutionsDir(), "op-drop")
	old := time.Now().AddDate(0, 0, -40)
	if err := os.Chtimes(dropDir, old, old); err != nil {
		t.Fatal(err)
	}
	// 一个不合法命名的目录（不应被清理触碰）
	junk := filepath.Join(ExecutionsDir(), "不是opID")
	if err := os.MkdirAll(junk, 0o755); err != nil {
		t.Fatal(err)
	}

	removed, reclaimed, err := CleanupExecutions(30 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("CleanupExecutions: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if reclaimed <= 0 {
		t.Fatalf("reclaimed = %d, want > 0", reclaimed)
	}
	if _, err := os.Stat(dropDir); !os.IsNotExist(err) {
		t.Fatal("expired batch dir still exists")
	}
	if _, err := os.Stat(filepath.Join(ExecutionsDir(), "op-keep")); err != nil {
		t.Fatal("recent batch dir was removed")
	}
	if _, err := os.Stat(junk); err != nil {
		t.Fatal("invalid-name dir must not be touched")
	}
}

// 保留期内全部批次 → 什么都不删；空目录/无目录也不报错。
func TestCleanupExecutions_NothingExpiredOrEmpty(t *testing.T) {
	withExecDir(t)

	removed, _, err := CleanupExecutions(30 * 24 * time.Hour)
	if err != nil || removed != 0 {
		t.Fatalf("empty dir: removed=%d err=%v, want 0/nil", removed, err)
	}

	writeBatch(t, "op-recent", 1)
	removed, reclaimed, err := CleanupExecutions(30 * 24 * time.Hour)
	if err != nil || removed != 0 || reclaimed != 0 {
		t.Fatalf("nothing expired: removed=%d reclaimed=%d err=%v", removed, reclaimed, err)
	}
}
