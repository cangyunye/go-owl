package history

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

// DB 数据库连接管理
type DB struct {
	conn *sql.DB
	path string
}

var globalDB *DB

// Config 历史记录配置
type Config struct {
	Enabled       bool
	DBPath        string
	RetentionDays int
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	dbPath := filepath.Join(homeDir, ".owl", "history.db")
	
	return &Config{
		Enabled:       true,
		DBPath:        dbPath,
		RetentionDays: 90,
	}
}

// NewDB 创建新的数据库连接
func NewDB(config *Config) (*DB, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ensureDBDir(config.DBPath)

	conn, err := sql.Open("duckdb", config.DBPath)
	if err != nil {
		return nil, err
	}

	db := &DB{
		conn: conn,
		path: config.DBPath,
	}

	if err := db.initSchema(); err != nil {
		return nil, err
	}

	globalDB = db
	return db, nil
}

// GetDB 获取全局DB实例
func GetDB() *DB {
	return globalDB
}

func (db *DB) ensureDBDir() {
	dir := filepath.Dir(db.path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
}

func ensureDBDir(path string) {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
}

func (db *DB) initSchema() error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS operations (
			id BIGINT PRIMARY KEY AUTOINCREMENT,
			task_id VARCHAR,
			op_type VARCHAR,
			command VARCHAR,
			targets JSON,
			status VARCHAR,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_operations_task_id ON operations (task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_operations_op_type ON operations (op_type);`,
		`CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations (created_at);`,
		
		`CREATE TABLE IF NOT EXISTS node_communications (
			id BIGINT PRIMARY KEY AUTOINCREMENT,
			task_id VARCHAR,
			node_id VARCHAR,
			node_address VARCHAR,
			direction VARCHAR,
			message_type VARCHAR,
			payload VARCHAR,
			success BOOLEAN,
			error VARCHAR,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_communications_task_id ON node_communications (task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_communications_node_id ON node_communications (node_id);`,
		`CREATE INDEX IF NOT EXISTS idx_communications_created_at ON node_communications (created_at);`,
		
		`CREATE TABLE IF NOT EXISTS command_executions (
			id BIGINT PRIMARY KEY AUTOINCREMENT,
			task_id VARCHAR,
			node_id VARCHAR,
			command VARCHAR,
			exit_code INTEGER,
			stdout VARCHAR,
			stderr VARCHAR,
			duration_ms INTEGER,
			success BOOLEAN,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_executions_task_id ON command_executions (task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_executions_node_id ON command_executions (node_id);`,
		`CREATE INDEX IF NOT EXISTS idx_executions_created_at ON command_executions (created_at);`,
		
		`CREATE TABLE IF NOT EXISTS file_transfers (
			id BIGINT PRIMARY KEY AUTOINCREMENT,
			task_id VARCHAR,
			node_id VARCHAR,
			file_name VARCHAR,
			file_size BIGINT,
			transfer_type VARCHAR,
			status VARCHAR,
			progress DOUBLE,
			error VARCHAR,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_task_id ON file_transfers (task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_node_id ON file_transfers (node_id);`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_created_at ON file_transfers (created_at);`,
	}

	for _, schema := range schemas {
		_, err := db.conn.Exec(schema)
		if err != nil {
			return err
		}
	}

	return nil
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	return db.conn.Close()
}

// Cleanup 清理过期数据
func (db *DB) Cleanup(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	
	tables := []string{"operations", "node_communications", "command_executions", "file_transfers"}
	for _, table := range tables {
		_, err := db.conn.Exec("DELETE FROM "+table+" WHERE created_at < ?", cutoff)
		if err != nil {
			return err
		}
	}
	
	return nil
}
