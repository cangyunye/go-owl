package test

import (
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/test/testdata"
)

// 此前本文件通篇只有 t.Logf、零断言，永远绿——测试数据集被破坏
// 时无法发现。以下断言数据集不变量与过滤器正确性。

func TestNodeDataSet(t *testing.T) {
	if len(testdata.TestNodes) == 0 {
		t.Fatal("测试节点数据集不应为空")
	}

	seen := map[string]bool{}
	for _, n := range testdata.TestNodes {
		if n.ID == "" {
			t.Errorf("节点 %q 缺少 ID", n.Name)
		}
		if seen[n.ID] {
			t.Errorf("节点 ID 重复: %s", n.ID)
		}
		seen[n.ID] = true
	}

	// 组过滤器：只返回声明了该组的节点
	for _, group := range []string{"web", "db", "test"} {
		nodes := testdata.GetNodesByGroup(group)
		if len(nodes) == 0 {
			t.Errorf("分组 %q 应有测试节点", group)
		}
		for _, n := range nodes {
			has := false
			for _, g := range n.Groups {
				if g == group {
					has = true
				}
			}
			if !has {
				t.Errorf("GetNodesByGroup(%q) 返回了不含该组的节点 %q", group, n.ID)
			}
		}
	}

	// 标签过滤器：只返回标签匹配的节点
	for _, kv := range [][2]string{{"env", "test"}, {"env", "prod"}, {"os", "linux"}, {"os", "windows"}} {
		nodes := testdata.GetNodesByLabel(kv[0], kv[1])
		for _, n := range nodes {
			if n.Labels[kv[0]] != kv[1] {
				t.Errorf("GetNodesByLabel(%s=%s) 返回了不匹配的节点 %q（标签=%v）",
					kv[0], kv[1], n.ID, n.Labels)
			}
		}
	}

	// 状态过滤器：三类状态计数之和等于总数，且每类只含该状态节点
	byStatus := map[model.NodeStatus][]*model.Node{
		model.NodeStatusOnline:  testdata.GetNodesByStatus(model.NodeStatusOnline),
		model.NodeStatusOffline: testdata.GetNodesByStatus(model.NodeStatusOffline),
		model.NodeStatusUnknown: testdata.GetNodesByStatus(model.NodeStatusUnknown),
	}
	sum := 0
	for status, nodes := range byStatus {
		sum += len(nodes)
		for _, n := range nodes {
			if n.Status != status {
				t.Errorf("GetNodesByStatus(%s) 返回了状态 %s 的节点 %q", status, n.Status, n.ID)
			}
		}
	}
	if sum != len(testdata.TestNodes) {
		t.Errorf("三类状态计数之和 %d 应等于总节点数 %d", sum, len(testdata.TestNodes))
	}
}

func TestQueryScenarios(t *testing.T) {
	actions := []string{"node", "exec", "playbook", "file"}
	total := 0
	for _, action := range actions {
		scenarios := testdata.GetScenariosByAction(action)
		total += len(scenarios)
		for _, s := range scenarios {
			if s.Name == "" {
				t.Errorf("%s 类场景缺少名称", action)
			}
			if s.NaturalInput == "" {
				t.Errorf("场景 %q 缺少自然语言输入", s.Name)
			}
			// 预期节点必须真实存在于数据集
			for _, name := range s.ExpectedNodes {
				if testdata.GetTestNodeByName(name) == nil {
					t.Errorf("场景 %q 的预期节点 %q 不在数据集中", s.Name, name)
				}
			}
		}
	}
	if total == 0 {
		t.Fatal("测试查询场景不应为空")
	}
}
