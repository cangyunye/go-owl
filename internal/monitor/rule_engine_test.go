package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func diskRuleTypes() []AlertType {
	return []AlertType{{
		ID: "OWL-DSK-001", Name: "磁盘使用率过高", DefaultSeverity: SeverityWarning,
		DefaultParams: RuleParams{Metric: "disk.usage.", Op: ">", Value: 90, Duration: 2},
		Enabled:       true, Builtin: true,
	}}
}

func highDiskSamples(v float64) []Sample {
	return []Sample{{NodeID: "n1", Metric: "disk.usage./", TS: 1, Value: v}}
}

// TestRuleEngine_Tick_Duration 验证规则需连续满足 Duration 次才触发。
func TestRuleEngine_Tick_Duration(t *testing.T) {
	e := NewRuleEngine()
	types := diskRuleTypes()

	// 第一次满足：计数 1，未达窗口（duration=2）
	out := e.Tick("n1", highDiskSamples(93), types)
	require.Empty(t, out.Triggered, "首次满足不应触发")
	require.Equal(t, map[string]bool{"OWL-DSK-001": true}, out.Matched)

	// 第二次满足：触发
	out = e.Tick("n1", highDiskSamples(94), types)
	require.Len(t, out.Triggered, 1)
	require.Equal(t, "OWL-DSK-001", out.Triggered[0].AlertTypeID)
	require.InDelta(t, 94, out.Triggered[0].Value, 0.001)
}

// TestRuleEngine_Tick_Reset 验证条件不满足时计数清零，需重新积累。
func TestRuleEngine_Tick_Reset(t *testing.T) {
	e := NewRuleEngine()
	types := diskRuleTypes()

	e.Tick("n1", highDiskSamples(93), types) // 计数 1
	e.Tick("n1", highDiskSamples(80), types) // 未满足 → 清零
	out := e.Tick("n1", highDiskSamples(95), types)
	require.Empty(t, out.Triggered, "清零后需重新积累")

	out = e.Tick("n1", highDiskSamples(96), types)
	require.Len(t, out.Triggered, 1, "重新积累到窗口后触发")
}

// TestRuleEngine_Tick_PerNode 验证计数按节点隔离。
func TestRuleEngine_Tick_PerNode(t *testing.T) {
	e := NewRuleEngine()
	types := diskRuleTypes()

	e.Tick("n1", highDiskSamples(93), types)
	out := e.Tick("n2", highDiskSamples(91), types)
	require.Empty(t, out.Triggered, "n2 不应继承 n1 的计数")
}

// TestRuleEngine_Tick_Disabled 验证禁用类型不参与评估。
func TestRuleEngine_Tick_Disabled(t *testing.T) {
	e := NewRuleEngine()
	types := []AlertType{{
		ID: "OWL-MEM-001", Name: "内存过高", DefaultSeverity: SeverityWarning,
		DefaultParams: RuleParams{Metric: "mem.used_pct", Op: ">", Value: 90, Duration: 1},
		Enabled:       false,
	}}
	out := e.Tick("n1", []Sample{{NodeID: "n1", Metric: "mem.used_pct", TS: 1, Value: 99}}, types)
	require.Empty(t, out.Triggered)
	require.Empty(t, out.Matched)
}
