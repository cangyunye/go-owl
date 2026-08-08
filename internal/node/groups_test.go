package node

import (
	"testing"
)

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		items    []string
		expected bool
	}{
		{"empty slice finds nothing", nil, []string{"web"}, false},
		{"empty items finds everything", []string{"web"}, nil, true},
		{"single match", []string{"web", "db"}, []string{"web"}, true},
		{"multiple match one", []string{"web", "db", "cache"}, []string{"worker", "db"}, true},
		{"no match", []string{"web", "db"}, []string{"worker"}, false},
		{"both empty", nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsAny(tt.slice, tt.items)
			if result != tt.expected {
				t.Errorf("ContainsAny(%v, %v) = %v, want %v", tt.slice, tt.items, result, tt.expected)
			}
		})
	}
}

func TestListNodesByGroups(t *testing.T) {
	source, err := NewLocalSource()
	if err != nil {
		t.Fatalf("NewLocalSource failed: %v", err)
	}
	source.AddNode(&LocalNode{ID: "n1", Groups: []string{"web", "api"}})
	source.AddNode(&LocalNode{ID: "n2", Groups: []string{"db"}})
	source.AddNode(&LocalNode{ID: "n3", Groups: []string{"web"}})

	resolver := &NodeResolver{localSource: source}

	nodes, err := ListNodesByGroups(resolver, []string{"web"})
	if err != nil {
		t.Fatalf("ListNodesByGroups failed: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes for group 'web', got %d", len(nodes))
	}

	nodes, err = ListNodesByGroups(resolver, []string{"web", "db"})
	if err != nil {
		t.Fatalf("ListNodesByGroups failed: %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("expected 3 unique nodes for groups [web, db], got %d", len(nodes))
	}

	nodes, err = ListNodesByGroups(resolver, []string{"nonexistent"})
	if err != nil {
		t.Fatalf("ListNodesByGroups failed: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes for nonexistent group, got %d", len(nodes))
	}
}
