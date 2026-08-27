package monitor

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store 指标存储：SQLite，按月分区表 metrics_YYYYMM，WAL 模式，批量写入。
// 300 节点 × 60 指标 × 60s 轮询量级下无需独立时序库（见设计文档 5.4）。
type Store struct {
	db *sql.DB
}

// OpenStore 打开（或创建）指标库，启用 WAL 并确保当前月份分区表存在。
func OpenStore(path string) (*Store, error) {
	if filepath.Ext(path) != ".db" {
		path += ".db"
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("monitor: 创建指标库目录失败: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("monitor: 打开指标库失败: %w", err)
	}
	db.SetMaxOpenConns(1) // 单写者，避免 SQLite 锁竞争
	s := &Store{db: db}
	if err := s.ensureTable(monthName(time.Now().Unix())); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.SeedAlertTypesIfEmpty(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.SeedBuiltinRemediesIfEmpty(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.EnsureNotifyTables(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭存储。
func (s *Store) Close() error {
	return s.db.Close()
}

func monthName(ts int64) string {
	return time.Unix(ts, 0).UTC().Format("200601")
}

func tableName(month string) string {
	return "metrics_" + month
}

func (s *Store) ensureTable(month string) error {
	_, err := s.db.Exec(fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			node_id TEXT NOT NULL,
			metric  TEXT NOT NULL,
			ts      INTEGER NOT NULL,
			value   REAL NOT NULL,
			PRIMARY KEY (node_id, metric, ts)
		) WITHOUT ROWID`, tableName(month)))
	return err
}

// InsertSamples 批量写入指标，按采样时间自动分月落表；同主键覆盖。
func (s *Store) InsertSamples(samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}
	// 按月份分组
	byMonth := make(map[string][]Sample)
	for _, smp := range samples {
		m := monthName(smp.TS)
		byMonth[m] = append(byMonth[m], smp)
	}
	for month, group := range byMonth {
		if err := s.insertBatch(month, group); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) insertBatch(month string, samples []Sample) error {
	if err := s.ensureTable(month); err != nil {
		return fmt.Errorf("monitor: 创建分区表 %s 失败: %w", tableName(month), err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("monitor: 开启写入事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(fmt.Sprintf(
		`INSERT OR REPLACE INTO %s (node_id, metric, ts, value) VALUES (?, ?, ?, ?)`, tableName(month)))
	if err != nil {
		return fmt.Errorf("monitor: 准备写入语句失败: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, smp := range samples {
		if _, err := stmt.Exec(smp.NodeID, smp.Metric, smp.TS, smp.Value); err != nil {
			return fmt.Errorf("monitor: 写入指标失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("monitor: 提交写入事务失败: %w", err)
	}
	return nil
}

// QuerySamples 查询指定节点、指标在 [from, to] 区间的采样，按时间升序。
// 自动跨分区合并查询。
func (s *Store) QuerySamples(nodeID, metric string, from, to int64) ([]Sample, error) {
	months := monthsInRange(from, to)
	var result []Sample
	for _, m := range months {
		exists, err := s.tableExists(tableName(m))
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		rows, err := s.db.Query(fmt.Sprintf(
			`SELECT node_id, metric, ts, value FROM %s WHERE node_id=? AND metric=? AND ts BETWEEN ? AND ?`,
			tableName(m)), nodeID, metric, from, to)
		if err != nil {
			return nil, fmt.Errorf("monitor: 查询分区表 %s 失败: %w", tableName(m), err)
		}
		for rows.Next() {
			var smp Sample
			if err := rows.Scan(&smp.NodeID, &smp.Metric, &smp.TS, &smp.Value); err != nil {
				_ = rows.Close()
				return nil, err
			}
			result = append(result, smp)
		}
		_ = rows.Close()
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].TS != result[j].TS {
			return result[i].TS < result[j].TS
		}
		return result[i].Metric < result[j].Metric
	})
	return result, nil
}

func monthsInRange(from, to int64) []string {
	start := time.Unix(from, 0).UTC()
	end := time.Unix(to, 0).UTC()
	cur := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	endMonth := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	var months []string
	// 防御：范围上限 130 年（1970→2100），防止异常边界导致天文数字迭代
	const maxMonths = 1560
	for !cur.After(endMonth) && len(months) < maxMonths {
		months = append(months, cur.Format("200601"))
		cur = cur.AddDate(0, 1, 0)
	}
	return months
}

func (s *Store) tableExists(name string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return n > 0, err
}

// Cleanup 清理过期数据：删除各分区表中早于 cutoff 的行，并 DROP
// 整体早于 cutoff 所在月份的旧分区表。幂等，可每日调用。
func (s *Store) Cleanup(retentionDays int) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	cutoffUnix := cutoff.Unix()

	tables, err := s.listMetricTables()
	if err != nil {
		return err
	}
	for _, t := range tables {
		month := strings.TrimPrefix(t, "metrics_")
		// 整表早于保留期 → 直接 DROP
		if monthCutoff, perr := time.Parse("200601", month); perr == nil &&
			monthCutoff.Before(cutoffMonth(cutoff)) {
			if _, derr := s.db.Exec("DROP TABLE IF EXISTS " + t); derr != nil {
				return fmt.Errorf("monitor: 删除过期分区表 %s 失败: %w", t, derr)
			}
			continue
		}
		if _, derr := s.db.Exec("DELETE FROM "+t+" WHERE ts < ?", cutoffUnix); derr != nil {
			return fmt.Errorf("monitor: 清理分区表 %s 过期行失败: %w", t, derr)
		}
	}
	return nil
}

func cutoffMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func (s *Store) listMetricTables() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'metrics\_%' ESCAPE '\'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}
