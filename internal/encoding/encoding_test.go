package encoding

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

func clearCharsetEnv(t *testing.T) func() {
	t.Helper()
	keys := []string{"OWL_IO_ENCODING", "LC_ALL", "LC_CTYPE", "LANG"}
	old := map[string]string{}
	for _, k := range keys {
		old[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	return func() {
		for _, k := range keys {
			if old[k] != "" {
				os.Setenv(k, old[k])
			} else {
				os.Unsetenv(k)
			}
		}
	}
}

func TestParseCharset(t *testing.T) {
	cases := []struct {
		in   string
		want Charset
		ok   bool
	}{
		{"utf8", UTF8, true},
		{"UTF-8", UTF8, true},
		{"65001", UTF8, true},
		{"gbk", GBK, true},
		{"GB2312", GBK, true},
		{"cp936", GBK, true},
		{"big5", Big5, true},
		{"cp950", Big5, true},
		{"bogus", UTF8, false},
		{"", UTF8, false},
	}
	for _, c := range cases {
		got, ok := ParseCharset(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseCharset(%q) = %v,%v want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestResolveCharsetPriority(t *testing.T) {
	restore := clearCharsetEnv(t)
	defer restore()

	os.Setenv("LC_ALL", "en_US.UTF-8")
	os.Setenv("OWL_IO_ENCODING", "gbk")
	if got := ResolveCharset(); got != GBK {
		t.Fatalf("OWL_IO_ENCODING should win: got %v", got)
	}
}

func TestResolveCharsetFromLC(t *testing.T) {
	restore := clearCharsetEnv(t)
	defer restore()

	os.Setenv("LANG", "zh_CN.GBK")
	if got := ResolveCharset(); got != GBK {
		t.Fatalf("LANG suffix: got %v", got)
	}
}

func TestUTF8GBKRoundtrip(t *testing.T) {
	src := []byte("你好 world 运维")
	gbk, err := EncodeCharset(src, GBK)
	if err != nil {
		t.Fatal(err)
	}
	if utf8.Valid(gbk) {
		t.Fatal("GBK output should not be valid UTF-8")
	}
	back, err := DecodeCharset(gbk, GBK)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, src) {
		t.Fatalf("roundtrip mismatch: %q", back)
	}
}

func TestNewEncoderWritesGBK(t *testing.T) {
	var buf bytes.Buffer
	w := NewEncoder(&buf, GBK)
	if _, err := w.Write([]byte("节点")); err != nil {
		t.Fatal(err)
	}
	if utf8.Valid(buf.Bytes()) {
		t.Fatal("expected GBK bytes")
	}
}

func TestNewDecoderReadsGBK(t *testing.T) {
	gbk, _ := EncodeCharset([]byte("节点"), GBK)
	r := NewDecoder(bytes.NewReader(gbk), GBK)
	out := make([]byte, 6)
	n, _ := r.Read(out)
	if string(out[:n]) != "节点" {
		t.Fatalf("decoded: %q", out[:n])
	}
}

func TestReadFileBytesUTF8AndGBK(t *testing.T) {
	dir := t.TempDir()
	utf8Path := filepath.Join(dir, "utf8.yaml")
	gbkPath := filepath.Join(dir, "gbk.yaml")

	os.WriteFile(utf8Path, []byte("描述: 部署"), 0644)
	os.WriteFile(gbkPath, []byte("描述: 部署"), 0644)

	// 转成 GBK 覆盖 gbk 文件
	gbkBytes, _ := EncodeCharset([]byte("描述: 部署"), GBK)
	os.WriteFile(gbkPath, gbkBytes, 0644)

	got, err := ReadFileBytes(utf8Path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "描述: 部署" {
		t.Fatalf("utf8 read: %q", got)
	}

	got2, err := ReadFileBytes(gbkPath, &gbkCS)
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != "描述: 部署" {
		t.Fatalf("gbk read: %q", got2)
	}
}

var gbkCS = GBK

func TestReadFileBytesAutoDetectGBKByEnv(t *testing.T) {
	restore := clearCharsetEnv(t)
	defer restore()
	os.Setenv("LANG", "zh_CN.GBK")

	dir := t.TempDir()
	gbkPath := filepath.Join(dir, "gbk.yaml")
	gbkBytes, _ := EncodeCharset([]byte("描述: 部署"), GBK)
	os.WriteFile(gbkPath, gbkBytes, 0644)

	got, err := ReadFileBytes(gbkPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "描述: 部署" {
		t.Fatalf("gbk env autodetect: %q", got)
	}
}

func TestReadFileBytesStripBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bom.yaml")
	os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, []byte("描述: x")...), 0644)
	got, err := ReadFileBytes(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "描述: x" {
		t.Fatalf("bom stripped: %q", got)
	}
}
