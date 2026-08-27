package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRuleParams_Evaluate_Prefix 验证前缀匹配规则取最坏值判定（磁盘多挂载点）。
func TestRuleParams_Evaluate_Prefix(t *testing.T) {
	p := RuleParams{Metric: "disk.usage.", Op: ">", Value: 90, Duration: 2}
	res, err := p.Evaluate([]Sample{
		{NodeID: "n1", Metric: "disk.usage./", TS: 1, Value: 93.5},
		{NodeID: "n1", Metric: "disk.usage./boot/efi", TS: 1, Value: 2},
	}, "n1")
	require.NoError(t, err)
	require.True(t, res.Matched)
	require.Equal(t, "disk.usage./", res.Metric)
	require.InDelta(t, 93.5, res.Value, 0.001)
}

// TestRuleParams_Evaluate_NoMatch 验证未达阈值不触发。
func TestRuleParams_Evaluate_NoMatch(t *testing.T) {
	p := RuleParams{Metric: "mem.used_pct", Op: ">", Value: 90, Duration: 3}
	res, err := p.Evaluate([]Sample{{NodeID: "n1", Metric: "mem.used_pct", TS: 1, Value: 85}}, "n1")
	require.NoError(t, err)
	require.False(t, res.Matched)
}

// TestRuleParams_Evaluate_PerCore 验证负载规则按核数动态阈值（load1 > cores×1.5）。
func TestRuleParams_Evaluate_PerCore(t *testing.T) {
	p := RuleParams{Metric: "load.load1", Op: ">", PerCore: 1.5, Duration: 3}

	// cores=8 → 阈值 12；load1=13 触发
	res, err := p.Evaluate([]Sample{
		{NodeID: "n1", Metric: "sys.cores", TS: 1, Value: 8},
		{NodeID: "n1", Metric: "load.load1", TS: 1, Value: 13},
	}, "n1")
	require.NoError(t, err)
	require.True(t, res.Matched)

	// load1=10 不触发
	res, err = p.Evaluate([]Sample{
		{NodeID: "n1", Metric: "sys.cores", TS: 1, Value: 8},
		{NodeID: "n1", Metric: "load.load1", TS: 1, Value: 10},
	}, "n1")
	require.NoError(t, err)
	require.False(t, res.Matched)
}

// TestRuleParams_Evaluate_LessThan 验证 "<" 语义（服务停止：active=0 < 1）。
func TestRuleParams_Evaluate_LessThan(t *testing.T) {
	p := RuleParams{Metric: "svc.active.", Op: "<", Value: 1, Duration: 2}
	res, err := p.Evaluate([]Sample{{NodeID: "n1", Metric: "svc.active.nginx", TS: 1, Value: 0}}, "n1")
	require.NoError(t, err)
	require.True(t, res.Matched)
	require.Equal(t, "svc.active.nginx", res.Metric)
}

// TestRuleParams_Evaluate_NoMetric 验证无匹配指标时不触发也不报错。
func TestRuleParams_Evaluate_NoMetric(t *testing.T) {
	p := RuleParams{Metric: "err.oom", Op: ">", Value: 0, Duration: 1}
	res, err := p.Evaluate([]Sample{{NodeID: "n1", Metric: "load.load1", TS: 1, Value: 0.5}}, "n1")
	require.NoError(t, err)
	require.False(t, res.Matched)
}
