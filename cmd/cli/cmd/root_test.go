package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"

	_ "github.com/mattn/go-sqlite3"
)

func writeTestNodesJSONCmd(t *testing.T, dir string, nodes []*common.NodeInfo) string {
	t.Helper()
	jsonPath := filepath.Join(dir, "nodes.json")
	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal nodes: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("failed to write nodes.json: %v", err)
	}
	return jsonPath
}

func TestPrintConflictReport(t *testing.T) {
	conflicts := []common.NodeConflict{
		{
			Type:        common.ConflictCrossSourceName,
			Description: "Same name 'web' but different IDs: DB=db-1, JSON=json-1",
			DBNode:      &common.NodeInfo{ID: "db-1", Name: "web", Address: "10.0.0.1"},
			JSONNode:    &common.NodeInfo{ID: "json-1", Name: "web", Address: "10.0.0.2"},
		},
		{
			Type:        common.ConflictCrossSourceIDFields,
			Description: "Same ID 'srv1' has different fields: port(22⇔2222), user(root⇔admin)",
			DBNode:      &common.NodeInfo{ID: "srv1", Name: "server", Address: "10.0.0.3", Port: 22, User: "root"},
			JSONNode:    &common.NodeInfo{ID: "srv1", Name: "server", Address: "10.0.0.3", Port: 2222, User: "admin"},
		},
	}

	common.PrintConflictReport(conflicts, 5, 3)
}

func TestRootCmdHasSubcommands(t *testing.T) {
	rootCmd := NewRootCmd()

	names := make(map[string]bool)
	for _, c := range rootCmd.Commands() {
		names[c.Name()] = true
	}

	expected := []string{"node", "exec", "file", "playbook", "settings", "ai", "history", "session", "async", "tui"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected subcommand %q in root command", name)
		}
	}
}
