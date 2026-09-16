package handler

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 传输记录与任务详情列表必须真分页（默认 20/页）：此前写死 LIMIT 50 全量
// 渲染，记录一多就把"文件中转站"顶出屏幕，且超过 50 条的旧记录永远看不到。
func TestTransferRecords_Pagination(t *testing.T) {
	db, _, router, _ := transferTestSetup(t)
	rs := store.NewTransferRecordStore(db)
	for i := 0; i < 25; i++ {
		_, err := rs.Create(t.Context(), fmt.Sprintf("/tmp/f-%02d.tar", i), "/opt/", "push", "")
		require.NoError(t, err)
	}

	w := authRequest(t, router, "GET", "/api/v1/transfer/records?page=1&page_size=20", nil, "operator")
	require.Equal(t, 200, w.Code)
	var p1 struct {
		Data []*store.TransferRecord `json:"data"`
		Meta struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p1))
	require.Len(t, p1.Data, 20, "默认每页 20 条")
	assert.Equal(t, 25, p1.Meta.Total)
	assert.Equal(t, 1, p1.Meta.Page)
	assert.Equal(t, 20, p1.Meta.PageSize)

	w2 := authRequest(t, router, "GET", "/api/v1/transfer/records?page=2&page_size=20", nil, "operator")
	require.Equal(t, 200, w2.Code)
	var p2 struct {
		Data []*store.TransferRecord `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &p2))
	require.Len(t, p2.Data, 5, "第二页为剩余记录")
	assert.Equal(t, 25, p2.Meta.Total)
}

// 任务详情列表（transfer: 前缀任务）同样分页
func TestTransferList_Pagination(t *testing.T) {
	db, _, router, _ := transferTestSetup(t)
	ts := store.NewTaskStore(db)
	for i := 0; i < 25; i++ {
		_, err := ts.CreateWithRecord(t.Context(), "node-1", fmt.Sprintf("transfer:/tmp/a-%02d -> /opt/", i), fmt.Sprintf("rec-%02d", i))
		require.NoError(t, err)
	}

	w := authRequest(t, router, "GET", "/api/v1/transfers?page=1&page_size=20", nil, "operator")
	require.Equal(t, 200, w.Code)
	var p1 struct {
		Data []*store.Task `json:"data"`
		Meta struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p1))
	require.Len(t, p1.Data, 20)
	assert.Equal(t, 25, p1.Meta.Total)

	w2 := authRequest(t, router, "GET", "/api/v1/transfers?page=2&page_size=20", nil, "operator")
	require.Equal(t, 200, w2.Code)
	var p2 struct {
		Data []*store.Task `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &p2))
	require.Len(t, p2.Data, 5)
}

// 未带分页参数时保持默认行为（20/页）
func TestTransferRecords_DefaultPage(t *testing.T) {
	db, _, router, _ := transferTestSetup(t)
	rs := store.NewTransferRecordStore(db)
	for i := 0; i < 3; i++ {
		_, err := rs.Create(t.Context(), fmt.Sprintf("/tmp/g-%d.tar", i), "/opt/", "push", "")
		require.NoError(t, err)
	}

	w := authRequest(t, router, "GET", "/api/v1/transfer/records", nil, "operator")
	require.Equal(t, 200, w.Code)
	var res struct {
		Meta struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Meta.Page)
	assert.Equal(t, 20, res.Meta.PageSize)
}
