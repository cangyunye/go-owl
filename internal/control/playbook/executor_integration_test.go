//go:build integration

package playbook

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/node"
)

func TestIntegration_PipelineMode_FailsFast_OnMacNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	resolver := node.NewNodeResolver()
	macNode, err := resolver.Resolve("mac")
	if err != nil {
		t.Fatalf("failed to resolve mac node: %v", err)
	}

	macModelNode := &model.Node{
		ID:      macNode.ID,
		Name:    macNode.Name,
		Address: macNode.Address,
		Port:    macNode.Port,
		User:    macNode.User,
	}

	cmdExec := command.NewExecutor(resolver)
	defer cmdExec.Close()

	adapterMgr := &mockNodeManagerForIntegration{
		nodes: map[string]*model.Node{macModelNode.ID: macModelNode},
	}

	executor := NewExecutor(adapterMgr, cmdExec, nil, resolver)

	playbookPath := filepath.Join("..", "..", "..", "tests", "testdata", "playbooks", "test-pipeline.yaml")
	parser := NewParser()
	parsed, err := parser.ParseFromFile(playbookPath)
	if err != nil {
		t.Fatalf("failed to parse playbook: %v", err)
	}

	exec, _ := executor.Execute(parsed, []*model.Node{macModelNode}, nil)

	if exec.Status != ExecutionStatusFailed {
		t.Errorf("expected Status Failed, got '%s'", exec.Status)
	}

	_, step3Executed := exec.Results["step3_should_not_run"]
	if step3Executed {
		t.Error("step3 should NOT have been executed in pipeline mode")
	}

	_, step1Executed := exec.Results["step1_success"]
	if !step1Executed {
		t.Error("step1 should have been executed")
	}

	fmt.Printf("Pipeline integration test passed. Status: %s, Executed steps: ", exec.Status)
	for name := range exec.Results {
		fmt.Printf("%s ", name)
	}
	fmt.Println()
}

func TestIntegration_FailContinueMode_RunsAll_OnMacNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	resolver := node.NewNodeResolver()
	macNode, err := resolver.Resolve("mac")
	if err != nil {
		t.Fatalf("failed to resolve mac node: %v", err)
	}

	macModelNode := &model.Node{
		ID:      macNode.ID,
		Name:    macNode.Name,
		Address: macNode.Address,
		Port:    macNode.Port,
		User:    macNode.User,
	}

	cmdExec := command.NewExecutor(resolver)
	defer cmdExec.Close()

	adapterMgr := &mockNodeManagerForIntegration{
		nodes: map[string]*model.Node{macModelNode.ID: macModelNode},
	}

	executor := NewExecutor(adapterMgr, cmdExec, nil, resolver)

	playbookPath := filepath.Join("..", "..", "..", "tests", "testdata", "playbooks", "test-fail-continue.yaml")
	parser := NewParser()
	parsed, err := parser.ParseFromFile(playbookPath)
	if err != nil {
		t.Fatalf("failed to parse playbook: %v", err)
	}

	exec, _ := executor.Execute(parsed, []*model.Node{macModelNode}, nil)

	_, step3Executed := exec.Results["step3_should_still_run"]
	if !step3Executed {
		t.Error("step3 should have been executed in fail_continue mode")
	}

	fmt.Printf("FailContinue integration test passed. Status: %s, Executed steps: ", exec.Status)
	for name := range exec.Results {
		fmt.Printf("%s ", name)
	}
	fmt.Println()
}

// mockNodeManagerForIntegration 简化版节点管理器
type mockNodeManagerForIntegration struct {
	nodes map[string]*model.Node
}

func (m *mockNodeManagerForIntegration) Register(n *model.Node) error { return nil }
func (m *mockNodeManagerForIntegration) Unregister(id string) error { return nil }
func (m *mockNodeManagerForIntegration) GetByID(id string) (*model.Node, error) {
	n, ok := m.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node %s not found", id)
	}
	return n, nil
}
func (m *mockNodeManagerForIntegration) List() []*model.Node {
	nodes := make([]*model.Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}
func (m *mockNodeManagerForIntegration) GetByGroup(group string) []*model.Node { return nil }
func (m *mockNodeManagerForIntegration) GetByLabels(labels map[string]string) []*model.Node { return nil }
func (m *mockNodeManagerForIntegration) UpdateStatus(id string, status model.NodeStatus) error { return nil }
func (m *mockNodeManagerForIntegration) GetOnlineNodes() []*model.Node { return m.List() }
func (m *mockNodeManagerForIntegration) Count() int { return len(m.nodes) }
func (m *mockNodeManagerForIntegration) SearchByName(pattern string) []*model.Node { return nil }
func (m *mockNodeManagerForIntegration) SearchByAddress(pattern string) []*model.Node { return nil }
