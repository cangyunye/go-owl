// Package complete 提供 TUI 表单字段的逗号分段输入补全:
// 节点 ID / 分组 / 标签(key 与 k=v)三类候选,以及可嵌入面板的菜单状态机。
// 纯逻辑无 IO,候选来自节点库全量(与各面板 resolveTargets 的匹配范围一致)。
package complete

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/theme"
)

// MaxRows 菜单最多渲染行数。
const MaxRows = 4

// Candidate 一条补全候选: Value=回填文本, Desc=菜单辅助说明。
type Candidate struct {
	Value string
	Desc  string
}

// TokenAt 定位光标所在逗号分段的 rune 区间 [start, end)。
// 光标落在分隔符上时视为右侧新段(空段)。
func TokenAt(value string, pos int) (start, end int) {
	r := []rune(value)
	if pos < 0 {
		pos = 0
	}
	if pos > len(r) {
		pos = len(r)
	}
	start = 0
	for i := pos - 1; i >= 0; i-- {
		if r[i] == ',' {
			start = i + 1
			break
		}
	}
	end = len(r)
	for i := pos; i < len(r); i++ {
		if r[i] == ',' {
			end = i
			break
		}
	}
	return start, end
}

// Menu 补全候选菜单的纯逻辑状态机。
// Sync 传入全量候选与当前 token 前缀;query 未变时保留选中项,
// 变化时重置到第一条——与 CLI Editor 的 lastQuery 防抖同语义。
type Menu struct {
	all    []Candidate
	items  []Candidate
	query  string
	active int
}

// Sync 更新全量候选与过滤词;query 未变时不重置 active。
func (m *Menu) Sync(all []Candidate, query string) {
	m.all = all
	if query != m.query {
		m.query = query
		m.active = 0
	}
	m.rebuild()
}

func (m *Menu) rebuild() {
	q := strings.ToLower(m.query)
	m.items = m.items[:0]
	for _, c := range m.all {
		if q == "" || strings.HasPrefix(strings.ToLower(c.Value), q) {
			m.items = append(m.items, c)
		}
	}
	if m.active >= len(m.items) {
		m.active = 0
	}
}

func (m *Menu) Len() int { return len(m.items) }

func (m *Menu) Active() int { return m.active }

func (m *Menu) Visible() []Candidate { return m.items }

// MoveDown 选中项下移,环绕。
func (m *Menu) MoveDown() {
	if n := m.Len(); n > 0 {
		m.active = (m.active + 1) % n
	}
}

// MoveUp 选中项上移,环绕。
func (m *Menu) MoveUp() {
	if n := m.Len(); n > 0 {
		m.active = (m.active - 1 + n) % n
	}
}

// Selected 当前选中候选;无匹配时返回 false。
func (m *Menu) Selected() (Candidate, bool) {
	if m.active < 0 || m.active >= len(m.items) {
		return Candidate{}, false
	}
	return m.items[m.active], true
}

// NodeCandidates 节点候选: 插入 ID(执行解析只认 ID),Desc 带名称与在线状态。
func NodeCandidates(nodes []*common.NodeInfo) []Candidate {
	out := make([]Candidate, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		status := "○离线"
		if n.Status == "online" {
			status = "●在线"
		}
		desc := status
		if n.Name != "" {
			desc = n.Name + " · " + status
		}
		out = append(out, Candidate{Value: n.ID, Desc: desc})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// GroupCandidates 分组候选: 组名并集,Desc 为该组节点数。
func GroupCandidates(nodes []*common.NodeInfo) []Candidate {
	count := map[string]int{}
	for _, n := range nodes {
		if n == nil {
			continue
		}
		for _, g := range n.Groups {
			count[g]++
		}
	}
	out := make([]Candidate, 0, len(count))
	for g, c := range count {
		out = append(out, Candidate{Value: g, Desc: fmt.Sprintf("%d 台", c)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// LabelCandidates 标签候选: 裸 key(存在性语义)与全部 k=v 对并集,Desc 为匹配节点数。
func LabelCandidates(nodes []*common.NodeInfo) []Candidate {
	count := map[string]int{}
	for _, n := range nodes {
		if n == nil {
			continue
		}
		for k, v := range n.Labels {
			count[k]++
			count[k+"="+v]++
		}
	}
	out := make([]Candidate, 0, len(count))
	for cond, c := range count {
		out = append(out, Candidate{Value: cond, Desc: fmt.Sprintf("%d 台", c)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// FieldQuery 取字段光标所在逗号分段(去首尾空白)。
func FieldQuery(f *textinput.Model) string {
	start, end := TokenAt(f.Value(), f.Position())
	return strings.TrimSpace(string([]rune(f.Value())[start:end]))
}

// FieldApply 用 value 替换字段光标所在 token,光标落在回填文本尾部。
func FieldApply(f *textinput.Model, value string) {
	runes := []rune(f.Value())
	start, end := TokenAt(f.Value(), f.Position())
	f.SetValue(string(runes[:start]) + value + string(runes[end:]))
	f.SetCursor(start + len([]rune(value)))
}

// Completion 嵌入面板的补全状态: 菜单 + Esc 抑制态。
// 交互契约(与 AI 面板斜杠菜单同款):
//   - 编辑态且 token 非空时菜单弹出;空 token(刚进编辑/逗号后)不弹;
//   - Esc 仅关菜单并进入抑制态,字段值再次变化才恢复;
//   - 菜单打开时 ↑↓ 移动选中项、Enter/Tab 回填、Esc 关菜单;
//     菜单关闭时 ↑↓ 交还面板用于切换字段。
type Completion struct {
	menu *Menu
	off  bool
}

// HandleKey 编辑态按键钩子: 菜单可见时消费 up/down/tab/enter/esc。
// esc 仅关菜单;tab/enter 回填选中候选。返回 true 表示按键已消费。
func (c *Completion) HandleKey(f *textinput.Model, km tea.KeyMsg) bool {
	if !c.MenuOpen() {
		return false
	}
	switch km.String() {
	case "up":
		c.menu.MoveUp()
		return true
	case "down":
		c.menu.MoveDown()
		return true
	case "tab", "enter":
		c.confirm(f)
		return true
	case "esc":
		c.Dismiss()
		return true
	}
	return false
}

// AfterEdit 字段 textinput.Update 之后调用: 值变化解除抑制,并按候选源刷新菜单。
// cands 为 nil 表示当前字段不参与补全。
func (c *Completion) AfterEdit(f *textinput.Model, before string, cands func() []Candidate) {
	if f.Value() != before {
		c.off = false
	}
	c.Sync(f, cands)
}

// Sync 按当前字段刷新菜单;抑制态/空 token/无候选来源时不弹。
func (c *Completion) Sync(f *textinput.Model, cands func() []Candidate) {
	if c.off {
		c.menu = nil
		return
	}
	query := FieldQuery(f)
	if query == "" || cands == nil {
		c.menu = nil
		return
	}
	all := cands()
	if len(all) == 0 {
		c.menu = nil
		return
	}
	if c.menu == nil {
		c.menu = &Menu{}
	}
	c.menu.Sync(all, query)
}

// Dismiss 关菜单并进入抑制态(Esc)。
func (c *Completion) Dismiss() {
	c.menu = nil
	c.off = true
}

// Reset 解除抑制(进编辑/切字段时)。
func (c *Completion) Reset() { c.off = false }

// Close 完全关闭(退出编辑态时)。
func (c *Completion) Close() {
	c.menu = nil
	c.off = false
}

// MenuOpen 菜单是否可见(有候选)。
func (c *Completion) MenuOpen() bool { return c.menu != nil && c.menu.Len() > 0 }

// Visible 当前过滤后的候选(渲染用);菜单不可见时返回 nil。
func (c *Completion) Visible() []Candidate {
	if c.menu == nil {
		return nil
	}
	return c.menu.Visible()
}

// Active 当前选中下标(渲染用)。
func (c *Completion) Active() int {
	if c.menu == nil {
		return 0
	}
	return c.menu.Active()
}

// confirm 回填选中候选到字段。
func (c *Completion) confirm(f *textinput.Model) {
	if c.menu == nil {
		return
	}
	cand, ok := c.menu.Selected()
	if !ok {
		return
	}
	FieldApply(f, cand.Value)
}

// View 渲染菜单窗口: ≤MaxRows 行,选中行 ❯ 高亮;菜单不可见时返回空串。
func (c *Completion) View() string {
	if !c.MenuOpen() {
		return ""
	}
	v := c.Visible()
	start := 0
	if c.Active() >= MaxRows {
		start = c.Active() - MaxRows + 1
	}
	end := start + MaxRows
	if end > len(v) {
		end = len(v)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		cand := v[i]
		line := cand.Value
		if cand.Desc != "" {
			line += " — " + cand.Desc
		}
		if i == c.Active() {
			b.WriteString(theme.Style(theme.SlotSelected).Render("  ❯ " + line))
		} else {
			b.WriteString(theme.Style(theme.SlotDim).Render("    " + line))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
