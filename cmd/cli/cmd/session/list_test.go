package session

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/history"
	_ "github.com/mattn/go-sqlite3"
)

func TestPrintSessionList_Empty(t *testing.T) {
	var buf bytes.Buffer
	printSessionList(&buf, nil)

	output := buf.String()
	if !strings.Contains(output, "暂无历史会话记录") {
		t.Error("expected '暂无历史会话记录' for empty list")
	}
	if !strings.Contains(output, "owl session attach") {
		t.Error("expected hint about 'owl session attach'")
	}
}

func TestPrintSessionList_Normal(t *testing.T) {
	var buf bytes.Buffer
	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.Local)

	sessions := []*history.Session{
		{
			ID:           "sess-20260523-100001",
			Mode:         "single",
			NodeIDs:      []string{"web-server-01"},
			Status:       "closed",
			CreatedAt:    now,
			CommandCount: 12,
			SuccessCount: 12,
			ErrorCount:   0,
		},
		{
			ID:           "sess-20260523-090001",
			Mode:         "multiple",
			NodeIDs:      []string{"web-01", "web-02", "db-01"},
			Status:       "closed",
			CreatedAt:    now.Add(-1 * time.Hour),
			CommandCount: 8,
			SuccessCount: 7,
			ErrorCount:   1,
		},
	}

	printSessionList(&buf, sessions)
	output := buf.String()

	if !strings.Contains(output, "sess-20260523-100001") {
		t.Error("expected sess-20260523-100001 in output")
	}
	if !strings.Contains(output, "sess-20260523-090001") {
		t.Error("expected sess-20260523-090001 in output")
	}
	if !strings.Contains(output, "single") {
		t.Error("expected 'single' mode in output")
	}
	if !strings.Contains(output, "multiple") {
		t.Error("expected 'multiple' mode in output")
	}
	if !strings.Contains(output, "web-server-01") {
		t.Error("expected node web-server-01 in output")
	}
	if !strings.Contains(output, "web-01") {
		t.Error("expected node web-01 in output")
	}
	if !strings.Contains(output, "100%") {
		t.Error("expected 100% success rate")
	}
	if !strings.Contains(output, "88%") {
		t.Error("expected 88% success rate (7/8)")
	}
	if !strings.Contains(output, "查看会话详情") {
		t.Error("expected hint for viewing details")
	}
}

func TestPrintSessionList_ActiveStatus(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now()

	sessions := []*history.Session{
		{
			ID:        "sess-active-001",
			Mode:      "single",
			NodeIDs:   []string{"node-01"},
			Status:    "active",
			CreatedAt: now,
		},
		{
			ID:        "sess-timeout-001",
			Mode:      "multiple",
			NodeIDs:   []string{"node-02", "node-03"},
			Status:    "timeout",
			CreatedAt: now,
		},
		{
			ID:        "sess-closed-001",
			Mode:      "single",
			NodeIDs:   []string{"node-04"},
			Status:    "closed",
			CreatedAt: now,
		},
	}

	printSessionList(&buf, sessions)
	output := buf.String()

	if !strings.Contains(output, "● active") {
		t.Error("expected '● active' for active status")
	}
	if !strings.Contains(output, "◌ timeout") {
		t.Error("expected '◌ timeout' for timeout status")
	}
	if !strings.Contains(output, "○ closed") {
		t.Error("expected '○ closed' for closed status")
	}
}

func TestPrintSessionList_ZeroCommands(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now()

	sessions := []*history.Session{
		{
			ID:           "sess-no-cmd",
			Mode:         "single",
			NodeIDs:      []string{"node-01"},
			Status:       "closed",
			CreatedAt:    now,
			CommandCount: 0,
			SuccessCount: 0,
			ErrorCount:   0,
		},
	}

	printSessionList(&buf, sessions)
	output := buf.String()

	if !strings.Contains(output, "N/A") {
		t.Error("expected 'N/A' success rate when no commands executed")
	}
	if strings.Contains(output, "NaN") || strings.Contains(output, "0%") {
		t.Error("should not show percentage when no commands")
	}
}

type testDB struct {
	conn *sql.DB
}

func (d *testDB) Connection() *sql.DB     { return d.conn }
func (d *testDB) InitSchema() error        { return nil }
func (d *testDB) Close() error             { return d.conn.Close() }
func (d *testDB) Cleanup(int) error        { return nil }

func setupTestDB(t *testing.T) func() {
	t.Helper()

	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	_, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			mode TEXT,
			node_ids TEXT,
			status TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			closed_at DATETIME,
			command_count INTEGER DEFAULT 0,
			success_count INTEGER DEFAULT 0,
			error_count INTEGER DEFAULT 0
		)
	`)
	if err != nil {
		conn.Close()
		t.Fatalf("failed to create sessions table: %v", err)
	}

	history.SetGlobalDB(&testDB{conn: conn})
	return func() {
		history.SetGlobalDB(nil)
		conn.Close()
	}
}

func insertTestSession(t *testing.T, s *history.Session) {
	t.Helper()

	nodeIDsJSON, err := json.Marshal(s.NodeIDs)
	if err != nil {
		t.Fatalf("failed to marshal node ids: %v", err)
	}

	var closedAt interface{}
	if s.ClosedAt != nil {
		closedAt = s.ClosedAt
	}

	_, err = history.GetGlobalDB().Connection().Exec(
		`INSERT INTO sessions (id, mode, node_ids, status, created_at, closed_at, command_count, success_count, error_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Mode, nodeIDsJSON, s.Status, s.CreatedAt, closedAt, s.CommandCount, s.SuccessCount, s.ErrorCount,
	)
	if err != nil {
		t.Fatalf("failed to insert test session: %v", err)
	}
}

func TestPrintSessionList_FromDB(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	closedTime := now.Add(-30 * time.Minute)

	insertTestSession(t, &history.Session{
		ID:           "sess-db-001",
		Mode:         "single",
		NodeIDs:      []string{"node-01"},
		Status:       "closed",
		CreatedAt:    now.Add(-1 * time.Hour),
		ClosedAt:     &closedTime,
		CommandCount: 5,
		SuccessCount: 5,
		ErrorCount:   0,
	})
	insertTestSession(t, &history.Session{
		ID:           "sess-db-002",
		Mode:         "multiple",
		NodeIDs:      []string{"node-02", "node-03"},
		Status:       "closed",
		CreatedAt:    now.Add(-2 * time.Hour),
		ClosedAt:     &closedTime,
		CommandCount: 10,
		SuccessCount: 8,
		ErrorCount:   2,
	})
	insertTestSession(t, &history.Session{
		ID:           "sess-db-003",
		Mode:         "single",
		NodeIDs:      []string{"node-04"},
		Status:       "active",
		CreatedAt:    now,
		CommandCount: 0,
		SuccessCount: 0,
		ErrorCount:   0,
	})

	sessions, err := history.QuerySessions(100)
	if err != nil {
		t.Fatalf("failed to query sessions: %v", err)
	}

	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	var buf bytes.Buffer
	printSessionList(&buf, sessions)
	output := buf.String()

	if !strings.Contains(output, "sess-db-001") {
		t.Error("expected sess-db-001 in output")
	}
	if !strings.Contains(output, "sess-db-002") {
		t.Error("expected sess-db-002 in output")
	}
	if !strings.Contains(output, "sess-db-003") {
		t.Error("expected sess-db-003 in output")
	}
	if !strings.Contains(output, "node-01") {
		t.Error("expected node-01 in output")
	}
	if !strings.Contains(output, "node-02,node-03") {
		t.Error("expected multi-node in output")
	}
	if !strings.Contains(output, "100%") {
		t.Error("expected 100% for sess-db-001")
	}
	if !strings.Contains(output, "80%") {
		t.Error("expected 80% for sess-db-002")
	}
	if !strings.Contains(output, "● active") {
		t.Error("expected ● active for sess-db-003")
	}
	if !strings.Contains(output, "○ closed") {
		t.Error("expected ○ closed for closed sessions")
	}
}

func TestPrintSessionList_EmptyDB(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sessions, err := history.QuerySessions(100)
	if err != nil {
		t.Fatalf("failed to query sessions: %v", err)
	}

	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions in empty DB, got %d", len(sessions))
	}

	var buf bytes.Buffer
	printSessionList(&buf, sessions)
	output := buf.String()

	if !strings.Contains(output, "暂无历史会话记录") {
		t.Error("expected '暂无历史会话记录' for empty DB")
	}
}

func TestPrintSessionList_SuccessRateCalculation(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now()

	tests := []struct {
		success, total int
		expected       string
	}{
		{0, 0, "N/A"},
		{10, 10, "100%"},
		{5, 10, "50%"},
		{1, 3, "33%"},
		{2, 3, "67%"},
		{99, 100, "99%"},
	}

	for i, tt := range tests {
		sessionID := "sess-rate-" + string(rune('a'+i))
		sessions := []*history.Session{
			{
				ID:           sessionID,
				Mode:         "single",
				NodeIDs:      []string{"node-01"},
				Status:       "closed",
				CreatedAt:    now,
				CommandCount: tt.total,
				SuccessCount: tt.success,
				ErrorCount:   tt.total - tt.success,
			},
		}

		buf.Reset()
		printSessionList(&buf, sessions)
		output := buf.String()

		if !strings.Contains(output, tt.expected) {
			t.Errorf("for success=%d total=%d: expected '%s' in output but got:\n%s",
				tt.success, tt.total, tt.expected, output)
		}
	}
}

func TestPrintSessionList_MultipleNodesFormat(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now()

	sessions := []*history.Session{
		{
			ID:        "sess-multi",
			Mode:      "multiple",
			NodeIDs:   []string{"web-01", "web-02", "web-03", "db-master", "db-slave"},
			Status:    "closed",
			CreatedAt: now,
		},
	}

	printSessionList(&buf, sessions)
	output := buf.String()

	if !strings.Contains(output, "web-01,web-02,web-03,db-master,db-slave") {
		t.Error("expected all node IDs joined by comma")
	}
}

func TestPrintSessionList_TableHeaders(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now()

	sessions := []*history.Session{
		{
			ID:        "sess-headers-001",
			Mode:      "single",
			NodeIDs:   []string{"node-01"},
			Status:    "closed",
			CreatedAt: now,
		},
	}

	printSessionList(&buf, sessions)
	output := buf.String()

	expectedHeaders := []string{"会话 ID", "模式", "节点", "状态", "创建时间", "命令数", "成功率"}
	for _, header := range expectedHeaders {
		if !strings.Contains(output, header) {
			t.Errorf("expected header '%s' in output", header)
		}
	}
}

func TestNewListCmd(t *testing.T) {
	cmd := NewListCmd()

	if cmd.Use != "list" {
		t.Errorf("expected Use='list', got '%s'", cmd.Use)
	}
	if cmd.Short != "列出历史会话记录" {
		t.Errorf("expected Short='列出历史会话记录', got '%s'", cmd.Short)
	}
	if cmd.RunE == nil {
		t.Error("expected RunE to be set")
	}
}

func TestPrintSessionList_DateFormats(t *testing.T) {
	var buf bytes.Buffer

	sessions := []*history.Session{
		{
			ID:        "sess-date-001",
			Mode:      "single",
			NodeIDs:   []string{"node-01"},
			Status:    "closed",
			CreatedAt: time.Date(2026, 5, 23, 14, 30, 0, 0, time.Local),
		},
	}

	printSessionList(&buf, sessions)
	output := buf.String()

	if !strings.Contains(output, "2026-05-23 14:30") {
		t.Errorf("expected date format '2026-05-23 14:30' in output, got:\n%s", output)
	}
}

func TestPrintSessionList_SingleNode(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now()

	sessions := []*history.Session{
		{
			ID:        "sess-single-node",
			Mode:      "single",
			NodeIDs:   []string{"production-web-01"},
			Status:    "active",
			CreatedAt: now,
		},
	}

	printSessionList(&buf, sessions)
	output := buf.String()

	if !strings.Contains(output, "production-web-01") {
		t.Error("expected single node ID in output")
	}
}

func TestRunList_NoGlobalDB(t *testing.T) {
	history.SetGlobalDB(nil)

	cmd := NewListCmd()
	cmd.SetOut(new(bytes.Buffer))
	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Errorf("expected no error when DB not initialized, got: %v", err)
	}
}

func TestRunList_WithDB(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	insertTestSession(t, &history.Session{
		ID:           "sess-runlist-001",
		Mode:         "single",
		NodeIDs:      []string{"test-node"},
		Status:       "closed",
		CreatedAt:    now,
		CommandCount: 3,
		SuccessCount: 3,
		ErrorCount:   0,
	})

	cmd := NewListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("runList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "sess-runlist-001") {
		t.Error("expected session ID in output")
	}
	if !strings.Contains(output, "100%") {
		t.Error("expected success rate in output")
	}
	if !strings.Contains(output, "查看会话详情") {
		t.Error("expected hint at end of output")
	}
}

func TestRunList_EmptyDBWithCommand(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	cmd := NewListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("runList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "暂无历史会话记录") {
		t.Error("expected empty message for empty DB")
	}
	if !strings.Contains(output, "owl session attach") {
		t.Error("expected hint for creating new session")
	}
}

func TestPrintSessionList_NilSlice(t *testing.T) {
	var buf bytes.Buffer

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("printSessionList panicked on nil slice: %v", r)
		}
	}()

	printSessionList(&buf, nil)
	output := buf.String()

	if !strings.Contains(output, "暂无历史会话记录") {
		t.Error("expected empty message for nil sessions")
	}
}

func TestPrintSessionList_EmptySlice(t *testing.T) {
	var buf bytes.Buffer
	printSessionList(&buf, []*history.Session{})
	output := buf.String()

	if !strings.Contains(output, "暂无历史会话记录") {
		t.Error("expected empty message for empty slice")
	}
}

func TestRunList_RealOutput(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	closedTime := now.Add(-10 * time.Minute)

	insertTestSession(t, &history.Session{
		ID:           "sess-real-001",
		Mode:         "multiple",
		NodeIDs:      []string{"web-prod-01", "web-prod-02", "db-prod-01"},
		Status:       "closed",
		CreatedAt:    now.Add(-12 * time.Hour),
		ClosedAt:     &closedTime,
		CommandCount: 25,
		SuccessCount: 24,
		ErrorCount:   1,
	})
	insertTestSession(t, &history.Session{
		ID:           "sess-real-002",
		Mode:         "single",
		NodeIDs:      []string{"dev-server"},
		Status:       "active",
		CreatedAt:    now.Add(-30 * time.Minute),
		CommandCount: 7,
		SuccessCount: 7,
		ErrorCount:   0,
	})

	cmd := NewListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("runList failed: %v", err)
	}

	output := buf.String()
	t.Logf("Full output:\n%s", output)

	checks := []struct {
		desc string
		str  string
	}{
		{"session sess-real-001", "sess-real-001"},
		{"session sess-real-002", "sess-real-002"},
		{"mode multiple", "multiple"},
		{"mode single", "single"},
		{"node web-prod-01", "web-prod-01"},
		{"node db-prod-01", "db-prod-01"},
		{"node dev-server", "dev-server"},
		{"closed status", "○ closed"},
		{"active status", "● active"},
		{"success rate 96%", "96%"},
		{"success rate 100%", "100%"},
		{"command count 25", "25"},
		{"command count 7", "7"},
		{"detail hint", "查看会话详情"},
	}

	for _, c := range checks {
		if !strings.Contains(output, c.str) {
			t.Errorf("expected '%s' (%s) in output", c.str, c.desc)
		}
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}
	if nonEmptyLines < 5 {
		t.Errorf("expected at least 5 non-empty output lines, got %d", nonEmptyLines)
	}
}

func init() {
	history.SetGlobalDB(nil)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
