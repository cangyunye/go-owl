package input

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrInterrupt 用户按 Ctrl+C / Ctrl+D 或输入流结束时返回。
var ErrInterrupt = errors.New("interrupt")

// KeySource 按键事件来源。真实终端由 term.go 的 Terminal 实现,测试注入预置队列。
type KeySource interface {
	ReadKey() (Key, error)
}

// EditorOptions 行编辑器配置。
type EditorOptions struct {
	Prompt     string              // 输入提示(可含 ANSI 颜色码)
	Commands   []SlashCommand      // 斜杠命令目录
	OnAction   func(SlashCommand)  // action 类命令确认后的回调
	History    []string            // 初始会话历史
	MaxHistory int                 // 历史上限,0 表示不限制
	TaskTag    string              // 菜单中 task 类命令的标签(本地化),空则不显示
	ActionTag  string              // 菜单中 action 类命令的标签(本地化),空则不显示
}

// Editor 轻量行编辑器: 单行编辑 + 斜杠命令补全菜单。
// 交互对齐网页端 SlashMenu:
//   - 输入以 "/" 开头时弹出命令菜单,继续输入按前缀过滤
//   - 菜单打开时 ↑↓ 选择候选,Enter/Tab 确认,Esc 取消(slash 输入被清空)
//   - 选中 task 命令展开为提示词模板,光标选中第一个 {arg} 占位符,直接输入即替换
//   - 选中 action 命令执行 OnAction 回调并返回空行
//   - 菜单关闭时 ↑↓ 导航会话历史
type Editor struct {
	keys       KeySource
	out        io.Writer
	prompt     string
	menu       *SlashMenu
	onAction   func(SlashCommand)
	history    []string
	maxHistory int

	buf        []rune
	cursor     int
	selStart   int // 选中区间起点(rune 索引),-1 表示无选区
	selEnd     int
	menuOpen    bool
	menuLines   int
	histIdx     int
	lastQuery   string // 上次菜单查询词,用于避免重复 SetQuery 重置选中项
	taskTag     string
	actionTag   string
}

const maxMenuItems = 10

// 注: 菜单项宽度超过终端宽度时不做换行保护,光标列定位可能偏差
// (现代终端会自动 wrap,仅影响视觉)。如需支持极窄终端可后续增加
// 基于 term.Size 的截断逻辑。

func NewEditor(keys KeySource, out io.Writer, opts EditorOptions) *Editor {
	history := opts.History
	if history == nil {
		history = []string{}
	}
	return &Editor{
		keys:       keys,
		out:        out,
		prompt:     opts.Prompt,
		menu:       NewSlashMenu(opts.Commands),
		onAction:   opts.OnAction,
		history:    history,
		maxHistory: opts.MaxHistory,
		taskTag:    opts.TaskTag,
		actionTag:  opts.ActionTag,
		selStart:   -1,
		selEnd:     -1,
		histIdx:    -1,
	}
}

// ReadLine 读取一行输入。
// Enter 提交返回行内容;Ctrl+C/Ctrl+D/EOF 返回 ErrInterrupt;
// action 类 slash 命令确认后执行回调并返回空行(调用方继续循环)。
func (e *Editor) ReadLine() (string, error) {
	e.buf = nil
	e.cursor = 0
	e.clearSel()
	e.menuOpen = false
	e.menuLines = 0
	e.histIdx = -1
	e.render()

	for {
		k, err := e.keys.ReadKey()
		if err != nil {
			return "", err
		}
		switch k.Code {
		case KeyRune:
			e.insertRune(k.Rune)
		case KeyEnter:
			line, done, err := e.handleEnter()
			if err != nil {
				return "", err
			}
			if done {
				if line != "" {
					// raw mode 下 Enter 不产生换行,手动换行避免后续输出粘在输入行
					fmt.Fprint(e.out, "\r\n")
				}
				return line, nil
			}
			// task 模板已展开,继续编辑(走下方统一 updateMenu+render)
		case KeyEsc:
			if e.menuOpen {
				// 取消 slash 输入,清空当前行;menuLines 保留给 render 清理旧菜单
				e.menuOpen = false
				e.buf = nil
				e.cursor = 0
				e.clearSel()
				e.render()
			}
			continue
		case KeyUp:
			if e.menuOpen {
				e.menu.MoveUp()
			} else {
				e.histBack()
			}
		case KeyDown:
			if e.menuOpen {
				e.menu.MoveDown()
			} else {
				e.histForward()
			}
		case KeyTab:
			if e.menuOpen {
				line, done, err := e.handleEnter()
				if err != nil {
					return "", err
				}
				if done {
					return line, nil
				}
				// task 模板已展开,继续编辑(走下方统一 updateMenu+render)
			}
			continue
		case KeyLeft:
			e.clearSel()
			if e.cursor > 0 {
				e.cursor--
			}
		case KeyRight:
			e.clearSel()
			if e.cursor < len(e.buf) {
				e.cursor++
			}
		case KeyHome, KeyCtrlA:
			e.clearSel()
			e.cursor = 0
		case KeyEnd, KeyCtrlE:
			e.clearSel()
			e.cursor = len(e.buf)
		case KeyBackspace:
			e.backspace()
		case KeyDelete:
			e.clearSel()
			if e.cursor < len(e.buf) {
				e.buf = append(e.buf[:e.cursor], e.buf[e.cursor+1:]...)
			}
		case KeyCtrlW:
			e.deleteWord()
		case KeyCtrlU:
			e.clearSel()
			e.buf = append([]rune{}, e.buf[e.cursor:]...)
			e.cursor = 0
		case KeyCtrlK:
			e.clearSel()
			e.buf = e.buf[:e.cursor]
		case KeyCtrlC, KeyCtrlD, KeyEOF:
			// 注: raw mode 下 Ctrl+D 即便行中有内容也直接中断;
			// 长时间 LLM 调用期间按 Ctrl+C 会等调用返回后才被处理(ISIG 关闭)。
			return "", ErrInterrupt
		case KeyNone:
			// 未知按键忽略
		}
		e.updateMenu()
		e.render()
	}
}

// handleEnter 处理确认键(Enter/Tab)。返回 (提交行, 是否结束 ReadLine, 错误)。
func (e *Editor) handleEnter() (string, bool, error) {
	if e.menuOpen {
		cmd, ok := e.menu.Selected()
		if !ok {
			// 菜单打开但无候选,按普通提交处理
			return e.submit(), true, nil
		}
		if cmd.Category == "action" {
			// menuLines 保留给 render 清理旧菜单
			e.menuOpen = false
			e.buf = nil
			e.cursor = 0
			e.clearSel()
			e.render()
			// 换行让 onAction 的输出从新行开始,而不是粘在提示行
			fmt.Fprint(e.out, "\r\n")
			if e.onAction != nil {
				e.onAction(cmd)
			}
			return "", true, nil
		}
		// task: 展开为提示词模板,选中第一个占位符;menuLines 保留给 render 清理旧菜单
		tpl := ExpandTemplate(cmd)
		e.buf = []rune(tpl)
		e.menuOpen = false
		if s, en, ok2 := PlaceholderRange(tpl); ok2 {
			sr := utf8.RuneCountInString(tpl[:s])
			er := utf8.RuneCountInString(tpl[:en])
			e.selStart, e.selEnd = sr, er
			e.cursor = sr
		} else {
			e.cursor = len(e.buf)
			e.clearSel()
		}
		return "", false, nil
	}
	return e.submit(), true, nil
}

func (e *Editor) submit() string {
	line := string(e.buf)
	e.commitHistory(line)
	e.buf = nil
	e.cursor = 0
	e.clearSel()
	return line
}

func (e *Editor) insertRune(rn rune) {
	if e.selStart >= 0 {
		e.buf = append(e.buf[:e.selStart], e.buf[e.selEnd:]...)
		e.cursor = e.selStart
		e.clearSel()
	}
	if e.cursor >= len(e.buf) {
		e.buf = append(e.buf, rn)
	} else {
		e.buf = append(e.buf, 0)
		copy(e.buf[e.cursor+1:], e.buf[e.cursor:])
		e.buf[e.cursor] = rn
	}
	e.cursor++
}

func (e *Editor) backspace() {
	if e.selStart >= 0 {
		e.buf = append(e.buf[:e.selStart], e.buf[e.selEnd:]...)
		e.cursor = e.selStart
		e.clearSel()
		return
	}
	if e.cursor > 0 {
		e.buf = append(e.buf[:e.cursor-1], e.buf[e.cursor:]...)
		e.cursor--
	}
}

func (e *Editor) deleteWord() {
	e.clearSel()
	i := e.cursor
	for i > 0 && unicode.IsSpace(e.buf[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(e.buf[i-1]) {
		i--
	}
	e.buf = append(e.buf[:i], e.buf[e.cursor:]...)
	e.cursor = i
}

func (e *Editor) clearSel() {
	e.selStart = -1
	e.selEnd = -1
}

func (e *Editor) histBack() {
	if len(e.history) == 0 {
		return
	}
	if e.histIdx == -1 {
		e.histIdx = len(e.history) - 1
	} else if e.histIdx > 0 {
		e.histIdx--
	} else {
		return
	}
	e.buf = []rune(e.history[e.histIdx])
	e.cursor = len(e.buf)
}

func (e *Editor) histForward() {
	if e.histIdx == -1 {
		return
	}
	e.histIdx++
	if e.histIdx >= len(e.history) {
		e.histIdx = -1
		e.buf = nil
		e.cursor = 0
		return
	}
	e.buf = []rune(e.history[e.histIdx])
	e.cursor = len(e.buf)
}

func (e *Editor) commitHistory(line string) {
	if line == "" {
		return
	}
	if len(e.history) > 0 && e.history[len(e.history)-1] == line {
		return
	}
	e.history = append(e.history, line)
	if e.maxHistory > 0 && len(e.history) > e.maxHistory {
		e.history = e.history[len(e.history)-e.maxHistory:]
	}
	e.histIdx = -1
}

// updateMenu 根据当前输入行决定菜单开关。
// 仅当查询词变化时才 SetQuery(它会重置选中项),避免 ↑↓ 选择被覆盖。
// 注意: 不在这里清零 menuLines,旧菜单行的清理统一由 render 完成。
func (e *Editor) updateMenu() {
	if len(e.buf) > 0 && e.buf[0] == '/' {
		q := ""
		if len(e.buf) > 1 {
			q = string(e.buf[1:])
		}
		if q != e.lastQuery {
			e.menu.SetQuery(q)
			e.lastQuery = q
		}
		e.menuOpen = len(e.menu.Visible()) > 0
		return
	}
	e.menuOpen = false
	e.lastQuery = ""
}

// render 重绘输入行与命令菜单,并把光标定位回输入行。
// 布局: 输入行占据 row R,菜单条目占据 R+1..R+n。
// 清理旧菜单时从输入行向下逐行清空,再回到输入行重绘。
func (e *Editor) render() {
	// 1) 清理输入行与旧菜单区(光标预期位于输入行)
	fmt.Fprint(e.out, "\r\x1b[2K")
	for i := 0; i < e.menuLines; i++ {
		fmt.Fprint(e.out, "\x1b[1B\x1b[2K")
	}
	if e.menuLines > 0 {
		fmt.Fprintf(e.out, "\x1b[%dA", e.menuLines)
	}

	// 2) 绘制输入行
	fmt.Fprint(e.out, e.prompt)
	fmt.Fprint(e.out, string(e.buf))

	// 3) 绘制菜单(每条条目独立一行,从输入行下一行开始)
	e.menuLines = 0
	if e.menuOpen {
		e.menuLines = e.renderMenu()
		if e.menuLines > 0 {
			fmt.Fprintf(e.out, "\x1b[%dA", e.menuLines)
		}
	}

	// 4) 定位光标到输入行的光标列
	col := 1 + displayWidth(stripANSI(e.prompt)) + displayWidth(string(e.buf[:e.cursor]))
	fmt.Fprintf(e.out, "\x1b[%dG", col)
}

func (e *Editor) renderMenu() int {
	items := e.menu.Visible()
	if len(items) == 0 {
		return 0
	}
	n := len(items)
	if n > maxMenuItems {
		n = maxMenuItems
	}
	for i := 0; i < n; i++ {
		// 每条菜单条目占一行(含第一条,从输入行下一行开始)
		fmt.Fprint(e.out, "\r\n")
		c := items[i]
		marker := "  "
		if i == e.menu.Active() {
			marker = "\x1b[36m>\x1b[0m "
		}
		tag := e.taskTag
		if c.Category == "action" {
			tag = e.actionTag
		}
		if tag == "" {
			fmt.Fprintf(e.out, "%s\x1b[90m/\x1b[0m%s %s %s %s\x1b[K",
				marker, c.Name, c.Icon, c.Label, c.Desc)
		} else {
			fmt.Fprintf(e.out, "%s\x1b[90m/\x1b[0m%s %s %s %s \x1b[90m%s\x1b[0m\x1b[K",
				marker, c.Name, c.Icon, c.Label, c.Desc, tag)
		}
	}
	return n
}

// stripANSI 去除 ANSI 转义序列,用于计算提示符显示宽度。
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// displayWidth 计算字符串的终端显示宽度(东亚宽字符与 emoji 按 2 列)。
func displayWidth(s string) int {
	w := 0
	for _, rn := range s {
		w += runeWidth(rn)
	}
	return w
}

func runeWidth(rn rune) int {
	switch {
	case rn >= 0x1100 && (rn <= 0x115f || rn == 0x2329 || rn == 0x232a ||
		(rn >= 0x2e80 && rn <= 0xa4cf && rn != 0x303f) ||
		(rn >= 0xac00 && rn <= 0xd7a3) ||
		(rn >= 0xf900 && rn <= 0xfaff) ||
		(rn >= 0xfe30 && rn <= 0xfe4f) ||
		(rn >= 0xff00 && rn <= 0xff60) ||
		(rn >= 0xffe0 && rn <= 0xffe6) ||
		(rn >= 0x1f300 && rn <= 0x1faff)):
		return 2
	}
	return 1
}
