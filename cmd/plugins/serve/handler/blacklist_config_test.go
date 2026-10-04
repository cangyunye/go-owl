package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/control/blacklist"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonUnmarshalHelper(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func jsonQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// unwrapData 解开 handler 的统一响应包裹 {"data": {...}} 再解析。
// 此前测试按顶层字段解析，响应统一加包裹后 path/content/warnings
// 全部解析成零值，4 个用例假失败。
func unwrapData(t *testing.T, body []byte, v interface{}) {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, jsonUnmarshalHelper(body, &env))
	require.NoError(t, jsonUnmarshalHelper(env.Data, v))
}

// 系统设置页编辑 ~/.owl/blacklist.yaml：GET 返回文件原文（保留注释）
// 与内置默认规则；PUT 校验后原子写入并热重载共享检查器（admin）。

func isolateBlacklistHome(t *testing.T) string {
	t.Helper()
	// 先在原 HOME 下物化共享实例并保存规则现场（必须在改 HOME 前），
	// 测试结束后原地恢复：共享检查器是进程级单例，不清场会污染
	// 依赖默认规则的其他用例
	prevCfg := blacklist.Shared().Config()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := blacklist.ReloadShared(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		blacklist.Shared().Reload(prevCfg)
	})
	return filepath.Join(home, ".owl", "blacklist.yaml")
}

func blacklistConfigRequest(t *testing.T, h *SettingsHandler, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if body == "" {
		c.Request = httptest.NewRequest(method, "/api/v1/blacklist", nil)
	} else {
		c.Request = httptest.NewRequest(method, "/api/v1/blacklist", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
	}
	if method == http.MethodGet {
		h.GetBlacklist(c)
	} else {
		h.SetBlacklist(c)
	}
	return w
}

func TestBlacklistConfig_Get_DefaultsWhenNoFile(t *testing.T) {
	path := isolateBlacklistHome(t)
	h := &SettingsHandler{}

	w := blacklistConfigRequest(t, h, http.MethodGet, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Path     string `json:"path"`
		Exists   bool   `json:"exists"`
		Content  string `json:"content"`
		Defaults string `json:"defaults"`
	}
	unwrapData(t, w.Body.Bytes(), &resp)
	assert.Equal(t, path, resp.Path)
	assert.False(t, resp.Exists, "文件不存在时 exists=false")
	assert.Contains(t, resp.Defaults, "rm -rf", "defaults 应渲染内置默认规则供载入")
}

func TestBlacklistConfig_Put_WritesReloads(t *testing.T) {
	path := isolateBlacklistHome(t)
	h := &SettingsHandler{}

	content := `rules:
  - user: "*"
    patterns:
      - "forbidden-ui-cmd "
`
	w := blacklistConfigRequest(t, h, http.MethodPut, `{"content":`+jsonQuote(content)+`}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	data, err := os.ReadFile(path)
	require.NoError(t, err, "保存后文件必须存在")
	assert.Equal(t, content, string(data), "文件内容必须与提交一致（保留注释与格式）")

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "黑名单是安全策略文件，权限应 0600")

	// 热重载：共享检查器立即命中新规则
	assert.True(t, blacklist.Shared().Check("anyone", "forbidden-ui-cmd --now").Blocked, "保存后应即时生效")
	assert.False(t, blacklist.Shared().Check("anyone", "uptime").Blocked)

	// GET 回读文件原文
	w2 := blacklistConfigRequest(t, h, http.MethodGet, "")
	var resp struct {
		Exists  bool   `json:"exists"`
		Content string `json:"content"`
	}
	unwrapData(t, w2.Body.Bytes(), &resp)
	assert.True(t, resp.Exists)
	assert.Equal(t, content, resp.Content)
}

func TestBlacklistConfig_Put_InvalidYAMLRejected(t *testing.T) {
	path := isolateBlacklistHome(t)
	h := &SettingsHandler{}

	w := blacklistConfigRequest(t, h, http.MethodPut, `{"content":"rules: [oops"}`)
	require.Equal(t, http.StatusBadRequest, w.Code, "非法 YAML 必须拒绝")

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "被拒绝的内容不得写盘")
}

func TestBlacklistConfig_Put_RuleWithoutUserRejected(t *testing.T) {
	isolateBlacklistHome(t)
	h := &SettingsHandler{}

	w := blacklistConfigRequest(t, h, http.MethodPut, `{"content":"rules:\n  - patterns: [\"x\"]"}`)
	require.Equal(t, http.StatusBadRequest, w.Code, "缺少 user 的规则是永不生效的死规则，必须拒绝")
}

func TestBlacklistConfig_Put_EmptyRulesAllowedWithWarning(t *testing.T) {
	isolateBlacklistHome(t)
	h := &SettingsHandler{}

	w := blacklistConfigRequest(t, h, http.MethodPut, `{"content":"rules: []"}`)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Warnings []string `json:"warnings"`
	}
	unwrapData(t, w.Body.Bytes(), &resp)
	require.NotEmpty(t, resp.Warnings, "清空规则等于关闭拦截，必须给出警告")
}

func TestBlacklistConfig_Put_RegexCompileWarning(t *testing.T) {
	isolateBlacklistHome(t)
	h := &SettingsHandler{}

	// 非法正则按字面匹配（黑名单的既有语义），保存成功但应提示
	w := blacklistConfigRequest(t, h, http.MethodPut, `{"content":"rules:\n  - user: root\n    patterns:\n      - \"a[b\"\n"}`)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Warnings []string `json:"warnings"`
	}
	unwrapData(t, w.Body.Bytes(), &resp)
	found := false
	for _, warn := range resp.Warnings {
		if strings.Contains(warn, "a[b") && strings.Contains(warn, "字面") {
			found = true
		}
	}
	assert.True(t, found, "无法编译为正则的 pattern 应给出字面匹配提示，实际: %v", resp.Warnings)
}
