package history

import (
	"encoding/json"
	"time"
)

// Operation 主机操作记录
type Operation struct {
	ID        int64
	TaskID    string
	OpType    string
	Command   string
	Targets   []string
	Status    string
	CreatedAt time.Time
}

// NodeCommunication 节点通信记录
type NodeCommunication struct {
	ID          int64
	TaskID      string
	NodeID      string
	NodeAddress string
	Direction   string
	MessageType string
	Payload     string
	Success     bool
	Error       string
	CreatedAt   time.Time
}

// CommandExecution 命令执行记录
type CommandExecution struct {
	ID         int64
	TaskID     string
	NodeID     string
	Command    string
	ExitCode   int
	Stdout     string
	Stderr     string
	DurationMs int64
	Success    bool
	CreatedAt  time.Time
}

// FileTransfer 文件传输记录
type FileTransfer struct {
	ID           int64
	TaskID       string
	NodeID       string
	FileName     string
	FileSize     int64
	TransferType string
	Status       string
	Progress     float64
	Error        string
	CreatedAt    time.Time
}

// Record 统一记录结构
type Record struct {
	Operation         *Operation
	CommandExecutions []*CommandExecution
	Communications    []*NodeCommunication
	Transfers         []*FileTransfer
}

// RecordOperation 记录操作
func (db *DB) RecordOperation(op *Operation) error {
	targetsJSON, _ := json.Marshal(op.Targets)

	_, err := db.impl.Connection().Exec(`
		INSERT INTO operations (task_id, op_type, command, targets, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, op.TaskID, op.OpType, op.Command, targetsJSON, op.Status, op.CreatedAt)
	return err
}

// RecordCommandExecution 记录命令执行
func (db *DB) RecordCommandExecution(exec *CommandExecution) error {
	_, err := db.impl.Connection().Exec(`
		INSERT INTO command_executions (task_id, node_id, command, exit_code, stdout, stderr, duration_ms, success, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, exec.TaskID, exec.NodeID, exec.Command, exec.ExitCode, exec.Stdout, exec.Stderr, exec.DurationMs, exec.Success, exec.CreatedAt)
	return err
}

// RecordNodeCommunication 记录节点通信
func (db *DB) RecordNodeCommunication(comm *NodeCommunication) error {
	_, err := db.impl.Connection().Exec(`
		INSERT INTO node_communications (task_id, node_id, node_address, direction, message_type, payload, success, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, comm.TaskID, comm.NodeID, comm.NodeAddress, comm.Direction, comm.MessageType, comm.Payload, comm.Success, comm.Error, comm.CreatedAt)
	return err
}

// RecordFileTransfer 记录文件传输
func (db *DB) RecordFileTransfer(transfer *FileTransfer) error {
	_, err := db.impl.Connection().Exec(`
		INSERT INTO file_transfers (task_id, node_id, file_name, file_size, transfer_type, status, progress, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, transfer.TaskID, transfer.NodeID, transfer.FileName, transfer.FileSize, transfer.TransferType, transfer.Status, transfer.Progress, transfer.Error, transfer.CreatedAt)
	return err
}

// QueryOptions 查询选项
type QueryOptions struct {
	TaskID    string
	NodeID    string
	OpType    string
	Status    string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	Offset    int
}

// Query 查询历史记录
func (db *DB) Query(opts *QueryOptions) ([]*Record, error) {
	var records []*Record

	baseSQL := `SELECT id, task_id, op_type, command, targets, status, created_at FROM operations WHERE 1=1`
	params := []interface{}{}
	argIndex := 1

	if opts.TaskID != "" {
		baseSQL += ` AND task_id = ?`
		params = append(params, opts.TaskID)
		argIndex++
	}
	if opts.OpType != "" {
		baseSQL += ` AND op_type = ?`
		params = append(params, opts.OpType)
		argIndex++
	}
	if opts.Status != "" {
		baseSQL += ` AND status = ?`
		params = append(params, opts.Status)
		argIndex++
	}
	if !opts.StartTime.IsZero() {
		baseSQL += ` AND created_at >= ?`
		params = append(params, opts.StartTime)
		argIndex++
	}
	if !opts.EndTime.IsZero() {
		baseSQL += ` AND created_at <= ?`
		params = append(params, opts.EndTime)
		argIndex++
	}

	baseSQL += ` ORDER BY created_at DESC`
	if opts.Limit > 0 {
		baseSQL += ` LIMIT ? OFFSET ?`
		params = append(params, opts.Limit, opts.Offset)
	}

	rows, err := db.impl.Connection().Query(baseSQL, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var op Operation
		var targetsJSON []byte
		err := rows.Scan(
			&op.ID, &op.TaskID, &op.OpType, &op.Command,
			&targetsJSON, &op.Status, &op.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(targetsJSON, &op.Targets)

		record := &Record{Operation: &op}
		records = append(records, record)
	}

	// 获取关联的详细信息
	for _, record := range records {
		if record.Operation != nil {
			execs, _ := db.getCommandExecutionsByTaskID(record.Operation.TaskID)
			comms, _ := db.getCommunicationsByTaskID(record.Operation.TaskID)
			transfers, _ := db.getFileTransfersByTaskID(record.Operation.TaskID)
			record.CommandExecutions = execs
			record.Communications = comms
			record.Transfers = transfers
		}
	}

	return records, nil
}

func (db *DB) getCommandExecutionsByTaskID(taskID string) ([]*CommandExecution, error) {
	rows, err := db.impl.Connection().Query(`
		SELECT id, task_id, node_id, command, exit_code, stdout, stderr, duration_ms, success, created_at
		FROM command_executions WHERE task_id = ? ORDER BY created_at
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*CommandExecution
	for rows.Next() {
		var exec CommandExecution
		err := rows.Scan(
			&exec.ID, &exec.TaskID, &exec.NodeID, &exec.Command,
			&exec.ExitCode, &exec.Stdout, &exec.Stderr,
			&exec.DurationMs, &exec.Success, &exec.CreatedAt,
		)
		if err != nil {
			continue
		}
		results = append(results, &exec)
	}
	return results, nil
}

func (db *DB) getCommunicationsByTaskID(taskID string) ([]*NodeCommunication, error) {
	rows, err := db.impl.Connection().Query(`
		SELECT id, task_id, node_id, node_address, direction, message_type, payload, success, error, created_at
		FROM node_communications WHERE task_id = ? ORDER BY created_at
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*NodeCommunication
	for rows.Next() {
		var comm NodeCommunication
		err := rows.Scan(
			&comm.ID, &comm.TaskID, &comm.NodeID, &comm.NodeAddress,
			&comm.Direction, &comm.MessageType, &comm.Payload,
			&comm.Success, &comm.Error, &comm.CreatedAt,
		)
		if err != nil {
			continue
		}
		results = append(results, &comm)
	}
	return results, nil
}

func (db *DB) getFileTransfersByTaskID(taskID string) ([]*FileTransfer, error) {
	rows, err := db.impl.Connection().Query(`
		SELECT id, task_id, node_id, file_name, file_size, transfer_type, status, progress, error, created_at
		FROM file_transfers WHERE task_id = ? ORDER BY created_at
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*FileTransfer
	for rows.Next() {
		var transfer FileTransfer
		err := rows.Scan(
			&transfer.ID, &transfer.TaskID, &transfer.NodeID,
			&transfer.FileName, &transfer.FileSize, &transfer.TransferType,
			&transfer.Status, &transfer.Progress, &transfer.Error,
			&transfer.CreatedAt,
		)
		if err != nil {
			continue
		}
		results = append(results, &transfer)
	}
	return results, nil
}

// ---------------- 全局便捷函数 ----------------

func RecordOperation(op *Operation) error {
	if GetGlobalDB() == nil {
		return nil
	}
	targetsJSON, _ := json.Marshal(op.Targets)
	_, err := GetGlobalDB().Connection().Exec(`
		INSERT INTO operations (task_id, op_type, command, targets, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, op.TaskID, op.OpType, op.Command, targetsJSON, op.Status, op.CreatedAt)
	return err
}

func RecordCommandExecution(exec *CommandExecution) error {
	if GetGlobalDB() == nil {
		return nil
	}
	_, err := GetGlobalDB().Connection().Exec(`
		INSERT INTO command_executions (task_id, node_id, command, exit_code, stdout, stderr, duration_ms, success, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, exec.TaskID, exec.NodeID, exec.Command, exec.ExitCode, exec.Stdout, exec.Stderr, exec.DurationMs, exec.Success, exec.CreatedAt)
	return err
}

func RecordNodeCommunication(comm *NodeCommunication) error {
	if GetGlobalDB() == nil {
		return nil
	}
	_, err := GetGlobalDB().Connection().Exec(`
		INSERT INTO node_communications (task_id, node_id, node_address, direction, message_type, payload, success, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, comm.TaskID, comm.NodeID, comm.NodeAddress, comm.Direction, comm.MessageType, comm.Payload, comm.Success, comm.Error, comm.CreatedAt)
	return err
}

func RecordFileTransfer(transfer *FileTransfer) error {
	if GetGlobalDB() == nil {
		return nil
	}
	_, err := GetGlobalDB().Connection().Exec(`
		INSERT INTO file_transfers (task_id, node_id, file_name, file_size, transfer_type, status, progress, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, transfer.TaskID, transfer.NodeID, transfer.FileName, transfer.FileSize, transfer.TransferType, transfer.Status, transfer.Progress, transfer.Error, transfer.CreatedAt)
	return err
}

func Query(opts *QueryOptions) ([]*Record, error) {
	if GetGlobalDB() == nil {
		return []*Record{}, nil
	}
	// 实现查询逻辑
	return nil, nil
}
