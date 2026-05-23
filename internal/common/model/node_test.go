package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNode_New(t *testing.T) {
	node := NewNode("node-1", "test-node", "192.168.1.100", 8080, "root")

	if node.ID != "node-1" {
		t.Errorf("expected ID 'node-1', got '%s'", node.ID)
	}
	if node.Name != "test-node" {
		t.Errorf("expected Name 'test-node', got '%s'", node.Name)
	}
	if node.Address != "192.168.1.100" {
		t.Errorf("expected Address '192.168.1.100', got '%s'", node.Address)
	}
	if node.Port != 8080 {
		t.Errorf("expected Port 8080, got %d", node.Port)
	}
	if node.Status != NodeStatusOffline {
		t.Errorf("expected Status 'offline', got '%s'", node.Status)
	}
	if len(node.Groups) != 0 {
		t.Errorf("expected empty Groups, got %d", len(node.Groups))
	}
	if len(node.Labels) != 0 {
		t.Errorf("expected empty Labels, got %d", len(node.Labels))
	}
	if node.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if node.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestNode_Validate(t *testing.T) {
	tests := []struct {
		name    string
		node    *Node
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid node",
			node:    NewNode("node-1", "test-node", "192.168.1.100", 8080, "root"),
			wantErr: false,
		},
		{
			name: "missing ID",
			node: &Node{
				Name:    "test-node",
				Address: "192.168.1.100",
				Port:    8080,
			},
			wantErr: true,
			errMsg:  "node ID is required",
		},
		{
			name: "missing name",
			node: &Node{
				ID:      "node-1",
				Address: "192.168.1.100",
				Port:    8080,
			},
			wantErr: true,
			errMsg:  "node name is required",
		},
		{
			name: "missing address",
			node: &Node{
				ID:   "node-1",
				Name: "test-node",
				Port: 8080,
			},
			wantErr: true,
			errMsg:  "node address is required",
		},
		{
			name: "invalid port - zero",
			node: &Node{
				ID:      "node-1",
				Name:    "test-node",
				Address: "192.168.1.100",
				Port:    0,
			},
			wantErr: true,
			errMsg:  "node port must be between 1 and 65535",
		},
		{
			name: "invalid port - negative",
			node: &Node{
				ID:      "node-1",
				Name:    "test-node",
				Address: "192.168.1.100",
				Port:    -1,
			},
			wantErr: true,
			errMsg:  "node port must be between 1 and 65535",
		},
		{
			name: "invalid port - too high",
			node: &Node{
				ID:      "node-1",
				Name:    "test-node",
				Address: "192.168.1.100",
				Port:    65536,
			},
			wantErr: true,
			errMsg:  "node port must be between 1 and 65535",
		},
		{
			name: "valid port - min",
			node: &Node{
				ID:      "node-1",
				Name:    "test-node",
				Address: "192.168.1.100",
				Port:    1,
			},
			wantErr: false,
		},
		{
			name: "valid port - max",
			node: &Node{
				ID:      "node-1",
				Name:    "test-node",
				Address: "192.168.1.100",
				Port:    65535,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.node.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error '%s', got nil", tt.errMsg)
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestNode_SetStatus(t *testing.T) {
	node := NewNode("node-1", "test-node", "192.168.1.100", 8080, "root")
	originalUpdatedAt := node.UpdatedAt

	time.Sleep(1 * time.Millisecond)
	node.SetStatus(NodeStatusOnline)

	if node.Status != NodeStatusOnline {
		t.Errorf("expected Status 'online', got '%s'", node.Status)
	}
	if !node.UpdatedAt.After(originalUpdatedAt) {
		t.Error("UpdatedAt should be updated")
	}
}

func TestNode_Groups(t *testing.T) {
	node := NewNode("node-1", "test-node", "192.168.1.100", 8080, "root")

	node.AddGroup("web")
	if len(node.Groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(node.Groups))
	}

	node.AddGroup("database")
	if len(node.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(node.Groups))
	}

	node.AddGroup("web")
	if len(node.Groups) != 2 {
		t.Errorf("expected 2 groups (no duplicate), got %d", len(node.Groups))
	}

	if !node.HasGroup("web") {
		t.Error("expected to have group 'web'")
	}
	if !node.HasGroup("database") {
		t.Error("expected to have group 'database'")
	}
	if node.HasGroup("cache") {
		t.Error("expected not to have group 'cache'")
	}

	node.RemoveGroup("web")
	if len(node.Groups) != 1 {
		t.Errorf("expected 1 group after removal, got %d", len(node.Groups))
	}
	if node.HasGroup("web") {
		t.Error("expected not to have group 'web' after removal")
	}
}

func TestNode_Labels(t *testing.T) {
	node := NewNode("node-1", "test-node", "192.168.1.100", 8080, "root")

	node.SetLabel("env", "production")
	node.SetLabel("region", "us-west")

	if !node.HasLabel("env", "production") {
		t.Error("expected to have label env=production")
	}
	if !node.HasLabel("region", "us-west") {
		t.Error("expected to have label region=us-west")
	}
	if node.HasLabel("env", "staging") {
		t.Error("expected not to have label env=staging")
	}

	node.RemoveLabel("env")
	if node.HasLabel("env", "production") {
		t.Error("expected not to have label env after removal")
	}
}

func TestNode_MatchLabels(t *testing.T) {
	node := NewNode("node-1", "test-node", "192.168.1.100", 8080, "root")
	node.SetLabel("env", "production")
	node.SetLabel("region", "us-west")

	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{
			name:   "empty labels - match all",
			labels: map[string]string{},
			want:   true,
		},
		{
			name:   "single matching label",
			labels: map[string]string{"env": "production"},
			want:   true,
		},
		{
			name:   "multiple matching labels",
			labels: map[string]string{"env": "production", "region": "us-west"},
			want:   true,
		},
		{
			name:   "single non-matching label",
			labels: map[string]string{"env": "staging"},
			want:   false,
		},
		{
			name:   "one matching, one non-matching",
			labels: map[string]string{"env": "production", "region": "us-east"},
			want:   false,
		},
		{
			name:   "non-existent label",
			labels: map[string]string{"os": "linux"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := node.MatchLabels(tt.labels); got != tt.want {
				t.Errorf("MatchLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNode_JSONSerialize(t *testing.T) {
	node := NewNode("node-1", "test-node", "192.168.1.100", 8080, "root")
	node.AddGroup("web")
	node.SetLabel("env", "production")
	node.SetMetadata("os", "linux")

	data, err := node.JSONSerialize()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("failed to parse JSON: %v", err)
	}

	if parsed["id"] != "node-1" {
		t.Errorf("expected id 'node-1', got '%v'", parsed["id"])
	}
	if parsed["name"] != "test-node" {
		t.Errorf("expected name 'test-node', got '%v'", parsed["name"])
	}
}

func TestNode_JSONDeserialize(t *testing.T) {
	jsonData := `{
		"id": "node-1",
		"name": "test-node",
		"address": "192.168.1.100",
		"port": 8080,
		"status": "online",
		"groups": ["web", "database"],
		"labels": {"env": "production"},
		"metadata": {"os": "linux"}
	}`

	node := &Node{}
	if err := node.JSONDeserialize([]byte(jsonData)); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if node.ID != "node-1" {
		t.Errorf("expected ID 'node-1', got '%s'", node.ID)
	}
	if node.Name != "test-node" {
		t.Errorf("expected Name 'test-node', got '%s'", node.Name)
	}
	if node.Address != "192.168.1.100" {
		t.Errorf("expected Address '192.168.1.100', got '%s'", node.Address)
	}
	if node.Port != 8080 {
		t.Errorf("expected Port 8080, got %d", node.Port)
	}
	if node.Status != NodeStatusOnline {
		t.Errorf("expected Status 'online', got '%s'", node.Status)
	}
	if len(node.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(node.Groups))
	}
	if node.Labels["env"] != "production" {
		t.Errorf("expected label env=production, got '%s'", node.Labels["env"])
	}
}

func TestNode_Clone(t *testing.T) {
	node := NewNode("node-1", "test-node", "192.168.1.100", 8080, "root")
	node.AddGroup("web")
	node.AddGroup("database")
	node.SetLabel("env", "production")
	node.SetMetadata("os", "linux")

	clone := node.Clone()

	if clone.ID != node.ID {
		t.Errorf("expected ID '%s', got '%s'", node.ID, clone.ID)
	}
	if clone.Name != node.Name {
		t.Errorf("expected Name '%s', got '%s'", node.Name, clone.Name)
	}
	if clone.Address != node.Address {
		t.Errorf("expected Address '%s', got '%s'", node.Address, clone.Address)
	}
	if clone.Port != node.Port {
		t.Errorf("expected Port %d, got %d", node.Port, clone.Port)
	}
	if len(clone.Groups) != len(node.Groups) {
		t.Errorf("expected %d groups, got %d", len(node.Groups), len(clone.Groups))
	}
	if clone.Labels["env"] != node.Labels["env"] {
		t.Errorf("expected label env='%s', got '%s'", node.Labels["env"], clone.Labels["env"])
	}

	clone.Name = "modified-node"
	clone.AddGroup("cache")
	clone.SetLabel("env", "staging")

	if node.Name != "test-node" {
		t.Errorf("original node name should not change, got '%s'", node.Name)
	}
	if len(node.Groups) != 2 {
		t.Errorf("original node groups should not change, got %d", len(node.Groups))
	}
	if node.Labels["env"] != "production" {
		t.Errorf("original node label should not change, got '%s'", node.Labels["env"])
	}
}

func TestNode_SetMetadata(t *testing.T) {
	node := NewNode("node-1", "test-node", "192.168.1.100", 8080, "root")

	node.SetMetadata("cpu", "8 cores")
	node.SetMetadata("memory", "16GB")

	if node.Metadata["cpu"] != "8 cores" {
		t.Errorf("expected metadata cpu='8 cores', got '%s'", node.Metadata["cpu"])
	}
	if node.Metadata["memory"] != "16GB" {
		t.Errorf("expected metadata memory='16GB', got '%s'", node.Metadata["memory"])
	}

	node.SetMetadata("cpu", "16 cores")
	if node.Metadata["cpu"] != "16 cores" {
		t.Errorf("expected metadata cpu='16 cores' (updated), got '%s'", node.Metadata["cpu"])
	}
}
