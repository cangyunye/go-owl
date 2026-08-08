package input

import (
	"os"

	"golang.org/x/term"
)

// Terminal 真实终端适配: 满足 KeySource + io.Writer,并管理 raw mode。
// 通过 golang.org/x/term 实现,Windows 与 Unix 均支持。
type Terminal struct {
	in       *os.File
	out      *os.File
	kr       *KeyReader
	raw      bool
	oldState *term.State
}

func NewTerminal(in, out *os.File) *Terminal {
	return &Terminal{in: in, out: out, kr: NewKeyReader(in)}
}

func (t *Terminal) ReadKey() (Key, error) { return t.kr.ReadKey() }

func (t *Terminal) Write(p []byte) (int, error) { return t.out.Write(p) }

// IsTerminal 输入是否为交互式终端。
func (t *Terminal) IsTerminal() bool {
	return term.IsTerminal(int(t.in.Fd()))
}

// MakeRaw 进入 raw mode(逐字符读取)。非终端输入直接返回 nil。
func (t *Terminal) MakeRaw() error {
	if t.raw {
		return nil
	}
	if !t.IsTerminal() {
		return nil
	}
	old, err := term.MakeRaw(int(t.in.Fd()))
	if err != nil {
		return err
	}
	t.oldState = old
	t.raw = true
	return nil
}

// Restore 恢复终端原始模式。
func (t *Terminal) Restore() error {
	if !t.raw {
		return nil
	}
	t.raw = false
	return term.Restore(int(t.in.Fd()), t.oldState)
}
