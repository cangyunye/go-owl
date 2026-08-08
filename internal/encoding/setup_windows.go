//go:build windows

package encoding

import (
	"os"

	"golang.org/x/sys/windows"
)

func platformDefaultCharset() Charset {
	// 无显式配置时随控制台代码页：936=GBK，65001=UTF-8，950=Big5。
	cp := currentConsoleOutputCP()
	switch cp {
	case 936:
		return GBK
	case 950:
		return Big5
	default:
		return UTF8
	}
}

func currentConsoleOutputCP() uint32 {
	cp, err := windows.GetConsoleOutputCP()
	if err != nil {
		return 65001
	}
	return cp
}

// Setup 将控制台输入输出切到 UTF-8（CP65001），使原始 UTF-8 字节直接正确显示。
func Setup() {
	windows.SetConsoleOutputCP(65001)
	windows.SetConsoleCP(65001)
}

// RawStdin / RawStdout / RawStderr 返回未包装的原始句柄，供子进程直连终端。
func RawStdin() *os.File  { return os.Stdin }
func RawStdout() *os.File { return os.Stdout }
func RawStderr() *os.File { return os.Stderr }