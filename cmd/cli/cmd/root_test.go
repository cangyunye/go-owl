package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"

	_ "modernc.org/sqlite"
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

func TestRootCmdHasSubcommands(t *testing.T) {
	rootCmd := NewRootCmd()

	names := make(map[string]bool)
	for _, c := range rootCmd.Commands() {
		names[c.Name()] = true
	}

	expected := append([]string{"node", "exec", "file", "playbook", "settings", "ai", "history", "session", "async", "serve", "metrics"}, extraRootCommands...)
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected subcommand %q in root command", name)
		}
	}
	if len(names) != len(expected) {
		t.Errorf("expected %d subcommands, got %d", len(expected), len(names))
	}
}
