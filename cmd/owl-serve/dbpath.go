package main

import (
	"os"
	"path/filepath"
)

// resolveDBPath 返回数据库路径：优先使用 OWL_DB_PATH 环境变量，否则
// 回退到 ~/.owl/owl.db。与 CLI 历史库（internal/history）保持一致。
func resolveDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	dbPath := filepath.Join(home, ".owl", "owl.db")
	if envPath := os.Getenv("OWL_DB_PATH"); envPath != "" {
		dbPath = envPath
	}
	return dbPath
}
