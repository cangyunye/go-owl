package monitor

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStore_OpenAndQuery 验证按月分区存储：写入后可按时段查询。
func TestStore_OpenAndQuery(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "monitor.db"))
	require.NoError(t, err)
	defer s.Close()

	now := time.Now().Unix()
	err = s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "load.load1", TS: now, Value: 0.5},
		{NodeID: "n1", Metric: "load.load1", TS: now + 1, Value: 0.6},
	})
	require.NoError(t, err)

	rows, err := s.QuerySamples("n1", "load.load1", now-10, now+10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.InDelta(t, 0.6, rows[1].Value, 0.001)
}

// TestStore_InsertGroupsByMonth 验证跨月批量写入自动落到对应分区表，
// 跨月查询合并返回。
func TestStore_InsertGroupsByMonth(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "monitor.db"))
	require.NoError(t, err)
	defer s.Close()

	// 上月末与本月开头各一条
	endOfPrevMonth := time.Date(2026, 5, 31, 23, 59, 0, 0, time.UTC)
	startOfThisMonth := time.Date(2026, 6, 1, 0, 1, 0, 0, time.UTC)
	err = s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "mem.used_pct", TS: endOfPrevMonth.Unix(), Value: 10},
		{NodeID: "n1", Metric: "mem.used_pct", TS: startOfThisMonth.Unix(), Value: 20},
	})
	require.NoError(t, err)

	rows, err := s.QuerySamples("n1", "mem.used_pct",
		endOfPrevMonth.Unix()-1, startOfThisMonth.Unix()+1)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.InDelta(t, 10, rows[0].Value, 0.001)
	require.InDelta(t, 20, rows[1].Value, 0.001)
}

// TestStore_Cleanup 验证清理任务：删除过期行、DROP 早于保留期的整张分区表。
func TestStore_Cleanup(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "monitor.db"))
	require.NoError(t, err)
	defer s.Close()

	now := time.Now().UTC()
	oldTS := now.AddDate(0, 0, -40).Unix()   // 40 天前 → 过期
	recentTS := now.AddDate(0, 0, -1).Unix() // 1 天前 → 保留

	// 构造一个必然早于保留期的旧分区表，模拟历史遗留
	oldMonth := "202401"
	_, err = s.db.Exec(fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS metrics_%s (node_id TEXT, metric TEXT, ts INTEGER, value REAL, PRIMARY KEY (node_id, metric, ts))`, oldMonth))
	require.NoError(t, err)
	oldMonthTS := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Unix()

	err = s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "load.load1", TS: oldTS, Value: 9},
		{NodeID: "n1", Metric: "load.load1", TS: recentTS, Value: 1},
		{NodeID: "n1", Metric: "load.load1", TS: oldMonthTS, Value: 7},
	})
	require.NoError(t, err)

	// 全部落库确认
	all, err := s.QuerySamples("n1", "load.load1", 0, now.AddDate(0, 0, 1).Unix())
	require.NoError(t, err)
	require.Len(t, all, 3)

	require.NoError(t, s.Cleanup(30))

	// 过期行被删除
	kept, err := s.QuerySamples("n1", "load.load1", 0, now.AddDate(0, 0, 1).Unix())
	require.NoError(t, err)
	require.Len(t, kept, 1)
	require.InDelta(t, 1, kept[0].Value, 0.001)

	// 旧分区表被 DROP
	var n int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='metrics_202401'`).Scan(&n)
	require.NoError(t, err)
	require.Equal(t, 0, n, "早于保留期的分区表应被删除")
}

// TestStore_Cleanup_Idempotent 验证重复清理不报错。
func TestStore_Cleanup_Idempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "monitor.db"))
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.Cleanup(30))
	require.NoError(t, s.Cleanup(30))
}

// TestStore_DedupOverwrite 验证同 (node, metric, ts) 重复写入为覆盖而非重复。
func TestStore_DedupOverwrite(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "monitor.db"))
	require.NoError(t, err)
	defer s.Close()

	ts := time.Now().Unix()
	require.NoError(t, s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "cpu.usage", TS: ts, Value: 1},
	}))
	require.NoError(t, s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "cpu.usage", TS: ts, Value: 2},
	}))

	rows, err := s.QuerySamples("n1", "cpu.usage", ts-1, ts+1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.InDelta(t, 2, rows[0].Value, 0.001)
}

// 编译期校验 Store 内部使用 *sql.DB 直连（测试需要访问原始连接做断言）。
var _ = sql.ErrNoRows
