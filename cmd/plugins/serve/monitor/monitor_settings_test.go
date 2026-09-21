package monitor

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	_ "modernc.org/sqlite"
)

func settingsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY, value TEXT NOT NULL
	)`)
	require.NoError(t, err)
	return db
}

func setSetting(t *testing.T, db *sql.DB, key, value string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	require.NoError(t, err)
}

// 指标保留天数：未设置 = 30（沿用历史默认）；非法/越界收敛到边界。
func TestReadMetricsRetentionDays(t *testing.T) {
	db := settingsDB(t)
	assert.Equal(t, 30, readMetricsRetentionDays(db))

	setSetting(t, db, "monitor.retention_days", "90")
	assert.Equal(t, 90, readMetricsRetentionDays(db))

	setSetting(t, db, "monitor.retention_days", "0")
	assert.Equal(t, 1, readMetricsRetentionDays(db))

	setSetting(t, db, "monitor.retention_days", "99999")
	assert.Equal(t, 3650, readMetricsRetentionDays(db))

	setSetting(t, db, "monitor.retention_days", "abc")
	assert.Equal(t, 30, readMetricsRetentionDays(db))
}

// 清理档位：未设置 = daily；大小写与空白归一；非法回退 daily。
func TestReadCleanupSchedule(t *testing.T) {
	db := settingsDB(t)
	assert.Equal(t, "daily", readCleanupSchedule(db))

	setSetting(t, db, "monitor.cleanup_schedule", " Weekly ")
	assert.Equal(t, "weekly", readCleanupSchedule(db))

	setSetting(t, db, "monitor.cleanup_schedule", "MONTHLY")
	assert.Equal(t, "monthly", readCleanupSchedule(db))

	setSetting(t, db, "monitor.cleanup_schedule", "nope")
	assert.Equal(t, "daily", readCleanupSchedule(db))
}

// 采集间隔秒数：未设置 = 60；下限 10s 防止打爆被采集节点；上限一天。
func TestReadIntervalSeconds(t *testing.T) {
	db := settingsDB(t)
	assert.Equal(t, 60, readIntervalSeconds(db))

	setSetting(t, db, "monitor.interval_seconds", "300")
	assert.Equal(t, 300, readIntervalSeconds(db))

	setSetting(t, db, "monitor.interval_seconds", "5")
	assert.Equal(t, 10, readIntervalSeconds(db))

	setSetting(t, db, "monitor.interval_seconds", "999999")
	assert.Equal(t, 86400, readIntervalSeconds(db))

	setSetting(t, db, "monitor.interval_seconds", "abc")
	assert.Equal(t, 60, readIntervalSeconds(db))
}

// 装配后的引擎配置函数从 settings 表实时读取（运行期可调）。
func TestEngineConfigFuncs_ReadFromSettings(t *testing.T) {
	db := settingsDB(t)
	var cfg owlmonitor.EngineConfig
	applyStorageSettings(db, &cfg)

	assert.Equal(t, 30, cfg.RetentionDaysFn())
	setSetting(t, db, "monitor.retention_days", "14")
	assert.Equal(t, 14, cfg.RetentionDaysFn())

	assert.Equal(t, time.Minute, cfg.IntervalFn())
	setSetting(t, db, "monitor.interval_seconds", "120")
	assert.Equal(t, 2*time.Minute, cfg.IntervalFn())

	assert.Equal(t, "daily", cfg.CleanupScheduleFn())
	setSetting(t, db, "monitor.cleanup_schedule", "weekly")
	assert.Equal(t, "weekly", cfg.CleanupScheduleFn())
}
