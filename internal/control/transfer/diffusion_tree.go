package transfer

import (
	"fmt"

	"github.com/cangyunye/go-owl/internal/common/model"
)

const (
	DefaultFanOutK           = 3
	DefaultMaxDepth          = 10
	DefaultThreshold         = 5
	DefaultChunkSizeTransfer = 64 * 1024
)

type TreeNode struct {
	ID       string
	ParentID string
	Children []string
	Level    int
}

type DiffusionTree struct {
	Root      string
	Nodes     map[string]*TreeNode
	FanOutK   int
	MaxDepth  int
	Threshold int
}

type TreeBuilder interface {
	Build(targets []*model.Node) *DiffusionTree
}

type diffusionTreeBuilder struct {
	fanOutK   int
	maxDepth  int
	threshold int
	// sourceCount 决定第一层直连控制端的源节点数；<=0 时与 fanOutK 一致（默认行为）
	sourceCount int
}

func NewTreeBuilder(fanOutK, maxDepth, threshold int) TreeBuilder {
	return NewTreeBuilderWithSources(fanOutK, 0, maxDepth, threshold)
}

// NewTreeBuilderWithSources 构建扩散树时显式指定第一层源节点数。
// sourceCount <= 0 或超过目标数时按实际目标数收敛。
func NewTreeBuilderWithSources(fanOutK, sourceCount, maxDepth, threshold int) TreeBuilder {
	if fanOutK <= 0 {
		fanOutK = DefaultFanOutK
	}
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	if threshold <= 0 {
		threshold = DefaultThreshold
	}
	return &diffusionTreeBuilder{
		fanOutK:     fanOutK,
		maxDepth:    maxDepth,
		threshold:   threshold,
		sourceCount: sourceCount,
	}
}

func (b *diffusionTreeBuilder) Build(targets []*model.Node) *DiffusionTree {
	if len(targets) == 0 {
		return nil
	}

	tree := &DiffusionTree{
		Root:      "control",
		Nodes:     make(map[string]*TreeNode),
		FanOutK:   b.fanOutK,
		MaxDepth:  b.maxDepth,
		Threshold: b.threshold,
	}

	tree.Nodes["control"] = &TreeNode{
		ID:       "control",
		ParentID: "",
		Children: make([]string, 0),
		Level:    0,
	}

	if len(targets) == 1 {
		nodeID := targets[0].ID
		tree.Nodes["control"].Children = append(tree.Nodes["control"].Children, nodeID)
		tree.Nodes[nodeID] = &TreeNode{
			ID:       nodeID,
			ParentID: "control",
			Children: make([]string, 0),
			Level:    1,
		}
		return tree
	}

	currentSources := []string{"control"}
	level := 1
	remainingNodes := make([]string, len(targets))
	for i, node := range targets {
		remainingNodes[i] = node.ID
	}

	for len(remainingNodes) > 0 && level <= b.maxDepth {
		nextSources := make([]string, 0)

		for _, sourceID := range currentSources {
			if len(remainingNodes) == 0 {
				break
			}

			// 第一层按 sourceCount 展开源节点，更深层每节点分支数为 fanOutK
			branch := b.fanOutK
			if sourceID == "control" && b.sourceCount > 0 {
				branch = b.sourceCount
			}
			childrenCount := min(branch, len(remainingNodes))
			children := remainingNodes[:childrenCount]
			remainingNodes = remainingNodes[childrenCount:]

			tree.Nodes[sourceID].Children = append(tree.Nodes[sourceID].Children, children...)

			for _, childID := range children {
				tree.Nodes[childID] = &TreeNode{
					ID:       childID,
					ParentID: sourceID,
					Children: make([]string, 0),
					Level:    level + 1,
				}
				nextSources = append(nextSources, childID)
			}
		}

		currentSources = nextSources
		level++
	}

	return tree
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (t *DiffusionTree) GetNode(id string) (*TreeNode, bool) {
	node, ok := t.Nodes[id]
	return node, ok
}

func (t *DiffusionTree) GetChildren(nodeID string) []string {
	node, ok := t.Nodes[nodeID]
	if !ok {
		return nil
	}
	children := make([]string, len(node.Children))
	copy(children, node.Children)
	return children
}

func (t *DiffusionTree) GetParent(nodeID string) string {
	node, ok := t.Nodes[nodeID]
	if !ok {
		return ""
	}
	return node.ParentID
}

func (t *DiffusionTree) GetLevel(nodeID string) int {
	node, ok := t.Nodes[nodeID]
	if !ok {
		return -1
	}
	return node.Level
}

func (t *DiffusionTree) GetLeafNodes() []string {
	leaves := make([]string, 0)
	for id, node := range t.Nodes {
		if id != t.Root && len(node.Children) == 0 {
			leaves = append(leaves, id)
		}
	}
	return leaves
}

func (t *DiffusionTree) GetSourceNodes() []string {
	sources := make([]string, 0)
	for id, node := range t.Nodes {
		if id != t.Root && len(node.Children) > 0 {
			sources = append(sources, id)
		}
	}
	return sources
}

func (t *DiffusionTree) NodeCount() int {
	return len(t.Nodes)
}

func (t *DiffusionTree) Validate() error {
	if t.Root == "" {
		return fmt.Errorf("tree root is empty")
	}
	if _, ok := t.Nodes[t.Root]; !ok {
		return fmt.Errorf("root node '%s' not found in tree", t.Root)
	}

	for id, node := range t.Nodes {
		if node.ParentID != "" {
			if _, ok := t.Nodes[node.ParentID]; !ok {
				return fmt.Errorf("parent node '%s' not found for node '%s'", node.ParentID, id)
			}
		}
	}

	return nil
}

func (t *DiffusionTree) GetSubTree(nodeID string) *DiffusionTree {
	node, ok := t.Nodes[nodeID]
	if !ok {
		return nil
	}

	subTree := &DiffusionTree{
		Root:      nodeID,
		Nodes:     make(map[string]*TreeNode),
		FanOutK:   t.FanOutK,
		MaxDepth:  t.MaxDepth,
		Threshold: t.Threshold,
	}

	queue := []string{nodeID}
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		currentNode := t.Nodes[currentID]
		subTree.Nodes[currentID] = &TreeNode{
			ID:       currentNode.ID,
			ParentID: currentNode.ParentID,
			Children: make([]string, len(currentNode.Children)),
			Level:    currentNode.Level - node.Level,
		}
		copy(subTree.Nodes[currentID].Children, currentNode.Children)

		queue = append(queue, currentNode.Children...)
	}

	return subTree
}

func (t *DiffusionTree) ReassignChildren(fromNodeID, toNodeID string) error {
	fromNode, ok := t.Nodes[fromNodeID]
	if !ok {
		return fmt.Errorf("source node '%s' not found", fromNodeID)
	}

	toNode, ok := t.Nodes[toNodeID]
	if !ok {
		return fmt.Errorf("target node '%s' not found", toNodeID)
	}

	children := make([]string, len(fromNode.Children))
	copy(children, fromNode.Children)

	for _, childID := range children {
		t.Nodes[childID].ParentID = toNodeID
		toNode.Children = append(toNode.Children, childID)
	}
	fromNode.Children = nil

	return nil
}

func (t *DiffusionTree) RemoveNode(nodeID string) error {
	node, ok := t.Nodes[nodeID]
	if !ok {
		return fmt.Errorf("node '%s' not found", nodeID)
	}

	if node.ParentID != "" {
		parent := t.Nodes[node.ParentID]
		newChildren := make([]string, 0)
		for _, childID := range parent.Children {
			if childID != nodeID {
				newChildren = append(newChildren, childID)
			}
		}
		parent.Children = newChildren
	}

	var queue []string
	for _, childID := range node.Children {
		queue = append(queue, childID)
	}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		currentChildren := t.Nodes[currentID].Children
		delete(t.Nodes, currentID)
		queue = append(queue, currentChildren...)
	}

	delete(t.Nodes, nodeID)
	return nil
}
