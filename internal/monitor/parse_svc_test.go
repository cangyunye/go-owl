package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseSystemctlShow 验证 systemctl show 多 unit 块输出解析：
// active=1/其余=0、NRestarts 透传、not-found unit 跳过、别名重复块去重。
func TestParseSystemctlShow(t *testing.T) {
	raw := `Id=ssh.service
LoadState=loaded
ActiveState=active
NRestarts=1

Id=ssh.service
LoadState=loaded
ActiveState=active
NRestarts=1

Id=nginx.service
LoadState=loaded
ActiveState=failed
NRestarts=2

Id=nonexistent-xyz.service
LoadState=not-found
ActiveState=inactive
NRestarts=0
`

	samples, err := ParseSystemctlShow(raw, "node-a", 1750000000)
	require.NoError(t, err)

	byMetric := map[string]float64{}
	for _, s := range samples {
		require.Equal(t, "node-a", s.NodeID)
		require.Equal(t, int64(1750000000), s.TS)
		byMetric[s.Metric] = s.Value
	}
	// 别名场景：systemctl 把 sshd 解析为规范 Id=ssh.service，两个同名块
	// 后者覆盖前者，不产生重复指标
	require.Equal(t, 4, len(byMetric), "ssh/sshd 别名去重后应为 2 unit × 2 指标")
	require.InDelta(t, 1, byMetric["svc.active.ssh"], 0.001)
	require.InDelta(t, 1, byMetric["svc.restarts.ssh"], 0.001)
	require.InDelta(t, 0, byMetric["svc.active.nginx"], 0.001, "failed 非 active → 0")
	require.InDelta(t, 2, byMetric["svc.restarts.nginx"], 0.001)
	_, hasNotFound := byMetric["svc.active.nonexistent-xyz"]
	require.False(t, hasNotFound, "not-found unit 应被跳过")
}

// TestParseSystemctlShow_Empty 验证空输出不报错、不产出指标。
func TestParseSystemctlShow_Empty(t *testing.T) {
	samples, err := ParseSystemctlShow("", "node-a", 1750000000)
	require.NoError(t, err)
	require.Empty(t, samples)
}

// TestParseSystemctlShow_EofFlush 验证最后一个 unit 块（无尾随空行）也被解析。
func TestParseSystemctlShow_EofFlush(t *testing.T) {
	raw := "Id=cron.service\nLoadState=loaded\nActiveState=inactive\nNRestarts=0\n"
	samples, err := ParseSystemctlShow(raw, "node-a", 1750000000)
	require.NoError(t, err)
	require.Len(t, samples, 2)
	byMetric := map[string]float64{}
	for _, s := range samples {
		byMetric[s.Metric] = s.Value
	}
	require.InDelta(t, 0, byMetric["svc.active.cron"], 0.001, "inactive → 0")
	require.InDelta(t, 0, byMetric["svc.restarts.cron"], 0.001)
}
