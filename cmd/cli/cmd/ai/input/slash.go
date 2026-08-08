package input

import (
	"strings"
	"unicode/utf8"
)

// SlashCommand 一条斜杠命令。对齐网页端 AI 助手的命令目录:
//   - category "task"   选中后展开为提示词模板(Template),用户补全 {arg} 占位符后发送
//   - category "action" 选中后直接执行 Action 回调(如 /help /new /clear /quit)
type SlashCommand struct {
	Name     string
	Category string
	Icon     string
	Label    string
	Desc     string
	Template string
	Args     []string
	Action   func()
}

// SlashMenu 斜杠命令补全菜单的纯逻辑状态机(无 IO,可独立单测)。
// 交互语义对齐网页端 SlashMenu: 输入以 "/" 开头时弹出,继续输入按前缀过滤,
// ↑↓ 选择,Enter 确认,菜单打开时 ↑↓ 不再用于会话历史。
type SlashMenu struct {
	commands []SlashCommand
	query    string
	active   int
}

func NewSlashMenu(commands []SlashCommand) *SlashMenu {
	return &SlashMenu{commands: commands}
}

// SetQuery 更新过滤词(不含 "/"),并重置选中项到第一个。
func (m *SlashMenu) SetQuery(q string) {
	m.query = q
	m.active = 0
}

// Visible 返回当前过滤后的候选命令列表。
func (m *SlashMenu) Visible() []SlashCommand {
	q := strings.ToLower(strings.TrimSpace(m.query))
	if q == "" {
		return m.commands
	}
	out := make([]SlashCommand, 0, len(m.commands))
	for _, c := range m.commands {
		if strings.HasPrefix(strings.ToLower(c.Name), q) {
			out = append(out, c)
		}
	}
	return out
}

func (m *SlashMenu) Len() int { return len(m.Visible()) }

func (m *SlashMenu) Active() int { return m.active }

func (m *SlashMenu) MoveDown() {
	n := m.Len()
	if n == 0 {
		return
	}
	m.active = (m.active + 1) % n
}

func (m *SlashMenu) MoveUp() {
	n := m.Len()
	if n == 0 {
		return
	}
	m.active = (m.active - 1 + n) % n
}

// Selected 返回当前选中的命令。
func (m *SlashMenu) Selected() (SlashCommand, bool) {
	v := m.Visible()
	if m.active >= len(v) {
		return SlashCommand{}, false
	}
	return v[m.active], true
}

// ExpandTemplate 返回命令展开后的提示词模板(原样,占位符保留给用户替换)。
func ExpandTemplate(cmd SlashCommand) string {
	return cmd.Template
}

// PlaceholderRange 返回模板中第一个 "{arg}" 占位符的字节区间 [start, end),
// 用于把光标定位到占位符上便于直接输入替换。
func PlaceholderRange(template string) (start, end int, ok bool) {
	i := strings.Index(template, "{")
	if i < 0 {
		return 0, 0, false
	}
	j := strings.Index(template[i:], "}")
	if j < 0 {
		return 0, 0, false
	}
	end = i + j + 1
	// 确保区间边界不落在多字节字符中间
	if !utf8.ValidString(template[i:end]) {
		return 0, 0, false
	}
	return i, end, true
}
