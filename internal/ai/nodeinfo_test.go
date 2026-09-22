package ai

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

type countNodeMgr struct{ nodes []*model.Node }

func (m *countNodeMgr) List() []*model.Node                        { return m.nodes }
func (m *countNodeMgr) UpdateStatus(id string, s model.NodeStatus) error { return nil }
func (m *countNodeMgr) Count() int                                 { return len(m.nodes) }
func (m *countNodeMgr) GetByName(name string) *model.Node          { return nil }
func (m *countNodeMgr) GetByID(id string) (*model.Node, error) {
	return nil, nil
}

func (m *countNodeMgr) Get(id string) (*model.Node, bool) { return nil, false }
func (m *countNodeMgr) Set(id string, n *model.Node)      {}
func (m *countNodeMgr) Delete(id string) bool             { return false }
func (m *countNodeMgr) GetAll() []*model.Node             { return m.nodes }
func (m *countNodeMgr) Register(n *model.Node) error      { return nil }
func (m *countNodeMgr) Unregister(id string) error        { return nil }
func (m *countNodeMgr) GetByGroup(g string) []*model.Node { return nil }
func (m *countNodeMgr) GetByLabels(l map[string]string) []*model.Node {
	return nil
}
func (m *countNodeMgr) SearchByName(p string) []*model.Node    { return nil }
func (m *countNodeMgr) SearchByAddress(p string) []*model.Node { return nil }

// TestGetNodeInfo_LargeFleetCompacted：超大舰队（>nodeInfoFullListMax）
// 按组压缩为计数+每组少量示例，不再全量拼所有节点名。
func TestGetNodeInfo_LargeFleetCompacted(t *testing.T) {
	var nodes []*model.Node
	for i := 0; i < 30; i++ {
		nodes = append(nodes, &model.Node{Name: fmt.Sprintf("web-%02d", i), Groups: []string{"web"}, Status: model.NodeStatusOnline})
	}
	for i := 0; i < 30; i++ {
		nodes = append(nodes, &model.Node{Name: fmt.Sprintf("db-%02d", i), Groups: []string{"db"}, Status: model.NodeStatusOffline})
	}
	agent := &Agent{nodeMgr: &countNodeMgr{nodes: nodes}}
	info := agent.getNodeInfo()

	if strings.Contains(info, "web-29") || strings.Contains(info, "db-25") {
		t.Fatal("large fleet must not enumerate every node")
	}
	if !strings.Contains(info, "web") || !strings.Contains(info, "db") {
		t.Fatal("group names must be preserved")
	}
	if !strings.Contains(info, "60") {
		t.Fatal("total count must be preserved")
	}

	// 小舰队保持全量
	small := &Agent{nodeMgr: &countNodeMgr{nodes: nodes[:3]}}
	if s := small.getNodeInfo(); !strings.Contains(s, "web-0") {
		t.Fatalf("small fleet should keep full names, got %q", s)
	}
}

func (m *countNodeMgr) GetOnlineNodes() []*model.Node { return m.nodes }
