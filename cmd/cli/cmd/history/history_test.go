package history

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/history"
)

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want string
	}{
		{"zero", 0, "N/A"},
		{"negative", -100, "N/A"},
		{"bytes", 500, "500 B"},
		{"KB", 2048, "2.0 KB"},
		{"MB", 1048576, "1.0 MB"},
		{"GB", 1073741824, "1.0 GB"},
		{"TB", 1099511627776, "1.0 TB"},
		{"exact 1KB", 1024, "1.0 KB"},
		{"fractional MB", 1572864, "1.5 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFileSize(tt.size)
			if got != tt.want {
				t.Errorf("formatFileSize(%d) = '%s', want '%s'", tt.size, got, tt.want)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantDur time.Duration
		wantErr bool
	}{
		{"empty", "", 0, false},
		{"hours", "2h", 2 * time.Hour, false},
		{"hours uppercase", "2H", 2 * time.Hour, false},
		{"days", "7d", 7 * 24 * time.Hour, false},
		{"days uppercase", "7D", 7 * 24 * time.Hour, false},
		{"minutes", "30m", 30 * time.Minute, false},
		{"seconds", "45s", 45 * time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.wantDur {
				t.Errorf("expected %v, got %v", tt.wantDur, got)
			}
		})
	}
}

func TestParseDuration_Fractional(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"1.5h", 1 * time.Hour},
		{"0.5d", 0},
		{"2.5H", 2 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestPrintVerboseDetails_EmptyRecord(t *testing.T) {
	var buf bytes.Buffer
	record := &history.Record{}

	printVerboseDetails(&buf, record)
}

func TestPrintVerboseDetails_CommandExecutions(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now()
	record := &history.Record{
		Operation: &history.Operation{
			TaskID:    "task-1",
			OpType:    "command",
			Command:   "echo hello",
			Status:    "completed",
			CreatedAt: now,
		},
		CommandExecutions: []*history.CommandExecution{
			{
				TaskID:     "task-1",
				NodeID:     "node-1",
				Command:    "echo hello",
				ExitCode:   0,
				DurationMs: 100,
				Success:    true,
			},
			{
				TaskID:     "task-1",
				NodeID:     "node-2",
				Command:    "echo hello",
				ExitCode:   1,
				DurationMs: 200,
				Success:    false,
			},
		},
	}

	printVerboseDetails(&buf, record)
	output := buf.String()

	if !strings.Contains(output, "Command Executions") {
		t.Error("expected 'Command Executions' section")
	}
	if !strings.Contains(output, "node-1") {
		t.Error("expected node-1 in output")
	}
	if !strings.Contains(output, "node-2") {
		t.Error("expected node-2 in output")
	}
	if !strings.Contains(output, "100ms") {
		t.Error("expected 100ms in output")
	}
}

func TestPrintVerboseDetails_FileTransfers(t *testing.T) {
	var buf bytes.Buffer
	record := &history.Record{
		Operation: &history.Operation{
			TaskID: "task-1",
			OpType: "file_transfer",
			Status: "completed",
		},
		Transfers: []*history.FileTransfer{
			{
				TaskID:       "task-1",
				NodeID:       "node-1",
				FileName:     "app.tar.gz",
				FileSize:     1048576,
				TransferType: "rsync",
				Status:       "completed",
				Progress:     100,
			},
			{
				TaskID:       "task-1",
				NodeID:       "node-2",
				FileName:     "app.tar.gz",
				FileSize:     1048576,
				TransferType: "scp",
				Status:       "failed",
				Error:        "connection refused",
			},
		},
	}

	printVerboseDetails(&buf, record)
	output := buf.String()

	if !strings.Contains(output, "File Transfers") {
		t.Error("expected 'File Transfers' section")
	}
	if !strings.Contains(output, "rsync") {
		t.Error("expected rsync in output")
	}
	if !strings.Contains(output, "1.0 MB") {
		t.Error("expected file size formatted in output")
	}
}

func TestPrintVerboseDetails_Communications(t *testing.T) {
	var buf bytes.Buffer
	record := &history.Record{
		Operation: &history.Operation{
			TaskID: "task-1",
			OpType: "node_manage",
			Status: "completed",
		},
		Communications: []*history.NodeCommunication{
			{
				TaskID:      "task-1",
				NodeID:      "node-1",
				Direction:   "send",
				MessageType: "ping",
				Success:     true,
			},
			{
				TaskID:      "task-1",
				NodeID:      "node-2",
				Direction:   "receive",
				MessageType: "pong",
				Success:     false,
				Error:       "timeout",
			},
		},
	}

	printVerboseDetails(&buf, record)
	output := buf.String()

	if !strings.Contains(output, "Node Communications") {
		t.Error("expected 'Node Communications' section")
	}
	if !strings.Contains(output, "send") {
		t.Error("expected 'send' direction in output")
	}
	if !strings.Contains(output, "receive") {
		t.Error("expected 'receive' direction in output")
	}
	if !strings.Contains(output, "ping") {
		t.Error("expected 'ping' in output")
	}
}

func TestPrintVerboseDetails_NilOperation(t *testing.T) {
	var buf bytes.Buffer
	record := &history.Record{}

	printVerboseDetails(&buf, record)
	if buf.Len() > 0 {
		t.Error("expected no output for nil operation")
	}
}

func TestPrintTable_Empty(t *testing.T) {
	var buf bytes.Buffer
	printTable(&buf, nil)
}

func TestPrintTable_Normal(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now()
	records := []*history.Record{
		{
			Operation: &history.Operation{
				TaskID:    "task-1",
				OpType:    "command",
				Command:   "echo hello",
				Targets:   []string{"node-1", "node-2"},
				Status:    "completed",
				CreatedAt: now,
			},
		},
	}

	printTable(&buf, records)
	output := buf.String()

	if !strings.Contains(output, "task-1") {
		t.Error("expected task-1 in output")
	}
	if !strings.Contains(output, "command") {
		t.Error("expected 'command' in output")
	}
	if !strings.Contains(output, "completed") {
		t.Error("expected 'completed' in output")
	}
	if !strings.Contains(output, "node-1") {
		t.Error("expected node-1 in targets")
	}
}

func TestPrintTable_WithCommandTruncation(t *testing.T) {
	var buf bytes.Buffer
	longCmd := strings.Repeat("a", 100)
	record := &history.Record{
		Operation: &history.Operation{
			TaskID:  "task-1",
			Command: longCmd,
			Status:  "completed",
		},
	}

	printTable(&buf, []*history.Record{record})
	output := buf.String()
	if strings.Contains(output, longCmd) {
		t.Error("expected command to be truncated")
	}
	if !strings.Contains(output, "...") {
		t.Error("expected truncation indicator")
	}
}
