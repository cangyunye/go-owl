package input

import (
	"os"
	"testing"
)

func TestTerminal_NonTTY(t *testing.T) {
	f, err := os.CreateTemp("", "term-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	term := NewTerminal(f, f)
	if term.IsTerminal() {
		t.Fatal("temp file should not be a terminal")
	}
	// 非终端输入下 raw mode 操作应静默成功
	if err := term.MakeRaw(); err != nil {
		t.Fatalf("MakeRaw on non-tty should not error, got %v", err)
	}
	if err := term.Restore(); err != nil {
		t.Fatalf("Restore error: %v", err)
	}
}

func TestTerminal_ReadKeyFromFile(t *testing.T) {
	f, err := os.CreateTemp("", "term-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	content := []byte("ok\x1b[A")
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	term := NewTerminal(f, f)
	keys := readAllKeys(t, term.kr)
	want := []Key{{Code: KeyRune, Rune: 'o'}, {Code: KeyRune, Rune: 'k'}, {Code: KeyUp}}
	if len(keys) != len(want) {
		t.Fatalf("expected %d keys, got %d: %v", len(want), len(keys), keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("key[%d] = %+v, want %+v", i, keys[i], want[i])
		}
	}
}
