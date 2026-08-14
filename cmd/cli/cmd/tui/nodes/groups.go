package nodes

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type GroupsModel struct {
	store  common.NodeStore
	nodeID string
	groups []string
	cursor int
	input  textinput.Model
	adding bool
	error  string
}

func NewGroupsModel(store common.NodeStore, nodeID string) *GroupsModel {
	ti := textinput.New()
	ti.Placeholder = "分组名"
	ti.Width = 30
	ti.CharLimit = 64
	ti.Blur()
	g := &GroupsModel{store: store, nodeID: nodeID, input: ti}
	g.reload()
	return g
}

func (g *GroupsModel) reload() {
	node, err := g.store.Get(g.nodeID)
	if err != nil {
		g.groups = nil
		return
	}
	g.groups = append([]string(nil), node.Groups...)
	sort.Strings(g.groups)
	if g.cursor >= len(g.groups) {
		g.cursor = len(g.groups) - 1
	}
	if g.cursor < 0 {
		g.cursor = 0
	}
}

func (g *GroupsModel) addGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	node, err := g.store.Get(g.nodeID)
	if err != nil {
		return err
	}
	for _, x := range node.Groups {
		if x == name {
			return nil
		}
	}
	node.Groups = append(node.Groups, name)
	if err := g.store.Update(node); err != nil {
		return err
	}
	g.reload()
	return g.store.Save()
}

func (g *GroupsModel) removeGroup(name string) error {
	node, err := g.store.Get(g.nodeID)
	if err != nil {
		return err
	}
	out := make([]string, 0, len(node.Groups))
	for _, x := range node.Groups {
		if x != name {
			out = append(out, x)
		}
	}
	node.Groups = out
	if err := g.store.Update(node); err != nil {
		return err
	}
	g.reload()
	return g.store.Save()
}
