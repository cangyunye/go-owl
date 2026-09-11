package exec

import (
	"os"
	"path/filepath"
	"testing"
)

// 回归:--inline 模式曾误入本地文件存在性检查,内容参数被当作路径而报
// "脚本文件不存在"(tests/scripts/test-exec.sh 内联用例实测失败)。
func TestValidateScriptArg(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(existing, []byte("echo ok"), 0o600); err != nil {
		t.Fatal(err)
	}

	defer func() { scriptInline = false }()

	// 文件模式:存在的文件通过,不存在的报错
	scriptInline = false
	if err := validateScriptArg(existing); err != nil {
		t.Errorf("存在的本地脚本应通过: %v", err)
	}
	if err := validateScriptArg(filepath.Join(dir, "nope.sh")); err == nil {
		t.Error("不存在的本地脚本应报错")
	}

	// 内联模式:参数是脚本内容,不应做文件检查
	scriptInline = true
	if err := validateScriptArg("echo inline-test"); err != nil {
		t.Errorf("--inline 内容不应做文件检查: %v", err)
	}

	// URL 直通:含恰好 8 字节的 "http://x",旧 len>8 + 切片前缀判断会漏判
	scriptInline = false
	for _, u := range []string{"http://x", "https://e", "https://example.com/s.sh"} {
		if err := validateScriptArg(u); err != nil {
			t.Errorf("URL %q 应直通: %v", u, err)
		}
	}
}
