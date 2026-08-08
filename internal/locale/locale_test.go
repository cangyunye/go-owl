package locale

import (
	"os"
	"testing"

	"github.com/cangyunye/go-owl/internal/encoding"
	"golang.org/x/text/language"
)

func clearEnv(t *testing.T) func() {
	t.Helper()
	keys := []string{"OWL_LANG", "OWL_IO_ENCODING", "LC_ALL", "LC_CTYPE", "LANG"}
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

func TestResolveDefaults(t *testing.T) {
	restore := clearEnv(t)
	defer restore()

	l := Resolve()
	if l.Lang != language.Chinese {
		t.Fatalf("default lang: got %v", l.Lang)
	}
	if l.Charset != encoding.UTF8 {
		t.Fatalf("default charset: got %v", l.Charset)
	}
}

func TestResolveOWLLangPriority(t *testing.T) {
	restore := clearEnv(t)
	defer restore()

	os.Setenv("LC_CTYPE", "en_US.UTF-8")
	os.Setenv("OWL_LANG", "zh")
	l := Resolve()
	if l.Lang != language.Chinese {
		t.Fatalf("OWL_LANG should win: got %v", l.Lang)
	}
}

func TestResolveZhCNGBK(t *testing.T) {
	restore := clearEnv(t)
	defer restore()

	os.Setenv("LC_ALL", "zh_CN.GBK")
	l := Resolve()
	if l.Lang != language.Chinese {
		t.Fatalf("lang from zh_CN.GBK: got %v", l.Lang)
	}
	if l.Charset != encoding.GBK {
		t.Fatalf("charset from zh_CN.GBK: got %v", l.Charset)
	}
}

func TestResolveIndependentDimensions(t *testing.T) {
	restore := clearEnv(t)
	defer restore()

	// 英文用户但字符集仍是 GBK —— 两维独立
	os.Setenv("OWL_LANG", "en")
	os.Setenv("LC_ALL", "zh_CN.GBK")
	l := Resolve()
	if l.Lang != language.English {
		t.Fatalf("lang: got %v", l.Lang)
	}
	if l.Charset != encoding.GBK {
		t.Fatalf("charset should stay GBK: got %v", l.Charset)
	}
}

func TestResolveIOEncodingPriority(t *testing.T) {
	restore := clearEnv(t)
	defer restore()

	os.Setenv("LC_ALL", "zh_CN.UTF-8")
	os.Setenv("OWL_IO_ENCODING", "gbk")
	l := Resolve()
	if l.Charset != encoding.GBK {
		t.Fatalf("OWL_IO_ENCODING should win: got %v", l.Charset)
	}
}
