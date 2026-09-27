package handler

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// 终端限额的设置解析：键缺失用默认值，非法字符串退回默认，负数等价于关闭该限制。
func TestTerminalLimits_FromSettings(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`)
	require.NoError(t, err)
	set := func(k, v string) {
		_, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`, k, v)
		require.NoError(t, err)
	}

	t.Run("无键则用默认值", func(t *testing.T) {
		idle, perUser := terminalLimits(db)
		assert.Equal(t, 30*time.Minute, idle)
		assert.Equal(t, 5, perUser)
	})

	t.Run("显式设置生效", func(t *testing.T) {
		set("terminal.idle_timeout_min", "10")
		set("terminal.max_per_user", "3")
		idle, perUser := terminalLimits(db)
		assert.Equal(t, 10*time.Minute, idle)
		assert.Equal(t, 3, perUser)
	})

	t.Run("零表示关闭该限制", func(t *testing.T) {
		set("terminal.idle_timeout_min", "0")
		set("terminal.max_per_user", "0")
		idle, perUser := terminalLimits(db)
		assert.Equal(t, time.Duration(0), idle)
		assert.Equal(t, 0, perUser)
	})

	t.Run("负数等价于关闭该限制", func(t *testing.T) {
		set("terminal.idle_timeout_min", "-2")
		set("terminal.max_per_user", "-2")
		idle, perUser := terminalLimits(db)
		assert.Equal(t, time.Duration(0), idle)
		assert.Equal(t, 0, perUser)
	})

	t.Run("非法字符串退回默认值", func(t *testing.T) {
		set("terminal.idle_timeout_min", "abc")
		set("terminal.max_per_user", "abc")
		idle, perUser := terminalLimits(db)
		assert.Equal(t, 30*time.Minute, idle)
		assert.Equal(t, 5, perUser)
	})
}

// 每用户并发终端注册表：达上限拒绝、断开释放、多余 release 不产生负计数、
// 不同用户互不影响、limit=0 表示不限制。
func TestTerminalSessions_AcquireRelease(t *testing.T) {
	s := newTerminalSessions()

	assert.True(t, s.acquire("alice", 2), "alice 第 1 个终端应可获取")
	assert.True(t, s.acquire("alice", 2), "alice 第 2 个终端应可获取")
	assert.False(t, s.acquire("alice", 2), "alice 第 3 个终端应被拒绝（达上限）")

	s.release("alice")
	assert.True(t, s.acquire("alice", 2), "释放后应可再次获取")

	assert.True(t, s.acquire("bob", 2), "bob 不受 alice 占用影响")

	s.release("alice")
	s.release("alice")
	assert.True(t, s.acquire("alice", 2), "多余 release 不应把计数推成负数")
}

// limit=0（不限制）时永不拒绝。
func TestTerminalSessions_Unlimited(t *testing.T) {
	s := newTerminalSessions()
	for i := 0; i < 50; i++ {
		assert.True(t, s.acquire("alice", 0), "limit=0 表示不限制")
	}
}
