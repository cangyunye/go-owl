package handler

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// page_size 超限必须钳制到 100 生效，而不是重置为默认 20：
// 前端分组统计按大 page_size 拉取节点计数，被重置成 20 会导致
// 分组计数只覆盖按名称排序的前 20 个节点（统计不正确）。
func TestNodeList_PageSizeOverLimitClampedTo100(t *testing.T) {
	db, h := crudTestSetup(t)
	for i := 0; i < 150; i++ {
		_, err := db.Exec(`INSERT INTO nodes (id, name, address, port, user, status) VALUES (?, ?, ?, 22, 'root', 'online')`,
			fmt.Sprintf("node-%03d", i), fmt.Sprintf("node-%03d", i), fmt.Sprintf("10.1.%d.%d", i/250, i%250+1))
		require.NoError(t, err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "GET", "/api/v1/nodes", model.RoleViewer, h.List)

	w := authRequest(t, router, "GET", "/api/v1/nodes?page=1&page_size=1000", nil, "viewer")
	require.Equal(t, 200, w.Code)
	var res struct {
		Data []json.RawMessage `json:"data"`
		Meta struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 151, res.Meta.Total, "150 个新节点 + 脚手架自带的 existing-node")
	assert.Equal(t, 100, res.Meta.PageSize, "超限 page_size 应钳制为 100")
	assert.Len(t, res.Data, 100, "单页应实际返回 100 条")

	// 合法范围内仍按请求值生效
	w2 := authRequest(t, router, "GET", "/api/v1/nodes?page=1&page_size=50", nil, "viewer")
	require.Equal(t, 200, w2.Code)
	var res2 struct {
		Meta struct {
			PageSize int `json:"page_size"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &res2))
	assert.Equal(t, 50, res2.Meta.PageSize)
}
