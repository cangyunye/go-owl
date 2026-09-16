package monitor

import (
	"fmt"
	"testing"
	"time"

	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/stretchr/testify/require"
)

// TestService_DebugCheck 验证调试执行：解析值/阈值判定/执行失败与无命令分支。
func TestService_DebugCheck(t *testing.T) {
	svc := &Service{}
	svc.debugExec = func(nodeID, command string, timeout time.Duration) (string, int, error) {
		if nodeID == "bad-node" {
			return "", 1, fmt.Errorf("dial tcp: connection refused")
		}
		return "93.5\n", 0, nil
	}
	at := owlmonitor.AlertType{ID: "OWL-CUS-1", CheckCmd: "check-queue", CheckMode: "value",
		DefaultParams: owlmonitor.RuleParams{Metric: "custom.owl-cus-1", Op: ">", Value: 90, Duration: 1}}

	// 正常：解析 93.5 > 90 → 会触发
	res, err := svc.DebugCheck("n1", at)
	require.NoError(t, err)
	require.NotNil(t, res.Value)
	require.InDelta(t, 93.5, *res.Value, 0.001)
	require.True(t, res.WouldTrigger)
	require.Equal(t, "custom.owl-cus-1", res.Metric)
	require.GreaterOrEqual(t, res.DurationMs, int64(0))

	// 未超阈值
	at.DefaultParams.Value = 95
	res, err = svc.DebugCheck("n1", at)
	require.NoError(t, err)
	require.False(t, res.WouldTrigger)

	// 执行失败：解析错误携带原因，不返回 error
	res, err = svc.DebugCheck("bad-node", at)
	require.NoError(t, err)
	require.False(t, res.WouldTrigger)
	require.Contains(t, res.ParseErr, "执行失败")

	// 无检查命令
	_, err = svc.DebugCheck("n1", owlmonitor.AlertType{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "没有自定义检查命令")
}
