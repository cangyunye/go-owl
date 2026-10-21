package monitor

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMetricsDBPath 验证独立指标库路径推导：同目录加 .metrics 后缀。
func TestMetricsDBPath(t *testing.T) {
	require.Equal(t, "/home/x/.owl/owl.metrics.db", MetricsDBPath("/home/x/.owl/owl.db"))
	require.Equal(t, "/home/x/.owl/owl.metrics.db", MetricsDBPath("/home/x/.owl/owl"))
	// 已带 .metrics.db 的路径不会重复叠加
	require.Equal(t, "/home/x/.owl/owl.metrics.db", MetricsDBPath("/home/x/.owl/owl.metrics.db"))
}

// TestStore_OpenAndQuery 验证按天分区存储：写入后可按时段查询。
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

// TestStore_InsertGroupsByDay 验证跨天批量写入自动落到对应分区表，
// 跨天查询合并返回。
func TestStore_InsertGroupsByDay(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "monitor.db"))
	require.NoError(t, err)
	defer s.Close()

	// 昨天末尾与今天开头各一条
	endOfPrevDay := time.Now().UTC().Truncate(24 * time.Hour).Add(-time.Minute)
	startOfThisDay := endOfPrevDay.Add(2 * time.Minute)
	err = s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "mem.used_pct", TS: endOfPrevDay.Unix(), Value: 10},
		{NodeID: "n1", Metric: "mem.used_pct", TS: startOfThisDay.Unix(), Value: 20},
	})
	require.NoError(t, err)

	rows, err := s.QuerySamples("n1", "mem.used_pct",
		endOfPrevDay.Unix()-1, startOfThisDay.Unix()+1)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.InDelta(t, 10, rows[0].Value, 0.001)
	require.InDelta(t, 20, rows[1].Value, 0.001)

	// 两个不同的天分区表都存在
	var n int
	require.NoError(t, s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN (?, ?)`,
		tableName(dayName(endOfPrevDay.Unix())), tableName(dayName(startOfThisDay.Unix()))).Scan(&n))
	require.Equal(t, 2, n)
}

// TestStore_Cleanup 验证清理任务：整天过期的分区表直接 DROP，
// 保留期内（含保留边界当天之前）的数据不受影响。
func TestStore_Cleanup(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "monitor.db"))
	require.NoError(t, err)
	defer s.Close()

	now := time.Now().UTC()
	oldTS := now.AddDate(0, 0, -40).Unix()   // 40 天前 → 整天过期 → DROP
	recentTS := now.AddDate(0, 0, -1).Unix() // 1 天前 → 保留

	// 构造一个必然早于保留期的旧分区表，模拟历史遗留
	oldDay := "20240101"
	_, err = s.db.Exec(fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS metrics_%s (node_id TEXT, metric TEXT, ts INTEGER, value REAL, PRIMARY KEY (node_id, metric, ts))`, oldDay))
	require.NoError(t, err)
	oldDayTS := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Unix()

	err = s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "load.load1", TS: oldTS, Value: 9},
		{NodeID: "n1", Metric: "load.load1", TS: recentTS, Value: 1},
		{NodeID: "n1", Metric: "load.load1", TS: oldDayTS, Value: 7},
	})
	require.NoError(t, err)

	// 全部落库确认
	all, err := s.QuerySamples("n1", "load.load1", 0, now.AddDate(0, 0, 1).Unix())
	require.NoError(t, err)
	require.Len(t, all, 3)

	sum, err := s.Cleanup(30)
	require.NoError(t, err)
	require.GreaterOrEqual(t, sum.DroppedTables, 2, "40 天前与 2024-01 的天分区表都应被 DROP")

	// 过期数据随分区表整体消失
	kept, err := s.QuerySamples("n1", "load.load1", 0, now.AddDate(0, 0, 1).Unix())
	require.NoError(t, err)
	require.Len(t, kept, 1)
	require.InDelta(t, 1, kept[0].Value, 0.001)

	// 旧分区表被 DROP
	var n int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='metrics_20240101'`).Scan(&n)
	require.NoError(t, err)
	require.Equal(t, 0, n, "早于保留期的分区表应被删除")
}

// TestStore_Cleanup_Idempotent 验证重复清理不报错。
func TestStore_Cleanup_Idempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "monitor.db"))
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Cleanup(30)
	require.NoError(t, err)
	_, err = s.Cleanup(30)
	require.NoError(t, err)
}

// TestStore_Cleanup_KeepsUnknownTables 命名不符合 YYYYMMDD 的 metrics_* 表
// 不被误删（防御外部造表）。
func TestStore_Cleanup_KeepsUnknownTables(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "monitor.db"))
	require.NoError(t, err)
	defer s.Close()

	_, err = s.db.Exec(`CREATE TABLE metrics_weird (ts INTEGER)`)
	require.NoError(t, err)

	_, err = s.Cleanup(30)
	require.NoError(t, err)

	var n int
	require.NoError(t, s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='metrics_weird'`).Scan(&n))
	require.Equal(t, 1, n)
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

// TestStore_LateSampleAfterDrop 迟到的旧采样在分区表被清理 DROP 之后
// 写入时能重建该表（缓存失效），不丢数据也不报错。
func TestStore_LateSampleAfterDrop(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "monitor.db"))
	require.NoError(t, err)
	defer s.Close()

	old := time.Now().UTC().AddDate(0, 0, -40)
	require.NoError(t, s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "cpu.usage", TS: old.Unix(), Value: 1},
	}))

	// 清理把 40 天前的天分区 DROP 掉（并失效缓存）
	sum, err := s.Cleanup(30)
	require.NoError(t, err)
	require.GreaterOrEqual(t, sum.DroppedTables, 1)

	// 同一天的迟到采样：需重建表并写入成功
	require.NoError(t, s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "cpu.usage", TS: old.Add(time.Minute).Unix(), Value: 2},
	}))
	rows, err := s.QuerySamples("n1", "cpu.usage", old.Unix()-1, old.Add(time.Hour).Unix())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.InDelta(t, 2, rows[0].Value, 0.001)
}

// 编译期校验 Store 内部使用 *sql.DB 直连（测试需要访问原始连接做断言）。
var _ = sql.ErrNoRows

// TestStore_ConcurrentQueries 连接池不能是单连接：告警中心等 HTTP 读与
// 引擎采集写入共用本 store，一条长事务占住唯一连接时，其它查询会被
// 串行化卡死（首次打开告警页转圈）。WAL 模式支持多读 + 单写，
// 池内应允许并发连接，写-写竞争由 busy_timeout 兜底。
func TestStore_ConcurrentQueries(t *testing.T) {
	s, _ := openFileStore(t)

	insertSamples(t, s, 10, time.Now().Unix())

	// 占住一条连接：结果集不消费、不关闭
	held, err := s.db.Query(`SELECT node_id FROM ` + "metrics_" + dayName(time.Now().Unix()))
	require.NoError(t, err)
	defer held.Close()

	type qResult struct {
		n   int
		err error
	}
	done := make(chan qResult, 1)
	go func() {
		rows, qerr := s.QuerySamples("n1", "cpu.usage", 0, 1<<62)
		if qerr != nil {
			done <- qResult{err: qerr}
			return
		}
		done <- qResult{n: len(rows)}
	}()

	select {
	case r := <-done:
		require.NoError(t, r.err)
		require.Equal(t, 10, r.n)
	case <-time.After(2 * time.Second):
		t.Fatal("第二条查询在长事务占用期间被阻塞：连接池仍是单连接串行")
	}
}

// TestDropLegacyMetricTables 旧版布局遗留在主库的 metrics_YYYYMMDD /
// metrics_YYYYMM 分区表被清除，其它表不受影响。
func TestDropLegacyMetricTables(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "main.db"))
	require.NoError(t, err)
	defer db.Close()

	for _, ddl := range []string{
		`CREATE TABLE nodes (id TEXT PRIMARY KEY)`,
		`CREATE TABLE metrics_20240101 (ts INTEGER)`, // 旧天分区
		`CREATE TABLE metrics_202401 (ts INTEGER)`,   // 更旧的月分区（6 位后缀）
		`CREATE TABLE metrics_20990101 (ts INTEGER)`, // 未来日期：同样按表清（死数据）
		`CREATE TABLE metrics_weird (ts INTEGER)`,    // 无法解析：保留
		`CREATE TABLE alert_types (id TEXT PRIMARY KEY)`,
	} {
		_, err := db.Exec(ddl)
		require.NoError(t, err)
	}

	dropped, err := DropLegacyMetricTables(db)
	require.NoError(t, err)
	require.Equal(t, 3, dropped, "应删除两个可解析日期的天分区与一个月分区")

	var names []string
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	require.NoError(t, err)
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		names = append(names, n)
	}
	require.NoError(t, rows.Close())
	require.ElementsMatch(t, []string{"nodes", "metrics_weird", "alert_types"}, names)
}
