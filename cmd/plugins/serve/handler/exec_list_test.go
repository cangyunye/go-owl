package handler

import (
	"encoding/json"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 执行页需要按 record 拉取本次提交的全部任务终态做对账兜底（WS 消息可能
// 因断线窗口丢失），列表接口必须支持 record_id 过滤，避免按节点发 N 次请求。
func TestExecList_FilterByRecordID(t *testing.T) {
	db, h := execTestSetup(t)
	ts := store.NewTaskStore(db)
	ctx := t.Context()

	_, err := ts.CreateWithRecord(ctx, "test-node", "echo a", "rec-1")
	require.NoError(t, err)
	_, err = ts.CreateWithRecord(ctx, "test-node", "echo b", "rec-1")
	require.NoError(t, err)
	_, err = ts.CreateWithRecord(ctx, "test-node", "echo c", "rec-2")
	require.NoError(t, err)

	router := execRBACRouter(t, h)
	w := authRequest(t, router, "GET", "/api/v1/tasks?record_id=rec-1", nil, "viewer")

	require.Equal(t, 200, w.Code)
	var res struct {
		Data []*store.Task `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.Len(t, res.Data, 2, "只应返回该 record 下的任务")
	assert.Equal(t, 2, res.Meta.Total)
	for _, task := range res.Data {
		assert.Equal(t, "rec-1", task.RecordID)
	}
}

// 未带 record_id 时保持原有分页行为不变
func TestExecList_WithoutRecordIDPaginates(t *testing.T) {
	db, h := execTestSetup(t)
	ts := store.NewTaskStore(db)
	for i := 0; i < 5; i++ {
		_, err := ts.Create(t.Context(), "test-node", "echo x")
		require.NoError(t, err)
	}

	router := execRBACRouter(t, h)
	w := authRequest(t, router, "GET", "/api/v1/tasks?page=1&page_size=2", nil, "viewer")

	require.Equal(t, 200, w.Code)
	var res struct {
		Data []*store.Task `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Len(t, res.Data, 2)
	assert.Equal(t, 5, res.Meta.Total)
}
