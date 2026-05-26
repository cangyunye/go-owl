package history

import (
	"database/sql"
	"os"
	"path/filepath"
)

// DB 是数据库接口的包装器，保持向后兼容
type DB struct {
	impl DBInterface
}

// Config 历史记录配置
type Config struct {
	Enabled       bool
	DBPath        string
	RetentionDays int
}

const (
	envDBPath = "OWL_DB_PATH"
)

// DefaultConfig 默认配置
// 可通过环境变量 OWL_DB_PATH 指定数据库路径，如: OWL_DB_PATH=/path/to/custom.db
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	dbPath := filepath.Join(homeDir, ".owl", "owl.db")

	if envPath := os.Getenv(envDBPath); envPath != "" {
		dbPath = envPath
	}

	return &Config{
		Enabled:       true,
		DBPath:        dbPath,
		RetentionDays: 90,
	}
}

// Connection 获取底层连接
func (d *DB) Connection() *sql.DB {
	if d == nil || d.impl == nil {
		return nil
	}
	return d.impl.Connection()
}

// Close 关闭连接
func (d *DB) Close() error {
	if d == nil || d.impl == nil {
		return nil
	}
	return d.impl.Close()
}

// Cleanup 清理过期数据
func (d *DB) Cleanup(retentionDays int) error {
	if d == nil || d.impl == nil {
		return nil
	}
	return d.impl.Cleanup(retentionDays)
}

func ensureDBDir(path string) {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
}
