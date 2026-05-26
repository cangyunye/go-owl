package common

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *NodeStoreDB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite3: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		address TEXT NOT NULL DEFAULT '',
		port INTEGER NOT NULL DEFAULT 22,
		user TEXT NOT NULL DEFAULT 'root',
		password TEXT NOT NULL DEFAULT '',
		ssh_key TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'offline',
		groups TEXT NOT NULL DEFAULT '[]',
		labels TEXT NOT NULL DEFAULT '{}',
		proxy_jump TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_check_at DATETIME
	)`)
	if err != nil {
		t.Fatalf("failed to create nodes table: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewNodeStoreDB(db)
}

func TestNodeStoreDB_InterfaceCompliance(t *testing.T) {
	var store NodeStore = (*NodeStoreDB)(nil)
	_ = store
}

func TestNodeStoreDB_Add(t *testing.T) {
	store := setupTestDB(t)

	node := &NodeInfo{
		ID:      "node-1",
		Name:    "test-node",
		Address: "192.168.1.1",
		Port:    22,
		User:    "root",
		Status:  "online",
		Groups:  []string{"web", "production"},
		Labels:  map[string]string{"env": "prod", "region": "cn-east"},
	}

	err := store.Add(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
	if node.UpdatedAt == "" {
		t.Error("expected UpdatedAt to be set")
	}

	stored, err := store.Get("node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.ID != node.ID {
		t.Errorf("expected ID %q, got %q", node.ID, stored.ID)
	}
	if stored.Name != node.Name {
		t.Errorf("expected Name %q, got %q", node.Name, stored.Name)
	}
	if len(stored.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(stored.Groups))
	}
	if stored.Groups[0] != "web" {
		t.Errorf("expected groups[0] %q, got %q", "web", stored.Groups[0])
	}
	if len(stored.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(stored.Labels))
	}
	if stored.Labels["env"] != "prod" {
		t.Errorf("expected labels[env] %q, got %q", "prod", stored.Labels["env"])
	}
}

func TestNodeStoreDB_Add_Duplicate(t *testing.T) {
	store := setupTestDB(t)

	node := &NodeInfo{ID: "node-1", Name: "first"}
	err := store.Add(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = store.Add(&NodeInfo{ID: "node-1", Name: "second"})
	if err == nil {
		t.Error("expected error for duplicate ID")
	}
}

func TestNodeStoreDB_Add_EmptyGroupsAndLabels(t *testing.T) {
	store := setupTestDB(t)

	node := &NodeInfo{ID: "node-1", Name: "test"}
	err := store.Add(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored, err := store.Get("node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Groups == nil {
		t.Error("expected non-nil Groups")
	}
	if stored.Labels == nil {
		t.Error("expected non-nil Labels")
	}
}

func TestNodeStoreDB_Get_NotFound(t *testing.T) {
	store := setupTestDB(t)

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent node")
	}
}

func TestNodeStoreDB_List_Empty(t *testing.T) {
	store := setupTestDB(t)

	nodes, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestNodeStoreDB_List_Multiple(t *testing.T) {
	store := setupTestDB(t)

	nodes := []*NodeInfo{
		{ID: "node-1", Name: "first", Groups: []string{"web"}, Labels: map[string]string{"env": "dev"}},
		{ID: "node-2", Name: "second", Groups: []string{"db"}, Labels: map[string]string{"env": "prod"}},
		{ID: "node-3", Name: "third", Groups: []string{"cache"}, Labels: map[string]string{"env": "staging"}},
	}
	for _, n := range nodes {
		if err := store.Add(n); err != nil {
			t.Fatalf("failed to add node %s: %v", n.ID, err)
		}
	}

	listed, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listed) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(listed))
	}

	idMap := make(map[string]*NodeInfo)
	for _, n := range listed {
		idMap[n.ID] = n
	}
	for _, original := range nodes {
		stored, ok := idMap[original.ID]
		if !ok {
			t.Errorf("expected node %s in list", original.ID)
			continue
		}
		if stored.Name != original.Name {
			t.Errorf("node %s: expected Name %q, got %q", original.ID, original.Name, stored.Name)
		}
		if len(stored.Groups) != len(original.Groups) {
			t.Errorf("node %s: expected %d groups, got %d", original.ID, len(original.Groups), len(stored.Groups))
		}
	}
}

func TestNodeStoreDB_Remove(t *testing.T) {
	store := setupTestDB(t)

	node := &NodeInfo{ID: "node-1", Name: "test"}
	if err := store.Add(node); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := store.Remove("node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = store.Get("node-1")
	if err == nil {
		t.Error("expected error after removal")
	}
}

func TestNodeStoreDB_Remove_NotFound(t *testing.T) {
	store := setupTestDB(t)

	err := store.Remove("nonexistent")
	if err == nil {
		t.Error("expected error for removing nonexistent node")
	}
}

func TestNodeStoreDB_Update(t *testing.T) {
	store := setupTestDB(t)

	node := &NodeInfo{
		ID:      "node-1",
		Name:    "original",
		Address: "10.0.0.1",
		Port:    22,
		User:    "root",
		Status:  "offline",
		Groups:  []string{"web"},
		Labels:  map[string]string{"ver": "1.0"},
	}
	if err := store.Add(node); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	originalUpdatedAt := node.UpdatedAt

	time.Sleep(1500 * time.Millisecond)

	node.Name = "updated"
	node.Address = "10.0.0.2"
	node.Port = 2222
	node.User = "admin"
	node.Status = "online"
	node.Groups = []string{"web", "db"}
	node.Labels = map[string]string{"ver": "2.0", "env": "prod"}
	node.ProxyJump = "jump.example.com"

	err := store.Update(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.UpdatedAt == originalUpdatedAt {
		t.Error("expected UpdatedAt to be refreshed on update")
	}

	stored, err := store.Get("node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Name != "updated" {
		t.Errorf("expected Name %q, got %q", "updated", stored.Name)
	}
	if stored.Address != "10.0.0.2" {
		t.Errorf("expected Address %q, got %q", "10.0.0.2", stored.Address)
	}
	if stored.Port != 2222 {
		t.Errorf("expected Port %d, got %d", 2222, stored.Port)
	}
	if stored.User != "admin" {
		t.Errorf("expected User %q, got %q", "admin", stored.User)
	}
	if stored.Status != "online" {
		t.Errorf("expected Status %q, got %q", "online", stored.Status)
	}
	if stored.ProxyJump != "jump.example.com" {
		t.Errorf("expected ProxyJump %q, got %q", "jump.example.com", stored.ProxyJump)
	}
	if len(stored.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(stored.Groups))
	}
	if stored.Groups[1] != "db" {
		t.Errorf("expected groups[1] %q, got %q", "db", stored.Groups[1])
	}
	if len(stored.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(stored.Labels))
	}
	if stored.Labels["ver"] != "2.0" {
		t.Errorf("expected labels[ver] %q, got %q", "2.0", stored.Labels["ver"])
	}
}

func TestNodeStoreDB_Update_NotFound(t *testing.T) {
	store := setupTestDB(t)

	err := store.Update(&NodeInfo{ID: "nonexistent", Name: "ghost"})
	if err == nil {
		t.Error("expected error for updating nonexistent node")
	}
}

func TestNodeStoreDB_Save(t *testing.T) {
	store := setupTestDB(t)

	err := store.Save()
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestNodeStoreDB_Load(t *testing.T) {
	store := setupTestDB(t)

	err := store.Load()
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestNodeStoreDB_Get_NullLastCheckAt(t *testing.T) {
	store := setupTestDB(t)

	node := &NodeInfo{ID: "node-1", Name: "test", LastCheckAt: ""}
	err := store.Add(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored, err := store.Get("node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.LastCheckAt != "" {
		t.Errorf("expected empty LastCheckAt, got %q", stored.LastCheckAt)
	}
}

func TestNodeStoreDB_Add_WithSSHKeyAndPassword(t *testing.T) {
	store := setupTestDB(t)

	node := &NodeInfo{
		ID:       "node-1",
		Name:     "ssh-node",
		Password: "secret123",
		SSHKey:   "/path/to/key",
	}
	err := store.Add(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored, err := store.Get("node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Password != "secret123" {
		t.Errorf("expected Password %q, got %q", "secret123", stored.Password)
	}
	if stored.SSHKey != "/path/to/key" {
		t.Errorf("expected SSHKey %q, got %q", "/path/to/key", stored.SSHKey)
	}
}

func TestNodeStoreDB_Constructor(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	store := NewNodeStoreDB(db)
	if store == nil {
		t.Error("expected non-nil NodeStoreDB")
	}
	if store.db != db {
		t.Error("expected same db connection")
	}
}
