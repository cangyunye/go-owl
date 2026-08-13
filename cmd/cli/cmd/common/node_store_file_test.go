package common

import (
	"path/filepath"
	"testing"
)

func TestInMemoryNodeStoreAt_RoundTrip(t *testing.T) {
	store := NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	if err := store.Add(&NodeInfo{ID: "n1", Name: "web", Address: "1.2.3.4", Port: 22, Status: "offline"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded := NewInMemoryNodeStoreAt(store.dataFile)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	nodes, _ := reloaded.List()
	if len(nodes) != 1 || nodes[0].ID != "n1" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}
