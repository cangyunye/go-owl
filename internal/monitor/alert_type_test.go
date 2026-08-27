package monitor

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuiltinAlertTypes_Complete 验证内置告警类型注册表完整且合法。
func TestBuiltinAlertTypes_Complete(t *testing.T) {
	types := BuiltinAlertTypes()
	require.NotEmpty(t, types)

	seen := map[string]bool{}
	for _, at := range types {
		// 告警 ID 必须符合 OWL-<类别>-<编号> 规范
		require.Regexp(t, `^OWL-[A-Z]{3}-\d{3}$`, at.ID, "告警 ID 格式不合法: %s", at.ID)
		require.False(t, seen[at.ID], "告警 ID 重复: %s", at.ID)
		seen[at.ID] = true

		require.True(t, at.Builtin, "内置类型应标记 builtin")
		require.NotEmpty(t, at.Name, "告警类型 %s 缺中文名", at.ID)
		require.Contains(t, []Severity{SeverityInfo, SeverityWarning, SeverityCritical}, at.DefaultSeverity)
	}

	// 关键种子必须存在（决策中的核心场景）
	for _, want := range []string{
		"OWL-DSK-001", "OWL-MEM-001", "OWL-CPU-001", "OWL-NET-001",
		"OWL-SVC-001", "OWL-ERR-002", "OWL-OSS-001",
	} {
		require.True(t, seen[want], "缺少内置告警类型 %s", want)
	}
}

// TestBuiltinAlertTypes_AutoApproveOffByDefault 验证自动执行放行默认全关。
func TestBuiltinAlertTypes_AutoApproveOffByDefault(t *testing.T) {
	for _, at := range BuiltinAlertTypes() {
		require.False(t, at.AutoApprove, "告警类型 %s 自动放行必须默认关闭", at.ID)
	}
}

// TestRuleParams_JSONRoundtrip 验证规则参数可 JSON 序列化（存储于 default_params）。
func TestRuleParams_JSONRoundtrip(t *testing.T) {
	p := RuleParams{Metric: "disk.usage.", Op: ">", Value: 90, Duration: 2}
	data, err := json.Marshal(p)
	require.NoError(t, err)

	var back RuleParams
	require.NoError(t, json.Unmarshal(data, &back))
	require.Equal(t, p, back)
}

// TestFindAlertType 验证按 ID 查找内置告警类型。
func TestFindAlertType(t *testing.T) {
	at, ok := FindAlertType("OWL-DSK-001")
	require.True(t, ok)
	require.Equal(t, "磁盘使用率过高", at.Name)

	_, ok = FindAlertType("OWL-NOPE-001")
	require.False(t, ok)
}
