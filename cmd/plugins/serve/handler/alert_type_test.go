package handler

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	monitorSvc "github.com/cangyunye/go-owl/cmd/plugins/serve/monitor"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alertTypeTestSetup(t *testing.T) (*gin.Engine, *owlmonitor.Store, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "owl.db")
	st, err := owlmonitor.OpenStore(file)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	db, err := sql.Open("sqlite", file)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	svc := &monitorSvc.Service{Store: st}
	h := NewMonitorHandler(db, svc)

	us := store.NewUserStore(db)
	as := service.NewAuthService(bindingTestSecret)
	ah := NewAuthHandler(us, as)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(ah.AuthMiddleware(), ah.RBACMiddleware("admin"))
	{
		api.POST("/alert-types", h.CreateAlertType)
		api.DELETE("/alert-types/:id", h.DeleteAlertType)
	}
	token, _ := as.GenerateToken("admin", "admin")
	return r, st, token
}

// TestCreateAlertType_Validation 验证自定义告警类型创建校验：
// ID 前缀、正则有效性，合法创建自动生成 ID 与 custom 指标名。
func TestCreateAlertType_Validation(t *testing.T) {
	r, st, token := alertTypeTestSetup(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"缺名称", `{"id":"OWL-CUS-1","check_cmd":"echo 1"}`, 400},
		{"ID 前缀不对", `{"id":"OWL-DSK-999","name":"x"}`, 400},
		{"非法 check_mode", `{"name":"x","check_mode":"magic"}`, 400},
		{"regex 缺 pattern", `{"name":"x","check_mode":"regex"}`, 400},
		{"regex 无效 pattern", `{"name":"x","check_mode":"regex","check_pattern":"([bad"}`, 400},
		{"非法级别", `{"name":"x","default_severity":"mega"}`, 400},
		{"合法（自动生成 ID 与指标）", `{"name":"队列积压","check_cmd":"check-queue","check_mode":"value","default_severity":"warn","enabled":true}`, 200},
		{"合法 regex", `{"id":"OWL-CUS-R1","name":"磁盘正则","check_cmd":"report","check_mode":"regex","check_pattern":"used=(\\d+)%","default_params":{"metric":"custom.owl-cus-r1","op":">","value":90,"duration":1},"enabled":true}`, 200},
	}
	for _, tc := range cases {
		w := doReq(r, token, "POST", "/api/v1/alert-types", tc.body)
		assert.Equal(t, tc.want, w.Code, tc.name)
	}

	// 显式 ID 创建：custom 分类 + 指标名自动派生
	at, exists, err := st.GetAlertType("OWL-CUS-R1")
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, "custom", at.Category)
	assert.False(t, at.Builtin)
	assert.Equal(t, "custom.owl-cus-r1", at.DefaultParams.Metric)

	// 无 ID 的合法创建：服务端生成 OWL-CUS- 前缀 ID
	types, err := st.ListAlertTypes()
	require.NoError(t, err)
	var generated int
	for _, x := range types {
		if strings.HasPrefix(x.ID, "OWL-CUS-") && x.ID != "OWL-CUS-R1" {
			generated++
			assert.Equal(t, "custom.owl-cus-"+strings.ToLower(strings.TrimPrefix(x.ID, "OWL-CUS-")), x.DefaultParams.Metric)
		}
	}
	assert.Equal(t, 1, generated, "无 ID 案例应自动生成一个 OWL-CUS-* 类型")

	// 删除自定义类型成功，内置类型拒绝
	w := doReq(r, token, "DELETE", "/api/v1/alert-types/OWL-CUS-R1", "")
	assert.Equal(t, 200, w.Code)
	w = doReq(r, token, "DELETE", "/api/v1/alert-types/OWL-DSK-001", "")
	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "内置")
}
