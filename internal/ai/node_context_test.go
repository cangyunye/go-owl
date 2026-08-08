package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

func nodeContextTestMgr() *mockNodeMgrForAI {
	return &mockNodeMgrForAI{nodes: []*model.Node{
		{Name: "web-01", Address: "10.0.0.1", Port: 22, Status: "online", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
		{Name: "web-02", Address: "10.0.0.2", Port: 22, Status: "online", Groups: []string{"web"}, Labels: map[string]string{"env": "dev"}},
		{Name: "db-01", Address: "10.0.0.3", Port: 22, Status: "online", Groups: []string{"db"}, Labels: map[string]string{"env": "prod"}},
	}}
}

func TestResolveToolTargets(t *testing.T) {
	agent := &Agent{nodeMgr: nodeContextTestMgr()}

	tests := []struct {
		name   string
		args   map[string]interface{}
		want   []string
		wantSrc string
	}{
		{"explicit nodes", map[string]interface{}{"nodes": []interface{}{"web-01", "db-01"}}, []string{"web-01", "db-01"}, "nodes=web-01,db-01"},
		{"ALL_NODES", map[string]interface{}{"nodes": []interface{}{"ALL_NODES"}}, []string{"web-01", "web-02", "db-01"}, "全部节点"},
		{"multi group", map[string]interface{}{"group": "web,db"}, []string{"web-01", "web-02", "db-01"}, "group=web,db"},
		{"single group", map[string]interface{}{"group": "web"}, []string{"web-01", "web-02"}, "group=web"},
		{"label", map[string]interface{}{"label": "env=prod"}, []string{"web-01", "db-01"}, "label=env=prod"},
		{"search", map[string]interface{}{"search": "web"}, []string{"web-01", "web-02"}, "search=web"},
		{"no target", map[string]interface{}{}, []string{"web-01", "web-02", "db-01"}, "全部节点"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, src := agent.resolveToolTargets(ToolCall{Name: "query_nodes", Arguments: tt.args})
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
			if !strings.Contains(src, tt.wantSrc) {
				t.Errorf("expected source to contain %q, got %q", tt.wantSrc, src)
			}
		})
	}
}

// TestSessionNodeContextSaved 验证工具执行后会话保存节点上下文（程序层确定性）。
func TestSessionNodeContextSaved(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"node_list",
		"```json\n{\"tool_calls\":[{\"name\":\"query_nodes\",\"arguments\":{\"group\":\"web\"}}]}\n```",
	})
	agent.nodeMgr = nodeContextTestMgr()
	sess := NewSession(agent)
	ctx := context.Background()

	if _, err := sess.Send(ctx, "列出 web 组的节点"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if sess.nodeContext == nil {
		t.Fatal("expected node context to be saved")
	}
	if strings.Join(sess.nodeContext.Nodes, ",") != "web-01,web-02" {
		t.Errorf("expected web nodes, got %v", sess.nodeContext.Nodes)
	}
	if !strings.Contains(sess.nodeContext.Source, "web") {
		t.Errorf("expected source to mention group, got %q", sess.nodeContext.Source)
	}
}

// TestSessionNodeContextOverwrite 验证新一轮节点查询覆盖旧上下文。
func TestSessionNodeContextOverwrite(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"node_list",
		"```json\n{\"tool_calls\":[{\"name\":\"query_nodes\",\"arguments\":{\"group\":\"web\"}}]}\n```",
		"node_list",
		"```json\n{\"tool_calls\":[{\"name\":\"query_nodes\",\"arguments\":{\"search\":\"db\"}}]}\n```",
	})
	agent.nodeMgr = nodeContextTestMgr()
	sess := NewSession(agent)
	ctx := context.Background()

	if _, err := sess.Send(ctx, "列出 web 组的节点"); err != nil {
		t.Fatalf("Send 1 failed: %v", err)
	}
	if strings.Join(sess.nodeContext.Nodes, ",") != "web-01,web-02" {
		t.Fatalf("expected web nodes first, got %v", sess.nodeContext.Nodes)
	}

	if _, err := sess.Send(ctx, "搜索 db 节点"); err != nil {
		t.Fatalf("Send 2 failed: %v", err)
	}
	if strings.Join(sess.nodeContext.Nodes, ",") != "db-01" {
		t.Errorf("expected node context overwritten to db-01, got %v", sess.nodeContext.Nodes)
	}
}

// TestSessionNodeContextMemory 验证上下文注入 buildMemory 供跨轮复用。
func TestSessionNodeContextMemory(t *testing.T) {
	agent := newTestAgentForRoute(nil)
	agent.nodeMgr = nodeContextTestMgr()
	sess := NewSession(agent)

	// 手动设置上下文（模拟工具执行后的回调）
	sess.nodeContext = &NodeContext{Nodes: []string{"web-01", "web-02"}, Source: "group=web 的节点"}

	mem := sess.buildMemory()
	if !strings.Contains(mem, "web-01") || !strings.Contains(mem, "web-02") {
		t.Errorf("expected node context in memory, got %q", mem)
	}
	if !strings.Contains(mem, "上一轮节点上下文") {
		t.Errorf("expected node context marker, got %q", mem)
	}

	sess.nodeContext = nil
	if mem := sess.buildMemory(); strings.Contains(mem, "web-01") {
		t.Errorf("expected no node context after reset, got %q", mem)
	}
}

// TestExecuteToolCallTriggersHook 验证 executeToolCall 成功后触发节点上下文回调。
func TestExecuteToolCallTriggersHook(t *testing.T) {
	agent := newTestAgentForRoute(nil)
	agent.nodeMgr = nodeContextTestMgr()

	var got []string
	agent.SetNodeContextHook(func(nodes []string, source string) {
		got = nodes
	})

	_, err := agent.ExecuteToolCall(context.Background(), ToolCall{
		Name:      "query_nodes",
		Arguments: map[string]interface{}{"group": "db"},
	})
	if err != nil {
		t.Fatalf("ExecuteToolCall failed: %v", err)
	}
	if strings.Join(got, ",") != "db-01" {
		t.Errorf("expected hook to receive db-01, got %v", got)
	}
}

// TestHookNotTriggeredForNonNodeTools 验证非节点工具不触发回调。
func TestHookNotTriggeredForNonNodeTools(t *testing.T) {
	agent := newTestAgentForRoute(nil)
	called := false
	agent.SetNodeContextHook(func(nodes []string, source string) {
		called = true
	})
	_, _ = agent.ExecuteToolCall(context.Background(), ToolCall{
		Name:      "list_playbooks",
		Arguments: map[string]interface{}{},
	})
	if called {
		t.Error("list_playbooks should not trigger node context hook")
	}
}
