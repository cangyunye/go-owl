package node

import (
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

func TestManager_Register(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node := model.NewNode("node-1", "test-node", "192.168.1.100", 8080)

	err := mgr.Register(node)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mgr.Count() != 1 {
		t.Errorf("expected 1 node, got %d", mgr.Count())
	}
}

func TestManager_Register_WithDuplicateID(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node1 := model.NewNode("node-1", "test-node-1", "192.168.1.100", 8080)
	node2 := model.NewNode("node-1", "test-node-2", "192.168.1.101", 8081)

	err := mgr.Register(node1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	err = mgr.Register(node2)
	if err == nil {
		t.Error("expected error for duplicate ID, got nil")
	}

	if mgr.Count() != 1 {
		t.Errorf("expected 1 node, got %d", mgr.Count())
	}
}

func TestManager_Register_WithInvalidNode(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node := model.NewNode("", "test-node", "192.168.1.100", 8080)

	err := mgr.Register(node)
	if err == nil {
		t.Error("expected error for invalid node, got nil")
	}

	if mgr.Count() != 0 {
		t.Errorf("expected 0 nodes, got %d", mgr.Count())
	}
}

func TestManager_Unregister(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node := model.NewNode("node-1", "test-node", "192.168.1.100", 8080)
	mgr.Register(node)

	err := mgr.Unregister("node-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mgr.Count() != 0 {
		t.Errorf("expected 0 nodes, got %d", mgr.Count())
	}
}

func TestManager_Unregister_NotFound(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	err := mgr.Unregister("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent node, got nil")
	}
}

func TestManager_GetByID(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node := model.NewNode("node-1", "test-node", "192.168.1.100", 8080)
	node.AddGroup("web")
	node.SetLabel("env", "production")
	mgr.Register(node)

	retrieved, err := mgr.GetByID("node-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if retrieved.ID != node.ID {
		t.Errorf("expected ID '%s', got '%s'", node.ID, retrieved.ID)
	}
	if retrieved.Name != node.Name {
		t.Errorf("expected Name '%s', got '%s'", node.Name, retrieved.Name)
	}
	if !retrieved.HasGroup("web") {
		t.Error("expected to have group 'web'")
	}
	if !retrieved.HasLabel("env", "production") {
		t.Error("expected to have label env=production")
	}
}

func TestManager_GetByID_NotFound(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	_, err := mgr.GetByID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent node, got nil")
	}
}

func TestManager_GetByID_IsClone(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node := model.NewNode("node-1", "test-node", "192.168.1.100", 8080)
	mgr.Register(node)

	retrieved1, _ := mgr.GetByID("node-1")
	retrieved1.Name = "modified-name"

	retrieved2, _ := mgr.GetByID("node-1")
	if retrieved2.Name != "test-node" {
		t.Errorf("expected original name 'test-node', got '%s'", retrieved2.Name)
	}
}

func TestManager_List(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node1 := model.NewNode("node-1", "test-node-1", "192.168.1.100", 8080)
	node2 := model.NewNode("node-2", "test-node-2", "192.168.1.101", 8081)
	node3 := model.NewNode("node-3", "test-node-3", "192.168.1.102", 8082)

	mgr.Register(node1)
	mgr.Register(node2)
	mgr.Register(node3)

	nodes := mgr.List()
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodes))
	}
}

func TestManager_List_IsClone(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node := model.NewNode("node-1", "test-node", "192.168.1.100", 8080)
	mgr.Register(node)

	nodes1 := mgr.List()
	nodes1[0].Name = "modified-name"

	nodes2 := mgr.List()
	if nodes2[0].Name != "test-node" {
		t.Errorf("expected original name 'test-node', got '%s'", nodes2[0].Name)
	}
}

func TestManager_GetByGroup(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node1 := model.NewNode("node-1", "test-node-1", "192.168.1.100", 8080)
	node1.AddGroup("web")
	node2 := model.NewNode("node-2", "test-node-2", "192.168.1.101", 8081)
	node2.AddGroup("web")
	node2.AddGroup("database")
	node3 := model.NewNode("node-3", "test-node-3", "192.168.1.102", 8082)
	node3.AddGroup("database")

	mgr.Register(node1)
	mgr.Register(node2)
	mgr.Register(node3)

	webNodes := mgr.GetByGroup("web")
	if len(webNodes) != 2 {
		t.Errorf("expected 2 web nodes, got %d", len(webNodes))
	}

	databaseNodes := mgr.GetByGroup("database")
	if len(databaseNodes) != 2 {
		t.Errorf("expected 2 database nodes, got %d", len(databaseNodes))
	}

	cacheNodes := mgr.GetByGroup("cache")
	if len(cacheNodes) != 0 {
		t.Errorf("expected 0 cache nodes, got %d", len(cacheNodes))
	}
}

func TestManager_GetByLabels(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node1 := model.NewNode("node-1", "test-node-1", "192.168.1.100", 8080)
	node1.SetLabel("env", "production")
	node1.SetLabel("region", "us-west")
	node2 := model.NewNode("node-2", "test-node-2", "192.168.1.101", 8081)
	node2.SetLabel("env", "production")
	node2.SetLabel("region", "us-east")
	node3 := model.NewNode("node-3", "test-node-3", "192.168.1.102", 8082)
	node3.SetLabel("env", "staging")

	mgr.Register(node1)
	mgr.Register(node2)
	mgr.Register(node3)

	prodNodes := mgr.GetByLabels(map[string]string{"env": "production"})
	if len(prodNodes) != 2 {
		t.Errorf("expected 2 production nodes, got %d", len(prodNodes))
	}

	usWestProdNodes := mgr.GetByLabels(map[string]string{"env": "production", "region": "us-west"})
	if len(usWestProdNodes) != 1 {
		t.Errorf("expected 1 us-west production node, got %d", len(usWestProdNodes))
	}

	stagingNodes := mgr.GetByLabels(map[string]string{"env": "staging"})
	if len(stagingNodes) != 1 {
		t.Errorf("expected 1 staging node, got %d", len(stagingNodes))
	}
}

func TestManager_UpdateStatus(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node := model.NewNode("node-1", "test-node", "192.168.1.100", 8080)
	mgr.Register(node)

	err := mgr.UpdateStatus("node-1", model.NodeStatusOffline)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	retrieved, _ := mgr.GetByID("node-1")
	if retrieved.Status != model.NodeStatusOffline {
		t.Errorf("expected status 'offline', got '%s'", retrieved.Status)
	}
}

func TestManager_UpdateStatus_NotFound(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	err := mgr.UpdateStatus("nonexistent", model.NodeStatusOffline)
	if err == nil {
		t.Error("expected error for nonexistent node, got nil")
	}
}

func TestManager_GetOnlineNodes(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	node1 := model.NewNode("node-1", "test-node-1", "192.168.1.100", 8080)
	node2 := model.NewNode("node-2", "test-node-2", "192.168.1.101", 8081)
	node3 := model.NewNode("node-3", "test-node-3", "192.168.1.102", 8082)

	mgr.Register(node1)
	mgr.Register(node2)
	mgr.Register(node3)

	mgr.UpdateStatus("node-2", model.NodeStatusOffline)

	onlineNodes := mgr.GetOnlineNodes()
	if len(onlineNodes) != 2 {
		t.Errorf("expected 2 online nodes, got %d", len(onlineNodes))
	}

	for _, node := range onlineNodes {
		if node.ID == "node-2" {
			t.Error("node-2 should not be in online nodes")
		}
	}
}

func TestManager_Count(t *testing.T) {
	store := NewInMemoryNodeStore()
	mgr := NewManager(store)

	if mgr.Count() != 0 {
		t.Errorf("expected 0 nodes, got %d", mgr.Count())
	}

	node1 := model.NewNode("node-1", "test-node-1", "192.168.1.100", 8080)
	mgr.Register(node1)
	if mgr.Count() != 1 {
		t.Errorf("expected 1 node, got %d", mgr.Count())
	}

	node2 := model.NewNode("node-2", "test-node-2", "192.168.1.101", 8081)
	mgr.Register(node2)
	if mgr.Count() != 2 {
		t.Errorf("expected 2 nodes, got %d", mgr.Count())
	}

	mgr.Unregister("node-1")
	if mgr.Count() != 1 {
		t.Errorf("expected 1 node after unregister, got %d", mgr.Count())
	}
}

func TestInMemoryNodeStore(t *testing.T) {
	store := NewInMemoryNodeStore()

	node := model.NewNode("node-1", "test-node", "192.168.1.100", 8080)

	store.Set("node-1", node)

	retrieved, ok := store.Get("node-1")
	if !ok {
		t.Error("expected node to be found")
	}
	if retrieved.ID != node.ID {
		t.Errorf("expected ID '%s', got '%s'", node.ID, retrieved.ID)
	}

	all := store.GetAll()
	if len(all) != 1 {
		t.Errorf("expected 1 node, got %d", len(all))
	}

	deleted := store.Delete("node-1")
	if !deleted {
		t.Error("expected node to be deleted")
	}

	_, ok = store.Get("node-1")
	if ok {
		t.Error("expected node to not be found after deletion")
	}

	deleted = store.Delete("nonexistent")
	if deleted {
		t.Error("expected false for nonexistent node deletion")
	}
}
