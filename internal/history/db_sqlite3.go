//go:build !duckdb
// +build !duckdb

package history

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var _ DBInterface = (*SQLite3)(nil)

// SQLite3 SQLite3 实现
type SQLite3 struct {
	conn *sql.DB
	path string
}

// NewDB 创建 SQLite3 数据库连接
func NewDB(config *Config) (DBInterface, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// 确保数据库文件以 .db 结尾
	dbPath := config.DBPath
	if filepath.Ext(dbPath) != ".db" {
		dbPath = dbPath + ".db"
	}

	ensureDBDir(dbPath)

	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// SQLite3 配置
	_, _ = conn.Exec("PRAGMA journal_mode=WAL")
	_, _ = conn.Exec("PRAGMA synchronous=NORMAL")
	_, _ = conn.Exec("PRAGMA foreign_keys=ON")

	db := &SQLite3{
		conn: conn,
		path: dbPath,
	}

	if err := db.InitSchema(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	globalDB = db
	return db, nil
}

// Connection 获取底层连接
func (s *SQLite3) Connection() *sql.DB {
	return s.conn
}

// InitSchema 初始化表结构（SQLite3 兼容版）
func (s *SQLite3) InitSchema() error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS operations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			op_type TEXT,
			command TEXT,
			targets TEXT,
			status TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_operations_task_id ON operations (task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_operations_op_type ON operations (op_type);`,
		`CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations (created_at);`,

		`CREATE TABLE IF NOT EXISTS node_communications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			node_id TEXT,
			node_address TEXT,
			direction TEXT,
			message_type TEXT,
			payload TEXT,
			success INTEGER,
			error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_communications_task_id ON node_communications (task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_communications_node_id ON node_communications (node_id);`,
		`CREATE INDEX IF NOT EXISTS idx_communications_created_at ON node_communications (created_at);`,

		`CREATE TABLE IF NOT EXISTS command_executions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			node_id TEXT,
			command TEXT,
			exit_code INTEGER,
			stdout TEXT,
			stderr TEXT,
			duration_ms INTEGER,
			success INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_executions_task_id ON command_executions (task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_executions_node_id ON command_executions (node_id);`,
		`CREATE INDEX IF NOT EXISTS idx_executions_created_at ON command_executions (created_at);`,

		`CREATE TABLE IF NOT EXISTS file_transfers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			node_id TEXT,
			file_name TEXT,
			file_size INTEGER,
			transfer_type TEXT,
			status TEXT,
			progress REAL,
			error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_task_id ON file_transfers (task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_node_id ON file_transfers (node_id);`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_created_at ON file_transfers (created_at);`,

		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			mode TEXT,
			node_ids TEXT,
			status TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			closed_at DATETIME,
			command_count INTEGER DEFAULT 0,
			success_count INTEGER DEFAULT 0,
			error_count INTEGER DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions (status);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions (created_at);`,

		`CREATE TABLE IF NOT EXISTS session_commands (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			command TEXT,
			targets TEXT,
			results TEXT,
			executed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_session_commands_session_id ON session_commands (session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_session_commands_executed_at ON session_commands (executed_at);`,

		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			address TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 22,
			user TEXT NOT NULL DEFAULT 'root',
			password TEXT NOT NULL DEFAULT '',
			ssh_key TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'offline',
			groups TEXT NOT NULL DEFAULT '[]',
			labels TEXT NOT NULL DEFAULT '{}',
			proxy_jump TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_check_at DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes (status);`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes (name);`,

		`CREATE TABLE IF NOT EXISTS aichat (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			step TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			prompt TEXT DEFAULT '',
			input TEXT DEFAULT '',
			output TEXT DEFAULT '',
			tool_calls TEXT DEFAULT '',
			tool_results TEXT DEFAULT '',
			duration_ms INTEGER DEFAULT 0,
			error TEXT DEFAULT '',
			metadata TEXT DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_session ON aichat(session_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_created ON aichat(created_at);`,
	}

	for _, schema := range schemas {
		_, err := s.conn.Exec(schema)
		if err != nil {
			return err
		}
	}

	return nil
}

// Close 关闭连接
func (s *SQLite3) Close() error {
	return s.conn.Close()
}

// Cleanup 清理过期数据
func (s *SQLite3) Cleanup(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	tables := []string{"operations", "node_communications", "command_executions", "file_transfers"}
	for _, table := range tables {
		_, err := s.conn.Exec("DELETE FROM "+table+" WHERE created_at < ?", cutoff)
		if err != nil {
			return err
		}
	}

	_, err := s.conn.Exec(`DELETE FROM aichat WHERE created_at < ?`, cutoff)
	if err != nil {
		return err
	}

	return nil
}

func (s *SQLite3) ensureDBDir() {
	dir := filepath.Dir(s.path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
}
