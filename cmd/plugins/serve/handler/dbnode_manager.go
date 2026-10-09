package handler

import (
	"context"
	"database/sql"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/node"
)

// DBNodeManager 以 nodes 表为数据源的节点管理器。
// serve 端 AI agent 原挂空内存管理器（NewInMemoryNodeStore 且从不加载），
// 本地降级链的节点名/分组名提取（defaultChatHandler → ParamExtractor）
// 拿不到任何数据——「查询db的主机」等语料在 serve 端因 groupNames 为空
// 而退化为全量列表，系统提示词的 {{.NodeInfo}} 亦恒为空。
// 每次访问前从 DB 全量同步到内存（AI 请求频率低，SQLite 查询廉价；
// web 端增删节点即刻对 AI 参数提取可见）。写操作沿用内存语义，
// 与原实现一致——节点写路径不经过 AI agent。
type DBNodeManager struct {
	store *node.InMemoryNodeStore
	mgr   node.Manager
	db    *sql.DB
}

func NewDBNodeManager(db *sql.DB) *DBNodeManager {
	st := node.NewInMemoryNodeStore()
	return &DBNodeManager{store: st, mgr: node.NewManager(st), db: db}
}

// refresh 从 nodes 表全量同步到内存：新增/更新覆盖，删除移除。
// DB 不可达时保留上一次快照（降级不致空）。
func (m *DBNodeManager) refresh() {
	rows, err := (&dbNodeSource{db: m.db}).List(context.Background())
	if err != nil {
		return
	}
	latest := make(map[string]*model.Node, len(rows))
	for _, r := range rows {
		latest[r.ID] = &model.Node{
			ID:     r.ID,
			Name:   r.Name,
			Status: model.NodeStatus(r.Status),
			Groups: r.Groups,
			Labels: r.Labels,
		}
	}
	for _, existing := range m.mgr.List() {
		if _, ok := latest[existing.ID]; !ok {
			m.store.Delete(existing.ID)
		}
	}
	for id, n := range latest {
		m.store.Set(id, n)
	}
}

func (m *DBNodeManager) Register(n *model.Node) error {
	m.refresh()
	return m.mgr.Register(n)
}

func (m *DBNodeManager) Unregister(id string) error {
	m.refresh()
	return m.mgr.Unregister(id)
}

func (m *DBNodeManager) GetByID(id string) (*model.Node, error) {
	m.refresh()
	return m.mgr.GetByID(id)
}

func (m *DBNodeManager) List() []*model.Node {
	m.refresh()
	return m.mgr.List()
}

func (m *DBNodeManager) GetByGroup(group string) []*model.Node {
	m.refresh()
	return m.mgr.GetByGroup(group)
}

func (m *DBNodeManager) GetByLabels(labels map[string]string) []*model.Node {
	m.refresh()
	return m.mgr.GetByLabels(labels)
}

func (m *DBNodeManager) SearchByName(pattern string) []*model.Node {
	m.refresh()
	return m.mgr.SearchByName(pattern)
}

func (m *DBNodeManager) SearchByAddress(pattern string) []*model.Node {
	m.refresh()
	return m.mgr.SearchByAddress(pattern)
}

func (m *DBNodeManager) UpdateStatus(id string, status model.NodeStatus) error {
	m.refresh()
	return m.mgr.UpdateStatus(id, status)
}

func (m *DBNodeManager) GetOnlineNodes() []*model.Node {
	m.refresh()
	return m.mgr.GetOnlineNodes()
}

func (m *DBNodeManager) Count() int {
	m.refresh()
	return m.mgr.Count()
}
