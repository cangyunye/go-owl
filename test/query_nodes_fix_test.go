package test

import (
	"context"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/common/model"
)

// mockNodeMgrForFixTest 是用于测试修复逻辑的模拟节点管理器
type mockNodeMgrForFixTest struct {
	nodes []*model.Node
}

func newMockNodeMgrForFixTest() *mockNodeMgrForFixTest {
	return &mockNodeMgrForFixTest{
		nodes: []*model.Node{
			{ID: "1", Name: "web-server-01", Address: "192.168.1.101", Port: 22, Status: "online", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
			{ID: "2", Name: "web-server-02", Address: "192.168.1.102", Port: 22, Status: "online", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
			{ID: "3", Name: "db-server-01", Address: "192.168.1.201", Port: 22, Status: "online", Groups: []string{"db"}, Labels: map[string]string{"env": "prod"}},
			{ID: "4", Name: "db-server-02", Address: "192.168.1.202", Port: 22, Status: "offline", Groups: []string{"db"}, Labels: map[string]string{"env": "test"}},
			{ID: "5", Name: "test-node-01", Address: "192.168.1.301", Port: 22, Status: "online", Groups: []string{"test"}, Labels: map[string]string{"env": "test"}},
			{ID: "6", Name: "cache-server-01", Address: "192.168.1.401", Port: 22, Status: "online", Groups: []string{"cache"}, Labels: map[string]string{"env": "prod"}},
		},
	}
}

func (m *mockNodeMgrForFixTest) Register(node *model.Node) error      { return nil }
func (m *mockNodeMgrForFixTest) Unregister(id string) error          { return nil }
func (m *mockNodeMgrForFixTest) GetByID(id string) (*model.Node, error) {
	for _, n := range m.nodes {
		if n.ID == id || n.Name == id {
			return n, nil
		}
	}
	return nil, nil
}
func (m *mockNodeMgrForFixTest) UpdateStatus(id string, status model.NodeStatus) error { return nil }
func (m *mockNodeMgrForFixTest) GetOnlineNodes() []*model.Node                         { return nil }
func (m *mockNodeMgrForFixTest) Count() int                                            { return len(m.nodes) }
func (m *mockNodeMgrForFixTest) GetByLabels(labels map[string]string) []*model.Node {
	result := make([]*model.Node, 0)
	for _, n := range m.nodes {
		match := true
		for k, v := range labels {
			if n.Labels[k] != v {
				match = false
				break
			}
		}
		if match {
			result = append(result, n)
		}
	}
	return result
}
func (m *mockNodeMgrForFixTest) SearchByName(pattern string) []*model.Node {
	if pattern == "" {
		return nil
	}
	result := make([]*model.Node, 0)
	lowerPattern := strings.ToLower(pattern)
	for _, n := range m.nodes {
		if strings.Contains(strings.ToLower(n.Name), lowerPattern) {
			result = append(result, n)
		}
	}
	return result
}
func (m *mockNodeMgrForFixTest) SearchByAddress(pattern string) []*model.Node { return nil }
func (m *mockNodeMgrForFixTest) List() []*model.Node                          { return m.nodes }
func (m *mockNodeMgrForFixTest) GetByGroup(group string) []*model.Node {
	result := make([]*model.Node, 0)
	for _, n := range m.nodes {
		for _, g := range n.Groups {
			if g == group {
				result = append(result, n)
				break
			}
		}
	}
	return result
}
func (m *mockNodeMgrForFixTest) Refresh() error { return nil }

// TestQueryNodes_FixSearchBeforeFilter 验证修复后的查询逻辑：先搜索后过滤
func TestQueryNodes_FixSearchBeforeFilter(t *testing.T) {
	mgr := newMockNodeMgrForFixTest()
	tool := ai.NewQueryNodesTool(mgr, nil)
	ctx := context.Background()

	// 测试场景1: 搜索"server"应该返回所有包含"server"的节点（4个）
	t.Run("Search only - should find all servers", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"search": "server"})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		// web-server-01, web-server-02, db-server-01, db-server-02, cache-server-01 = 5个
		if !strings.Contains(result, "web-server-01") ||
			!strings.Contains(result, "web-server-02") ||
			!strings.Contains(result, "db-server-01") ||
			!strings.Contains(result, "db-server-02") ||
			!strings.Contains(result, "cache-server-01") {
			t.Errorf("Expected search 'server' to find all server nodes, got: %s", result)
		}
	})

	// 测试场景2: 搜索"server" + 过滤 env=test
	// 预期结果: db-server-02（因为它是唯一包含"server"且env=test的节点）
	t.Run("Search server + filter env=test", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"search": "server",
			"labels": map[string]interface{}{"env": "test"},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		// 应该找到 db-server-02（包含server且env=test）
		if !strings.Contains(result, "db-server-02") {
			t.Errorf("Expected search 'server' + env=test to find db-server-02, got: %s", result)
		}
		// 不应该找到 web-server-01/02（它们是env=prod）
		if strings.Contains(result, "web-server-01") || strings.Contains(result, "web-server-02") {
			t.Errorf("Expected search 'server' + env=test NOT to find web-servers, got: %s", result)
		}
	})

	// 测试场景3: 搜索"node" + 过滤 env=prod
	// 预期结果: 空（test-node-01是唯一包含"node"的，但它是env=test）
	t.Run("Search node + filter env=prod - should return empty", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"search": "node",
			"labels": map[string]interface{}{"env": "prod"},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !strings.Contains(result, "No matching nodes found") {
			t.Errorf("Expected search 'node' + env=prod to return empty, got: %s", result)
		}
	})

	// 测试场景4: 搜索"db" + 过滤 group=db
	// 预期结果: db-server-01, db-server-02
	t.Run("Search db + filter group=db", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"search": "db",
			"group":  "db",
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !strings.Contains(result, "db-server-01") || !strings.Contains(result, "db-server-02") {
			t.Errorf("Expected search 'db' + group=db to find db-servers, got: %s", result)
		}
	})

	// 测试场景5: 搜索"cache" + 过滤 status=online
	// 预期结果: cache-server-01
	t.Run("Search cache + filter status=online", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"search": "cache",
			"status": "online",
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !strings.Contains(result, "cache-server-01") {
			t.Errorf("Expected search 'cache' + status=online to find cache-server-01, got: %s", result)
		}
	})

	// 测试场景6: 验证搜索顺序修复 - 先搜索所有节点，再过滤
	t.Run("Verify search order fix", func(t *testing.T) {
		// 这个测试验证修复的核心逻辑
		// 场景：假设用户想搜索"server"但只在test环境中搜索
		// 修复前：先过滤env=test，得到 test-node-01，再搜索"server" -> 空结果
		// 修复后：先搜索所有节点找"server"，得到5个节点，再过滤env=test -> db-server-02

		result, err := tool.Execute(ctx, map[string]interface{}{
			"search": "server",
			"labels": map[string]interface{}{"env": "test"},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// 修复后应该找到 db-server-02（因为它包含"server"且env=test）
		if !strings.Contains(result, "db-server-02") {
			t.Errorf("BUG: Search order is wrong! Expected to find db-server-02, but got: %s", result)
		}
	})

	// 测试场景7: 按地址搜索
	t.Run("Search by address", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"search": "192.168.1.20"})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !strings.Contains(result, "db-server-01") || !strings.Contains(result, "db-server-02") {
			t.Errorf("Expected search by address to find db-servers, got: %s", result)
		}
	})
}
