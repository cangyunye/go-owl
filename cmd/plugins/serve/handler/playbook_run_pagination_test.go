package handler

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func playbookRunListSetup(t *testing.T) (*gin.Engine, *store.PlaybookRunStore) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	prs := store.NewPlaybookRunStore(db)
	require.NoError(t, prs.Init(t.Context()))

	h := NewPlaybookHandler(db, store.NewPlaybookStore(db), prs, store.NewNodeStore(db), nil)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/runs", h.RunList)
	return r, prs
}

// 运行历史必须真分页：默认 20/页，支持 page/page_size，meta 返回 total，
// 超过一页的旧记录要能翻到（此前固定 LIMIT 50，更早的永远看不到）。
func TestPlaybookRunList_Pagination(t *testing.T) {
	r, prs := playbookRunListSetup(t)

	for i := 0; i < 25; i++ {
		_, err := prs.Create(t.Context(),
			"pb", "deploy", "deploy.yaml", []string{"n1"}, nil, "", false)
		require.NoError(t, err)
	}

	type metaResp struct {
		Total    int `json:"total"`
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	}
	listRuns := func(query string) ([]*model.PlaybookRun, metaResp) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/runs"+query, nil))
		require.Equal(t, 200, w.Code)
		var res struct {
			Data []*model.PlaybookRun `json:"data"`
			Meta metaResp             `json:"meta"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
		return res.Data, res.Meta
	}

	data, meta := listRuns("")
	require.Len(t, data, 20, "默认每页 20 条")
	assert.Equal(t, 25, meta.Total)
	assert.Equal(t, 1, meta.Page)
	assert.Equal(t, 20, meta.PageSize)

	data2, meta2 := listRuns("?page=2&page_size=20")
	require.Len(t, data2, 5, "第二页为剩余记录")
	assert.Equal(t, 25, meta2.Total)

	data3, _ := listRuns("?page=0&page_size=1000")
	require.Len(t, data3, 20, "非法 page 与超限 page_size 应回退默认")
}
