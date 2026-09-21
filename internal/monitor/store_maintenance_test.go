package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// openFileStore 打开文件级指标库（ Vacuum / freelist 断言需要真实文件，
// :memory: 无法观察空间回收）。
func openFileStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metrics.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path + ".db" // OpenStore 会补 .db 后缀
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
func TestVacuum_ReclaimsFreelistAndTruncatesWAL(t *testing.T) {
	s, dbPath := openFileStore(t)
	old := time.Now().UTC().AddDate(0, 0, -40).Unix()

	insertSamples(t, s, 20000, old)

	sum, err := s.Cleanup(30)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if sum.DeletedRows != 20000 {
		t.Fatalf("cleanup deleted %d rows, want 20000", sum.DeletedRows)
	}
	if fc := pragmaInt(t, s, "PRAGMA freelist_count"); fc == 0 {
		t.Fatal("freelist empty after delete; expected released pages waiting for vacuum")
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

// Cleanup 删除过期整月分区表：DROP 的表计入 DroppedTables，剩余表内的
// 过期行计入 DeletedRows。
func TestCleanup_SummaryCountsDroppedTablesAndRows(t *testing.T) {
	s, _ := openFileStore(t)
	old := time.Now().UTC().AddDate(0, 0, -40).Unix()

	// 手工造一张"上上月"的旧分区表 + 当前月表各 100 行
	if err := s.ensureTable("200001"); err != nil {
		t.Fatal(err)
	}
	insertSamples(t, s, 100, old)
	insertSamples(t, s, 100, time.Now().Unix())

	sum, err := s.Cleanup(30)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if sum.DroppedTables != 1 {
		t.Fatalf("dropped %d tables, want 1 (metrics_200001)", sum.DroppedTables)
	}
	if sum.DeletedRows != 100 {
		t.Fatalf("deleted %d rows, want 100 (current-month expired rows)", sum.DeletedRows)
	}
	var n int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='metrics_200001'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("expired partition table still exists")
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
