package nodes

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type LabelsModel struct {
	store  common.NodeStore
	nodeID string
	keys   []string
	cursor int
	input  textinput.Model
	adding bool
	error  string
}

func NewLabelsModel(store common.NodeStore, nodeID string) *LabelsModel {
	ti := textinput.New()
	ti.Placeholder = "key=value"
	ti.Width = 30
	ti.CharLimit = 128
	ti.Blur()
	l := &LabelsModel{store: store, nodeID: nodeID, input: ti}
	l.reload()
	return l
}

func (l *LabelsModel) reload() {
	node, err := l.store.Get(l.nodeID)
	if err != nil {
		l.keys = nil
		return
	}
	l.keys = make([]string, 0, len(node.Labels))
	for k := range node.Labels {
		l.keys = append(l.keys, k)
	}
	sort.Strings(l.keys)
	if l.cursor >= len(l.keys) {
		l.cursor = len(l.keys) - 1
	}
	if l.cursor < 0 {
		l.cursor = 0
	}
}

// setLabel 设置 key=value;value 为空表示删除该 key(对齐 node labels set key= / remove key)
func (l *LabelsModel) setLabel(kv string) error {
	kv = strings.TrimSpace(kv)
	if kv == "" {
		return nil
	}
	parts := strings.SplitN(kv, "=", 2)
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return nil
	}
	node, err := l.store.Get(l.nodeID)
	if err != nil {
		return err
	}
	if node.Labels == nil {
		node.Labels = map[string]string{}
	}
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		node.Labels[key] = strings.TrimSpace(parts[1])
	} else {
		delete(node.Labels, key)
	}
	if err := l.store.Update(node); err != nil {
		return err
	}
	l.reload()
	return l.store.Save()
}

func (l *LabelsModel) removeLabel(key string) error {
	node, err := l.store.Get(l.nodeID)
	if err != nil {
		return err
	}
	delete(node.Labels, key)
	if err := l.store.Update(node); err != nil {
		return err
	}
	l.reload()
	return l.store.Save()
}
