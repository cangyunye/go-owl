package transfer

import (
	"testing"
)

func TestNewDiffusionTransfer(t *testing.T) {
	tree := &DiffusionTree{
		Root:      "control",
		FanOutK:   3,
		Threshold: 5,
		Nodes:     make(map[string]*TreeNode),
	}

	transfer := NewDiffusionTransfer(
		"task-1",
		"test.txt",
		"/source/test.txt",
		"/dest/test.txt",
		1024,
		"hash123",
		tree,
	)

	if transfer.TaskID != "task-1" {
		t.Errorf("expected TaskID 'task-1', got '%s'", transfer.TaskID)
	}
	if transfer.FileName != "test.txt" {
		t.Errorf("expected FileName 'test.txt', got '%s'", transfer.FileName)
	}
	if transfer.FileSize != 1024 {
		t.Errorf("expected FileSize 1024, got %d", transfer.FileSize)
	}
	if transfer.Status != DiffusionStatusPending {
		t.Errorf("expected Status 'pending', got '%s'", transfer.Status)
	}
}

func TestDiffusionTransfer_InitializeStatuses(t *testing.T) {
	tree := &DiffusionTree{
		Root:    "control",
		FanOutK: 3,
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{"node-1", "node-2"}},
			"node-1":  {ID: "node-1", ParentID: "control", Children: []string{}},
			"node-2":  {ID: "node-2", ParentID: "control", Children: []string{}},
		},
	}

	transfer := NewDiffusionTransfer("task-1", "test.txt", "/source", "/dest", 1024, "hash", tree)
	transfer.InitializeStatuses()

	if len(transfer.NodeStatuses) != 3 {
		t.Errorf("expected 3 statuses, got %d", len(transfer.NodeStatuses))
	}

	if transfer.NodeStatuses["control"].IsSource != true {
		t.Errorf("expected control to be source")
	}
}

func TestDiffusionTransfer_UpdateNodeStatus(t *testing.T) {
	tree := &DiffusionTree{
		Root: "control",
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{"node-1"}},
			"node-1":  {ID: "node-1", ParentID: "control", Children: []string{}},
		},
	}

	transfer := NewDiffusionTransfer("task-1", "test.txt", "/source", "/dest", 1024, "hash", tree)
	transfer.InitializeStatuses()

	transfer.UpdateNodeStatus("node-1", DiffusionStatusCompleted, 100.0, "")

	status := transfer.NodeStatuses["node-1"]
	if status.Status != DiffusionStatusCompleted {
		t.Errorf("expected status 'completed', got '%s'", status.Status)
	}
	if status.Progress != 100.0 {
		t.Errorf("expected progress 100.0, got %f", status.Progress)
	}
}

func TestDiffusionTransfer_recalculateStatus(t *testing.T) {
	tree := &DiffusionTree{
		Root: "control",
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{"node-1", "node-2"}},
			"node-1":  {ID: "node-1", ParentID: "control", Children: []string{}},
			"node-2":  {ID: "node-2", ParentID: "control", Children: []string{}},
		},
	}

	transfer := NewDiffusionTransfer("task-1", "test.txt", "/source", "/dest", 1024, "hash", tree)
	transfer.InitializeStatuses()

	if transfer.Status != DiffusionStatusPending {
		t.Errorf("expected status 'pending', got '%s'", transfer.Status)
	}

	transfer.UpdateNodeStatus("node-1", DiffusionStatusCompleted, 100.0, "")
	if transfer.Status != DiffusionStatusInProgress {
		t.Errorf("expected status 'in_progress', got '%s'", transfer.Status)
	}

	transfer.UpdateNodeStatus("node-2", DiffusionStatusCompleted, 100.0, "")
	transfer.UpdateNodeStatus("control", DiffusionStatusCompleted, 100.0, "")
	if transfer.Status != DiffusionStatusCompleted {
		t.Errorf("expected status 'completed', got '%s'", transfer.Status)
	}
}

func TestCalculateChunks(t *testing.T) {
	tests := []struct {
		fileSize  int64
		chunkSize int64
		expected  int64
	}{
		{1024, 256, 4},
		{1000, 256, 4},
		{0, 256, 1},
		{256, 256, 1},
		{257, 256, 2},
		{100, 0, 1},
	}

	for _, tt := range tests {
		result := calculateChunks(tt.fileSize, tt.chunkSize)
		if result != tt.expected {
			t.Errorf("calculateChunks(%d, %d) = %d, expected %d", tt.fileSize, tt.chunkSize, result, tt.expected)
		}
	}
}

func TestDiffusionStatus(t *testing.T) {
	statuses := []DiffusionTransferStatus{
		DiffusionStatusPending,
		DiffusionStatusInProgress,
		DiffusionStatusCompleted,
		DiffusionStatusPartialFail,
		DiffusionStatusFailed,
	}

	for _, status := range statuses {
		if status == "" {
			t.Error("expected non-empty status")
		}
	}
}
