package transfer

import (
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
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

func TestDiffusionTransfer_UpdateNodeProgress(t *testing.T) {
	tree := &DiffusionTree{
		Root: "control",
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{"node-1"}},
			"node-1":  {ID: "node-1", ParentID: "control", Children: []string{}},
		},
	}

	transfer := NewDiffusionTransfer("task-1", "test.txt", "/source", "/dest", 1024, "hash", tree)
	transfer.InitializeStatuses()

	transfer.UpdateNodeProgress("node-1", 5, 10)

	status := transfer.NodeStatuses["node-1"]
	if status.ChunksSent != 5 {
		t.Errorf("expected ChunksSent 5, got %d", status.ChunksSent)
	}
	if status.ChunksTotal != 10 {
		t.Errorf("expected ChunksTotal 10, got %d", status.ChunksTotal)
	}
	if status.Progress != 50.0 {
		t.Errorf("expected Progress 50.0, got %f", status.Progress)
	}
}

func TestDiffusionTransfer_GetPendingNodes(t *testing.T) {
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

	pending := transfer.GetPendingNodes()
	if len(pending) != 2 {
		t.Errorf("expected 2 pending nodes, got %d", len(pending))
	}

	transfer.UpdateNodeStatus("node-1", DiffusionStatusCompleted, 100.0, "")
	pending = transfer.GetPendingNodes()
	if len(pending) != 1 {
		t.Errorf("expected 1 pending node, got %d", len(pending))
	}
}

func TestDiffusionTransfer_GetAvailableSources(t *testing.T) {
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

	sources := transfer.GetAvailableSources()
	if len(sources) != 0 {
		t.Errorf("expected 0 sources (control is not marked as available source), got %d", len(sources))
	}

	transfer.MarkNodeAsSource("node-1")
	transfer.UpdateNodeStatus("node-1", DiffusionStatusCompleted, 100.0, "")

	sources = transfer.GetAvailableSources()
	if len(sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(sources))
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

func TestDiffusionTransfer_GetOverallProgress(t *testing.T) {
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

	progress := transfer.GetOverallProgress()
	if progress != 0 {
		t.Errorf("expected progress 0, got %f", progress)
	}

	transfer.UpdateNodeStatus("node-1", DiffusionStatusCompleted, 100.0, "")
	progress = transfer.GetOverallProgress()
	if progress < 33.0 || progress > 34.0 {
		t.Errorf("expected progress ~33.33 (1/3), got %f", progress)
	}
}

func TestDiffusionTransfer_ShouldUseDiffusion(t *testing.T) {
	tree := &DiffusionTree{
		Root:      "control",
		Threshold: 5,
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{}},
			"node-1":  {ID: "node-1", ParentID: "control", Children: []string{}},
			"node-2":  {ID: "node-2", ParentID: "control", Children: []string{}},
		},
	}

	transfer := NewDiffusionTransfer("task-1", "test.txt", "/source", "/dest", 1024, "hash", tree)
	if transfer.ShouldUseDiffusion() {
		t.Error("expected false for 3 nodes < threshold 5")
	}

	tree2 := &DiffusionTree{
		Root:      "control",
		Threshold: 5,
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{}},
			"node-1":  {ID: "node-1", ParentID: "control", Children: []string{}},
			"node-2":  {ID: "node-2", ParentID: "control", Children: []string{}},
			"node-3":  {ID: "node-3", ParentID: "control", Children: []string{}},
			"node-4":  {ID: "node-4", ParentID: "control", Children: []string{}},
			"node-5":  {ID: "node-5", ParentID: "control", Children: []string{}},
		},
	}

	transfer2 := NewDiffusionTransfer("task-2", "test.txt", "/source", "/dest", 1024, "hash", tree2)
	if !transfer2.ShouldUseDiffusion() {
		t.Error("expected true for 6 nodes >= threshold 5")
	}
}

func TestDiffusionTransfer_GetSubTaskForNode(t *testing.T) {
	tree := &DiffusionTree{
		Root: "control",
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{"node-1"}},
			"node-1":  {ID: "node-1", ParentID: "control", Children: []string{"node-2", "node-3"}},
			"node-2":  {ID: "node-2", ParentID: "node-1", Children: []string{}},
			"node-3":  {ID: "node-3", ParentID: "node-1", Children: []string{}},
		},
	}

	transfer := NewDiffusionTransfer("task-1", "test.txt", "/source", "/dest", 1024, "hash", tree)
	transfer.InitializeStatuses()

	subTask, err := transfer.GetSubTaskForNode("node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if subTask.SourceNodeID != "node-1" {
		t.Errorf("expected SourceNodeID 'node-1', got '%s'", subTask.SourceNodeID)
	}
	if len(subTask.TargetNodeIDs) != 2 {
		t.Errorf("expected 2 target nodes, got %d", len(subTask.TargetNodeIDs))
	}
}

func TestDiffusionTransfer_GetSubTaskForNode_NoChildren(t *testing.T) {
	tree := &DiffusionTree{
		Root: "control",
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{"node-1"}},
			"node-1":  {ID: "node-1", ParentID: "control", Children: []string{}},
		},
	}

	transfer := NewDiffusionTransfer("task-1", "test.txt", "/source", "/dest", 1024, "hash", tree)
	transfer.InitializeStatuses()

	_, err := transfer.GetSubTaskForNode("node-1")
	if err == nil {
		t.Error("expected error for node without children")
	}
}

func TestNewSubTransferTask(t *testing.T) {
	task := NewSubTransferTask(
		"parent-task",
		"source-node",
		[]string{"target-1", "target-2"},
		"test.txt",
		1024,
		"hash123",
		"/dest/test.txt",
	)

	if task.ParentTaskID != "parent-task" {
		t.Errorf("expected ParentTaskID 'parent-task', got '%s'", task.ParentTaskID)
	}
	if task.SourceNodeID != "source-node" {
		t.Errorf("expected SourceNodeID 'source-node', got '%s'", task.SourceNodeID)
	}
	if len(task.TargetNodeIDs) != 2 {
		t.Errorf("expected 2 target nodes, got %d", len(task.TargetNodeIDs))
	}
	if task.Status != DiffusionStatusPending {
		t.Errorf("expected Status 'pending', got '%s'", task.Status)
	}
}

func TestSubTransferTask_UpdateTargetProgress(t *testing.T) {
	task := NewSubTransferTask("parent", "source", []string{"target-1", "target-2"}, "test.txt", 1024, "hash", "/dest")

	task.UpdateTargetProgress("target-1", 50.0)
	if task.Progress["target-1"] != 50.0 {
		t.Errorf("expected progress 50.0, got %f", task.Progress["target-1"])
	}
}

func TestSubTransferTask_GetOverallProgress(t *testing.T) {
	task := NewSubTransferTask("parent", "source", []string{"target-1", "target-2"}, "test.txt", 1024, "hash", "/dest")

	progress := task.GetOverallProgress()
	if progress != 0 {
		t.Errorf("expected progress 0, got %f", progress)
	}

	task.UpdateTargetProgress("target-1", 100.0)
	task.UpdateTargetProgress("target-2", 50.0)

	progress = task.GetOverallProgress()
	if progress != 75.0 {
		t.Errorf("expected progress 75.0, got %f", progress)
	}
}

func TestSubTransferTask_IsCompleted(t *testing.T) {
	task := NewSubTransferTask("parent", "source", []string{"target-1", "target-2"}, "test.txt", 1024, "hash", "/dest")

	if task.IsCompleted() {
		t.Error("expected false for incomplete task")
	}

	task.UpdateTargetProgress("target-1", 100.0)
	if task.IsCompleted() {
		t.Error("expected false when only some targets are complete")
	}

	task.UpdateTargetProgress("target-2", 100.0)
	if !task.IsCompleted() {
		t.Error("expected true when all targets are complete")
	}
}

func TestDiffusionScheduler_CreateTransfer(t *testing.T) {
	scheduler := NewDiffusionScheduler()

	targets := []*model.Node{
		{ID: "node-1"},
		{ID: "node-2"},
		{ID: "node-3"},
	}

	transfer, err := scheduler.CreateTransfer("task-1", targets, "/source", "/dest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if transfer.TaskID != "task-1" {
		t.Errorf("expected TaskID 'task-1', got '%s'", transfer.TaskID)
	}
}

func TestDiffusionScheduler_GetTransfer(t *testing.T) {
	scheduler := NewDiffusionScheduler()

	targets := []*model.Node{{ID: "node-1"}}
	created, _ := scheduler.CreateTransfer("task-1", targets, "/source", "/dest")

	retrieved, ok := scheduler.GetTransfer("task-1")
	if !ok {
		t.Fatal("expected transfer to be found")
	}
	if retrieved.TaskID != created.TaskID {
		t.Errorf("expected TaskID '%s', got '%s'", created.TaskID, retrieved.TaskID)
	}

	_, ok = scheduler.GetTransfer("nonexistent")
	if ok {
		t.Error("expected transfer not to be found")
	}
}

func TestDiffusionScheduler_UpdateNodeStatus(t *testing.T) {
	scheduler := NewDiffusionScheduler()

	targets := []*model.Node{{ID: "node-1"}}
	scheduler.CreateTransfer("task-1", targets, "/source", "/dest")

	err := scheduler.UpdateNodeStatus("task-1", "node-1", DiffusionStatusCompleted, 100.0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	transfer, _ := scheduler.GetTransfer("task-1")
	if transfer.NodeStatuses["node-1"].Status != DiffusionStatusCompleted {
		t.Errorf("expected status 'completed', got '%s'", transfer.NodeStatuses["node-1"].Status)
	}
}

func TestDiffusionScheduler_ReassignFailedNode(t *testing.T) {
	scheduler := NewDiffusionScheduler()

	targets := []*model.Node{{ID: "node-1"}, {ID: "node-2"}}
	scheduler.CreateTransfer("task-1", targets, "/source", "/dest")

	transfer, _ := scheduler.GetTransfer("task-1")
	transfer.NodeStatuses["node-1"].Children = []string{"node-2"}

	children, err := scheduler.ReassignFailedNode("task-1", "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(children) != 1 || children[0] != "node-2" {
		t.Errorf("expected ['node-2'], got %v", children)
	}
}

func TestDiffusionScheduler_ListTransfers(t *testing.T) {
	scheduler := NewDiffusionScheduler()

	targets := []*model.Node{{ID: "node-1"}}
	scheduler.CreateTransfer("task-1", targets, "/source", "/dest")
	scheduler.CreateTransfer("task-2", targets, "/source", "/dest")

	transfers := scheduler.ListTransfers()
	if len(transfers) != 2 {
		t.Errorf("expected 2 transfers, got %d", len(transfers))
	}
}

func TestDiffusionScheduler_GetTransferReport(t *testing.T) {
	scheduler := NewDiffusionScheduler()

	targets := []*model.Node{{ID: "node-1"}, {ID: "node-2"}}
	scheduler.CreateTransfer("task-1", targets, "/source", "/dest")

	transfer, _ := scheduler.GetTransfer("task-1")
	transfer.FileName = "test.txt"
	transfer.FileSize = 1024

	scheduler.UpdateNodeStatus("task-1", "control", DiffusionStatusCompleted, 100.0, "")
	scheduler.UpdateNodeStatus("task-1", "node-1", DiffusionStatusCompleted, 100.0, "")
	scheduler.UpdateNodeStatus("task-1", "node-2", DiffusionStatusCompleted, 100.0, "")

	report, err := scheduler.GetTransferReport("task-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.TotalNodes != 3 {
		t.Errorf("expected TotalNodes 3, got %d", report.TotalNodes)
	}
	if report.SuccessCount != 3 {
		t.Errorf("expected SuccessCount 3, got %d", report.SuccessCount)
	}
	if report.OverallProgress != 100.0 {
		t.Errorf("expected OverallProgress 100.0, got %f", report.OverallProgress)
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

func TestDiffusionTransfer_GetSuccessCount(t *testing.T) {
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

	if transfer.GetSuccessCount() != 0 {
		t.Errorf("expected 0 successes, got %d", transfer.GetSuccessCount())
	}

	transfer.UpdateNodeStatus("node-1", DiffusionStatusCompleted, 100.0, "")
	if transfer.GetSuccessCount() != 1 {
		t.Errorf("expected 1 success, got %d", transfer.GetSuccessCount())
	}
}

func TestDiffusionTransfer_GetFailureCount(t *testing.T) {
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

	if transfer.GetFailureCount() != 0 {
		t.Errorf("expected 0 failures, got %d", transfer.GetFailureCount())
	}

	transfer.UpdateNodeStatus("node-1", DiffusionStatusFailed, 0, "error")
	if transfer.GetFailureCount() != 1 {
		t.Errorf("expected 1 failure, got %d", transfer.GetFailureCount())
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
