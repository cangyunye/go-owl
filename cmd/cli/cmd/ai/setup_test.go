package ai

import (
	"fmt"
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
	// 密闭:不依赖真实 ~/.owl/config.yaml——真实环境里该文件可能被
	// serve/settings 流程写入无 ai.provider 的内容,导致默认值断言失败
	t.Setenv("HOME", t.TempDir())
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

type errStore struct{ common.NodeStore }

func (errStore) List() ([]*common.NodeInfo, error) {
	return nil, fmt.Errorf("list boom")
}

func TestSetupSession_StoreListError(t *testing.T) {
	_, _, err := SetupSession(errStore{}, nil, false)
	if err == nil {
		t.Fatal("expected error from store list failure")
	}
}
