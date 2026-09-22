package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSettingsHandler_LogsKeysValidation 验证执行日志清理设置键校验：
// 非法值 400，合法值 200（0 = 关闭定期清理，允许）。
func TestSettingsHandler_LogsKeysValidation(t *testing.T) {
	_, h := settingsTestSetup(t)
	ginSetMode()
	router := settingsRBACRouter(t, h)
	token := settingsAdminToken()

	cases := []struct {
		key, value string
		want       int
	}{
		{"logs.executions_retention_days", "abc", 400},
		{"logs.executions_retention_days", "-1", 400},
		{"logs.executions_retention_days", "3651", 400},
		{"logs.executions_retention_days", "0", 200},
		{"logs.executions_retention_days", "30", 200},
		{"logs.executions_retention_days", "3650", 200},
	}
	for _, tc := range cases {
		w := doSettingsPut(router, token, tc.key, tc.value)
		assert.Equal(t, tc.want, w.Code, "%s=%q", tc.key, tc.value)
	}
}
