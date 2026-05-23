package common

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

func TestParseLabels_Valid(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  map[string]string
	}{
		{
			name:  "single key=value",
			input: []string{"env=prod"},
			want:  map[string]string{"env": "prod"},
		},
		{
			name:  "multiple key=value pairs",
			input: []string{"env=prod", "appname=owl", "region=cn-east"},
			want:  map[string]string{"env": "prod", "appname": "owl", "region": "cn-east"},
		},
		{
			name:  "value with spaces",
			input: []string{"name=Web Server"},
			want:  map[string]string{"name": "Web Server"},
		},
		{
			name:  "key with spaces trimmed",
			input: []string{" env =prod"},
			want:  map[string]string{"env": "prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLabels(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d labels, got %d: %v", len(tt.want), len(got), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("label[%s]: expected '%s', got '%s'", k, v, got[k])
				}
			}
		})
	}
}

func TestParseLabels_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input []string
	}{
		{"no equals sign", []string{"envprod"}},
		{"empty string entry", []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseLabels(tt.input)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestParseLabels_Empty(t *testing.T) {
	labels, err := ParseLabels(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("expected empty map, got %v", labels)
	}

	labels, err = ParseLabels([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("expected empty map, got %v", labels)
	}
}

func TestParseNodeList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single node", "node1", []string{"node1"}},
		{"multiple nodes", "node1,node2,node3", []string{"node1", "node2", "node3"}},
		{"with spaces", "node1, node2, node3", []string{"node1", "node2", "node3"}},
		{"empty string", "", nil},
		{"trailing comma", "node1,", []string{"node1"}},
		{"leading comma", ",node1", []string{"node1"}},
		{"only commas", ",,,", nil},
		{"mixed whitespace", "  node1  ,  node2  ", []string{"node1", "node2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseNodeList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d items, got %d: %v", len(tt.want), len(got), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("item[%d]: expected '%s', got '%s'", i, tt.want[i], got[i])
				}
			}
		})
	}
}

func TestParseGroupList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"single group", "web", 1},
		{"multiple groups", "web,production,frontend", 3},
		{"empty string", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGroupList(tt.input)
			if len(got) != tt.want {
				t.Errorf("expected %d groups, got %d: %v", tt.want, len(got), got)
			}
		})
	}
}

func TestParseGroupList_IsAlias(t *testing.T) {
	result := ParseGroupList("web,production")
	expected := []string{"web", "production"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d groups, got %d", len(expected), len(result))
	}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("item[%d]: expected '%s', got '%s'", i, expected[i], result[i])
		}
	}
}

func TestNewOutputFormatter(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		color      bool
		wantFormat OutputFormat
	}{
		{"table format", "table", false, OutputFormatTable},
		{"json format", "json", false, OutputFormatJSON},
		{"json shorthand", "js", false, OutputFormatJSON},
		{"yaml format", "yaml", false, OutputFormatYAML},
		{"yaml shorthand", "yml", false, OutputFormatYAML},
		{"yaml y shorthand", "y", false, OutputFormatYAML},
		{"empty defaults to table", "", false, OutputFormatTable},
		{"unknown defaults to table", "csv", false, OutputFormatTable},
		{"uppercase JSON", "JSON", false, OutputFormatJSON},
		{"with color", "table", true, OutputFormatTable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewOutputFormatter(tt.format, tt.color)
			if f.Format != tt.wantFormat {
				t.Errorf("expected Format '%s', got '%s'", tt.wantFormat, f.Format)
			}
			if f.Color != tt.color {
				t.Errorf("expected Color %v, got %v", tt.color, f.Color)
			}
		})
	}
}

func TestOutputFormatter_FormatNodes_JSON(t *testing.T) {
	f := NewOutputFormatter("json", false)
	nodes := []*model.Node{
		{ID: "node-1", Name: "Node 1", Address: "192.168.1.1", Port: 22, User: "root", Status: model.NodeStatusOnline},
	}

	captured := captureStdout(func() {
		f.FormatNodes(nodes)
	})

	if captured == "" {
		t.Error("expected JSON output, got empty string")
	}
	if !containsAny(captured, `"id": "node-1"`, `"name": "Node 1"`) {
		t.Errorf("expected JSON content, got: %s", captured)
	}
}

func TestOutputFormatter_FormatNodes_YAML(t *testing.T) {
	f := NewOutputFormatter("yaml", false)
	nodes := []*model.Node{
		{ID: "node-1", Name: "Node 1", Address: "192.168.1.1", Port: 22, User: "root", Status: model.NodeStatusOnline},
	}

	captured := captureStdout(func() {
		f.FormatNodes(nodes)
	})

	if captured == "" {
		t.Error("expected YAML output, got empty string")
	}
	if !containsAny(captured, "id: node-1", "name: Node 1") {
		t.Errorf("expected YAML content, got: %s", captured)
	}
}

func TestOutputFormatter_FormatNodes_Table(t *testing.T) {
	f := NewOutputFormatter("table", false)
	nodes := []*model.Node{
		{ID: "node-1", Name: "Node 1", Address: "192.168.1.1", Port: 22, User: "root", Status: model.NodeStatusOnline, Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
	}

	captured := captureStdout(func() {
		f.FormatNodes(nodes)
	})

	if !containsAny(captured, "ID", "Name", "Address", "User", "Status") {
		t.Errorf("expected table headers, got: %s", captured)
	}
	if !containsAny(captured, "node-1") {
		t.Errorf("expected 'node-1' in table, got: %s", captured)
	}
	if !containsAny(captured, "Total: 1 nodes") {
		t.Errorf("expected 'Total: 1 nodes', got: %s", captured)
	}
}

func TestOutputFormatter_FormatNodes_TableEmpty(t *testing.T) {
	f := NewOutputFormatter("table", false)

	captured := captureStdout(func() {
		f.FormatNodes(nil)
	})

	if !containsAny(captured, "No nodes found") {
		t.Errorf("expected 'No nodes found', got: %s", captured)
	}
}

func TestOutputFormatter_FormatNode_Detail(t *testing.T) {
	f := NewOutputFormatter("table", false)
	node := &model.Node{
		ID:      "node-1",
		Name:    "Test Node",
		Address: "192.168.1.1",
		Port:    22,
		User:    "admin",
		Status:  model.NodeStatusOnline,
		Groups:  []string{"web"},
		Labels:  map[string]string{"env": "prod"},
	}

	captured := captureStdout(func() {
		f.FormatNode(node)
	})

	if !containsAny(captured, "Test Node") {
		t.Errorf("expected 'Test Node', got: %s", captured)
	}
	if !containsAny(captured, "node-1") {
		t.Errorf("expected 'node-1', got: %s", captured)
	}
	if !containsAny(captured, "admin") {
		t.Errorf("expected 'admin' user, got: %s", captured)
	}
}

func TestOutputFormatter_FormatNode_DetailNoUser(t *testing.T) {
	f := NewOutputFormatter("table", false)
	node := &model.Node{
		ID:      "node-1",
		Name:    "Test Node",
		Address: "192.168.1.1",
		Port:    22,
		Status:  model.NodeStatusOnline,
	}

	captured := captureStdout(func() {
		f.FormatNode(node)
	})

	if containsAny(captured, "User:") {
		t.Errorf("expected no User field, got: %s", captured)
	}
}

func TestFormatLabelsStr(t *testing.T) {
	t.Run("empty labels", func(t *testing.T) {
		got := formatLabelsStr(map[string]string{})
		if got != "" {
			t.Errorf("expected empty string, got '%s'", got)
		}
	})

	t.Run("nil labels", func(t *testing.T) {
		got := formatLabelsStr(nil)
		if got != "" {
			t.Errorf("expected empty string, got '%s'", got)
		}
	})

	t.Run("single label", func(t *testing.T) {
		got := formatLabelsStr(map[string]string{"env": "prod"})
		if got != "env=prod" {
			t.Errorf("expected 'env=prod', got '%s'", got)
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
	}{
		{"shorter than max", "hello", 10},
		{"equal to max", "hello", 5},
		{"longer than max", "hello world", 8},
		{"very long", "this is a very long string", 10},
		{"short max", "abcdef", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.maxLen)
			if len(got) > tt.maxLen {
				t.Errorf("truncate(%q, %d) = %q, length %d exceeds max", tt.s, tt.maxLen, got, len(got))
			}
			if len(tt.s) > tt.maxLen {
				if len(got) < tt.maxLen-3 || !containsAny(got, "...") {
					t.Errorf("truncate(%q, %d) = %q, expected truncation with '...'", tt.s, tt.maxLen, got)
				}
			}
		})
	}
}

func TestNodeSelector_Fields(t *testing.T) {
	selector := NodeSelector{
		Nodes:  []string{"node1", "node2"},
		Groups: []string{"web"},
		Labels: []string{"env=prod"},
		Status: model.NodeStatusOnline,
	}

	if len(selector.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(selector.Nodes))
	}
	if len(selector.Groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(selector.Groups))
	}
	if selector.Status != model.NodeStatusOnline {
		t.Errorf("expected 'online' status, got '%s'", selector.Status)
	}
}

func TestOutputFormat_Constants(t *testing.T) {
	if OutputFormatTable != "table" {
		t.Errorf("expected 'table', got '%s'", OutputFormatTable)
	}
	if OutputFormatJSON != "json" {
		t.Errorf("expected 'json', got '%s'", OutputFormatJSON)
	}
	if OutputFormatYAML != "yaml" {
		t.Errorf("expected 'yaml', got '%s'", OutputFormatYAML)
	}
}

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
