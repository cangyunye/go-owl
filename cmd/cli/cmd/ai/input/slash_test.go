package input

import "testing"

func testCommands() []SlashCommand {
	return []SlashCommand{
		{Name: "exec", Category: "task", Icon: "▶️", Label: "执行命令", Template: "在 {nodes} 上执行命令 {command}", Args: []string{"nodes", "command"}},
		{Name: "check", Category: "task", Icon: "🩺", Label: "节点连通性检查", Template: "检查 {nodes} 的 SSH 连通性，找出不可达节点", Args: []string{"nodes"}},
		{Name: "diagnose", Category: "task", Icon: "🔍", Label: "故障诊断", Template: "对 {target} 进行全栈故障诊断并给出修复建议", Args: []string{"target"}},
		{Name: "query", Category: "task", Icon: "📊", Label: "节点查询", Template: "查询 {condition} 的节点信息", Args: []string{"condition"}},
		{Name: "playbook", Category: "task", Icon: "🛠️", Label: "生成剧本", Template: "生成一个 playbook 实现 {requirement}", Args: []string{"requirement"}},
		{Name: "transfer", Category: "task", Icon: "📤", Label: "传输文件", Template: "把 {source_file} 传输到 {nodes} 的 {dest_dir}", Args: []string{"source_file", "nodes", "dest_dir"}},
		{Name: "script", Category: "task", Icon: "🧩", Label: "执行脚本", Template: "在 {nodes} 上运行脚本 {script}", Args: []string{"nodes", "script"}},
		{Name: "help", Category: "action", Icon: "ℹ️", Label: "命令帮助"},
		{Name: "new", Category: "action", Icon: "➕", Label: "新会话"},
		{Name: "clear", Category: "action", Icon: "🗑️", Label: "清空会话"},
		{Name: "quit", Category: "action", Icon: "👋", Label: "退出"},
	}
}

func TestSlashMenu_EmptyQueryShowsAll(t *testing.T) {
	m := NewSlashMenu(testCommands())
	if got := m.Visible(); len(got) != len(testCommands()) {
		t.Fatalf("expected all %d commands, got %d", len(testCommands()), len(got))
	}
}

func TestSlashMenu_PrefixFilter(t *testing.T) {
	m := NewSlashMenu(testCommands())
	m.SetQuery("ex")
	got := m.Visible()
	if len(got) != 1 || got[0].Name != "exec" {
		t.Fatalf("expected only 'exec', got %v", names(got))
	}
}

func TestSlashMenu_PrefixFilterCaseInsensitive(t *testing.T) {
	m := NewSlashMenu(testCommands())
	m.SetQuery("EX")
	got := m.Visible()
	if len(got) != 1 || got[0].Name != "exec" {
		t.Fatalf("expected case-insensitive match 'exec', got %v", names(got))
	}
}

func TestSlashMenu_NoMatch(t *testing.T) {
	m := NewSlashMenu(testCommands())
	m.SetQuery("xyz")
	if got := m.Visible(); len(got) != 0 {
		t.Fatalf("expected no match, got %v", names(got))
	}
	if _, ok := m.Selected(); ok {
		t.Fatal("expected Selected to be false when no match")
	}
}

func TestSlashMenu_SetQueryResetsActive(t *testing.T) {
	m := NewSlashMenu(testCommands())
	m.MoveDown()
	m.MoveDown()
	if m.Active() != 2 {
		t.Fatalf("expected active 2, got %d", m.Active())
	}
	m.SetQuery("p")
	if m.Active() != 0 {
		t.Fatalf("expected SetQuery to reset active to 0, got %d", m.Active())
	}
	if got := m.Visible(); len(got) != 1 || got[0].Name != "playbook" {
		t.Fatalf("expected 'playbook' filtered, got %v", names(got))
	}
}

func TestSlashMenu_MoveDownUpCycles(t *testing.T) {
	m := NewSlashMenu(testCommands())
	total := len(m.Visible())

	m.MoveDown()
	if m.Active() != 1 {
		t.Fatalf("expected active 1, got %d", m.Active())
	}
	// 循环: 从底部向下回到顶部
	for i := 0; i < total-1; i++ {
		m.MoveDown()
	}
	if m.Active() != 0 {
		t.Fatalf("expected cycle to active 0, got %d", m.Active())
	}
	// 循环: 从顶部向上到底部
	m.MoveUp()
	if m.Active() != total-1 {
		t.Fatalf("expected cycle to active %d, got %d", total-1, m.Active())
	}
}

func TestSlashMenu_SelectedReturnsVisibleItem(t *testing.T) {
	m := NewSlashMenu(testCommands())
	m.SetQuery("diag")
	cmd, ok := m.Selected()
	if !ok || cmd.Name != "diagnose" {
		t.Fatalf("expected 'diagnose', got %+v ok=%v", cmd, ok)
	}
}

func TestExpandTemplate_KeepsPlaceholders(t *testing.T) {
	m := NewSlashMenu(testCommands())
	cmd, _ := m.Selected() // 空 query 时 active=0 → exec
	if got := ExpandTemplate(cmd); got != "在 {nodes} 上执行命令 {command}" {
		t.Fatalf("unexpected template: %q", got)
	}
}

func TestPlaceholderRange_FirstPlaceholder(t *testing.T) {
	m := NewSlashMenu(testCommands())
	cmd, _ := m.Selected()
	tpl := ExpandTemplate(cmd)
	start, end, ok := PlaceholderRange(tpl)
	if !ok {
		t.Fatal("expected a placeholder to be found")
	}
	// 模板 "在 {nodes} 上执行命令 {command}",第一个占位符是 {nodes}
	if got := tpl[start:end]; got != "{nodes}" {
		t.Fatalf("expected first placeholder '{nodes}', got %q", got)
	}
}

func TestPlaceholderRange_NoPlaceholder(t *testing.T) {
	if _, _, ok := PlaceholderRange("没有任何占位符"); ok {
		t.Fatal("expected no placeholder")
	}
}

func names(cmds []SlashCommand) []string {
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, c.Name)
	}
	return out
}
