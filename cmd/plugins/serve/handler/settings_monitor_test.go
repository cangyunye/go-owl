package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestSettingsHandler_MonitorKeysValidation 验证监控告警设置键的格式校验：
// 非法值 400，合法值（含 collect_window 空值=全天）200。
func TestSettingsHandler_MonitorKeysValidation(t *testing.T) {
	_, h := settingsTestSetup(t)
	ginSetMode()
	router := settingsRBACRouter(t, h)
	token := settingsAdminToken()

	cases := []struct {
		key, value string
		want       int
	}{
		{"monitor.enabled", "banana", 400},
		{"monitor.enabled", "false", 200},
		{"monitor.collect_window", "25:99-08:00", 400},
		{"monitor.collect_window", "08:00-22:00", 200},
		{"monitor.collect_window", "22:00-06:00", 200},
		{"monitor.collect_window", "", 200}, // 空 = 全天，允许清空
		{"monitor.alert_retention_days", "-5", 400},
		{"monitor.alert_retention_days", "abc", 400},
		{"monitor.alert_retention_days", "30", 200},
		{"monitor.alert_retention_days", "0", 200},
		{"monitor.realert_window_minutes", "abc", 400},
		{"monitor.realert_window_minutes", "-1", 400},
		{"monitor.realert_window_minutes", "0", 200},
		{"monitor.realert_window_minutes", "1440", 200},
		{"monitor.escalate_after_minutes", "0", 400},
		{"monitor.escalate_after_minutes", "60", 200},
		{"monitor.rollback_enabled", "banana", 400},
		{"monitor.rollback_enabled", "false", 200},
		{"monitor.verify_enabled", "banana", 400},
		{"monitor.verify_enabled", "false", 200},
		{"monitor.verify_enabled", "true", 200},
		{"monitor.verify_delay_seconds", "-1", 400},
		{"monitor.verify_delay_seconds", "601", 400},
		{"monitor.verify_delay_seconds", "0", 200},
		{"monitor.verify_delay_seconds", "120", 200},
	}
	for _, tc := range cases {
		w := doSettingsPut(router, token, tc.key, tc.value)
		assert.Equal(t, tc.want, w.Code, "%s=%q", tc.key, tc.value)
	}
}

func ginSetMode() { gin.SetMode(gin.TestMode) }

func settingsAdminToken() string {
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	t, _ := as.GenerateToken("admin", "admin")
	return t
}

func doSettingsPut(r *gin.Engine, token, key, value string) *httptest.ResponseRecorder {
	body := `{"value":` + strconv.Quote(value) + `}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/settings/"+key, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}
