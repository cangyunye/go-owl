package ai

import (
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/ai"
)

func testStore(t *testing.T) common.NodeStore {
	t.Helper()
	store := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	return store
}

func TestSetupSession_WithExplicitConfig(t *testing.T) {
	cfg := &ai.Config{AI: ai.AIConfig{
		Provider: "openai", Model: "gpt-4o",
		APIKey: "test-key", BaseURL: "http://localhost:1/v1", Timeout: 5,
	}}
	agent, gotCfg, err := SetupSession(testStore(t), cfg, false)
	if err != nil {
		t.Fatalf("SetupSession: %v", err)
	}
	if agent == nil {
		t.Fatal("agent is nil")
	}
	if gotCfg != cfg {
		t.Fatal("cfg should be passed through unchanged")
	}
}

func TestSetupSession_NilConfigLoadsFileOrDefault(t *testing.T) {
	agent, cfg, err := SetupSession(testStore(t), nil, false)
	if err != nil {
		t.Fatalf("SetupSession: %v", err)
	}
	if agent == nil {
		t.Fatal("agent is nil")
	}
	if cfg == nil || cfg.AI.Provider == "" {
		t.Fatal("cfg should be loaded from file or default")
	}
}
