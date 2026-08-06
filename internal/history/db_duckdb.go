//go:build duckdb
// +build duckdb

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
		`CREATE SEQUENCE IF NOT EXISTS seq_operations_id START 1;`,
		`CREATE SEQUENCE IF NOT EXISTS seq_node_comm_id START 1;`,
		`CREATE SEQUENCE IF NOT EXISTS seq_command_exec_id START 1;`,
		`CREATE SEQUENCE IF NOT EXISTS seq_file_transfer_id START 1;`,
		`CREATE SEQUENCE IF NOT EXISTS seq_session_cmd_id START 1;`,

		`CREATE TABLE IF NOT EXISTS operations (
			id BIGINT PRIMARY KEY DEFAULT NEXTVAL('seq_operations_id'),
			task_id VARCHAR,
			op_type VARCHAR,
			command VARCHAR,
			targets JSON,
			execution_mode VARCHAR DEFAULT '',
			playbook_path VARCHAR DEFAULT '',
			current_task_index INTEGER DEFAULT 0,
			current_task_phase VARCHAR DEFAULT '',
			forced INTEGER DEFAULT 0,
			status VARCHAR,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_operations_task_id ON operations (task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_operations_op_type ON operations (op_type);`,
		`CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations (created_at);`,

		`CREATE TABLE IF NOT EXISTS node_communications (
			id BIGINT PRIMARY KEY DEFAULT NEXTVAL('seq_node_comm_id'),
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
			id BIGINT PRIMARY KEY DEFAULT NEXTVAL('seq_command_exec_id'),
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
			id BIGINT PRIMARY KEY DEFAULT NEXTVAL('seq_file_transfer_id'),
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
			id BIGINT PRIMARY KEY DEFAULT NEXTVAL('seq_session_cmd_id'),
			session_id VARCHAR,
			command VARCHAR,
			targets JSON,
			results JSON,
			executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_session_commands_session_id ON session_commands (session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_session_commands_executed_at ON session_commands (executed_at);`,

		`CREATE TABLE IF NOT EXISTS nodes (
			id VARCHAR PRIMARY KEY,
			name VARCHAR NOT NULL DEFAULT '',
			address VARCHAR NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 22,
			user VARCHAR NOT NULL DEFAULT 'root',
			password VARCHAR NOT NULL DEFAULT '',
			ssh_key VARCHAR NOT NULL DEFAULT '',
			status VARCHAR NOT NULL DEFAULT 'offline',
			groups JSON NOT NULL DEFAULT '[]',
			labels JSON NOT NULL DEFAULT '{}',
			proxy_jump VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_check_at TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes (status);`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes (name);`,

		`CREATE TABLE IF NOT EXISTS aichat (
			id TEXT PRIMARY KEY DEFAULT uuid(),
			session_id TEXT NOT NULL,
			step TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			prompt TEXT DEFAULT '',
			input TEXT DEFAULT '',
			output TEXT DEFAULT '',
			tool_calls TEXT DEFAULT '',
			tool_results TEXT DEFAULT '',
			duration_ms BIGINT DEFAULT 0,
			error TEXT DEFAULT '',
			metadata TEXT DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_session ON aichat(session_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_created ON aichat(created_at);`,

		`CREATE SEQUENCE IF NOT EXISTS seq_step_states_id START 1;`,

		`CREATE TABLE IF NOT EXISTS playbook_run_states (
			id VARCHAR PRIMARY KEY,
			playbook_name VARCHAR NOT NULL,
			playbook_hash VARCHAR NOT NULL,
			nodes JSON NOT NULL,
			status VARCHAR NOT NULL DEFAULT 'running',
			started_at TIMESTAMP NOT NULL,
			finished_at TIMESTAMP,
			total_steps INTEGER NOT NULL,
			completed_steps INTEGER DEFAULT 0,
			failed_steps INTEGER DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_run_states_status ON playbook_run_states(status);`,
		`CREATE INDEX IF NOT EXISTS idx_run_states_name ON playbook_run_states(playbook_name);`,

		`CREATE TABLE IF NOT EXISTS playbook_step_states (
			id BIGINT PRIMARY KEY DEFAULT NEXTVAL('seq_step_states_id'),
			run_id VARCHAR NOT NULL,
			node_id VARCHAR NOT NULL,
			step_index INTEGER NOT NULL,
			step_name VARCHAR NOT NULL,
			action VARCHAR NOT NULL,
			status VARCHAR NOT NULL DEFAULT 'pending',
			started_at TIMESTAMP,
			finished_at TIMESTAMP,
			duration_ms INTEGER,
			exit_code INTEGER,
			stdout VARCHAR,
			stderr VARCHAR,
			error VARCHAR,
			retry_count INTEGER DEFAULT 0,
			UNIQUE(run_id, node_id, step_index)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_step_states_run ON playbook_step_states(run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_step_states_node ON playbook_step_states(run_id, node_id);`,
	}

	for _, schema := range schemas {
		_, err := d.conn.Exec(schema)
		if err != nil {
			return err
		}
	}

	// 迁移：兼容旧表缺少的列
	_, _ = d.conn.Exec("ALTER TABLE nodes ADD COLUMN last_check_at TIMESTAMP")

	return d.EnsureOperationColumns()
}

// operationColumnSpecsDuckDB 与 operationColumnSpecs（sqlite3）等价，
// DuckDB 用 VARCHAR 类型与 ADD COLUMN IF NOT EXISTS 幂等写法。
var operationColumnSpecsDuckDB = []struct {
	name string
	ddl  string
}{
	{"execution_mode", `ALTER TABLE operations ADD COLUMN IF NOT EXISTS execution_mode VARCHAR DEFAULT ''`},
	{"playbook_path", `ALTER TABLE operations ADD COLUMN IF NOT EXISTS playbook_path VARCHAR DEFAULT ''`},
	{"current_task_index", `ALTER TABLE operations ADD COLUMN IF NOT EXISTS current_task_index INTEGER DEFAULT 0`},
	{"current_task_phase", `ALTER TABLE operations ADD COLUMN IF NOT EXISTS current_task_phase VARCHAR DEFAULT ''`},
	{"forced", `ALTER TABLE operations ADD COLUMN IF NOT EXISTS forced INTEGER DEFAULT 0`},
}

// EnsureOperationColumns 为存量库补齐 operations 缺失的列（幂等）。
func (d *DuckDB) EnsureOperationColumns() error {
	for _, spec := range operationColumnSpecsDuckDB {
		if _, err := d.conn.Exec(spec.ddl); err != nil {
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

	_, err := d.conn.Exec(`DELETE FROM aichat WHERE created_at < ?`, cutoff)
	if err != nil {
		return err
	}

	return nil
}

func (d *DuckDB) ensureDBDir() {
	dir := filepath.Dir(d.path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
}
