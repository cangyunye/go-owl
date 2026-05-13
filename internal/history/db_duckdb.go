//go:build !sqlite3
// +build !sqlite3

package history

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

var _ DBInterface = (*DuckDB)(nil)

// DuckDB DuckDB 实现
type DuckDB struct {
	conn *sql.DB
	path string
}

// NewDB 创建 DuckDB 数据库连接（默认实现）
func NewDB(config *Config) (DBInterface, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ensureDBDir(config.DBPath)

	conn, err := sql.Open("duckdb", config.DBPath)
	if err != nil {
		return nil, err
	}

	db := &DuckDB{
		conn: conn,
		path: config.DBPath,
	}

	if err := db.InitSchema(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	globalDB = db
	return db, nil
}

// Connection 获取底层连接
func (d *DuckDB) Connection() *sql.DB {
	return d.conn
}

// InitSchema 初始化表结构
func (d *DuckDB) InitSchema() error {
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

		`CREATE TABLE IF NOT EXISTS sessions (
			id VARCHAR PRIMARY KEY,
			mode VARCHAR,
			node_ids JSON,
			status VARCHAR,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			closed_at TIMESTAMP,
			command_count INTEGER DEFAULT 0,
			success_count INTEGER DEFAULT 0,
			error_count INTEGER DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions (status);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions (created_at);`,

		`CREATE TABLE IF NOT EXISTS session_commands (
			id BIGINT PRIMARY KEY AUTOINCREMENT,
			session_id VARCHAR,
			command VARCHAR,
			targets JSON,
			results JSON,
			executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_session_commands_session_id ON session_commands (session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_session_commands_executed_at ON session_commands (executed_at);`,
	}

	for _, schema := range schemas {
		_, err := d.conn.Exec(schema)
		if err != nil {
			return err
		}
	}

	return nil
}

// Close 关闭连接
func (d *DuckDB) Close() error {
	return d.conn.Close()
}

// Cleanup 清理过期数据
func (d *DuckDB) Cleanup(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	tables := []string{"operations", "node_communications", "command_executions", "file_transfers"}
	for _, table := range tables {
		_, err := d.conn.Exec("DELETE FROM "+table+" WHERE created_at < ?", cutoff)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *DuckDB) ensureDBDir() {
	dir := filepath.Dir(d.path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
}
