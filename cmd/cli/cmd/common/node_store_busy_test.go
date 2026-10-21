package common

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// openBusyTestDB 打开短 busy_timeout 的文件库（:memory: 每连接独立，
// 模拟不了跨连接锁竞争）。
func openBusyTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_busy_timeout=10&_journal_mode=WAL")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		address TEXT NOT NULL DEFAULT '',
		port INTEGER NOT NULL DEFAULT 22,
		user TEXT NOT NULL DEFAULT 'root',
		password TEXT NOT NULL DEFAULT '',
		ssh_key TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'offline',
		groups TEXT NOT NULL DEFAULT '[]',
		labels TEXT NOT NULL DEFAULT '{}',
		proxy_jump TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_check_at DATETIME
	)`)
	require.NoError(t, err)
	return db
}

// holdWriteLock 在独立连接上持有写锁一段时间，模拟 owl-serve 的
// 清理/VACUUM 长事务。
func holdWriteLock(t *testing.T, path string, d time.Duration) {
	t.Helper()
	ctx := context.Background()
	holder, err := sql.Open("sqlite", path+"?_busy_timeout=10&_journal_mode=WAL")
	require.NoError(t, err)
	defer holder.Close()
	conn, err := holder.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `UPDATE nodes SET status = status WHERE id = 'n1'`)
	require.NoError(t, err)
	time.Sleep(d)
	_, err = conn.ExecContext(ctx, `COMMIT`)
	require.NoError(t, err)
}

// 与 owl-serve（另一进程/另一连接池）并发写时，busy_timeout 超时的
// SQLITE_BUSY 由 NodeStoreDB 有限重试兜底：锁释放后重试成功。
func TestNodeStoreDB_Update_RetriesOnBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.db")
	store := NewNodeStoreDB(openBusyTestDB(t, path))

	require.NoError(t, store.Add(&NodeInfo{ID: "n1", Address: "10.0.0.1"}))

	go holdWriteLock(t, path, 700*time.Millisecond)
	time.Sleep(50 * time.Millisecond) // 确保锁先被持有

	start := time.Now()
	err := store.Update(&NodeInfo{ID: "n1", Groups: []string{"web"}, Address: "10.0.0.1"})
	elapsed := time.Since(start)

	require.NoError(t, err, "锁释放后重试应成功")
	require.Greater(t, elapsed, 500*time.Millisecond, "应经历至少一次重试等待")

	got, err := store.Get("n1")
	require.NoError(t, err)
	require.Equal(t, []string{"web"}, got.Groups)
}

// 重试耗尽仍拿不到锁时，错误要带上可行动的提示，而不是裸的
// "database is locked (5)"。
func TestNodeStoreDB_Update_BusyHintAfterRetriesExhausted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.db")
	store := NewNodeStoreDB(openBusyTestDB(t, path))

	require.NoError(t, store.Add(&NodeInfo{ID: "n1", Address: "10.0.0.1"}))

	go holdWriteLock(t, path, 3*time.Second)
	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	err := store.Update(&NodeInfo{ID: "n1", Groups: []string{"web"}, Address: "10.0.0.1"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "数据库被其它进程占用")
	require.Contains(t, err.Error(), "database is locked", "底层错误应保留供排查")
	require.Less(t, start.Sub(start), time.Minute)
}

// 非锁类错误不重试：节点不存在立即返回。
func TestNodeStoreDB_Update_NotFoundNotRetried(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.db")
	store := NewNodeStoreDB(openBusyTestDB(t, path))

	start := time.Now()
	err := store.Update(&NodeInfo{ID: "ghost", Address: "10.0.0.9"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "node not found")
	require.Less(t, time.Since(start), 200*time.Millisecond, "非 SQLITE_BUSY 不应重试")
}
