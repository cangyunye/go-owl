package ai

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/control/playbook"
)

// SetupSession 装配 owl ai 的完整依赖链: 节点桥/管理器/CLI 执行器/LLM 配置/Agent。
// owl ai 命令与 TUI AI 面板共用此入口,避免装配逻辑漂移。
// cfg 为 nil 时自动从 ~/.owl/config.yaml 加载(含环境变量回退)。
func SetupSession(store common.NodeStore, cfg *ai.Config, verbose bool) (*ai.Agent, *ai.Config, error) {
	ai.SetLogVerbose(verbose)
	ai.SetLLMLogVerbose(verbose)

	nodeStoreAdapter := createBridgeAdapter(store)
	bridge := ai.NewNodeStoreBridge()
	if err := bridge.SyncFromStore(nodeStoreAdapter); err != nil {
		return nil, nil, fmt.Errorf("同步节点数据失败: %w", err)
	}

	nodeMgr := ai.InitNodeManager(bridge)
	if nodeMgr == nil {
		return nil, nil, fmt.Errorf("节点管理器初始化失败")
	}

	if cfg == nil {
		home, _ := os.UserHomeDir()
		fileConfig, err := ai.LoadConfig(filepath.Join(home, ".owl", "config.yaml"))
		if err != nil {
			fileConfig = ai.DefaultConfig()
		}
		cfg = fileConfig
	}

	executor := ai.NewCLIExecutor(nodeMgr, nodeStoreAdapter)
	playbookParser := playbook.NewParser()
	agent, err := ai.NewAgent(executor, cfg, nodeMgr, nodeStoreAdapter, playbookParser, verbose)
	if err != nil {
		return nil, nil, err
	}
	return agent, cfg, nil
}
