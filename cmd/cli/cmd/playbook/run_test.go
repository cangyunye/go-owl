package playbook

import (
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/node"
)

func TestNewAdapterNodeManager(t *testing.T) {
	resolvedNodes := []*node.ResolvedNode{
		{ID: "node-1", Name: "Node 1", Address: "192.168.1.1", Port: 22, User: "root", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
		{ID: "node-2", Name: "Node 2", Address: "192.168.1.2", Port: 22, User: "root", Groups: []string{"db"}, Labels: map[string]string{"env": "prod"}},
	}

	mgr := newAdapterNodeManager(nil, resolvedNodes)

	if mgr.Count() != 2 {
		t.Errorf("expected Count()=2, got %d", mgr.Count())
	}
}

func TestAdapterNodeManager_GetByID(t *testing.T) {
	resolvedNodes := []*node.ResolvedNode{
		{ID: "node-1", Name: "Node 1", Address: "192.168.1.1", Port: 22, User: "root"},
	}
	mgr := newAdapterNodeManager(nil, resolvedNodes)

	n, err := mgr.GetByID("node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.ID != "node-1" {
		t.Errorf("expected ID 'node-1', got '%s'", n.ID)
	}
	if n.Name != "Node 1" {
		t.Errorf("expected Name 'Node 1', got '%s'", n.Name)
	}
	if n.Port != 22 {
		t.Errorf("expected Port 22, got %d", n.Port)
	}

	_, err = mgr.GetByID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent node")
	}
}

func TestAdapterNodeManager_List(t *testing.T) {
	resolvedNodes := []*node.ResolvedNode{
		{ID: "node-1"},
		{ID: "node-2"},
		{ID: "node-3"},
	}
	mgr := newAdapterNodeManager(nil, resolvedNodes)

	nodes := mgr.List()
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodes))
	}
}

func TestAdapterNodeManager_GetByGroup(t *testing.T) {
	resolvedNodes := []*node.ResolvedNode{
		{ID: "node-1", Groups: []string{"web", "app"}},
		{ID: "node-2", Groups: []string{"db"}},
		{ID: "node-3", Groups: []string{"web"}},
	}
	mgr := newAdapterNodeManager(nil, resolvedNodes)

	webNodes := mgr.GetByGroup("web")
	if len(webNodes) != 2 {
		t.Errorf("expected 2 web nodes, got %d", len(webNodes))
	}

	dbNodes := mgr.GetByGroup("db")
	if len(dbNodes) != 1 {
		t.Errorf("expected 1 db node, got %d", len(dbNodes))
	}

	cacheNodes := mgr.GetByGroup("cache")
	if len(cacheNodes) != 0 {
		t.Errorf("expected 0 cache nodes, got %d", len(cacheNodes))
	}
}

func TestAdapterNodeManager_GetByLabels(t *testing.T) {
	resolvedNodes := []*node.ResolvedNode{
		{ID: "node-1", Labels: map[string]string{"env": "prod", "tier": "frontend"}},
		{ID: "node-2", Labels: map[string]string{"env": "prod", "tier": "backend"}},
		{ID: "node-3", Labels: map[string]string{"env": "staging"}},
	}
	mgr := newAdapterNodeManager(nil, resolvedNodes)

	prodNodes := mgr.GetByLabels(map[string]string{"env": "prod"})
	if len(prodNodes) != 2 {
		t.Errorf("expected 2 prod nodes, got %d", len(prodNodes))
	}

	frontendNodes := mgr.GetByLabels(map[string]string{"env": "prod", "tier": "frontend"})
	if len(frontendNodes) != 1 {
		t.Errorf("expected 1 frontend node, got %d", len(frontendNodes))
	}

	nonexistentNodes := mgr.GetByLabels(map[string]string{"env": "nonexistent"})
	if len(nonexistentNodes) != 0 {
		t.Errorf("expected 0 nonexistent nodes, got %d", len(nonexistentNodes))
	}
}

func TestAdapterNodeManager_GetOnlineNodes(t *testing.T) {
	resolvedNodes := []*node.ResolvedNode{
		{ID: "node-1"},
		{ID: "node-2"},
	}
	mgr := newAdapterNodeManager(nil, resolvedNodes)

	onlineNodes := mgr.GetOnlineNodes()
	if len(onlineNodes) != 2 {
		t.Errorf("expected 2 online nodes, got %d", len(onlineNodes))
	}
}

func TestAdapterNodeManager_RegisterUnregister(t *testing.T) {
	mgr := newAdapterNodeManager(nil, nil)
	err := mgr.Register(&model.Node{ID: "test"})
	if err != nil {
		t.Errorf("Register should not error: %v", err)
	}
	err = mgr.Unregister("test")
	if err != nil {
		t.Errorf("Unregister should not error: %v", err)
	}
	err = mgr.UpdateStatus("test", model.NodeStatusOnline)
	if err != nil {
		t.Errorf("UpdateStatus should not error: %v", err)
	}
}

func TestParseNodeIDsList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single", "node1", []string{"node1"}},
		{"multiple", "node1,node2,node3", []string{"node1", "node2", "node3"}},
		{"with spaces", " node1 , node2 ", []string{"node1", "node2"}},
		{"empty", "", []string{}},
		{"trailing comma", "node1,", []string{"node1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNodeIDsList(tt.input)
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

func TestSplitStringList(t *testing.T) {
	tests := []struct {
		name string
		s    string
		sep  string
		want []string
	}{
		{"comma", "a,b,c", ",", []string{"a", "b", "c"}},
		{"single", "abc", ",", []string{"abc"}},
		{"empty", "", ",", []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitStringList(tt.s, tt.sep)
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

func TestTrimStringList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no trim", "hello", "hello"},
		{"leading spaces", "  hello", "hello"},
		{"trailing spaces", "hello  ", "hello"},
		{"both spaces", "  hello  ", "hello"},
		{"tabs", "\thello\t", "hello"},
		{"empty", "", ""},
		{"only spaces", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimStringList(tt.input)
			if got != tt.want {
				t.Errorf("expected '%s', got '%s'", tt.want, got)
			}
		})
	}
}

func TestSplitKeyValueList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"simple", "key=value", []string{"key", "value"}},
		{"no value", "key=", []string{"key", ""}},
		{"no equals", "keyvalue", []string{"keyvalue"}},
		{"multiple equals", "key=val=ue", []string{"key", "val=ue"}},
		{"empty", "", []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitKeyValueList(tt.input)
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

func TestParsePlaybookRunExtraVars(t *testing.T) {
	vars := parsePlaybookRunExtraVars([]string{"version=1.2.3", "env=prod"})
	if len(vars) != 2 {
		t.Errorf("expected 2 vars, got %d", len(vars))
	}
	if vars["version"] != "1.2.3" {
		t.Errorf("expected version='1.2.3', got '%v'", vars["version"])
	}
	if vars["env"] != "prod" {
		t.Errorf("expected env='prod', got '%v'", vars["env"])
	}

	empty := parsePlaybookRunExtraVars(nil)
	if len(empty) != 0 {
		t.Errorf("expected 0 vars for nil input, got %d", len(empty))
	}

	empty2 := parsePlaybookRunExtraVars([]string{})
	if len(empty2) != 0 {
		t.Errorf("expected 0 vars for empty input, got %d", len(empty2))
	}
}

func TestContainsNodeIDList(t *testing.T) {
	list := []string{"node1", "node2", "node3"}

	if !containsNodeIDList(list, "node1") {
		t.Error("expected node1 to be found")
	}
	if !containsNodeIDList(list, "node3") {
		t.Error("expected node3 to be found")
	}
	if containsNodeIDList(list, "node4") {
		t.Error("expected node4 not to be found")
	}
	if containsNodeIDList(nil, "node1") {
		t.Error("expected false for nil list")
	}
}

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"long string", "hello world", 8, "hello..."},
		{"empty", "", 5, ""},
		{"min length", "hello", 3, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateStr(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("expected '%s', got '%s'", tt.want, got)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single line", "hello", []string{"hello"}},
		{"multiple lines", "line1\nline2\nline3", []string{"line1", "line2", "line3"}},
		{"trailing newline", "line1\nline2\n", []string{"line1", "line2"}},
		{"empty", "", []string{}},
		{"only newline", "\n", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("expected %d lines, got %d: %v", len(tt.want), len(got), got)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line[%d]: expected '%s', got '%s'", i, tt.want[i], got[i])
				}
			}
		})
	}
}

func TestRunSamplePlaybook(t *testing.T) {
	nodes := []*model.Node{
		{ID: "node-1", Status: model.NodeStatusOnline},
		{ID: "node-2", Status: model.NodeStatusOnline},
	}
	runSamplePlaybook(nodes)
}

func TestAdapterCommandExecutor_Execute(t *testing.T) {
	exec := &adapterCommandExecutor{v2Exec: nil}
	err := exec.Execute(nil, nil)
	if err != nil {
		t.Errorf("Execute should not error: %v", err)
	}
}

func TestAdapterNodeManager_CreateWithEmptyNodes(t *testing.T) {
	mgr := newAdapterNodeManager(nil, nil)
	if mgr.Count() != 0 {
		t.Errorf("expected empty manager, got Count()=%d", mgr.Count())
	}

	nodes := mgr.List()
	if len(nodes) != 0 {
		t.Errorf("expected empty list, got %d", len(nodes))
	}
}

func TestAdapterNodeManager_GetByID_StatusPreserved(t *testing.T) {
	resolvedNodes := []*node.ResolvedNode{
		{ID: "node-1", Name: "Web Server", Address: "10.0.0.1", Port: 2222, User: "admin", Groups: []string{"web"}, Labels: map[string]string{"env": "staging"}},
	}
	mgr := newAdapterNodeManager(nil, resolvedNodes)

	n, _ := mgr.GetByID("node-1")
	if n.Status != model.NodeStatusOnline {
		t.Errorf("expected Status 'online', got '%s'", n.Status)
	}
	if n.User != "admin" {
		t.Errorf("expected User 'admin', got '%s'", n.User)
	}
	if n.Address != "10.0.0.1" {
		t.Errorf("expected Address '10.0.0.1', got '%s'", n.Address)
	}
	if n.Port != 2222 {
		t.Errorf("expected Port 2222, got %d", n.Port)
	}
	if len(n.Groups) != 1 || n.Groups[0] != "web" {
		t.Errorf("expected Groups ['web'], got %v", n.Groups)
	}
	if n.Labels["env"] != "staging" {
		t.Errorf("expected Labels['env']='staging', got '%s'", n.Labels["env"])
	}
}

func TestPlaybookRunCmdCreation(t *testing.T) {
	cmd := NewPlaybookRunCmd()
	if cmd == nil {
		t.Fatal("expected command to be created")
	}
	if cmd.Use != "run <playbook-file>" {
		t.Errorf("expected Use 'run <playbook-file>', got '%s'", cmd.Use)
	}
}
