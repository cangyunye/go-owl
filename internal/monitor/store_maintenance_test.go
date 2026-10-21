package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// openFileStore 打开文件级指标库（Vacuum / freelist 断言需要真实文件，
// :memory: 无法观察空间回收）。第二个返回值是实际落盘的 metrics 库路径
// （OpenStore 从主库路径推导，落盘文件带 .metrics 后缀）。
func openFileStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "main.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, MetricsDBPath(path)
}

func insertSamples(t *testing.T, s *Store, count int, ts int64) {
	t.Helper()
	samples := make([]Sample, count)
	for i := 0; i < count; i++ {
		samples[i] = Sample{NodeID: "n1", Metric: "cpu.usage", TS: ts + int64(i), Value: float64(i)}
	}
	if err := s.InsertSamples(samples); err != nil {
		t.Fatalf("insert samples: %v", err)
	}
}

func pragmaInt(t *testing.T, s *Store, pragma string) int64 {
	t.Helper()
	var v int64
	if err := s.db.QueryRow(pragma).Scan(&v); err != nil {
		t.Fatalf("pragma %s: %v", pragma, err)
	}
	return v
}

// 过期数据清理后 SQLite 仅将页移入 freelist，文件不缩；Vacuum 后
// freelist 清零、WAL 截断，空间才真正归还操作系统。
// 按天分区后，过期数据随整天分区 DROP 回收（不再逐行 DELETE）。
func TestVacuum_ReclaimsFreelistAndTruncatesWAL(t *testing.T) {
	s, dbPath := openFileStore(t)
	// 40 天前的一整天：保留期 30 天 → 整张天分区表过期 DROP
	old := time.Now().UTC().AddDate(0, 0, -40).Truncate(24 * time.Hour)

	insertSamples(t, s, 20000, old.Unix())

	sum, err := s.Cleanup(30)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if sum.DroppedTables < 1 {
		t.Fatalf("dropped %d tables, want >= 1 (expired day partition)", sum.DroppedTables)
	}
	if fc := pragmaInt(t, s, "PRAGMA freelist_count"); fc == 0 {
		t.Fatal("freelist empty after drop; expected released pages waiting for vacuum")
	}

	if err := s.Vacuum(); err != nil {
		t.Fatalf("vacuum: %v", err)
	}
	if fc := pragmaInt(t, s, "PRAGMA freelist_count"); fc != 0 {
		t.Fatalf("freelist_count = %d after vacuum, want 0", fc)
	}
	walInfo, err := os.Stat(dbPath + "-wal")
	if err == nil && walInfo.Size() > 0 {
		// checkpoint(TRUNCATE) 后 WAL 应为 0 字节
		if walInfo.Size() != 0 {
			t.Fatalf("wal size = %d after vacuum, want 0", walInfo.Size())
		}
	}
}

// Cleanup 删除过期天分区表：DROP 的表计入 DroppedTables；保留期内的
// 数据（含刚过期的前一天）不受影响；逐行删除分支已不存在（DeletedRows 恒 0）。
func TestCleanup_SummaryCountsDroppedTablesAndRows(t *testing.T) {
	s, _ := openFileStore(t)

	// 手工造一张远古分区表（metrics_20000101，必然整天过期 → DROP），
	// 10 天前（保留期内）与当前各 100 条
	if err := s.ensureTable("20000101"); err != nil {
		t.Fatal(err)
	}
	recent := time.Now().UTC().AddDate(0, 0, -10)
	insertSamples(t, s, 100, recent.Unix())
	insertSamples(t, s, 100, time.Now().Unix())

	sum, err := s.Cleanup(30)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if sum.DroppedTables != 1 {
		t.Fatalf("dropped %d tables, want 1 (metrics_20000101)", sum.DroppedTables)
	}
	if sum.DeletedRows != 0 {
		t.Fatalf("deleted %d rows, want 0 (按天分区不再逐行删除)", sum.DeletedRows)
	}
	var n int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='metrics_20000101'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("expired partition table still exists")
	}
	kept, err := s.QuerySamples("n1", "cpu.usage", 0, 1<<62)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 200 {
		t.Fatalf("kept rows = %d, want 200 (保留期内数据不受清理影响)", len(kept))
	}
}

// Vacuum 期间及之后存储仍可正常读写（单写者连接复用）。
func TestVacuum_StoreStillUsable(t *testing.T) {
	s, _ := openFileStore(t)
	now := time.Now().Unix()
	insertSamples(t, s, 100, now)
	if err := s.Vacuum(); err != nil {
		t.Fatalf("vacuum: %v", err)
	}
	insertSamples(t, s, 100, now+1000)
	got, err := s.QuerySamples("n1", "cpu.usage", now, now+2000)
	if err != nil {
		t.Fatalf("query after vacuum: %v", err)
	}
	if len(got) != 200 {
		t.Fatalf("rows after vacuum = %d, want 200", len(got))
	}
}
