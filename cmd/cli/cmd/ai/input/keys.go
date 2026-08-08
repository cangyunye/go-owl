// Package input 提供 CLI AI 交互所需的轻量终端输入能力:
// 原始字节流解析为按键事件、斜杠命令补全菜单、以及一个不依赖第三方
// readline 库的行编辑器。交互设计对齐网页端 AI 助手的 SlashMenu。
package input

import (
	"errors"
	"io"
	"os"
	"time"
	"unicode/utf8"
)

// KeyCode 按键类别。KeyRune 表示可打印字符(含中文等多字节 UTF-8)。
type KeyCode int

const (
	KeyNone KeyCode = iota // 未知/需丢弃的按键
	KeyRune
	KeyEnter
	KeyTab
	KeyEsc
	KeyBackspace
	KeyDelete
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyCtrlA
	KeyCtrlC
	KeyCtrlD
	KeyCtrlE
	KeyCtrlK
	KeyCtrlU
	KeyCtrlW
	KeyEOF
)

// Key 一个按键事件。
type Key struct {
	Code KeyCode
	Rune rune
}

var errEOF = io.EOF

// parseKey 解析字节流前缀为一个按键事件,返回消费的字节数。
// 返回 n==0 表示当前字节不足,需要更多数据(用于 UTF-8 多字节序列跨读)。
func parseKey(bs []byte) (Key, int) {
	if len(bs) == 0 {
		return Key{Code: KeyEOF}, 0
	}
	b := bs[0]
	switch {
	case b == 0x1b:
		return parseEsc(bs)
	case b == '\r' || b == '\n':
		return Key{Code: KeyEnter}, 1
	case b == '\t':
		return Key{Code: KeyTab}, 1
	case b == 0x7f:
		return Key{Code: KeyBackspace}, 1
	case b < 0x20:
		code := controlCode(b)
		if code == KeyNone {
			return Key{Code: KeyNone}, 1
		}
		return Key{Code: code}, 1
	case b >= 0x80:
		r, size := utf8.DecodeRune(bs)
		if r == utf8.RuneError && size <= 1 {
			if len(bs) < utf8.UTFMax {
				return Key{}, 0 // 不完整序列,等待更多字节
			}
			// 超过 4 字节仍是坏字节,消费 1 字节避免死循环
			return Key{Code: KeyRune, Rune: utf8.RuneError}, 1
		}
		return Key{Code: KeyRune, Rune: r}, size
	default:
		return Key{Code: KeyRune, Rune: rune(b)}, 1
	}
}

func controlCode(b byte) KeyCode {
	switch b {
	case 0x01:
		return KeyCtrlA
	case 0x03:
		return KeyCtrlC
	case 0x04:
		return KeyCtrlD
	case 0x05:
		return KeyCtrlE
	case 0x0b:
		return KeyCtrlK
	case 0x15:
		return KeyCtrlU
	case 0x17:
		return KeyCtrlW
	default:
		return KeyNone
	}
}

// parseEsc 处理 ESC(0x1b) 开头的序列。
func parseEsc(bs []byte) (Key, int) {
	if len(bs) == 1 {
		return Key{Code: KeyEsc}, 1
	}
	switch bs[1] {
	case '[':
		return parseCSI(bs)
	default:
		// 其他转义序列(如 \x1bO 应用键区),整体丢弃
		return Key{Code: KeyNone}, 2
	}
}

// parseCSI 处理 CSI(ESC [) 序列,如方向键、Home/End、Delete。
// 序列不完整(缓冲以参数结束、缺少终止符)时返回 n==0 等待更多字节。
func parseCSI(bs []byte) (Key, int) {
	i := 2
	terminated := false
	for i < len(bs) {
		c := bs[i]
		i++
		if c >= 0x40 && c <= 0x7e {
			terminated = true
			break // 终止符(@~)
		}
	}
	if !terminated {
		return Key{}, 0 // 序列不完整,等待更多字节
	}
	seq := string(bs[:i])
	switch seq {
	case "\x1b[A":
		return Key{Code: KeyUp}, i
	case "\x1b[B":
		return Key{Code: KeyDown}, i
	case "\x1b[C":
		return Key{Code: KeyRight}, i
	case "\x1b[D":
		return Key{Code: KeyLeft}, i
	case "\x1b[H":
		return Key{Code: KeyHome}, i
	case "\x1b[F":
		return Key{Code: KeyEnd}, i
	case "\x1b[1~":
		return Key{Code: KeyHome}, i
	case "\x1b[3~":
		return Key{Code: KeyDelete}, i
	case "\x1b[4~":
		return Key{Code: KeyEnd}, i
	}
	return Key{Code: KeyNone}, i
}

// KeyReader 从 io.Reader 读取原始字节并解析为按键事件流。
type KeyReader struct {
	r          io.Reader
	buf        []byte
	pendingErr error
}

func NewKeyReader(r io.Reader) *KeyReader {
	return &KeyReader{r: r}
}

// escConfirmTimeout 单独 ESC 等待后续字节确认是否为转义序列开头的时长。
const escConfirmTimeout = 50 * time.Millisecond

// readDeadliner 底层输入支持设置读取截止时间(如 *os.File 的终端)。
// 用于单独 ESC 的短等待确认;不支持时直接返回 Esc。
type readDeadliner interface {
	SetReadDeadline(t time.Time) error
}

// ReadKey 返回下一个按键事件。底层输入结束时返回 {Code: KeyEOF}。
// 处理转义序列跨读: 单独的 ESC 会短等待后续字节确认是否为
// 完整转义序列的开头(底层支持 deadline 时);未知 CSI 整体丢弃;
// ESC 后跟非 '['(如独立 Esc 键后紧接普通字符)时 Esc 独立返回,后续字节保留。
func (kr *KeyReader) ReadKey() (Key, error) {
	for {
		if len(kr.buf) == 0 {
			kr.fill()
			if len(kr.buf) == 0 {
				if kr.pendingErr == io.EOF {
					return Key{Code: KeyEOF}, nil
				}
				if kr.pendingErr != nil {
					return Key{}, kr.pendingErr
				}
				continue
			}
		}

		k, n := parseKey(kr.buf)

		// 单独的 ESC: 可能是转义序列开头,短等待确认(底层支持 deadline 时)
		if k.Code == KeyEsc && n == 1 && len(kr.buf) == 1 {
			if dl, ok := kr.r.(readDeadliner); ok {
				if err := dl.SetReadDeadline(time.Now().Add(escConfirmTimeout)); err == nil {
					kr.fill()
					_ = dl.SetReadDeadline(time.Time{})
					if isTimeoutErr(kr.pendingErr) {
						kr.pendingErr = nil // 超时视为独立 Esc 键
					}
					if len(kr.buf) > 1 {
						continue // 有新字节,重新解析
					}
				}
			}
			// 无 deadline 支持(如 Windows console)或设置失败:
			// 立即确认 Esc,不阻塞等待后续字节。Windows console 的
			// 转义序列通常整段到达,split 罕见,优先保证 Esc 响应及时。
			kr.buf = nil
			return Key{Code: KeyEsc}, nil
		}

		if n == 0 {
			// 不完整序列(如 UTF-8 多字节未读全),等待更多字节
			kr.fill()
			if len(kr.buf) > 0 && kr.pendingErr == nil {
				continue
			}
			// 输入已耗尽,残余字节无法解析,按 EOF 处理
			kr.buf = nil
			return Key{Code: KeyEOF}, nil
		}

		// ESC 后跟普通字符(非 CSI 序列): Esc 独立返回,后续字节保留
		if k.Code == KeyNone && n > 1 && kr.buf[0] == 0x1b && kr.buf[1] != '[' {
			kr.buf = kr.buf[1:]
			return Key{Code: KeyEsc}, nil
		}

		kr.buf = kr.buf[n:]
		if k.Code != KeyNone {
			return k, nil
		}
		// 未知序列已整体丢弃,继续
	}
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne interface{ Timeout() bool }
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return false
}

// fill 从底层读取数据追加到缓冲;记录底层错误供后续处理。
func (kr *KeyReader) fill() {
	if kr.pendingErr != nil {
		return
	}
	tmp := make([]byte, 512)
	n, err := kr.r.Read(tmp)
	if n > 0 {
		kr.buf = append(kr.buf, tmp[:n]...)
	}
	if err != nil {
		kr.pendingErr = err
	}
}
