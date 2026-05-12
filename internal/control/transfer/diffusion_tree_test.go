package transfer

import (
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

func TestDiffusionTree_Build(t *testing.T) {
	builder := NewTreeBuilder(3, 10, 5)
	nodes := []*model.Node{
		{ID: "node-1"},
		{ID: "node-2"},
		{ID: "node-3"},
		{ID: "node-4"},
		{ID: "node-5"},
	}

	tree := builder.Build(nodes)

	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	if tree.Root != "control" {
		t.Errorf("expected root 'control', got '%s'", tree.Root)
	}
	if tree.NodeCount() != 6 {
		t.Errorf("expected 6 nodes, got %d", tree.NodeCount())
	}

	controlChildren := tree.GetChildren("control")
	if len(controlChildren) != 3 {
		t.Errorf("expected 3 children for control node, got %d", len(controlChildren))
	}
}

func TestDiffusionTree_Build_WithK(t *testing.T) {
	builder := NewTreeBuilder(2, 10, 5)
	nodes := make([]*model.Node, 10)
	for i := 0; i < 10; i++ {
		nodes[i] = &model.Node{ID: "node-" + string(rune('1'+i))}
	}

	tree := builder.Build(nodes)

	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	if tree.FanOutK != 2 {
		t.Errorf("expected fanOutK 2, got %d", tree.FanOutK)
	}

	controlChildren := tree.GetChildren("control")
	if len(controlChildren) != 2 {
		t.Errorf("expected 2 children for control node with k=2, got %d", len(controlChildren))
	}
}

func TestDiffusionTree_Build_WithDepth(t *testing.T) {
	builder := NewTreeBuilder(3, 2, 5)
	nodes := make([]*model.Node, 20)
	for i := 0; i < 20; i++ {
		nodes[i] = &model.Node{ID: "node-" + string(rune('1'+i))}
	}

	tree := builder.Build(nodes)

	if tree == nil {
		t.Fatal("expected tree, got nil")
	}

	maxLevel := 0
	for _, node := range tree.Nodes {
		if node.Level > maxLevel {
			maxLevel = node.Level
		}
	}

	if maxLevel > 3 {
		t.Errorf("expected max level <= 3 (root=0 + maxDepth=2 + leaf), got %d", maxLevel)
	}
}

func TestDiffusionTree_Unequal(t *testing.T) {
	builder := NewTreeBuilder(3, 10, 5)
	nodes := []*model.Node{
		{ID: "node-1"},
		{ID: "node-2"},
		{ID: "node-3"},
		{ID: "node-4"},
	}

	tree := builder.Build(nodes)

	if tree == nil {
		t.Fatal("expected tree, got nil")
	}

	controlChildren := tree.GetChildren("control")
	if len(controlChildren) != 3 {
		t.Errorf("expected 3 children for control, got %d", len(controlChildren))
	}

	leafCount := len(tree.GetLeafNodes())
	if leafCount != 3 {
		t.Errorf("expected 3 leaf nodes (node-4 becomes child of node-1), got %d", leafCount)
	}
}

func TestDiffusionTree_SingleNode(t *testing.T) {
	builder := NewTreeBuilder(3, 10, 5)
	nodes := []*model.Node{
		{ID: "node-1"},
	}

	tree := builder.Build(nodes)

	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	if tree.NodeCount() != 2 {
		t.Errorf("expected 2 nodes (control + 1 target), got %d", tree.NodeCount())
	}

	controlChildren := tree.GetChildren("control")
	if len(controlChildren) != 1 {
		t.Errorf("expected 1 child for control, got %d", len(controlChildren))
	}
	if controlChildren[0] != "node-1" {
		t.Errorf("expected child 'node-1', got '%s'", controlChildren[0])
	}
}

func TestDiffusionTree_Empty(t *testing.T) {
	builder := NewTreeBuilder(3, 10, 5)
	var nodes []*model.Node

	tree := builder.Build(nodes)

	if tree != nil {
		t.Errorf("expected nil for empty nodes, got tree with %d nodes", tree.NodeCount())
	}
}

func TestDiffusionTree_GetNode(t *testing.T) {
	builder := NewTreeBuilder(3, 10, 5)
	nodes := []*model.Node{
		{ID: "node-1"},
		{ID: "node-2"},
	}

	tree := builder.Build(nodes)

	node, ok := tree.GetNode("node-1")
	if !ok {
		t.Fatal("expected node-1 to exist")
	}
	if node.ID != "node-1" {
		t.Errorf("expected node ID 'node-1', got '%s'", node.ID)
	}
	if node.ParentID != "control" {
		t.Errorf("expected parent 'control', got '%s'", node.ParentID)
	}

	_, ok = tree.GetNode("nonexistent")
	if ok {
		t.Error("expected nonexistent node to not exist")
	}
}

func TestDiffusionTree_GetParent(t *testing.T) {
	builder := NewTreeBuilder(3, 10, 5)
	nodes := []*model.Node{
		{ID: "node-1"},
		{ID: "node-2"},
	}

	tree := builder.Build(nodes)

	parent := tree.GetParent("node-1")
	if parent != "control" {
		t.Errorf("expected parent 'control', got '%s'", parent)
	}

	parent = tree.GetParent("control")
	if parent != "" {
		t.Errorf("expected empty parent for root, got '%s'", parent)
	}

	parent = tree.GetParent("nonexistent")
	if parent != "" {
		t.Errorf("expected empty parent for nonexistent, got '%s'", parent)
	}
}

func TestDiffusionTree_GetLevel(t *testing.T) {
	builder := NewTreeBuilder(2, 10, 5)
	nodes := make([]*model.Node, 7)
	for i := 0; i < 7; i++ {
		nodes[i] = &model.Node{ID: "node-" + string(rune('1'+i))}
	}

	tree := builder.Build(nodes)

	level := tree.GetLevel("control")
	if level != 0 {
		t.Errorf("expected level 0 for control, got %d", level)
	}

	level = tree.GetLevel("nonexistent")
	if level != -1 {
		t.Errorf("expected level -1 for nonexistent, got %d", level)
	}
}

func TestDiffusionTree_GetLeafNodes(t *testing.T) {
	builder := NewTreeBuilder(2, 10, 5)
	nodes := make([]*model.Node, 5)
	for i := 0; i < 5; i++ {
		nodes[i] = &model.Node{ID: "node-" + string(rune('1'+i))}
	}

	tree := builder.Build(nodes)

	leaves := tree.GetLeafNodes()
	if len(leaves) != 3 {
		t.Errorf("expected 3 leaf nodes (k=2: control->2, node-1->2, node-2->1, leaves=3), got %d", len(leaves))
	}

	for _, leaf := range leaves {
		if leaf == "control" {
			t.Error("control should not be in leaf nodes")
		}
	}
}

func TestDiffusionTree_GetSourceNodes(t *testing.T) {
	builder := NewTreeBuilder(2, 10, 5)
	nodes := make([]*model.Node, 7)
	for i := 0; i < 7; i++ {
		nodes[i] = &model.Node{ID: "node-" + string(rune('1'+i))}
	}

	tree := builder.Build(nodes)

	sources := tree.GetSourceNodes()

	for _, source := range sources {
		if source == "control" {
			t.Error("control should not be in source nodes")
		}
		children := tree.GetChildren(source)
		if len(children) == 0 {
			t.Errorf("source node %s should have children", source)
		}
	}
}

func TestDiffusionTree_Validate(t *testing.T) {
	builder := NewTreeBuilder(3, 10, 5)
	nodes := []*model.Node{
		{ID: "node-1"},
		{ID: "node-2"},
	}

	tree := builder.Build(nodes)
	if err := tree.Validate(); err != nil {
		t.Errorf("expected valid tree, got error: %v", err)
	}
}

func TestDiffusionTree_Validate_EmptyRoot(t *testing.T) {
	tree := &DiffusionTree{
		Root:  "",
		Nodes: make(map[string]*TreeNode),
	}

	err := tree.Validate()
	if err == nil {
		t.Error("expected error for empty root")
	}
}

func TestDiffusionTree_GetSubTree(t *testing.T) {
	builder := NewTreeBuilder(2, 10, 5)
	nodes := make([]*model.Node, 7)
	for i := 0; i < 7; i++ {
		nodes[i] = &model.Node{ID: "node-" + string(rune('1'+i))}
	}

	tree := builder.Build(nodes)

	controlChildren := tree.GetChildren("control")
	if len(controlChildren) == 0 {
		t.Skip("no children for control node")
	}

	subTree := tree.GetSubTree(controlChildren[0])
	if subTree == nil {
		t.Fatal("expected subTree, got nil")
	}

	if subTree.Root != controlChildren[0] {
		t.Errorf("expected subTree root '%s', got '%s'", controlChildren[0], subTree.Root)
	}
}

func TestDiffusionTree_GetSubTree_NotFound(t *testing.T) {
	tree := &DiffusionTree{
		Root:  "control",
		Nodes: make(map[string]*TreeNode),
	}

	subTree := tree.GetSubTree("nonexistent")
	if subTree != nil {
		t.Error("expected nil for nonexistent node")
	}
}

func TestDiffusionTree_ReassignChildren(t *testing.T) {
	builder := NewTreeBuilder(2, 10, 5)
	nodes := make([]*model.Node, 7)
	for i := 0; i < 7; i++ {
		nodes[i] = &model.Node{ID: "node-" + string(rune('1'+i))}
	}

	tree := builder.Build(nodes)

	controlChildren := tree.GetChildren("control")
	if len(controlChildren) < 2 {
		t.Skip("not enough children for reassignment test")
	}

	sourceID := controlChildren[0]
	targetID := controlChildren[1]
	sourceChildrenBefore := tree.GetChildren(sourceID)
	targetChildrenBefore := tree.GetChildren(targetID)

	if len(sourceChildrenBefore) == 0 {
		t.Skip("source has no children to reassign")
	}

	err := tree.ReassignChildren(sourceID, targetID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	sourceChildrenAfter := tree.GetChildren(sourceID)
	if len(sourceChildrenAfter) != 0 {
		t.Errorf("expected 0 children after reassignment, got %d", len(sourceChildrenAfter))
	}

	targetChildrenAfter := tree.GetChildren(targetID)
	expectedChildren := len(targetChildrenBefore) + len(sourceChildrenBefore)
	if len(targetChildrenAfter) != expectedChildren {
		t.Errorf("expected %d children for target, got %d", expectedChildren, len(targetChildrenAfter))
	}
}

func TestDiffusionTree_ReassignChildren_SourceNotFound(t *testing.T) {
	tree := &DiffusionTree{
		Root: "control",
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{}},
			"target":  {ID: "target", ParentID: "control", Children: []string{}},
		},
	}

	err := tree.ReassignChildren("nonexistent", "target")
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}

func TestDiffusionTree_ReassignChildren_TargetNotFound(t *testing.T) {
	tree := &DiffusionTree{
		Root: "control",
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{"source"}},
			"source":  {ID: "source", ParentID: "control", Children: []string{"child"}},
		},
	}

	err := tree.ReassignChildren("source", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent target")
	}
}

func TestDiffusionTree_RemoveNode(t *testing.T) {
	builder := NewTreeBuilder(2, 10, 5)
	nodes := make([]*model.Node, 5)
	for i := 0; i < 5; i++ {
		nodes[i] = &model.Node{ID: "node-" + string(rune('1'+i))}
	}

	tree := builder.Build(nodes)

	controlChildren := tree.GetChildren("control")
	if len(controlChildren) == 0 {
		t.Skip("no children to remove")
	}

	nodeToRemove := controlChildren[0]
	nodeCountBefore := tree.NodeCount()

	childrenToRemove := len(tree.GetChildren(nodeToRemove))
	totalToRemove := 1 + childrenToRemove

	err := tree.RemoveNode(nodeToRemove)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	nodeCountAfter := tree.NodeCount()
	expectedCount := nodeCountBefore - totalToRemove
	if nodeCountAfter != expectedCount {
		t.Errorf("expected %d nodes after removal, got %d", expectedCount, nodeCountAfter)
	}

	_, ok := tree.GetNode(nodeToRemove)
	if ok {
		t.Error("expected node to be removed")
	}
}

func TestDiffusionTree_RemoveNode_WithChildren(t *testing.T) {
	tree := &DiffusionTree{
		Root: "control",
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{"parent"}},
			"parent":  {ID: "parent", ParentID: "control", Children: []string{"child1", "child2"}},
			"child1":  {ID: "child1", ParentID: "parent", Children: []string{}},
			"child2":  {ID: "child2", ParentID: "parent", Children: []string{}},
		},
	}

	err := tree.RemoveNode("parent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if tree.NodeCount() != 1 {
		t.Errorf("expected 1 node after removal, got %d", tree.NodeCount())
	}

	_, ok := tree.GetNode("child1")
	if ok {
		t.Error("expected child1 to be removed")
	}
}

func TestDiffusionTree_RemoveNode_NotFound(t *testing.T) {
	tree := &DiffusionTree{
		Root: "control",
		Nodes: map[string]*TreeNode{
			"control": {ID: "control", Children: []string{}},
		},
	}

	err := tree.RemoveNode("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent node")
	}
}

func TestNewDiffusionTree(t *testing.T) {
	tree := NewDiffusionTree()

	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	if tree.Nodes == nil {
		t.Error("expected Nodes to be initialized")
	}
	if tree.Root != "" {
		t.Errorf("expected empty root, got '%s'", tree.Root)
	}
}

func TestDiffusionTree_DefaultValues(t *testing.T) {
	builder := NewTreeBuilder(0, 0, 0)
	nodes := []*model.Node{
		{ID: "node-1"},
		{ID: "node-2"},
	}

	tree := builder.Build(nodes)

	if tree.FanOutK != DefaultFanOutK {
		t.Errorf("expected default FanOutK %d, got %d", DefaultFanOutK, tree.FanOutK)
	}
	if tree.MaxDepth != DefaultMaxDepth {
		t.Errorf("expected default MaxDepth %d, got %d", DefaultMaxDepth, tree.MaxDepth)
	}
	if tree.Threshold != DefaultThreshold {
		t.Errorf("expected default Threshold %d, got %d", DefaultThreshold, tree.Threshold)
	}
}
