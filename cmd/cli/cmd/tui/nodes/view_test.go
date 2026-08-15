package nodes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
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

func TestRenderCellSelected(t *testing.T) {
	withColor(t, func() {
		n := &common.NodeInfo{ID: "n1", Status: "online", Labels: map[string]string{"a": "1"}}
		sel := renderCell(n, "status", 10, true)
		if sel == "online" {
			t.Fatal("选中行应整体高亮")
		}
		unsel := renderCell(n, "status", 10, false)
		if unsel == "online" {
			t.Fatal("非选中 Status 应着色")
		}
		if got := renderCell(n, "id", 10, false); got == "" {
			t.Fatal("ID 列应正常渲染")
		}
	})
}

func TestRainbowLabelsWidth(t *testing.T) {
	if got := rainbowLabelsWidth("", 10); got != "" {
		t.Fatalf("空串原样, got %q", got)
	}
	// 宽度极小 → 省略号
	got := rainbowLabelsWidth("a=1", 2)
	if got == "" {
		t.Fatal("不应为空")
	}
	// 宽列全量彩虹
	withColor(t, func() {
		wide := rainbowLabelsWidth("a=1,b=2", 30)
		if wide == "a=1,b=2" {
			t.Fatal("应带 ANSI 着色")
		}
	})
	// 窄列截断不切 ANSI
	short := rainbowLabelsWidth("a=1,b=2", 6)
	if short == "" {
		t.Fatal("窄列应非空")
	}
}

func TestRenderCellLabels(t *testing.T) {
	n := &common.NodeInfo{Labels: map[string]string{"a": "1"}}
	cell := renderCell(n, "labels", 20, false)
	if cell == "" {
		t.Fatal("labels 列应渲染")
	}
}

func TestRainbowLabelsWidthTrailingSpace(t *testing.T) {
	// truncateCell 会补尾部空格,rainbowLabelsWidth 需先 trim 再解析
	raw := "a=1  "
	got := rainbowLabelsWidth(raw, 8)
	if got == "" {
		t.Fatal("带尾部空格也应正常渲染")
	}
}

func TestRainbowLabelsWidthNoOverflow(t *testing.T) {
	// 段宽刚好耗尽 remaining 时,省略号不应使可见宽度超过列宽
	got := rainbowLabelsWidth("a=1,b=2", 3)
	if common.DisplayWidth(got) > 3 {
		t.Fatalf("可见宽度 %d 超过列宽 3, got %q", common.DisplayWidth(got), got)
	}
}
