package handler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// 默认 bufio.Scanner 单行上限 64KB：压缩成一行的 JSON/base64 等超长行会让
// Scan 返回 false，该行及其后的全部输出被静默丢弃——命令看起来"打印了一些
// 就不再输出后面的内容"。
func TestReadLines_LongLineNotTruncated(t *testing.T) {
	longLine := strings.Repeat("x", 200*1024) // 200KB，超过默认 64KB 上限
	input := longLine + "\nsecond line\n"

	var got []string
	err := readLines(strings.NewReader(input), func(line string) bool {
		got = append(got, line)
		return true
	})
	require.NoError(t, err)
	require.Len(t, got, 2, "超长行之后的输出必须保留")
	assert.Len(t, got[0], len(longLine), "超长行不得被截断")
	assert.Equal(t, "second line", got[1])
}

func TestReadLines_OverLimitLineReportsError(t *testing.T) {
	hugeLine := strings.Repeat("y", maxOutputLineBytes+1024) // 超过硬上限

	var got []string
	err := readLines(strings.NewReader(hugeLine+"\n"), func(line string) bool {
		got = append(got, line)
		return true
	})
	require.Error(t, err, "超硬上限必须报错而不是静默截断")
	assert.Empty(t, got)
}

func TestReadLines_StopsWhenEmitReturnsFalse(t *testing.T) {
	var got []string
	err := readLines(strings.NewReader("a\nb\nc\n"), func(line string) bool {
		got = append(got, line)
		return false // 模拟 ctx 取消/下游退出
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, got, "emit 返回 false 应立即停止读取")
}

func TestReadLines_PlainLines(t *testing.T) {
	var got []string
	err := readLines(strings.NewReader("one\ntwo\nthree\n"), func(line string) bool {
		got = append(got, line)
		return true
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"one", "two", "three"}, got)
}

// 真实 SSH 链路验证：单行 200KB 的输出必须完整采集（localhost 不可用时跳过）。
func TestExecuteStream_LongLineE2E(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	key, err := os.ReadFile(filepath.Join(home, ".ssh", "id_rsa"))
	if err != nil {
		t.Skip("no ~/.ssh/id_rsa, skipping long-line e2e")
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "root"
	}

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, ssh_key, status) VALUES (?,?,?,?,?,?,?)`,
		"localhost-longline", "localhost", "127.0.0.1", 22, user, string(key), "online")
	require.NoError(t, err)

	exec := &sshExecutor{db: db}
	info := &nodeSSHInfo{Address: "127.0.0.1", Port: 22, User: user, SSHKey: string(key)}
	if c, sc, derr := dialSFTP(info); derr != nil {
		t.Skipf("localhost SSH unavailable: %v", derr)
	} else {
		c.Close()
		sc.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	outputCh := make(chan OutputLine, 256)
	const longLen = 200 * 1024
	cmd := fmt.Sprintf(`head -c %d /dev/zero | tr '\0' 'x'; echo; echo TAIL-MARKER`, longLen)

	// 注意：ExecuteStream 不关闭 outputCh（调用方以返回值为完成信号），
	// 因此用 done channel 汇合，不能 range。
	resCh := make(chan struct{}, 1)
	go func() {
		_, _ = exec.ExecuteStream(ctx, "localhost-longline", cmd, outputCh)
		resCh <- struct{}{}
	}()

	var lines []string
	collect := func() {
		for {
			select {
			case line := <-outputCh:
				lines = append(lines, line.Line)
			default:
				return
			}
		}
	}
	for {
		select {
		case line := <-outputCh:
			lines = append(lines, line.Line)
		case <-resCh:
			collect() // 返回值到达后仍可能有缓冲行
			goto assertDone
		case <-ctx.Done():
			t.Fatal("执行超时")
		}
	}
assertDone:
	require.GreaterOrEqual(t, len(lines), 2, "超长行与后续输出都必须采集到")
	assert.Len(t, lines[0], longLen, "超长行不得被 64KB 默认上限截断")
	assert.Equal(t, "TAIL-MARKER", lines[len(lines)-1], "超长行之后的输出必须保留")
}
