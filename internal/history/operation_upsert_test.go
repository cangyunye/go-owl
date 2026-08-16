package history

import (
	"path/filepath"
	"testing"
	"time"
)

// TestRecordOperation_DedupByTaskID 验证同一 task_id 的终态记录会更新既有
// "running" 行，而不是插入重复行（修复 owl history 重复显示问题）。
func TestRecordOperation_DedupByTaskID(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := NewDB(&Config{Enabled: true, DBPath: filepath.Join(tmpDir, "test.db"), RetentionDays: 30})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer db.Close()
	SetGlobalDB(db)
	defer SetGlobalDB(nil)

	now := time.Now()
	if err := RecordOperation(&Operation{
		TaskID:    "task-1",
		OpType:    "command",
		Command:   "echo hi",
		Targets:   []string{"node-a"},
		Status:    "running",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("record running: %v", err)
	}

	if err := RecordOperation(&Operation{
		TaskID:    "task-1",
		OpType:    "command",
		Command:   "echo hi",
		Targets:   []string{"node-a"},
		Status:    "failed",
		CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("record terminal: %v", err)
	}

	records, err := GetDB().Query(&QueryOptions{TaskID: "task-1"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected exactly 1 operation row for task-1, got %d", len(records))
	}
	op := records[0].Operation
	if op == nil {
		t.Fatal("expected operation in record")
	}
	if op.Status != "failed" {
		t.Errorf("expected status 'failed' (terminal overwrites running), got %q", op.Status)
	}
}

// TestRecordOperation_DifferentTaskIDsSeparateRows 验证不同 task_id 仍各自成行。
func TestRecordOperation_DifferentTaskIDsSeparateRows(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := NewDB(&Config{Enabled: true, DBPath: filepath.Join(tmpDir, "test.db"), RetentionDays: 30})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer db.Close()
	SetGlobalDB(db)
	defer SetGlobalDB(nil)

	now := time.Now()
	for _, id := range []string{"task-a", "task-b"} {
		if err := RecordOperation(&Operation{
			TaskID: id, OpType: "command", Command: "echo hi", Status: "running", CreatedAt: now,
		}); err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
	}
	records, err := GetDB().Query(&QueryOptions{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 operation rows, got %d", len(records))
	}
}
