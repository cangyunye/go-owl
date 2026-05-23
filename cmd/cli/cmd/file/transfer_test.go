package file

import (
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/node"
)

func TestResolvedToModelNodes(t *testing.T) {
	resolved := []*node.ResolvedNode{
		{
			ID:      "node-1",
			Name:    "Node 1",
			Address: "192.168.1.1",
			Port:    22,
			User:    "root",
			Groups:  []string{"web", "app"},
			Labels:  map[string]string{"env": "prod", "tier": "frontend"},
		},
		{
			ID:      "node-2",
			Name:    "Node 2",
			Address: "192.168.1.2",
			Port:    22,
			User:    "root",
			Groups:  []string{"db"},
			Labels:  map[string]string{"env": "prod"},
		},
	}

	models := resolvedToModelNodes(resolved)
	if len(models) != 2 {
		t.Fatalf("expected 2 model nodes, got %d", len(models))
	}

	m1 := models[0]
	if m1.ID != "node-1" {
		t.Errorf("expected ID 'node-1', got '%s'", m1.ID)
	}
	if m1.Name != "Node 1" {
		t.Errorf("expected Name 'Node 1', got '%s'", m1.Name)
	}
	if m1.Address != "192.168.1.1" {
		t.Errorf("expected Address '192.168.1.1', got '%s'", m1.Address)
	}
	if m1.Port != 22 {
		t.Errorf("expected Port 22, got %d", m1.Port)
	}
	if m1.User != "root" {
		t.Errorf("expected User 'root', got '%s'", m1.User)
	}
	if m1.Status != model.NodeStatusOnline {
		t.Errorf("expected Status 'online', got '%s'", m1.Status)
	}
	if len(m1.Groups) != 2 || m1.Groups[0] != "web" || m1.Groups[1] != "app" {
		t.Errorf("expected Groups ['web','app'], got %v", m1.Groups)
	}
	if m1.Labels["env"] != "prod" || m1.Labels["tier"] != "frontend" {
		t.Errorf("unexpected Labels: %v", m1.Labels)
	}

	m2 := models[1]
	if m2.ID != "node-2" {
		t.Errorf("expected ID 'node-2', got '%s'", m2.ID)
	}
}

func TestResolvedToModelNodes_Empty(t *testing.T) {
	models := resolvedToModelNodes(nil)
	if len(models) != 0 {
		t.Errorf("expected empty result for nil input, got %d", len(models))
	}

	models = resolvedToModelNodes([]*node.ResolvedNode{})
	if len(models) != 0 {
		t.Errorf("expected empty result for empty slice, got %d", len(models))
	}
}

func TestResolvedToModelNodes_NilFields(t *testing.T) {
	resolved := []*node.ResolvedNode{
		{
			ID: "node-1",
		},
	}

	models := resolvedToModelNodes(resolved)
	if len(models) != 1 {
		t.Fatalf("expected 1 model node, got %d", len(models))
	}

	m := models[0]
	if m.Groups == nil {
		t.Error("expected non-nil Groups")
	}
	if m.Labels == nil {
		t.Error("expected non-nil Labels")
	}
}

func TestResolvedToModelNodes_OriginalUnmodified(t *testing.T) {
	original := &node.ResolvedNode{
		ID:     "node-1",
		Groups: []string{"web"},
		Labels: map[string]string{"env": "prod"},
	}

	models := resolvedToModelNodes([]*node.ResolvedNode{original})
	m := models[0]

	m.Groups[0] = "modified"
	m.Labels["env"] = "modified"

	if original.Groups[0] != "web" {
		t.Error("original Groups should not be modified")
	}
	if original.Labels["env"] != "prod" {
		t.Error("original Labels should not be modified")
	}
}

func TestGenerateProgressBar(t *testing.T) {
	tests := []struct {
		name    string
		percent float64
		width   int
		wantLen int
	}{
		{"0 percent", 0, 10, 12},       // "[..........]" = 12 chars
		{"50 percent", 50, 10, 12},     // "[=====-----]" = 12 chars
		{"100 percent", 100, 10, 12},   // "[==========]" = 12 chars
		{"25 percent", 25, 8, 10},      // "[==------]" = 10 chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateProgressBar(tt.percent, tt.width)
			if len(got) != tt.wantLen {
				t.Errorf("expected length %d, got %d: %s", tt.wantLen, len(got), got)
			}
			if got[0] != '[' || got[len(got)-1] != ']' {
				t.Errorf("expected brackets, got '%s'", got)
			}
		})
	}
}

func TestGenerateProgressBar_EdgeCases(t *testing.T) {
	t.Run("zero width", func(t *testing.T) {
		got := generateProgressBar(50, 0)
		if got != "[]" {
			t.Errorf("expected '[]', got '%s'", got)
		}
	})

	t.Run("negative percent", func(t *testing.T) {
		got := generateProgressBar(-10, 5)
		expected := "[-----]"
		if got != expected {
			t.Errorf("expected '%s', got '%s'", expected, got)
		}
	})

	t.Run("over 100 percent", func(t *testing.T) {
		got := generateProgressBar(150, 5)
		expected := "[=======]"
		if got != expected {
			t.Errorf("expected '%s', got '%s'", expected, got)
		}
	})
}

func TestParseNodeList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single", "node1", []string{"node1"}},
		{"multiple", "node1,node2,node3", []string{"node1", "node2", "node3"}},
		{"empty", "", []string{}},
		{"trailing comma", "node1,", []string{"node1"}},
		{"leading comma", ",node1", []string{"node1"}},
		{"only comma", ",,,", []string(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNodeList(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("expected %d items, got %d: %v", len(tt.want), len(got), got)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("item[%d]: expected '%s', got '%s'", i, tt.want[i], got[i])
				}
			}
		})
	}
}

func TestGetFileNameFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"simple file", "app.tar.gz", "app.tar.gz"},
		{"with dir", "/path/to/file.txt", "file.txt"},
		{"with trailing slash", "/path/to/dir/", ""},
		{"root file", "/file.txt", "file.txt"},
		{"empty", "", ""},
		{"windows path", "C:\\path\\file.txt", "C:\\path\\file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getFileNameFromPath(tt.path)
			if got != tt.want {
				t.Errorf("expected '%s', got '%s'", tt.want, got)
			}
		})
	}
}
