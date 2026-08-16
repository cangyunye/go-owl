//go:build !duckdb
// +build !duckdb

package history

import (
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(&Config{Enabled: true, DBPath: dbPath, RetentionDays: 30})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	return func() {
		db.Close()
		globalDB = nil
	}
}

func TestCreateAndGetPlaybookRun(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	run := &PlaybookRun{
		ID:           "run-1",
		PlaybookName: "deploy.yml",
		PlaybookHash: "hash-abc",
		Nodes:        []string{"node-a", "node-b"},
		Status:       "running",
		StartedAt:    time.Now(),
		TotalSteps:   5,
	}

	if err := CreatePlaybookRun(run); err != nil {
		t.Fatalf("CreatePlaybookRun failed: %v", err)
	}

	got, err := GetPlaybookRun("run-1")
	if err != nil {
		t.Fatalf("GetPlaybookRun failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected run, got nil")
	}

	if got.ID != "run-1" {
		t.Errorf("ID = %q, want %q", got.ID, "run-1")
	}
	if got.PlaybookName != "deploy.yml" {
		t.Errorf("PlaybookName = %q, want %q", got.PlaybookName, "deploy.yml")
	}
	if got.PlaybookHash != "hash-abc" {
		t.Errorf("PlaybookHash = %q, want %q", got.PlaybookHash, "hash-abc")
	}
	if len(got.Nodes) != 2 || got.Nodes[0] != "node-a" || got.Nodes[1] != "node-b" {
		t.Errorf("Nodes = %v, want [node-a node-b]", got.Nodes)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want %q", got.Status, "running")
	}
	if got.TotalSteps != 5 {
		t.Errorf("TotalSteps = %d, want 5", got.TotalSteps)
	}
}

func TestFinishPlaybookRun(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	run := &PlaybookRun{
		ID:           "run-finish",
		PlaybookName: "deploy.yml",
		PlaybookHash: "hash",
		Nodes:        []string{"node-a"},
		Status:       "running",
		StartedAt:    time.Now(),
		TotalSteps:   3,
	}
	if err := CreatePlaybookRun(run); err != nil {
		t.Fatalf("CreatePlaybookRun failed: %v", err)
	}

	if err := FinishPlaybookRun("run-finish", "completed", 3, 0); err != nil {
		t.Fatalf("FinishPlaybookRun failed: %v", err)
	}

	got, err := GetPlaybookRun("run-finish")
	if err != nil {
		t.Fatalf("GetPlaybookRun failed: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q, want %q", got.Status, "completed")
	}
	if got.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
	if got.CompletedSteps != 3 {
		t.Errorf("CompletedSteps = %d, want 3", got.CompletedSteps)
	}
	if got.FailedSteps != 0 {
		t.Errorf("FailedSteps = %d, want 0", got.FailedSteps)
	}
}

func TestListPlaybookRuns(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	base := time.Now()
	runs := []*PlaybookRun{
		{ID: "r1", PlaybookName: "deploy.yml", PlaybookHash: "h", Nodes: []string{"n"}, Status: "completed", StartedAt: base, TotalSteps: 1},
		{ID: "r2", PlaybookName: "deploy.yml", PlaybookHash: "h", Nodes: []string{"n"}, Status: "failed", StartedAt: base.Add(time.Second), TotalSteps: 1},
		{ID: "r3", PlaybookName: "backup.yml", PlaybookHash: "h", Nodes: []string{"n"}, Status: "running", StartedAt: base.Add(2 * time.Second), TotalSteps: 1},
	}
	for _, r := range runs {
		if err := CreatePlaybookRun(r); err != nil {
			t.Fatalf("CreatePlaybookRun(%s) failed: %v", r.ID, err)
		}
	}

	all, err := ListPlaybookRuns("", "", 10)
	if err != nil {
		t.Fatalf("ListPlaybookRuns all failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("all: got %d runs, want 3", len(all))
	}

	byName, err := ListPlaybookRuns("deploy.yml", "", 10)
	if err != nil {
		t.Fatalf("ListPlaybookRuns by name failed: %v", err)
	}
	if len(byName) != 2 {
		t.Errorf("by name: got %d runs, want 2", len(byName))
	}
	for _, r := range byName {
		if r.PlaybookName != "deploy.yml" {
			t.Errorf("by name: unexpected name %q", r.PlaybookName)
		}
	}

	byStatus, err := ListPlaybookRuns("", "failed", 10)
	if err != nil {
		t.Fatalf("ListPlaybookRuns by status failed: %v", err)
	}
	if len(byStatus) != 1 {
		t.Fatalf("by status: got %d runs, want 1", len(byStatus))
	}
	if byStatus[0].ID != "r2" {
		t.Errorf("by status: got ID %q, want r2", byStatus[0].ID)
	}

	limited, err := ListPlaybookRuns("", "", 1)
	if err != nil {
		t.Fatalf("ListPlaybookRuns limited failed: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("limited: got %d runs, want 1", len(limited))
	}
}

func TestUpsertStepState_Insert(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	step := &PlaybookStepState{
		RunID:      "run-1",
		NodeID:     "node-a",
		StepIndex:  0,
		StepName:   "install",
		Action:     "shell",
		Status:     "running",
		DurationMs: 120,
		ExitCode:   0,
		Stdout:     "out",
		Stderr:     "err",
		Error:      "",
		RetryCount: 1,
	}
	if err := UpsertStepState(step); err != nil {
		t.Fatalf("UpsertStepState failed: %v", err)
	}

	steps, err := GetStepStates("run-1", "node-a", "")
	if err != nil {
		t.Fatalf("GetStepStates failed: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}

	got := steps[0]
	if got.RunID != "run-1" {
		t.Errorf("RunID = %q, want run-1", got.RunID)
	}
	if got.NodeID != "node-a" {
		t.Errorf("NodeID = %q, want node-a", got.NodeID)
	}
	if got.StepIndex != 0 {
		t.Errorf("StepIndex = %d, want 0", got.StepIndex)
	}
	if got.StepName != "install" {
		t.Errorf("StepName = %q, want install", got.StepName)
	}
	if got.Action != "shell" {
		t.Errorf("Action = %q, want shell", got.Action)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if got.DurationMs != 120 {
		t.Errorf("DurationMs = %d, want 120", got.DurationMs)
	}
	if got.Stdout != "out" {
		t.Errorf("Stdout = %q, want out", got.Stdout)
	}
	if got.Stderr != "err" {
		t.Errorf("Stderr = %q, want err", got.Stderr)
	}
	if got.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", got.RetryCount)
	}
}

func TestUpsertStepState_Conflict(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	first := &PlaybookStepState{
		RunID:     "run-1",
		NodeID:    "node-a",
		StepIndex: 0,
		StepName:  "install",
		Action:    "shell",
		Status:    "running",
		Error:     "",
	}
	if err := UpsertStepState(first); err != nil {
		t.Fatalf("first UpsertStepState failed: %v", err)
	}

	second := &PlaybookStepState{
		RunID:     "run-1",
		NodeID:    "node-a",
		StepIndex: 0,
		StepName:  "install",
		Action:    "shell",
		Status:    "failed",
		Error:     "boom",
		ExitCode:  1,
	}
	if err := UpsertStepState(second); err != nil {
		t.Fatalf("second UpsertStepState failed: %v", err)
	}

	var count int
	err := GetGlobalDB().Connection().QueryRow(
		`SELECT COUNT(*) FROM playbook_step_states WHERE run_id = ? AND node_id = ? AND step_index = ?`,
		"run-1", "node-a", 0,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1", count)
	}

	steps, err := GetStepStates("run-1", "node-a", "")
	if err != nil {
		t.Fatalf("GetStepStates failed: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	if steps[0].Status != "failed" {
		t.Errorf("Status = %q, want failed", steps[0].Status)
	}
	if steps[0].Error != "boom" {
		t.Errorf("Error = %q, want boom", steps[0].Error)
	}
	if steps[0].ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", steps[0].ExitCode)
	}
}

func TestGetStepStates_Filters(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	steps := []*PlaybookStepState{
		{RunID: "run-1", NodeID: "node-a", StepIndex: 0, StepName: "s0", Action: "shell", Status: "completed"},
		{RunID: "run-1", NodeID: "node-a", StepIndex: 1, StepName: "s1", Action: "shell", Status: "failed"},
		{RunID: "run-1", NodeID: "node-b", StepIndex: 0, StepName: "s0", Action: "shell", Status: "pending"},
	}
	for _, s := range steps {
		if err := UpsertStepState(s); err != nil {
			t.Fatalf("UpsertStepState failed: %v", err)
		}
	}

	all, err := GetStepStates("run-1", "", "")
	if err != nil {
		t.Fatalf("GetStepStates all failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("all: got %d steps, want 3", len(all))
	}

	byNode, err := GetStepStates("run-1", "node-a", "")
	if err != nil {
		t.Fatalf("GetStepStates by node failed: %v", err)
	}
	if len(byNode) != 2 {
		t.Errorf("by node: got %d steps, want 2", len(byNode))
	}
	for _, s := range byNode {
		if s.NodeID != "node-a" {
			t.Errorf("by node: unexpected node %q", s.NodeID)
		}
	}

	incomplete, err := GetStepStates("run-1", "", "incomplete")
	if err != nil {
		t.Fatalf("GetStepStates incomplete failed: %v", err)
	}
	if len(incomplete) != 2 {
		t.Fatalf("incomplete: got %d steps, want 2", len(incomplete))
	}
	for _, s := range incomplete {
		if s.Status != "failed" && s.Status != "pending" {
			t.Errorf("incomplete: unexpected status %q", s.Status)
		}
	}
}

func TestComputePlaybookHash(t *testing.T) {
	h1 := ComputePlaybookHash("content", []string{"a", "b"})
	h2 := ComputePlaybookHash("content", []string{"a", "b"})
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("expected non-empty hash")
	}
	if len(h1) != 64 {
		t.Errorf("hash length = %d, want 64", len(h1))
	}

	h3 := ComputePlaybookHash("different content", []string{"a", "b"})
	if h1 == h3 {
		t.Error("different content produced same hash")
	}

	h4 := ComputePlaybookHash("content", []string{"a", "c"})
	if h1 == h4 {
		t.Error("different nodes produced same hash")
	}
}

func TestGlobalDB_NilSafety(t *testing.T) {
	globalDB = nil
	defer func() { globalDB = nil }()

	if err := CreatePlaybookRun(&PlaybookRun{ID: "x"}); err != nil {
		t.Errorf("CreatePlaybookRun: expected nil error, got %v", err)
	}
	if err := FinishPlaybookRun("x", "completed", 0, 0); err != nil {
		t.Errorf("FinishPlaybookRun: expected nil error, got %v", err)
	}
	if err := UpsertStepState(&PlaybookStepState{RunID: "x"}); err != nil {
		t.Errorf("UpsertStepState: expected nil error, got %v", err)
	}

	runs, err := ListPlaybookRuns("", "", 10)
	if err != nil || runs != nil {
		t.Errorf("ListPlaybookRuns: expected (nil, nil), got (%v, %v)", runs, err)
	}

	run, err := GetPlaybookRun("x")
	if err != nil || run != nil {
		t.Errorf("GetPlaybookRun: expected (nil, nil), got (%v, %v)", run, err)
	}

	steps, err := GetStepStates("x", "", "")
	if err != nil || steps != nil {
		t.Errorf("GetStepStates: expected (nil, nil), got (%v, %v)", steps, err)
	}
}
