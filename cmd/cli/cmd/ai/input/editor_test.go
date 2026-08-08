package input

import (
	"bytes"
	"errors"
	"testing"
)

// keyQueue 预置按键队列,模拟终端按键流。
type keyQueue struct {
	keys []Key
}

func (q *keyQueue) ReadKey() (Key, error) {
	if len(q.keys) == 0 {
		return Key{Code: KeyEOF}, nil
	}
	k := q.keys[0]
	q.keys = q.keys[1:]
	return k, nil
}

func r(r rune) Key { return Key{Code: KeyRune, Rune: r} }

var (
	kEnter = Key{Code: KeyEnter}
	kTab   = Key{Code: KeyTab}
	kEsc   = Key{Code: KeyEsc}
	kBS    = Key{Code: KeyBackspace}
	kDel   = Key{Code: KeyDelete}
	kUp    = Key{Code: KeyUp}
	kDown  = Key{Code: KeyDown}
	kLeft  = Key{Code: KeyLeft}
	kRight = Key{Code: KeyRight}
	kHome  = Key{Code: KeyHome}
	kEnd   = Key{Code: KeyEnd}
	kCtrlC = Key{Code: KeyCtrlC}
	kCtrlD = Key{Code: KeyCtrlD}
	kCtrlU = Key{Code: KeyCtrlU}
	kCtrlK = Key{Code: KeyCtrlK}
	kCtrlW = Key{Code: KeyCtrlW}
)

// runKeys 用给定按键序列驱动一次 ReadLine,返回行内容。
func runKeys(t *testing.T, keys ...Key) (string, string) {
	t.Helper()
	var out bytes.Buffer
	q := &keyQueue{keys: keys}
	ed := NewEditor(q, &out, EditorOptions{
		Prompt:   "您> ",
		Commands: testCommands(),
	})
	line, err := ed.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine error: %v", err)
	}
	return line, out.String()
}

func TestEditor_PlainInput(t *testing.T) {
	line, _ := runKeys(t, r('h'), r('i'), kEnter)
	if line != "hi" {
		t.Fatalf("expected 'hi', got %q", line)
	}
}

func TestEditor_ChineseInput(t *testing.T) {
	line, _ := runKeys(t, r('查'), r('询'), r('节'), r('点'), kEnter)
	if line != "查询节点" {
		t.Fatalf("expected '查询节点', got %q", line)
	}
}

func TestEditor_Backspace(t *testing.T) {
	line, _ := runKeys(t, r('a'), r('b'), r('c'), kBS, kEnter)
	if line != "ab" {
		t.Fatalf("expected 'ab', got %q", line)
	}
}

func TestEditor_InsertAtCursor(t *testing.T) {
	line, _ := runKeys(t, r('a'), r('c'), kLeft, r('b'), kEnter)
	if line != "abc" {
		t.Fatalf("expected 'abc', got %q", line)
	}
}

func TestEditor_HomeEnd(t *testing.T) {
	line, _ := runKeys(t, r('b'), r('c'), kHome, r('a'), kEnd, r('d'), kEnter)
	if line != "abcd" {
		t.Fatalf("expected 'abcd', got %q", line)
	}
}

func TestEditor_CtrlU_K(t *testing.T) {
	line, _ := runKeys(t, r('a'), r('b'), r('c'), kCtrlU, r('x'), kEnter)
	if line != "x" {
		t.Fatalf("expected 'x' after Ctrl+U, got %q", line)
	}
	line2, _ := runKeys(t, r('a'), r('b'), r('c'), kHome, kCtrlK, kEnter)
	if line2 != "" {
		t.Fatalf("expected '' after Ctrl+K from home, got %q", line2)
	}
}

func TestEditor_SlashOpensMenu(t *testing.T) {
	_, out := runKeys(t, r('/'), kEsc, kEnter)
	if !contains(out, "exec") || !contains(out, "执行命令") {
		t.Fatalf("expected slash menu rendered with exec entry, output: %q", out)
	}
}

func TestEditor_RenderMenuOnOwnRow(t *testing.T) {
	// 菜单第一条必须独立成行(输入行之后换行),否则光标上移数学错位
	_, out := runKeys(t, r('/'), kEsc, kEnter)
	if !contains(out, "您> /\r\n") {
		t.Fatalf("expected menu first item on its own line after input, output: %q", out)
	}
}

func TestEditor_RenderMenuRowsCleared(t *testing.T) {
	// 菜单打开时渲染了 11 行;Esc 关闭后应清理: 输出应包含下移清行序列
	_, out := runKeys(t, r('/'), kEsc, kEnter)
	if !contains(out, "\x1b[1B\x1b[2K") {
		t.Fatalf("expected menu rows cleared with down+erase-line, output: %q", out)
	}
}

func TestEditor_SlashFilterAndExpand(t *testing.T) {
	line, _ := runKeys(t, r('/'), r('e'), r('x'), kEnter, kEnter)
	want := "在 {nodes} 上执行命令 {command}"
	if line != want {
		t.Fatalf("expected %q, got %q", want, line)
	}
}

func TestEditor_TaskExpandClearsMenu(t *testing.T) {
	// 选中 /exec 展开模板后,旧菜单行必须被清理(下移清行序列)
	_, out := runKeys(t, r('/'), r('e'), r('x'), kEnter, kEnter)
	if !contains(out, "\x1b[1B\x1b[2K") {
		t.Fatalf("expected old menu rows cleared after template expansion, output: %q", out)
	}
}

func TestEditor_SlashPlaceholderReplace(t *testing.T) {
	// 选中 /exec 模板后,光标停在 {nodes} 占位符上,直接输入即替换占位符
	line, _ := runKeys(t, r('/'), r('e'), r('x'), kEnter, r('n'), r('1'), kEnter)
	want := "在 n1 上执行命令 {command}"
	if line != want {
		t.Fatalf("expected %q, got %q", want, line)
	}
}

func TestEditor_SlashArrowSelectsSecond(t *testing.T) {
	// 过滤 'q' 得到 [query, quit],Down 选到 quit(action),确认后触发回调
	var got []string
	var out bytes.Buffer
	q := &keyQueue{keys: []Key{r('/'), r('q'), kDown, kEnter}}
	ed := NewEditor(q, &out, EditorOptions{
		Prompt:   "您> ",
		Commands: testCommands(),
		OnAction: func(c SlashCommand) { got = append(got, c.Name) },
	})
	line, err := ed.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine error: %v", err)
	}
	if line != "" {
		t.Fatalf("expected empty line after action command, got %q", line)
	}
	if len(got) != 1 || got[0] != "quit" {
		t.Fatalf("expected OnAction('quit'), got %v", got)
	}
}

func TestEditor_SlashActionCallback(t *testing.T) {
	var got []string
	var out bytes.Buffer
	q := &keyQueue{keys: []Key{r('/'), r('h'), r('e'), r('l'), r('p'), kEnter}}
	ed := NewEditor(q, &out, EditorOptions{
		Prompt:   "您> ",
		Commands: testCommands(),
		OnAction: func(c SlashCommand) { got = append(got, c.Name) },
	})
	line, err := ed.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine error: %v", err)
	}
	if line != "" {
		t.Fatalf("expected empty line, got %q", line)
	}
	if len(got) != 1 || got[0] != "help" {
		t.Fatalf("expected OnAction('help'), got %v", got)
	}
}

func TestEditor_EscCancelsSlashInput(t *testing.T) {
	// Esc 取消 slash 输入并清空行,后续输入正常
	line, _ := runKeys(t, r('/'), kEsc, r('h'), r('i'), kEnter)
	if line != "hi" {
		t.Fatalf("expected 'hi' after Esc cancel, got %q", line)
	}
}

func TestEditor_TabConfirmsSelected(t *testing.T) {
	line, _ := runKeys(t, r('/'), kTab, kEnter)
	want := "在 {nodes} 上执行命令 {command}"
	if line != want {
		t.Fatalf("expected %q via Tab, got %q", want, line)
	}
}

func TestEditor_HistoryNavigation(t *testing.T) {
	var out bytes.Buffer
	q := &keyQueue{keys: []Key{r('h'), r('i'), kEnter}}
	ed := NewEditor(q, &out, EditorOptions{Prompt: "您> ", Commands: testCommands()})
	line, err := ed.ReadLine()
	if err != nil || line != "hi" {
		t.Fatalf("first ReadLine: line=%q err=%v", line, err)
	}

	// 第二次读取: Up 调出历史,再 Enter 提交
	q.keys = []Key{kUp, kEnter}
	line2, err := ed.ReadLine()
	if err != nil || line2 != "hi" {
		t.Fatalf("second ReadLine with history: line=%q err=%v", line2, err)
	}
}

func TestEditor_CtrlC_Interrupt(t *testing.T) {
	var out bytes.Buffer
	q := &keyQueue{keys: []Key{r('x'), kCtrlC}}
	ed := NewEditor(q, &out, EditorOptions{Prompt: "您> ", Commands: testCommands()})
	_, err := ed.ReadLine()
	if !errors.Is(err, ErrInterrupt) {
		t.Fatalf("expected ErrInterrupt, got %v", err)
	}
}

func TestEditor_EOF_Interrupt(t *testing.T) {
	var out bytes.Buffer
	q := &keyQueue{}
	ed := NewEditor(q, &out, EditorOptions{Prompt: "您> ", Commands: testCommands()})
	_, err := ed.ReadLine()
	if !errors.Is(err, ErrInterrupt) {
		t.Fatalf("expected ErrInterrupt on EOF, got %v", err)
	}
}

func TestEditor_DeleteKey(t *testing.T) {
	// 光标在 'c' 前,Delete 删除光标处字符 'c'
	line, _ := runKeys(t, r('a'), r('b'), r('c'), kLeft, kDel, kEnter)
	if line != "ab" {
		t.Fatalf("expected 'ab', got %q", line)
	}
}

func TestEditor_CtrlW_DeleteWord(t *testing.T) {
	line, _ := runKeys(t, r('a'), r(' '), r('b'), r('c'), kCtrlW, kEnter)
	if line != "a " {
		t.Fatalf("expected 'a ', got %q", line)
	}
}

func contains(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}
