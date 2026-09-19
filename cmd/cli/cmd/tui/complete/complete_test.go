package complete

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestTokenAt(t *testing.T) {
	cases := []struct {
		name       string
		value      string
		pos        int // rune index
		start, end int
	}{
		{"空值", "", 0, 0, 0},
		{"单段_光标在尾", "n1", 2, 0, 2},
		{"单段_光标在中", "n1", 1, 0, 2},
		{"两段_光标在第二段", "n1,n2", 4, 3, 5},
		{"两段_光标在首段尾", "n1,n2", 2, 0, 2},
		{"两段_光标紧跟逗号", "n1,", 3, 3, 3},
		{"三段_光标在中段", "n1,n2,n3", 4, 3, 5},
		{"三段_光标在末段", "n1,n2,n3", 7, 6, 8},
		{"含中文按rune切", "n1,节点", 4, 3, 5},
	}
	for _, c := range cases {
		start, end := TokenAt(c.value, c.pos)
		if start != c.start || end != c.end {
			t.Fatalf("%s: TokenAt(%q,%d) = (%d,%d), want (%d,%d)", c.name, c.value, c.pos, start, end, c.start, c.end)
		}
	}
}

func seedNodes() []*common.NodeInfo {
	return []*common.NodeInfo{
		{ID: "n1", Name: "web-1", Status: "online", Groups: []string{"web", "test"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "db-1", Status: "offline", Groups: []string{"db"}, Labels: map[string]string{"env": "dev", "role": "backup"}},
		{ID: "n3", Status: "online", Groups: []string{"test"}, Labels: map[string]string{"env": "prod"}},
	}
}

func TestNodeCandidates(t *testing.T) {
	cands := NodeCandidates(seedNodes())
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(cands))
	}
	if cands[0].Value != "n1" || cands[1].Value != "n2" || cands[2].Value != "n3" {
		t.Fatalf("应按 ID 排序: %+v", cands)
	}
	if !strings.Contains(cands[0].Desc, "web-1") || !strings.Contains(cands[0].Desc, "在线") {
		t.Fatalf("n1 Desc 应含名称与在线状态: %q", cands[0].Desc)
	}
	if !strings.Contains(cands[1].Desc, "离线") {
		t.Fatalf("n2 Desc 应含离线状态: %q", cands[1].Desc)
	}
}

func TestGroupCandidates(t *testing.T) {
	cands := GroupCandidates(seedNodes())
	want := []struct {
		value string
		desc  string
	}{
		{"db", "1 台"},
		{"test", "2 台"},
		{"web", "1 台"},
	}
	if len(cands) != len(want) {
		t.Fatalf("expected %d candidates, got %d: %+v", len(want), len(cands), cands)
	}
	for i, w := range want {
		if cands[i].Value != w.value || !strings.Contains(cands[i].Desc, w.desc) {
			t.Fatalf("cands[%d] = %+v, want %v", i, cands[i], w)
		}
	}
}

func TestLabelCandidates(t *testing.T) {
	cands := LabelCandidates(seedNodes())
	var vals []string
	for _, c := range cands {
		vals = append(vals, c.Value)
	}
	want := []string{"env", "env=dev", "env=prod", "role", "role=backup"}
	if strings.Join(vals, ",") != strings.Join(want, ",") {
		t.Fatalf("候选 = %v, want %v", vals, want)
	}
	for _, c := range cands {
		if !strings.Contains(c.Desc, "台") {
			t.Fatalf("Desc 应含匹配节点数: %+v", c)
		}
	}
}

func TestMenu(t *testing.T) {
	all := []Candidate{
		{Value: "env", Desc: "3 台"},
		{Value: "env=dev", Desc: "1 台"},
		{Value: "env=prod", Desc: "2 台"},
		{Value: "role", Desc: "1 台"},
	}
	m := &Menu{}
	m.Sync(all, "")
	if m.Len() != 4 || m.Visible()[0].Value != "env" {
		t.Fatalf("空 query 应显示全量: %d", m.Len())
	}
	m.Sync(all, "ENV=")
	if m.Len() != 2 || m.Visible()[0].Value != "env=dev" || m.Visible()[1].Value != "env=prod" {
		t.Fatalf("过滤失败: %+v", m.Visible())
	}
	m.MoveDown()
	if m.Active() != 1 {
		t.Fatalf("down 应到 1, got %d", m.Active())
	}
	m.MoveDown()
	if m.Active() != 0 {
		t.Fatalf("应环绕回 0, got %d", m.Active())
	}
	m.Sync(all, "ENV=")
	if m.Active() != 0 {
		t.Fatalf("query 未变不应重置选中, got %d", m.Active())
	}
	m.Sync(all, "role")
	if m.Active() != 0 || m.Len() != 1 {
		t.Fatalf("query 变化应重置并过滤: len=%d active=%d", m.Len(), m.Active())
	}
	cand, ok := m.Selected()
	if !ok || cand.Value != "role" {
		t.Fatalf("Selected = %+v ok=%v", cand, ok)
	}
	m.Sync(all, "zz")
	if m.Len() != 0 {
		t.Fatalf("无匹配应为空: %d", m.Len())
	}
	if _, ok := m.Selected(); ok {
		t.Fatal("无匹配时 Selected 应失败")
	}
}

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestFieldQueryAndApply(t *testing.T) {
	f := textinput.New()
	f.SetValue("n1,web,n3")
	f.SetCursor(4) // "web" 段内
	if got := FieldQuery(&f); got != "web" {
		t.Fatalf("FieldQuery = %q, want web", got)
	}
	FieldApply(&f, "cache")
	if got := f.Value(); got != "n1,cache,n3" {
		t.Fatalf("FieldApply = %q, want n1,cache,n3", got)
	}
	if pos := f.Position(); pos != 8 { // 3 + len("cache")
		t.Fatalf("光标应落在回填尾部, got %d", pos)
	}
}

func TestCompletionLifecycle(t *testing.T) {
	var c Completion
	cands := func() []Candidate {
		return []Candidate{{Value: "n1", Desc: "web-1"}, {Value: "n2", Desc: "db-1"}}
	}
	f := textinput.New()

	// 空 token 不弹
	f.SetValue("")
	c.Sync(&f, cands)
	if c.MenuOpen() {
		t.Fatal("空 token 不应弹菜单")
	}
	// 打字后弹出
	f.SetValue("n")
	c.Sync(&f, cands)
	if !c.MenuOpen() || c.Visible()[0].Value != "n1" {
		t.Fatalf("非空 token 应弹出: %+v", c.Visible())
	}
	// esc 进入抑制态: Sync 不再弹出(值未变)
	c.Dismiss()
	c.Sync(&f, cands)
	if c.MenuOpen() {
		t.Fatal("抑制态下 Sync 不应弹出")
	}
	// 值变化解除抑制
	f.SetValue("n2")
	c.AfterEdit(&f, "n", cands)
	if !c.MenuOpen() || c.Visible()[0].Value != "n2" {
		t.Fatalf("值变化应恢复弹出: %+v", c.Visible())
	}
	// 无候选来源不弹
	c.Sync(&f, nil)
	if c.MenuOpen() {
		t.Fatal("无候选来源不应弹")
	}
	// 菜单不可见时 HandleKey 不消费(↑↓/esc 交还面板)
	c.Dismiss()
	if c.HandleKey(&f, tea.KeyMsg{Type: tea.KeyDown}) {
		t.Fatal("菜单关闭时 HandleKey 不应消费")
	}
	if c.HandleKey(&f, tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Fatal("菜单关闭时 esc 应交还面板(退出编辑)")
	}
	// 菜单打开时消费导航/确认,未匹配键透传
	c.Reset() // 切字段/进编辑会解除抑制
	f.SetValue("n")
	c.Sync(&f, cands) // 2 条候选
	if c.HandleKey(&f, runeKey('x')) {
		t.Fatal("未匹配键应透传给面板")
	}
	if !c.HandleKey(&f, tea.KeyMsg{Type: tea.KeyDown}) {
		t.Fatal("菜单打开时 down 应被消费")
	}
	if c.Active() != 1 {
		t.Fatalf("down 应移动选中: %d", c.Active())
	}
	if !c.HandleKey(&f, tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Fatal("菜单打开时 enter 应被消费(回填)")
	}
	if got := f.Value(); got != "n2" {
		t.Fatalf("enter 应回填选中候选: %q", got)
	}
}
