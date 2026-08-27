package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseLoadavg 验证 /proc/loadavg 解析：前三个字段为 load1/load5/load15。
func TestParseLoadavg(t *testing.T) {
	raw := "0.52 0.47 0.41 2/345 12345\n"

	samples, err := ParseLoadavg(raw, "node-a", 1750000000)
	require.NoError(t, err)
	require.Equal(t, []Sample{
		{NodeID: "node-a", Metric: "load.load1", TS: 1750000000, Value: 0.52},
		{NodeID: "node-a", Metric: "load.load5", TS: 1750000000, Value: 0.47},
		{NodeID: "node-a", Metric: "load.load15", TS: 1750000000, Value: 0.41},
	}, samples)
}

// TestParseLoadavg_BadInput 验证非三字段输入返回错误。
func TestParseLoadavg_BadInput(t *testing.T) {
	_, err := ParseLoadavg("not-a-load\n", "node-a", 0)
	require.Error(t, err)
}
