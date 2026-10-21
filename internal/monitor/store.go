package monitor

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store 指标存储：独立 SQLite 文件（与应用主库 owl.db 分离），按**天**分区
// 表 metrics_YYYYMMDD，WAL 模式，批量写入。
// 过期数据按"整天过期 → DROP TABLE"回收，不再对大表执行全表扫描的
// `DELETE WHERE ts < ?`——按月分区时代，数百节点规模下该语句会持写锁
// 扫表分钟级，并连带整库 VACUUM 拖垮同库的节点/用户/历史写入。
// 见设计文档 04_MONITORING_ALERTING.md 5.4。
type Store struct {
	db *sql.DB
	// created 缓存已确认存在的分区表。每轮每节点各 ensureTable 一次会在
	// 数百节点下产生每分钟数百次并发 DDL，schema 锁冲突会绕过 busy_timeout
	// 立即返回 SQLITE_BUSY（实测 ~4ms 失败），丢指标；缓存后每张表只建一次。
	mu      sync.Mutex
	created map[string]bool
	// nodeGroups 分组 → 命中节点 ID 的跨库解析器（serve 装配时基于主库
	// nodes 表注入；nil = 未注入，告警分组筛选回退同库 nodes 子查询）。
	nodeGroups func(groups []string) ([]string, error)
}

// MetricsDBPath 由主库路径推导独立指标库路径：同目录、加 .metrics 后缀。
// 如 ~/.owl/owl.db → ~/.owl/owl.metrics.db；自定义 OWL_DB_PATH 时指标库
// 跟随主库位置，避免多部署互相覆盖。已是 .metrics.db 结尾的路径原样返回。
func MetricsDBPath(mainPath string) string {
	p := mainPath
	if filepath.Ext(p) != ".db" {
		p += ".db"
	}
	if strings.HasSuffix(p, ".metrics.db") {
		return p
	}
	return strings.TrimSuffix(p, ".db") + ".metrics.db"
}

// OpenStore 打开（或创建）独立指标库，启用 WAL 并确保当天的分区表存在。
// 入参是主库路径（owl.db），指标实际落在 MetricsDBPath(mainPath)。
func OpenStore(mainPath string) (*Store, error) {
	path := MetricsDBPath(mainPath)
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("monitor: 创建指标库目录失败: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=15000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("monitor: 打开指标库失败: %w", err)
	}
	// WAL 支持多读 + 单写：池内保留少量并发连接，让告警中心等 HTTP 读
	// 不被引擎采集写入/清理事务串行阻塞；写-写竞争由 DSN busy_timeout 兜底。
	// 之前 MaxOpenConns(1) 时，一条长事务会卡住告警页的全部请求。
	db.SetMaxOpenConns(4)
	s := &Store{db: db, created: map[string]bool{}}
	if err := s.ensureTable(dayName(time.Now().Unix())); err != nil {
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
	if err := s.EnsureRemedyRunTables(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// NewStoreFromDB 复用现有 *sql.DB 构造 Store，不执行任何建表或种子初始化。
// 供测试注入备选驱动（如 duckdb 兼容验证）；生产路径请使用 OpenStore。
func NewStoreFromDB(db *sql.DB) *Store {
	return &Store{db: db, created: map[string]bool{}}
}

// Close 关闭存储。
func (s *Store) Close() error {
	return s.db.Close()
}

func dayName(ts int64) string {
	return time.Unix(ts, 0).UTC().Format("20060102")
}

// dayNameClamped 与 dayName 等价，但对越界时间戳收敛到可表示范围：
// 调用方常用 0 / 1<<62 这类"全历史/到永远"的边界，直接 Format 会产生
// 无法与正常日期字符串比较的年份（如 "+292277..."），导致区间过滤失效。
func dayNameClamped(ts int64) string {
	const maxTS = int64(253402300799) // 9999-12-31 23:59:59 UTC
	if ts < 0 {
		return dayName(0)
	}
	if ts > maxTS {
		return dayName(maxTS)
	}
	return dayName(ts)
}

func tableName(day string) string {
	return "metrics_" + day
}

func (s *Store) ensureTable(day string) error {
	s.mu.Lock()
	if s.created[day] {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	if _, err := s.db.Exec(fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			node_id TEXT NOT NULL,
			metric  TEXT NOT NULL,
			ts      INTEGER NOT NULL,
			value   REAL NOT NULL,
			PRIMARY KEY (node_id, metric, ts)
		) WITHOUT ROWID`, tableName(day))); err != nil {
		return err
	}
	s.mu.Lock()
	s.created[day] = true
	s.mu.Unlock()
	return nil
}

// forgetTable 分区表被 DROP 后失效缓存，迟到的旧采样需要能重建该表。
func (s *Store) forgetTable(day string) {
	s.mu.Lock()
	delete(s.created, day)
	s.mu.Unlock()
}

// InsertSamples 批量写入指标，按采样时间自动按天落表；同主键覆盖。
func (s *Store) InsertSamples(samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}
	// 按天分组
	byDay := make(map[string][]Sample)
	for _, smp := range samples {
		d := dayName(smp.TS)
		byDay[d] = append(byDay[d], smp)
	}
	for day, group := range byDay {
		if err := s.insertBatch(day, group); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) insertBatch(day string, samples []Sample) error {
	if err := s.ensureTable(day); err != nil {
		return fmt.Errorf("monitor: 创建分区表 %s 失败: %w", tableName(day), err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("monitor: 开启写入事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(fmt.Sprintf(
		`INSERT OR REPLACE INTO %s (node_id, metric, ts, value) VALUES (?, ?, ?, ?)`, tableName(day)))
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
// 表名即日期（metrics_YYYYMMDD），先列出实际存在的分区再按区间过滤，
// 天然支持任意大/久远的时间范围，无需逐日探测表是否存在。
func (s *Store) QuerySamples(nodeID, metric string, from, to int64) ([]Sample, error) {
	tables, err := s.listMetricTables()
	if err != nil {
		return nil, err
	}
	sort.Strings(tables)
	fromDay, toDay := dayNameClamped(from), dayNameClamped(to)

	var result []Sample
	for _, t := range tables {
		day := strings.TrimPrefix(t, "metrics_")
		if len(day) != 8 || day < fromDay || day > toDay {
			continue
		}
		rows, err := s.db.Query(fmt.Sprintf(
			`SELECT node_id, metric, ts, value FROM %s WHERE node_id=? AND metric=? AND ts BETWEEN ? AND ?`,
			t), nodeID, metric, from, to)
		if err != nil {
			return nil, fmt.Errorf("monitor: 查询分区表 %s 失败: %w", t, err)
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

func (s *Store) tableExists(name string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return n > 0, err
}

// CleanupSummary 一次保留期清理的回收量统计。
type CleanupSummary struct {
	DroppedTables int   // 整体过期的分区表张数（DROP）
	DeletedRows   int64 // 兼容字段：按天分区后过期数据随整表 DROP 回收，不再逐行删除，恒为 0
}

// Cleanup 清理过期数据：整体早于保留期的**天分区表直接 DROP**（毫秒级）。
// 按天分区后不存在"表内部分行过期"，无需逐行 DELETE。幂等，可每日调用。
// DROP 只把页移入 freelist，文件并不缩小；需要随后调用 Vacuum 才会把
// 空间归还操作系统（本库独立于 owl.db，VACUUM 不影响业务写入）。
func (s *Store) Cleanup(retentionDays int) (CleanupSummary, error) {
	var sum CleanupSummary
	cutoffDay := time.Now().UTC().AddDate(0, 0, -retentionDays).Format("20060102")

	tables, err := s.listMetricTables()
	if err != nil {
		return sum, err
	}
	for _, t := range tables {
		day := strings.TrimPrefix(t, "metrics_")
		// 非 YYYYMMDD 命名的表不认识，跳过不误删
		if len(day) != 8 {
			continue
		}
		if _, perr := time.Parse("20060102", day); perr != nil {
			continue
		}
		if day < cutoffDay {
			if _, derr := s.db.Exec("DROP TABLE IF EXISTS " + t); derr != nil {
				return sum, fmt.Errorf("monitor: 删除过期分区表 %s 失败: %w", t, derr)
			}
			s.forgetTable(day)
			sum.DroppedTables++
		}
	}
	return sum, nil
}

// Vacuum 空间回收：重建数据库文件（VACUUM）并把 WAL 截断为零。
// 本库独立于 owl.db：VACUUM 只影响指标读写，不会卡住节点/用户/历史等
// 业务写入；建议低峰期调用。
func (s *Store) Vacuum() error {
	if _, err := s.db.Exec("VACUUM"); err != nil {
		return fmt.Errorf("monitor: VACUUM 失败: %w", err)
	}
	if _, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("monitor: WAL checkpoint 失败: %w", err)
	}
	return nil
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

// DropLegacyMetricTables 删除旧版布局遗留在**主库**里的 metrics_* 分区表
// （旧天分区 metrics_YYYYMMDD 与旧月分区 metrics_YYYYMM 都认）。
// 指标存储迁移到独立 metrics 库后，owl.db 里的历史指标表成为死数据
// （可再生的监控采样，保留期本就有限），服务启动时自动清除；
// 释放的空间需在设置页执行一次 VACUUM 才会归还操作系统。
// 返回删除的表数量。无法解析日期后缀的表不动。
func DropLegacyMetricTables(db *sql.DB) (int, error) {
	rows, err := db.Query(
		`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'metrics\_%' ESCAPE '\'`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return 0, err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	isLegacyPartition := func(name string) bool {
		suffix := strings.TrimPrefix(name, "metrics_")
		switch len(suffix) {
		case 8: // 旧天分区
			_, err := time.Parse("20060102", suffix)
			return err == nil
		case 6: // 旧月分区
			_, err := time.Parse("200601", suffix)
			return err == nil
		default:
			return false
		}
	}

	dropped := 0
	for _, t := range tables {
		if !isLegacyPartition(t) {
			continue
		}
		if _, derr := db.Exec("DROP TABLE IF EXISTS " + t); derr != nil {
			return dropped, derr
		}
		dropped++
	}
	return dropped, nil
}
