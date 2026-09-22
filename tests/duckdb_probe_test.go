//go:build duckdb
// +build duckdb

// 存储引擎选型探针：metrics 工作负载下 SQLite（internal/monitor.Store，生产实现）
// vs DuckDB（候选方案）。同规模数据（8 节点 × 10 指标 × 10 天 × 60s ≈ 115 万行，
// 保留期 3 天 → 清理 70%），验证三件事：
//  1. 空间占用：同规模采样落盘大小（列存压缩收益）
//  2. 膨胀回收：DELETE 过期数据后 VACUUM / CHECKPOINT 能否真正缩小文件
//  3. 查询性能：仪表盘点查（node+metric 最近 1h）与聚合（全节点按小时均值）
//
// 运行：go test -tags duckdb ./tests/ -run TestProbeStorage -v -timeout 30m
package integration

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/monitor"

	_ "github.com/duckdb/duckdb-go/v2"
	_ "modernc.org/sqlite"
)

const (
	probeNodes     = 8
	probeMetrics   = 10
	probeDays      = 10 // 数据跨度 10 天
	probeEpochs    = probeDays * 24 * 60
	probeRows      = probeNodes * probeMetrics * probeEpochs // 1,152,000
	probeRetention = 3                                       // 保留 3 天 → 清理 70%
)

func fileSizeBytes(paths ...string) int64 {
	var total int64
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil {
			total += st.Size()
		}
	}
	return total
}

func mb(n int64) string { return fmt.Sprintf("%.1fMB", float64(n)/1024/1024) }

func dur(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

func monthOf(ts int64) string { return time.Unix(ts, 0).UTC().Format("200601") }

func measurePointQuery(t *testing.T, db *sql.DB, table string) time.Duration {
	t.Helper()
	q := fmt.Sprintf(
		`SELECT ts, value FROM %s WHERE node_id='n3' AND metric='m5' AND ts BETWEEN ? AND ? ORDER BY ts`,
		table)
	start := time.Now()
	for i := 0; i < 10; i++ {
		rows, err := db.Query(q, base()+int64(probeDays*86400)-3600, base()+int64(probeDays*86400))
		if err != nil {
			t.Fatalf("point query: %v", err)
		}
		n := 0
		for rows.Next() {
			var ts int64
			var v float64
			if err := rows.Scan(&ts, &v); err != nil {
				t.Fatal(err)
			}
			n++
		}
		rows.Close()
		if n != 60 {
			t.Fatalf("point query rows = %d, want 60", n)
		}
	}
	return time.Since(start) / 10
}

func measureAggregate(t *testing.T, db *sql.DB, table string) (time.Duration, int) {
	t.Helper()
	q := fmt.Sprintf(
		`SELECT (ts/3600)*3600 AS bucket, AVG(value) FROM %s WHERE ts BETWEEN ? AND ? GROUP BY 1 ORDER BY 1`,
		table)
	start := time.Now()
	buckets := 0
	rows, err := db.Query(q, base()+int64((probeDays-probeRetention)*86400), base()+int64(probeDays*86400))
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	for rows.Next() {
		var b, v any // bucket 类型因引擎而异（sqlite 整除=int64，duckdb 浮除=float64）
		if err := rows.Scan(&b, &v); err != nil {
			t.Fatal(err)
		}
		buckets++
	}
	rows.Close()
	return time.Since(start), buckets
}

var baseTS = time.Now().AddDate(0, 0, -probeDays).Unix()

func base() int64 { return baseTS }

func TestProbeStorageSQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m_sqlite.db") // 扩展名已是 .db，OpenStore 不再追加
	store, err := monitor.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	start := time.Now()
	const chunk = 5000
	for done := 0; done < probeRows; done += chunk {
		n := chunk
		if probeRows-done < chunk {
			n = probeRows - done
		}
		samples := make([]monitor.Sample, 0, n)
		for i := done; i < done+n; i++ {
			samples = append(samples, monitor.Sample{
				NodeID: fmt.Sprintf("n%d", i%probeNodes),
				Metric: fmt.Sprintf("m%d", (i/probeNodes)%probeMetrics),
				TS:     base() + int64(i/(probeNodes*probeMetrics))*60,
				Value:  float64(i%100) + 0.5,
			})
		}
		if err := store.InsertSamples(samples); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	insertDur := time.Since(start)
	sizeInsert := fileSizeBytes(dbPath, dbPath+"-wal")

	delStart := time.Now()
	if _, err := store.Cleanup(probeRetention); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	delDur := time.Since(delStart)
	sizeDeleted := fileSizeBytes(dbPath, dbPath+"-wal")

	vacStart := time.Now()
	if err := store.Vacuum(); err != nil {
		t.Fatalf("vacuum: %v", err)
	}
	vacDur := time.Since(vacStart)
	sizeVac := fileSizeBytes(dbPath, dbPath+"-wal")

	// 只读连接做查询测量（生产查询走 QuerySamples，点查语义等价）
	reader, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	pointDur := measurePointQuery(t, reader, "metrics_"+monthOf(time.Now().Unix()))
	aggDur, buckets := measureAggregate(t, reader, "metrics_"+monthOf(time.Now().Unix()))

	t.Logf("RESULT sqlite: insert=%s (%.0f rows/s) | size insert=%s delete=%s vacuum=%s | delete=%s vacuum=%s | point_1h=%s agg_hourly=%s(%d buckets)",
		dur(insertDur), float64(probeRows)/insertDur.Seconds(),
		mb(sizeInsert), mb(sizeDeleted), mb(sizeVac),
		dur(delDur), dur(vacDur), pointDur, aggDur, buckets)
}

func TestProbeStorageDuckDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m_duck.db")

	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE metrics (
		node_id VARCHAR NOT NULL,
		metric  VARCHAR NOT NULL,
		ts      BIGINT NOT NULL,
		value   DOUBLE NOT NULL,
		PRIMARY KEY (node_id, metric, ts)
	)`); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if _, err := db.Exec(fmt.Sprintf(`
		INSERT INTO metrics
		SELECT 'n' || (i %% %d),
		       'm' || ((i // %d) %% %d),
		       %d + (i // %d) * 60,
		       (i %% 100) + 0.5
		FROM range(%d) t(i)`,
		probeNodes, probeNodes, probeMetrics,
		base(), probeNodes*probeMetrics, probeRows)); err != nil {
		t.Fatalf("insert: %v", err)
	}
	insertDur := time.Since(start)
	sizeInsert := fileSizeBytes(dbPath, dbPath+".wal")

	var inserted int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM metrics`).Scan(&inserted); err != nil {
		t.Fatal(err)
	}
	if inserted != probeRows {
		t.Fatalf("inserted %d rows, want %d", inserted, probeRows)
	}

	delStart := time.Now()
	cutoff := base() + int64((probeDays-probeRetention)*86400)
	if _, err := db.Exec(`DELETE FROM metrics WHERE ts < ?`, cutoff); err != nil {
		t.Fatalf("delete: %v", err)
	}
	delDur := time.Since(delStart)
	sizeDeleted := fileSizeBytes(dbPath, dbPath+".wal")

	var remaining int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM metrics`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	t.Logf("duckdb debug: inserted=%d remaining_after_delete=%d (want ~%d = 30%% retained)",
		inserted, remaining, probeRows*probeRetention/probeDays)

	cpStart := time.Now()
	if _, err := db.Exec(`CHECKPOINT`); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	cpDur := time.Since(cpStart)
	sizeCP := fileSizeBytes(dbPath, dbPath+".wal")

	// 兜底：VACUUM + 再 CHECKPOINT（部分版本压缩只在此路径释放）
	vacStart := time.Now()
	if _, err := db.Exec(`VACUUM`); err == nil {
		if _, err := db.Exec(`CHECKPOINT`); err == nil {
			t.Logf("duckdb: VACUUM+CHECKPOINT ok in %s", dur(time.Since(vacStart)))
		}
	}
	sizeVac := fileSizeBytes(dbPath, dbPath+".wal")

	pointDur := measurePointQuery(t, db, "metrics")
	aggDur, buckets := measureAggregate(t, db, "metrics")

	t.Logf("RESULT duckdb: insert=%s (%.0f rows/s) | size insert=%s delete=%s checkpoint=%s post_vacuum=%s | delete=%s checkpoint=%s | point_1h=%s agg_hourly=%s(%d buckets)",
		dur(insertDur), float64(probeRows)/insertDur.Seconds(),
		mb(sizeInsert), mb(sizeDeleted), mb(sizeCP), mb(sizeVac),
		dur(delDur), dur(cpDur), pointDur, aggDur, buckets)
}
