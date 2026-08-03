package node

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func mkNode(id string, groups []string, labels map[string]string, status string) *common.NodeInfo {
	return &common.NodeInfo{ID: id, Groups: groups, Labels: labels, Status: status}
}

func TestFilterCheckNodes_NoFilter(t *testing.T) {
	nodes := []*common.NodeInfo{
		mkNode("n1", []string{"web"}, nil, "online"),
		mkNode("n2", []string{"db"}, nil, "offline"),
	}
	result := filterCheckNodes(nodes, nil, nil, false)
	if len(result) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result))
	}
}

func TestFilterCheckNodes_ByGroup(t *testing.T) {
	nodes := []*common.NodeInfo{
		mkNode("n1", []string{"web"}, nil, "online"),
		mkNode("n2", []string{"db"}, nil, "offline"),
		mkNode("n3", []string{"web", "cache"}, nil, "offline"),
	}
	result := filterCheckNodes(nodes, []string{"web"}, nil, false)
	if len(result) != 2 {
		t.Fatalf("expected 2 web nodes, got %d", len(result))
	}
	for _, n := range result {
		if n.ID == "n2" {
			t.Errorf("n2 (db) should be filtered out")
		}
	}
}

func TestFilterCheckNodes_ByLabel(t *testing.T) {
	nodes := []*common.NodeInfo{
		mkNode("n1", nil, map[string]string{"env": "prod"}, "online"),
		mkNode("n2", nil, map[string]string{"env": "staging"}, "offline"),
		mkNode("n3", nil, map[string]string{"env": "prod", "role": "db"}, "online"),
	}
	result := filterCheckNodes(nodes, nil, []string{"env=prod"}, false)
	if len(result) != 2 {
		t.Fatalf("expected 2 prod nodes, got %d", len(result))
	}
	for _, n := range result {
		if n.ID == "n2" {
			t.Errorf("n2 (staging) should be filtered out")
		}
	}
}

func TestFilterCheckNodes_MultipleLabelsAnd(t *testing.T) {
	nodes := []*common.NodeInfo{
		mkNode("n1", nil, map[string]string{"env": "prod", "role": "db"}, "online"),
		mkNode("n2", nil, map[string]string{"env": "prod", "role": "web"}, "online"),
	}
	result := filterCheckNodes(nodes, nil, []string{"env=prod", "role=db"}, false)
	if len(result) != 1 || result[0].ID != "n1" {
		t.Fatalf("expected only n1 to match both labels, got %d nodes", len(result))
	}
}

func TestFilterCheckNodes_GroupAndLabelCombined(t *testing.T) {
	nodes := []*common.NodeInfo{
		mkNode("n1", []string{"web"}, map[string]string{"env": "prod"}, "online"),
		mkNode("n2", []string{"db"}, map[string]string{"env": "prod"}, "online"),
		mkNode("n3", []string{"web"}, map[string]string{"env": "staging"}, "offline"),
	}
	result := filterCheckNodes(nodes, []string{"web"}, []string{"env=prod"}, false)
	if len(result) != 1 || result[0].ID != "n1" {
		t.Fatalf("expected only n1, got %d nodes", len(result))
	}
}

func TestFilterCheckNodes_OnlyFailed(t *testing.T) {
	nodes := []*common.NodeInfo{
		mkNode("n1", nil, nil, "online"),
		mkNode("n2", nil, nil, "offline"),
		mkNode("n3", nil, nil, "offline"),
	}
	result := filterCheckNodes(nodes, nil, nil, true)
	if len(result) != 2 {
		t.Fatalf("expected 2 offline nodes, got %d", len(result))
	}
	for _, n := range result {
		if n.Status != "offline" {
			t.Errorf("node %s with status %q should be filtered out", n.ID, n.Status)
		}
	}
}

func TestFilterCheckNodes_OnlyFailedCombinedWithGroup(t *testing.T) {
	nodes := []*common.NodeInfo{
		mkNode("n1", []string{"web"}, nil, "online"),
		mkNode("n2", []string{"web"}, nil, "offline"),
		mkNode("n3", []string{"db"}, nil, "offline"),
	}
	result := filterCheckNodes(nodes, []string{"web"}, nil, true)
	if len(result) != 1 || result[0].ID != "n2" {
		t.Fatalf("expected only offline web node n2, got %d nodes", len(result))
	}
}