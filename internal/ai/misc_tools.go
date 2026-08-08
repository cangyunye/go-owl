package ai

import (
	"context"
	"fmt"
	"strings"
)

// ---------- async ----------

type AsyncListTool struct {
	executor Executor
}

func NewAsyncListTool(executor Executor) *AsyncListTool { return &AsyncListTool{executor: executor} }

func (t *AsyncListTool) Name() string        { return "async_list" }
func (t *AsyncListTool) Description() string { return "List async execution tasks." }
func (t *AsyncListTool) Parameters() string  { return `{"type":"object","properties":{}}` }
func (t *AsyncListTool) Validate(p map[string]interface{}) error { return nil }
func (t *AsyncListTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		result, err := t.executor.AsyncList(ctx)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("async_list failed")
}

type AsyncStatusTool struct {
	executor Executor
}

func NewAsyncStatusTool(executor Executor) *AsyncStatusTool { return &AsyncStatusTool{executor: executor} }

func (t *AsyncStatusTool) Name() string        { return "async_status" }
func (t *AsyncStatusTool) Description() string { return "Show status of an async execution task." }
func (t *AsyncStatusTool) Parameters() string  { return asyncStatusParamsSchema }
func (t *AsyncStatusTool) Validate(p map[string]interface{}) error {
	if strings.TrimSpace(strOf(p["task_id"])) == "" {
		return fmt.Errorf("task_id is required")
	}
	return nil
}

const asyncStatusParamsSchema = `{
	"type": "object",
	"properties": {"task_id": {"type": "string", "description": "Async task id"}},
	"required": ["task_id"]
}`

func (t *AsyncStatusTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		result, err := t.executor.AsyncStatus(ctx, AsyncStatusParams{TaskID: strOf(params["task_id"])})
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("async_status failed")
}

type AsyncCancelTool struct {
	executor Executor
}

func NewAsyncCancelTool(executor Executor) *AsyncCancelTool { return &AsyncCancelTool{executor: executor} }

func (t *AsyncCancelTool) Name() string        { return "async_cancel" }
func (t *AsyncCancelTool) Description() string { return "Cancel an async execution task." }
func (t *AsyncCancelTool) Parameters() string  { return asyncStatusParamsSchema }
func (t *AsyncCancelTool) Validate(p map[string]interface{}) error {
	if strings.TrimSpace(strOf(p["task_id"])) == "" {
		return fmt.Errorf("task_id is required")
	}
	return nil
}

func (t *AsyncCancelTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		result, err := t.executor.AsyncCancel(ctx, AsyncStatusParams{TaskID: strOf(params["task_id"])})
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("async_cancel failed")
}

// ---------- settings ----------

type SettingsShowTool struct {
	executor Executor
}

func NewSettingsShowTool(executor Executor) *SettingsShowTool { return &SettingsShowTool{executor: executor} }

func (t *SettingsShowTool) Name() string        { return "settings_show" }
func (t *SettingsShowTool) Description() string { return "Show current owl settings." }
func (t *SettingsShowTool) Parameters() string  { return `{"type":"object","properties":{}}` }
func (t *SettingsShowTool) Validate(p map[string]interface{}) error { return nil }
func (t *SettingsShowTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		result, err := t.executor.SettingsShow(ctx)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("settings_show failed")
}

type SettingsSetTool struct {
	executor Executor
}

func NewSettingsSetTool(executor Executor) *SettingsSetTool { return &SettingsSetTool{executor: executor} }

func (t *SettingsSetTool) Name() string        { return "settings_set" }
func (t *SettingsSetTool) Description() string { return "Set an owl setting (e.g. ssh timeout, default groups)." }
func (t *SettingsSetTool) Parameters() string  { return settingsSetParamsSchema }
func (t *SettingsSetTool) Validate(p map[string]interface{}) error {
	if strings.TrimSpace(strOf(p["key"])) == "" {
		return fmt.Errorf("key is required")
	}
	if strings.TrimSpace(strOf(p["value"])) == "" {
		return fmt.Errorf("value is required")
	}
	return nil
}

const settingsSetParamsSchema = `{
	"type": "object",
	"properties": {
		"key": {"type": "string", "description": "Setting key, e.g. ssh.timeout"},
		"value": {"type": "string", "description": "Setting value"}
	},
	"required": ["key", "value"]
}`

func (t *SettingsSetTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := SettingsSetParams{Key: strOf(params["key"]), Value: strOf(params["value"])}
		result, err := t.executor.SettingsSet(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("settings_set failed")
}

// ---------- history (执行历史) ----------

type HistoryListTool struct {
	executor Executor
}

func NewHistoryListTool(executor Executor) *HistoryListTool { return &HistoryListTool{executor: executor} }

func (t *HistoryListTool) Name() string        { return "history_list" }
func (t *HistoryListTool) Description() string { return "List execution history records." }
func (t *HistoryListTool) Parameters() string  { return historyListParamsSchema }
func (t *HistoryListTool) Validate(p map[string]interface{}) error { return nil }

const historyListParamsSchema = `{
	"type": "object",
	"properties": {
		"node_id": {"type": "string", "description": "Filter by node"},
		"op_type": {"type": "string", "description": "Filter by operation type"},
		"limit": {"type": "integer", "description": "Max rows, default 50"}
	}
}`

func (t *HistoryListTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := HistoryListParams{
			NodeID: strOf(params["node_id"]),
			OpType: strOf(params["op_type"]),
			Limit:  intOf(params["limit"]),
		}
		result, err := t.executor.HistoryList(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("history_list failed")
}

type HistoryCleanTool struct {
	executor Executor
}

func NewHistoryCleanTool(executor Executor) *HistoryCleanTool { return &HistoryCleanTool{executor: executor} }

func (t *HistoryCleanTool) Name() string        { return "history_clean" }
func (t *HistoryCleanTool) Description() string { return "Clean execution history older than N days." }
func (t *HistoryCleanTool) Parameters() string  { return historyCleanParamsSchema }
func (t *HistoryCleanTool) Validate(p map[string]interface{}) error { return nil }

const historyCleanParamsSchema = `{
	"type": "object",
	"properties": {"days": {"type": "integer", "description": "Retention days, default 30"}}
}`

func (t *HistoryCleanTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := HistoryCleanParams{Days: intOf(params["days"])}
		result, err := t.executor.HistoryClean(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("history_clean failed")
}
