package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLocalSource(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed, got error: %v", err)
	}
	if source == nil {
		t.Fatal("Expected NewLocalSource to return non-nil")
	}
	if source.nodes == nil {
		t.Fatal("Expected nodes map to be initialized")
	}
}

func TestAddAndGetNode(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed, got error: %v", err)
	}

	testNode := &LocalNode{
		ID:      "test-node-1",
		Name:    "Test Node 1",
		Address: "192.168.1.1",
		Port:    22,
		User:    "testuser",
		Groups:  []string{"web", "db"},
		Labels:  map[string]string{"env": "test"},
	}

	err = source.AddNode(testNode)
	if err != nil {
		t.Fatalf("Expected AddNode to succeed, got error: %v", err)
	}

	node, err := source.GetNode("test-node-1")
	if err != nil {
		t.Fatalf("Expected GetNode to find node, got error: %v", err)
	}
	if node.ID != "test-node-1" {
		t.Errorf("Expected node ID to be 'test-node-1', got '%s'", node.ID)
	}

	nodeByName, err := source.GetNode("Test Node 1")
	if err != nil {
		t.Fatalf("Expected GetNode by name to find node, got error: %v", err)
	}
	if nodeByName.ID != "test-node-1" {
		t.Errorf("Expected same node by name, got different ID '%s'", nodeByName.ID)
	}
}

func TestGetNonExistentNode(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed, got error: %v", err)
	}
	node, err := source.GetNode("non-existent")
	if err == nil {
		t.Fatal("Expected error for non-existent node, got nil")
	}
	if node != nil {
		t.Fatal("Expected nil node for non-existent ID, got non-nil")
	}
}

func TestRemoveNode(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed, got error: %v", err)
	}

	testNode := &LocalNode{
		ID:   "to-remove",
		Name: "To Remove",
	}
	err = source.AddNode(testNode)
	if err != nil {
		t.Fatalf("Expected AddNode to succeed, got error: %v", err)
	}

	err = source.RemoveNode("to-remove")
	if err != nil {
		t.Fatalf("Expected RemoveNode to succeed, got error: %v", err)
	}

	_, err = source.GetNode("to-remove")
	if err == nil {
		t.Fatal("Expected node to be removed, but GetNode succeeded")
	}
}

func TestRemoveNonExistentNode(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed, got error: %v", err)
	}
	err = source.RemoveNode("non-existent")
	if err == nil {
		t.Fatal("Expected error when removing non-existent node")
	}
}

func TestListNodes(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed, got error: %v", err)
	}

	initialNodes, _ := source.ListNodes(nil)
	initialCount := len(initialNodes)

	node1 := &LocalNode{ID: "node1", Name: "Node 1", Groups: []string{"web"}}
	node2 := &LocalNode{ID: "node2", Name: "Node 2", Groups: []string{"db"}}

	source.AddNode(node1)
	source.AddNode(node2)

	nodes, err := source.ListNodes(nil)
	if err != nil {
		t.Fatalf("Expected ListNodes to succeed, got error: %v", err)
	}
	expectedCount := initialCount + 2
	if len(nodes) != expectedCount {
		t.Errorf("Expected %d nodes (initial %d + 2 added), got %d", expectedCount, initialCount, len(nodes))
	}
}

func TestListNodesWithGroupFilter(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed, got error: %v", err)
	}

	node1 := &LocalNode{ID: "node1", Name: "Node 1", Groups: []string{"web"}}
	node2 := &LocalNode{ID: "node2", Name: "Node 2", Groups: []string{"db"}}

	source.AddNode(node1)
	source.AddNode(node2)

	opts := &ListOptions{Group: "web"}
	nodes, err := source.ListNodes(opts)
	if err != nil {
		t.Fatalf("Expected ListNodes with filter to succeed, got error: %v", err)
	}
	if len(nodes) < 1 {
		t.Errorf("Expected at least 1 node with group 'web', got %d", len(nodes))
	}
}

func TestListNodesWithLabelFilter(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed, got error: %v", err)
	}

	node1 := &LocalNode{ID: "node1", Name: "Node 1", Labels: map[string]string{"env": "prod"}}
	node2 := &LocalNode{ID: "node2", Name: "Node 2", Labels: map[string]string{"env": "test"}}

	source.AddNode(node1)
	source.AddNode(node2)

	opts := &ListOptions{Label: "env=prod"}
	nodes, err := source.ListNodes(opts)
	if err != nil {
		t.Fatalf("Expected ListNodes with label filter to succeed, got error: %v", err)
	}
	if len(nodes) < 1 {
		t.Errorf("Expected at least 1 node with label 'env=prod', got %d", len(nodes))
	}
}

func TestListNodesWithNameFilter(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed, got error: %v", err)
	}

	node1 := &LocalNode{ID: "node1", Name: "Web Server"}
	node2 := &LocalNode{ID: "node2", Name: "DB Server"}

	source.AddNode(node1)
	source.AddNode(node2)

	opts := &ListOptions{Name: "Web Server"}
	nodes, err := source.ListNodes(opts)
	if err != nil {
		t.Fatalf("Expected ListNodes with name filter to succeed, got error: %v", err)
	}
	if len(nodes) < 1 {
		t.Errorf("Expected at least 1 node with name 'Web Server', got %d", len(nodes))
	}
}

func TestAddNodeWithEmptyID(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed, got error: %v", err)
	}

	node := &LocalNode{Name: "Test Node"}
	err = source.AddNode(node)
	if err != nil {
		t.Fatalf("Expected AddNode to succeed with empty ID, got error: %v", err)
	}
	if node.ID != "Test Node" {
		t.Errorf("Expected node ID to be set to name, got '%s'", node.ID)
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{"empty slice", []string{}, "test", false},
		{"item present", []string{"a", "b", "c"}, "b", true},
		{"item not present", []string{"a", "b", "c"}, "d", false},
		{"multiple matches", []string{"a", "a", "a"}, "a", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("Expected contains to return %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "node-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save original home dir env
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	// Create .owl directory
	owlDir := filepath.Join(tmpDir, ".owl")
	os.MkdirAll(owlDir, 0755)

	// Test that loading from empty dir works
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to succeed with empty nodes file, got error: %v", err)
	}
	if source == nil {
		t.Fatal("Expected NewLocalSource to succeed with empty nodes file")
	}
}

func TestLoadFromFileWithInvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "node-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	owlDir := filepath.Join(tmpDir, ".owl")
	os.MkdirAll(owlDir, 0755)
	nodesFile := filepath.Join(owlDir, "nodes.json")

	if err := os.WriteFile(nodesFile, []byte("invalid json {"), 0644); err != nil {
		t.Fatalf("Failed to write invalid JSON: %v", err)
	}

	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to gracefully handle invalid JSON, got error: %v", err)
	}
	if source == nil {
		t.Fatal("Expected NewLocalSource to return non-nil with invalid JSON")
	}
}

func TestLoadFromFileWithUnreadableFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "node-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	owlDir := filepath.Join(tmpDir, ".owl")
	os.MkdirAll(owlDir, 0755)
	nodesFile := filepath.Join(owlDir, "nodes.json")

	if err := os.WriteFile(nodesFile, []byte("{}"), 0000); err != nil {
		t.Skip("Cannot create unreadable file, skipping test")
	}

	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("Expected NewLocalSource to gracefully handle unreadable file, got error: %v", err)
	}
	if source == nil {
		t.Fatal("Expected NewLocalSource to return non-nil with unreadable file")
	}
}
