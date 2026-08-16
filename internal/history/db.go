package history

import (
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

func ensureDBDir(path string) {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
}
