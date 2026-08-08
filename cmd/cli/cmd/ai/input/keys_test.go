package input

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// ---- parseKey: 字节流前缀 -> 按键事件 ----

func TestParseKey_ASCII(t *testing.T) {
	k, n := parseKey([]byte("abc"))
	if n != 1 || k.Code != KeyRune || k.Rune != 'a' {
		t.Fatalf("expected rune 'a' consuming 1 byte, got %+v (n=%d)", k, n)
	}
}

func TestParseKey_UTF8(t *testing.T) {
	bs := []byte("你好")
	k, n := parseKey(bs)
	if n != 3 || k.Code != KeyRune || k.Rune != '你' {
		t.Fatalf("expected rune '你' consuming 3 bytes, got %+v (n=%d)", k, n)
	}
}

func TestParseKey_Control(t *testing.T) {
	cases := []struct {
		name string
		in   byte
		want KeyCode
	}{
		{"enter_cr", '\r', KeyEnter},
		{"enter_lf", '\n', KeyEnter},
		{"tab", '\t', KeyTab},
		{"backspace", 0x7f, KeyBackspace},
		{"ctrl_c", 0x03, KeyCtrlC},
		{"ctrl_d", 0x04, KeyCtrlD},
		{"ctrl_a", 0x01, KeyCtrlA},
		{"ctrl_e", 0x05, KeyCtrlE},
		{"ctrl_w", 0x17, KeyCtrlW},
		{"ctrl_u", 0x15, KeyCtrlU},
		{"ctrl_k", 0x0b, KeyCtrlK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, n := parseKey([]byte{tc.in})
			if n != 1 || k.Code != tc.want {
				t.Fatalf("expected code %d consuming 1 byte, got %+v (n=%d)", tc.want, k, n)
			}
		})
	}
}

func TestParseKey_EscAlone(t *testing.T) {
	k, n := parseKey([]byte{0x1b})
	if n != 1 || k.Code != KeyEsc {
		t.Fatalf("expected Esc consuming 1 byte, got %+v (n=%d)", k, n)
	}
}

func TestParseKey_CSIArrows(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want KeyCode
	}{
		{"up", "\x1b[A", KeyUp},
		{"down", "\x1b[B", KeyDown},
		{"right", "\x1b[C", KeyRight},
		{"left", "\x1b[D", KeyLeft},
		{"home", "\x1b[H", KeyHome},
		{"end", "\x1b[F", KeyEnd},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, n := parseKey([]byte(tc.in))
			if n != len(tc.in) || k.Code != tc.want {
				t.Fatalf("expected code %d consuming %d bytes, got %+v (n=%d)", tc.want, len(tc.in), k, n)
			}
		})
	}
}

func TestParseKey_CSITilde(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want KeyCode
	}{
		{"home_1", "\x1b[1~", KeyHome},
		{"delete_3", "\x1b[3~", KeyDelete},
		{"end_4", "\x1b[4~", KeyEnd},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, n := parseKey([]byte(tc.in))
			if n != len(tc.in) || k.Code != tc.want {
				t.Fatalf("expected code %d consuming %d bytes, got %+v (n=%d)", tc.want, len(tc.in), k, n)
			}
		})
	}
}

func TestParseKey_UnknownCSI(t *testing.T) {
	// 未知 CSI 序列应整体丢弃,不影响后续输入
	k, n := parseKey([]byte("\x1b[Z"))
	if n != 3 || k.Code != KeyNone {
		t.Fatalf("expected unknown CSI dropped consuming 3 bytes, got %+v (n=%d)", k, n)
	}
}

func TestParseKey_UnfinishedEsc(t *testing.T) {
	// Esc 后跟的字节不足以构成完整序列时,不应 panic
	_, _ = parseKey([]byte{0x1b, '['})
	_, _ = parseKey([]byte{0x1b, '['})
}

// ---- KeyReader: io.Reader -> 按键事件流 ----

func TestKeyReader_Sequence(t *testing.T) {
	// 混合序列: "查" + Up + "询" + Enter
	bs := []byte("查")
	bs = append(bs, "\x1b[A"...)
	bs = append(bs, "询"...)
	bs = append(bs, '\r')

	kr := NewKeyReader(strings.NewReader(string(bs)))
	got := readAllKeys(t, kr)

	want := []Key{
		{Code: KeyRune, Rune: '查'},
		{Code: KeyUp},
		{Code: KeyRune, Rune: '询'},
		{Code: KeyEnter},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d keys, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("key[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestKeyReader_SplitUTF8AcrossReads(t *testing.T) {
	// 模拟一次只给 1 字节的慢速终端: 3 字节 UTF-8 应被完整拼成 1 个 rune
	bs := []byte("你")
	kr := NewKeyReader(&oneByteReader{b: bs})
	got := readAllKeys(t, kr)
	if len(got) != 1 || got[0].Rune != '你' {
		t.Fatalf("expected single rune '你', got %v", got)
	}
}

func TestKeyReader_SplitCSIAcrossReads(t *testing.T) {
	// ESC 与 "[A" 分两次读入(慢速终端/SSH),应解析为 Up 而非 Esc+文本
	kr := NewKeyReader(&deadlineChunkReader{chunks: [][]byte{{0x1b}, []byte("[A")}})
	got := readAllKeys(t, kr)
	if len(got) != 1 || got[0].Code != KeyUp {
		t.Fatalf("expected single Up key from split CSI, got %v", got)
	}
}

func TestKeyReader_SplitCSIUnterminatedThenByte(t *testing.T) {
	// "\x1b[" 与 "A" 分两次读入: parseCSI 不完整时应等待而非丢弃
	kr := NewKeyReader(&chunkedReader{chunks: [][]byte{[]byte("\x1b["), []byte("A")}})
	got := readAllKeys(t, kr)
	if len(got) != 1 || got[0].Code != KeyUp {
		t.Fatalf("expected single Up key from unterminated-then-byte CSI, got %v", got)
	}
}

func TestKeyReader_EscThenChar(t *testing.T) {
	// Esc 后跟普通字符(非 CSI): Esc 独立返回,字符保留
	kr := NewKeyReader(&chunkedReader{chunks: [][]byte{{0x1b, 'x'}}})
	got := readAllKeys(t, kr)
	want := []Key{{Code: KeyEsc}, {Code: KeyRune, Rune: 'x'}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected Esc then 'x', got %v", got)
	}
}

func TestKeyReader_LoneEscTimeout(t *testing.T) {
	// 支持 deadline 的终端上,单独 Esc 等待 50ms 无后续字节则确认为 Esc 键
	kr := NewKeyReader(&deadlineChunkReader{chunks: [][]byte{{0x1b}}})
	got := readAllKeys(t, kr)
	if len(got) != 1 || got[0].Code != KeyEsc {
		t.Fatalf("expected lone Esc via timeout, got %v", got)
	}
}

func TestKeyReader_LoneEscNoDeadlineFallback(t *testing.T) {
	// 无 deadline 支持(Windows console 等): 立即返回 Esc,不阻塞等待
	kr := NewKeyReader(&chunkedReader{chunks: [][]byte{{0x1b}}})
	got := readAllKeys(t, kr)
	if len(got) != 1 || got[0].Code != KeyEsc {
		t.Fatalf("expected lone Esc via fallback, got %v", got)
	}
}

func TestKeyReader_UnknownCSIDroppedThenChar(t *testing.T) {
	// 未知 CSI 整体丢弃,后续字符保留
	kr := NewKeyReader(&chunkedReader{chunks: [][]byte{[]byte("\x1b[Z"), []byte("q")}})
	got := readAllKeys(t, kr)
	if len(got) != 1 || got[0].Rune != 'q' {
		t.Fatalf("expected only 'q' after dropped unknown CSI, got %v", got)
	}
}

type chunkedReader struct {
	chunks [][]byte
	i      int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.i >= len(r.chunks) {
		return 0, io.EOF
	}
	c := r.chunks[r.i]
	r.i++
	return copy(p, c), nil
}

// deadlineChunkReader 模拟支持 SetReadDeadline 的终端输入(Unix TTY)。
// chunk 读完后若 deadline 未到返回超时错误,否则 EOF。
type deadlineChunkReader struct {
	chunks   [][]byte
	i        int
	deadline time.Time
}

func (r *deadlineChunkReader) Read(p []byte) (int, error) {
	if r.i < len(r.chunks) {
		c := r.chunks[r.i]
		r.i++
		return copy(p, c), nil
	}
	if !r.deadline.IsZero() && time.Now().Before(r.deadline) {
		return 0, os.ErrDeadlineExceeded
	}
	return 0, io.EOF
}

func (r *deadlineChunkReader) SetReadDeadline(t time.Time) error {
	r.deadline = t
	return nil
}

func readAllKeys(t *testing.T, kr *KeyReader) []Key {
	t.Helper()
	var keys []Key
	for {
		k, err := kr.ReadKey()
		if err != nil {
			t.Fatalf("ReadKey error: %v", err)
		}
		if k.Code == KeyNone {
			continue
		}
		if k.Code == KeyEOF {
			break
		}
		keys = append(keys, k)
	}
	return keys
}

type oneByteReader struct {
	b []byte
	i int
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, ioEOF
	}
	p[0] = r.b[r.i]
	r.i++
	return 1, nil
}

// ioEOF 便于测试内引用,避免额外 import
var ioEOF = errEOF
