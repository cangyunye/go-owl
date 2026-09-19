package exec

import (
	"strings"
	"testing"

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
		start, end := tokenAt(c.value, c.pos)
		if start != c.start || end != c.end {
			t.Fatalf("%s: tokenAt(%q,%d) = (%d,%d), want (%d,%d)", c.name, c.value, c.pos, start, end, c.start, c.end)
		}
	}
}

func seedCandidatesNodes() []*common.NodeInfo {
	return []*common.NodeInfo{
		{ID: "n1", Name: "web-1", Status: "online", Groups: []string{"web", "test"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "db-1", Status: "offline", Groups: []string{"db"}, Labels: map[string]string{"env": "dev", "role": "backup"}},
		{ID: "n3", Status: "online", Groups: []string{"test"}, Labels: map[string]string{"env": "prod"}},
	}
}

func TestNodeCandidates(t *testing.T) {
	cands := nodeCandidates(seedCandidatesNodes())
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(cands))
	}
	if cands[0].value != "n1" || cands[1].value != "n2" || cands[2].value != "n3" {
		t.Fatalf("应按 ID 排序: %+v", cands)
	}
	if !strings.Contains(cands[0].desc, "web-1") || !strings.Contains(cands[0].desc, "在线") {
		t.Fatalf("n1 desc 应含名称与在线状态: %q", cands[0].desc)
	}
	if !strings.Contains(cands[1].desc, "离线") {
		t.Fatalf("n2 desc 应含离线状态: %q", cands[1].desc)
	}
}

func TestGroupCandidates(t *testing.T) {
	cands := groupCandidates(seedCandidatesNodes())
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
		if cands[i].value != w.value || !strings.Contains(cands[i].desc, w.desc) {
			t.Fatalf("cands[%d] = %+v, want %v", i, cands[i], w)
		}
	}
}

func TestLabelCandidates(t *testing.T) {
	cands := labelCandidates(seedCandidatesNodes())
	var vals []string
	for _, c := range cands {
		vals = append(vals, c.value)
	}
	want := []string{"env", "env=dev", "env=prod", "role", "role=backup"}
	if strings.Join(vals, ",") != strings.Join(want, ",") {
		t.Fatalf("候选 = %v, want %v", vals, want)
	}
	for _, c := range cands {
		if !strings.Contains(c.desc, "台") {
			t.Fatalf("desc 应含匹配节点数: %+v", c)
		}
	}
}

func TestCompletionMenu(t *testing.T) {
	all := []candidate{
		{value: "env", desc: "3 台"},
		{value: "env=dev", desc: "1 台"},
		{value: "env=prod", desc: "2 台"},
		{value: "role", desc: "1 台"},
	}
	m := newCompletionMenu(all)
	m.sync(all, "")
	if m.Len() != 4 || m.Visible()[0].value != "env" {
		t.Fatalf("空 query 应显示全量: %d", m.Len())
	}
	// 前缀过滤 + 大小写不敏感
	m.sync(all, "ENV=")
	if m.Len() != 2 || m.Visible()[0].value != "env=dev" || m.Visible()[1].value != "env=prod" {
		t.Fatalf("过滤失败: %+v", m.Visible())
	}
	// 环绕
	m.MoveDown()
	if m.Active() != 1 {
		t.Fatalf("down 应到 1, got %d", m.Active())
	}
	m.MoveDown()
	if m.Active() != 0 {
		t.Fatalf("应环绕回 0, got %d", m.Active())
	}
	// 相同 query 不重置选中
	m.sync(all, "ENV=")
	if m.Active() != 0 {
		t.Fatalf("query 未变不应重置选中, got %d", m.Active())
	}
	// query 变化重置选中
	m.sync(all, "role")
	if m.Active() != 0 || m.Len() != 1 {
		t.Fatalf("query 变化应重置并过滤: len=%d active=%d", m.Len(), m.Active())
	}
	cand, ok := m.Selected()
	if !ok || cand.value != "role" {
		t.Fatalf("Selected = %+v ok=%v", cand, ok)
	}
	// 无匹配
	m.sync(all, "zz")
	if m.Len() != 0 {
		t.Fatalf("无匹配应为空: %d", m.Len())
	}
	if _, ok := m.Selected(); ok {
		t.Fatal("无匹配时 Selected 应失败")
	}
}
