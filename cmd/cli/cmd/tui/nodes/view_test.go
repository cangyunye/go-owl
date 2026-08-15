package nodes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// withColor forces a TrueColor profile so coloring tests see ANSI codes;
// default renderer under `go test` (NoTTY) strips color entirely.
func withColor(t *testing.T, fn func()) {
	t.Helper()
	prevProfile := lipgloss.DefaultRenderer().ColorProfile()
	prevDark := lipgloss.DefaultRenderer().HasDarkBackground()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	defer func() {
		lipgloss.SetColorProfile(prevProfile)
		lipgloss.SetHasDarkBackground(prevDark)
	}()
	fn()
}

func TestView_ListRendersNodesAndDetail(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	v := m.View()
	if !strings.Contains(v, "n1") || !strings.Contains(v, "web-1") {
		t.Fatalf("list missing node: %q", v)
	}
	if !strings.Contains(v, "db-1") {
		t.Fatalf("detail missing selected node name: %q", v)
	}
	if !strings.Contains(v, "env=prod") {
		t.Fatalf("detail missing labels: %q", v)
	}
	if !strings.Contains(v, "Groups") {
		t.Fatalf("detail missing Groups label: %q", v)
	}
}

func TestView_EmptyList(t *testing.T) {
	m := NewModel(newTestStore(t))
	if m.View() == "" {
		t.Fatal("expected non-empty empty-state view")
	}
}

func TestView_FormRendersFields(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	v := m.View()
	for _, label := range []string{"ID", "Name", "Address", "Port", "Groups", "Labels"} {
		if !strings.Contains(v, label) {
			t.Fatalf("form missing %s: %q", label, v)
		}
	}
}

func TestView_ConfirmRendersNode(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	v := m.View()
	if !strings.Contains(v, "n1") {
		t.Fatalf("confirm missing node: %q", v)
	}
}

func TestView_ColumnsRendersFields(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('c'))
	m = nm.(Model)
	v := m.View()
	for _, label := range []string{"id", "name", "status", "labels"} {
		if !strings.Contains(v, label) {
			t.Fatalf("columns missing %s: %q", label, v)
		}
	}
}

func TestListView_ShowsMarkBoxes(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey(' '))
	m = nm.(Model)
	got := m.View()
	if !strings.Contains(got, "[x]") || !strings.Contains(got, "[ ]") {
		t.Fatalf("expected mark boxes in list view:\n%s", got)
	}
}

func TestStatusBar_ShowsMarkCount(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	m.marked["n1"] = true
	m.marked["n2"] = true
	got := m.View()
	if !strings.Contains(got, "已选 2") {
		t.Fatalf("expected mark count in status bar:\n%s", got)
	}
}

func TestStyleForStatus(t *testing.T) {
	cases := []struct {
		status string
	}{
		{"online"}, {"offline"}, {"unknown"}, {"bogus"},
	}
	withColor(t, func() {
		for _, c := range cases {
			s := styleForStatus(c.status)
			if s.Render("x") == "x" {
				t.Fatalf("styleForStatus(%q) 应有着色", c.status)
			}
		}
	})
}

func TestColoredValue(t *testing.T) {
	withColor(t, func() {
		if got := coloredValue("Status", "online"); got == "online" {
			t.Fatal("Status 应着色")
		}
		if got := coloredValue("Labels", "a=1,b=2"); got == "a=1,b=2" {
			t.Fatal("Labels 应彩虹着色")
		}
		if got := coloredValue("Address", "1.2.3.4"); got == "1.2.3.4" {
			t.Fatal("Address 应着色")
		}
		if got := coloredValue("User", "root"); got == "root" {
			t.Fatal("User 应着色")
		}
		if got := coloredValue("Groups", "web,db"); got == "web,db" {
			t.Fatal("Groups 应着色")
		}
		if got := coloredValue("ID", "n1"); got != "n1" {
			t.Fatalf("ID 不着色, got %q", got)
		}
	})
}

func TestRainbowLabelsFull(t *testing.T) {
	withColor(t, func() {
		if got := rainbowLabelsFull(""); got != "" {
			t.Fatalf("空串原样返回, got %q", got)
		}
		got := rainbowLabelsFull("a=1,b=2")
		if got == "a=1,b=2" {
			t.Fatal("多 label 应逐 label 彩虹")
		}
		out := rainbowLabelsFull("notanassignment")
		if out == "" {
			t.Fatal("异常格式不应为空")
		}
	})
}
