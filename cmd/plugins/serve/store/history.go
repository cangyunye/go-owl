package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// 表结构须与 internal/history/db_sqlite3.go 保持一致，唯一例外：
// operations.username 是 serve 专属列（CLI 无用户/权限概念，本地库能运行即能查，
// 归属无意义），CLI 的建表与迁移列表不含该项。CLI 的 INSERT/SELECT 均为显式列清单，
// 不引用 username，故该列存在与否均不影响 CLI；serve 侧在 ensureOperationColumns
// 中对存量表 ALTER 补齐。切勿按"逐字一致"刻板把该列加回 internal/history。
// CLI 与 Web 共用 ~/.owl/owl.db，两者以 CREATE TABLE IF NOT EXISTS 建同名表，
// 先建者生效；其余列 schema 不一致会导致读写错乱。

type Operation struct {
	ID               int64     `json:"id"`
	TaskID           string    `json:"task_id"`
	OpType           string    `json:"op_type"`
	Command          string    `json:"command"`
	Targets          []string  `json:"targets"`
	Status           string    `json:"status"`
	ExecutionMode    string    `json:"execution_mode"`
	PlaybookPath     string    `json:"playbook_path"`
	CurrentTaskIndex int       `json:"current_task_index"`
	CurrentTaskPhase string    `json:"current_task_phase"`
	Forced           bool      `json:"forced"`
	Username         string    `json:"username,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type CommandExecution struct {
	ID         int64     `json:"id"`
	TaskID     string    `json:"task_id"`
	NodeID     string    `json:"node_id"`
	Command    string    `json:"command"`
	ExitCode   int       `json:"exit_code"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	DurationMs int64     `json:"duration_ms"`
	Success    bool      `json:"success"`
	CreatedAt  time.Time `json:"created_at"`
}

type FileTransfer struct {
	ID           int64     `json:"id"`
	TaskID       string    `json:"task_id"`
	NodeID       string    `json:"node_id"`
	FileName     string    `json:"file_name"`
	FileSize     int64     `json:"file_size"`
	TransferType string    `json:"transfer_type"`
	Status       string    `json:"status"`
	Progress     float64   `json:"progress"`
	Error        string    `json:"error"`
	CreatedAt    time.Time `json:"created_at"`
}

type NodeCommunication struct {
	ID          int64     `json:"id"`
	TaskID      string    `json:"task_id"`
	NodeID      string    `json:"node_id"`
	NodeAddress string    `json:"node_address"`
	Direction   string    `json:"direction"`
	MessageType string    `json:"message_type"`
	Payload     string    `json:"payload"`
	Success     bool      `json:"success"`
	Error       string    `json:"error"`
	CreatedAt   time.Time `json:"created_at"`
}

type Record struct {
	Operation         *Operation         `json:"operation"`
	CommandExecutions []*CommandExecution `json:"command_executions,omitempty"`
	Transfers         []*FileTransfer     `json:"transfers,omitempty"`
	Communications    []*NodeCommunication `json:"communications,omitempty"`
}

type QueryOptions struct {
	TaskID    string
	NodeID    string
	OpType    string
	Status    string
	Command   string
	User      string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	Offset    int
}

type Stats struct {
	Total    int            `json:"total"`
	ByOpType map[string]int `json:"by_op_type"`
	ByStatus map[string]int `json:"by_status"`
}

type HistoryStore struct {
	db *sql.DB
}

func NewHistoryStore(db *sql.DB) *HistoryStore {
	return &HistoryStore{db: db}
}

func (s *HistoryStore) Init(ctx context.Context) error {
	if s == nil {
		return nil
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS operations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			op_type TEXT,
			command TEXT,
			targets TEXT,
			status TEXT,
			execution_mode TEXT DEFAULT '',
			playbook_path TEXT DEFAULT '',
			current_task_index INTEGER DEFAULT 0,
			current_task_phase TEXT DEFAULT '',
			forced INTEGER DEFAULT 0,
			username TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_operations_task_id ON operations (task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_operations_op_type ON operations (op_type)`,
		`CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations (created_at)`,
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_task_id ON command_executions (task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_node_id ON command_executions (node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_created_at ON command_executions (created_at)`,
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_task_id ON file_transfers (task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_node_id ON file_transfers (node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_created_at ON file_transfers (created_at)`,
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_communications_task_id ON node_communications (task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_communications_node_id ON node_communications (node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_communications_created_at ON node_communications (created_at)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return s.ensureOperationColumns(ctx)
}

// operationColumnSpecs 存量 operations 表可能缺失的列。
// 早期 CLI schema 仅含 id/task_id/op_type/command/targets/status/created_at，
// execution_mode 等列为后续新增；CREATE TABLE IF NOT EXISTS 对存量表不生效，
// 必须逐列 ALTER 补齐，否则写入时报 "has no column named execution_mode"。
// username 为 serve 专属列（CLI 无用户概念，见文件头注释），
// 仅 serve 侧需要补齐，internal/history 侧的 operationColumnSpecs 不含此项。
var operationColumnSpecs = []struct {
	name string
	ddl  string
}{
	{"execution_mode", `ALTER TABLE operations ADD COLUMN execution_mode TEXT DEFAULT ''`},
	{"playbook_path", `ALTER TABLE operations ADD COLUMN playbook_path TEXT DEFAULT ''`},
	{"current_task_index", `ALTER TABLE operations ADD COLUMN current_task_index INTEGER DEFAULT 0`},
	{"current_task_phase", `ALTER TABLE operations ADD COLUMN current_task_phase TEXT DEFAULT ''`},
	{"forced", `ALTER TABLE operations ADD COLUMN forced INTEGER DEFAULT 0`},
	{"username", `ALTER TABLE operations ADD COLUMN username TEXT DEFAULT ''`},
}

// ensureOperationColumns 为存量库补齐 operations 缺失的列（幂等）。
// CLI 与 serve 可能并发迁移同一旧库，后到者的 ALTER 会收到
// "duplicate column name: xxx"，视为成功（与 playbook_run.go 的容错模式一致）。
func (s *HistoryStore) ensureOperationColumns(ctx context.Context) error {
	cols := map[string]bool{}
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(operations)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		cols[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, spec := range operationColumnSpecs {
		if cols[spec.name] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, spec.ddl); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return err
		}
	}
	// username 索引依赖该列存在，必须放在 ALTER 补齐之后创建，
	// 否则 legacy 旧库（无 username 列）在迁移前建索引会报 no such column。
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_operations_username ON operations (username)`); err != nil {
		return err
	}
	return nil
}

func (s *HistoryStore) RecordOperation(ctx context.Context, op *Operation) error {
	if s == nil {
		return nil
	}
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now().UTC()
	}
	targetsJSON, _ := json.Marshal(op.Targets)
	forced := 0
	if op.Forced {
		forced = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO operations (task_id, op_type, command, targets, status, execution_mode, playbook_path, current_task_index, current_task_phase, forced, username, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, op.TaskID, op.OpType, op.Command, string(targetsJSON), op.Status, op.ExecutionMode, op.PlaybookPath, op.CurrentTaskIndex, op.CurrentTaskPhase, forced, op.Username, op.CreatedAt)
	return err
}

func (s *HistoryStore) RecordCommandExecution(ctx context.Context, exec *CommandExecution) error {
	if s == nil {
		return nil
	}
	if exec.CreatedAt.IsZero() {
		exec.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO command_executions (task_id, node_id, command, exit_code, stdout, stderr, duration_ms, success, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, exec.TaskID, exec.NodeID, exec.Command, exec.ExitCode, exec.Stdout, exec.Stderr, exec.DurationMs, exec.Success, exec.CreatedAt)
	return err
}

func (s *HistoryStore) RecordFileTransfer(ctx context.Context, tr *FileTransfer) error {
	if s == nil {
		return nil
	}
	if tr.CreatedAt.IsZero() {
		tr.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO file_transfers (task_id, node_id, file_name, file_size, transfer_type, status, progress, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, tr.TaskID, tr.NodeID, tr.FileName, tr.FileSize, tr.TransferType, tr.Status, tr.Progress, tr.Error, tr.CreatedAt)
	return err
}

func (s *HistoryStore) RecordNodeCommunication(ctx context.Context, c *NodeCommunication) error {
	if s == nil {
		return nil
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO node_communications (task_id, node_id, node_address, direction, message_type, payload, success, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.TaskID, c.NodeID, c.NodeAddress, c.Direction, c.MessageType, c.Payload, c.Success, c.Error, c.CreatedAt)
	return err
}

func (s *HistoryStore) Query(ctx context.Context, opts *QueryOptions) ([]*Record, int, error) {
	if s == nil {
		return nil, 0, nil
	}
	where := " WHERE 1=1"
	args := []interface{}{}
	if opts.TaskID != "" {
		where += " AND task_id = ?"
		args = append(args, opts.TaskID)
	}
	if opts.OpType != "" {
		where += " AND op_type = ?"
		args = append(args, opts.OpType)
	}
	if opts.Status != "" {
		where += " AND status = ?"
		args = append(args, opts.Status)
	}
	if opts.NodeID != "" {
		where += " AND targets LIKE ?"
		args = append(args, "%"+opts.NodeID+"%")
	}
	if opts.Command != "" {
		where += " AND command LIKE ?"
		args = append(args, "%"+opts.Command+"%")
	}
	if opts.User != "" {
		where += " AND username = ?"
		args = append(args, opts.User)
	}
	if !opts.StartTime.IsZero() {
		where += " AND created_at >= ?"
		args = append(args, opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		where += " AND created_at <= ?"
		args = append(args, opts.EndTime)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM operations"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := "SELECT id, task_id, op_type, command, targets, status, execution_mode, playbook_path, current_task_index, current_task_phase, forced, username, created_at FROM operations" + where + " ORDER BY created_at DESC"
	listArgs := append([]interface{}{}, args...)
	if opts.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		listArgs = append(listArgs, opts.Limit, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	records := []*Record{}
	for rows.Next() {
		var op Operation
		var targetsJSON string
		var forced int
		if err := rows.Scan(&op.ID, &op.TaskID, &op.OpType, &op.Command, &targetsJSON, &op.Status, &op.ExecutionMode, &op.PlaybookPath, &op.CurrentTaskIndex, &op.CurrentTaskPhase, &forced, &op.Username, &op.CreatedAt); err != nil {
			continue
		}
		op.Forced = forced == 1
		json.Unmarshal([]byte(targetsJSON), &op.Targets)
		records = append(records, &Record{Operation: &op})
	}

	for _, rec := range records {
		rec.CommandExecutions, _ = s.executionsByTaskID(ctx, rec.Operation.TaskID)
		rec.Transfers, _ = s.transfersByTaskID(ctx, rec.Operation.TaskID)
		rec.Communications, _ = s.commsByTaskID(ctx, rec.Operation.TaskID)
	}
	return records, total, nil
}

func (s *HistoryStore) GetByTaskID(ctx context.Context, taskID string) (*Record, error) {
	recs, _, err := s.Query(ctx, &QueryOptions{TaskID: taskID})
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, sql.ErrNoRows
	}
	return recs[0], nil
}

func (s *HistoryStore) executionsByTaskID(ctx context.Context, taskID string) ([]*CommandExecution, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, node_id, command, exit_code, stdout, stderr, duration_ms, success, created_at
		FROM command_executions WHERE task_id = ? ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []*CommandExecution{}
	for rows.Next() {
		var e CommandExecution
		if err := rows.Scan(&e.ID, &e.TaskID, &e.NodeID, &e.Command, &e.ExitCode, &e.Stdout, &e.Stderr, &e.DurationMs, &e.Success, &e.CreatedAt); err != nil {
			continue
		}
		results = append(results, &e)
	}
	return results, nil
}

func (s *HistoryStore) transfersByTaskID(ctx context.Context, taskID string) ([]*FileTransfer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, node_id, file_name, file_size, transfer_type, status, progress, error, created_at
		FROM file_transfers WHERE task_id = ? ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []*FileTransfer{}
	for rows.Next() {
		var tr FileTransfer
		if err := rows.Scan(&tr.ID, &tr.TaskID, &tr.NodeID, &tr.FileName, &tr.FileSize, &tr.TransferType, &tr.Status, &tr.Progress, &tr.Error, &tr.CreatedAt); err != nil {
			continue
		}
		results = append(results, &tr)
	}
	return results, nil
}

func (s *HistoryStore) commsByTaskID(ctx context.Context, taskID string) ([]*NodeCommunication, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, node_id, node_address, direction, message_type, payload, success, error, created_at
		FROM node_communications WHERE task_id = ? ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []*NodeCommunication{}
	for rows.Next() {
		var c NodeCommunication
		if err := rows.Scan(&c.ID, &c.TaskID, &c.NodeID, &c.NodeAddress, &c.Direction, &c.MessageType, &c.Payload, &c.Success, &c.Error, &c.CreatedAt); err != nil {
			continue
		}
		results = append(results, &c)
	}
	return results, nil
}

func (s *HistoryStore) UpdateOperationStatus(ctx context.Context, taskID, status string) error {
	if s == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE operations SET status = ? WHERE task_id = ?`, status, taskID)
	return err
}

func (s *HistoryStore) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	if s == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	var total int64
	for _, table := range []string{"operations", "command_executions", "file_transfers", "node_communications"} {
		res, err := s.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE created_at < ?", cutoff)
		if err != nil {
			return total, err
		}
		if n, err := res.RowsAffected(); err == nil {
			total += n
		}
	}
	return total, nil
}

func (s *HistoryStore) Stats(ctx context.Context) (*Stats, error) {
	if s == nil {
		return &Stats{ByOpType: map[string]int{}, ByStatus: map[string]int{}}, nil
	}
	st := &Stats{ByOpType: map[string]int{}, ByStatus: map[string]int{}}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operations`).Scan(&st.Total); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT op_type, COUNT(*) FROM operations GROUP BY op_type`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err == nil {
			st.ByOpType[k] = n
		}
	}
	rows.Close()

	rows2, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM operations GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var k string
		var n int
		if err := rows2.Scan(&k, &n); err == nil {
			st.ByStatus[k] = n
		}
	}
	return st, nil
}
