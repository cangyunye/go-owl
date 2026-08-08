//go:build !windows

package encoding

import "os"

func platformDefaultCharset() Charset {
	return UTF8
}

// Setup 非 Windows 平台无需切换控制台代码页。
func Setup() {}

// RawStdin / RawStdout / RawStderr 返回未包装的原始句柄，供子进程直连终端。
func RawStdin() *os.File  { return os.Stdin }
func RawStdout() *os.File { return os.Stdout }
func RawStderr() *os.File { return os.Stderr }