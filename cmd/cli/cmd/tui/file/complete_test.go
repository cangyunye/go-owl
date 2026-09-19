package file

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func feedKeys(m FileModel, msgs ...tea.Msg) FileModel {
	for _, msg := range msgs {
		nm, _ := m.Update(msg)
		m = nm.(FileModel)
	}
	return m
}

func typeRunes(m FileModel, text string) FileModel {
	for _, r := range text {
		nm, _ := m.Update(runeKey(r))
		m = nm.(FileModel)
	}
	return m
}

func TestFileCompletionNodesField(t *testing.T) {
	m := newTestModel(t)
	m = feedKeys(m, key(tea.KeyDown), key(tea.KeyEnter)) // cursor=节点, 进 Insert
	if strings.Contains(m.View(), "❯") {
		t.Fatal("空 token 不应弹菜单(打了字才弹)")
	}
	m = typeRunes(m, "n2")
	if v := m.View(); !strings.Contains(v, "❯ n2") || strings.Contains(v, "❯ n1") {
		t.Fatalf("应按 token 前缀过滤: %s", v)
	}
	m = feedKeys(m, key(tea.KeyEnter))
	if got := m.nodesInput.Value(); got != "n2" {
		t.Fatalf("确认应回填候选, got %q", got)
	}
}

func TestFileCompletionGroupsLabelsChain(t *testing.T) {
	m := newTestModel(t)
	m = feedKeys(m, key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyEnter)) // cursor=分组
	m = typeRunes(m, "w")
	m = feedKeys(m, key(tea.KeyEnter)) // web
	if got := m.groupsInput.Value(); got != "web" {
		t.Fatalf("分组回填: got %q", got)
	}
	// 标签: Esc 关菜单 → Esc 退编辑 → ↓ 到标签 → Enter 编辑
	m = feedKeys(m, key(tea.KeyEsc), key(tea.KeyEsc), key(tea.KeyDown), key(tea.KeyEnter))
	m = typeRunes(m, "env=")
	if v := m.View(); !strings.Contains(v, "env=dev") || !strings.Contains(v, "env=prod") {
		t.Fatalf("k= 应补全候选值: %s", v)
	}
	m = feedKeys(m, key(tea.KeyEnter)) // 选中 env=dev(首位)
	if got := m.labelsInput.Value(); got != "env=dev" {
		t.Fatalf("标签回填: got %q", got)
	}
}

func TestFileCompletionArrowSwitchesFieldWhenDismissed(t *testing.T) {
	m := newTestModel(t)
	m = feedKeys(m, key(tea.KeyDown), key(tea.KeyEnter)) // 节点字段 Insert
	m = typeRunes(m, "n")
	m = feedKeys(m, key(tea.KeyEsc)) // 关菜单(抑制态)
	m = feedKeys(m, key(tea.KeyDown))
	if m.cursor != 2 {
		t.Fatalf("菜单关闭时 ↓ 应切到分组字段, cursor=%d", m.cursor)
	}
	if !m.InsertMode() {
		t.Fatal("切字段应保持编辑态")
	}
	m = typeRunes(m, "c")
	if v := m.View(); !strings.Contains(v, "❯ cache") {
		t.Fatalf("分组字段打字后应弹候选: %s", v)
	}
	m = feedKeys(m, key(tea.KeyEnter))
	if got := m.GroupsValue(); got != "cache" {
		t.Fatalf("跨字段补全回填: got %q", got)
	}
}

func TestFileCompletionPathFieldsNoMenu(t *testing.T) {
	m := newTestModel(t)
	// cursor=0 是本地文件字段(upload),Enter 打开浏览器而非编辑;用 download 才直接进编辑
	m.op = OpDownload
	m = feedKeys(m, key(tea.KeyEnter))
	if strings.Contains(m.View(), "❯") {
		t.Fatal("文件路径字段不应有补全菜单")
	}
	// 目标目录字段(最后一个)也不补全
	m = feedKeys(m, key(tea.KeyEsc), key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyEnter))
	if strings.Contains(m.View(), "❯") {
		t.Fatal("目录字段不应有补全菜单")
	}
}
