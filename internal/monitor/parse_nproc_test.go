package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseNproc 验证 nproc 输出解析为 CPU 核数。
func TestParseNproc(t *testing.T) {
	samples, err := ParseNproc("8\n", "node-a", 1750000000)
	require.NoError(t, err)
	require.Equal(t, []Sample{
		{NodeID: "node-a", Metric: "sys.cores", TS: 1750000000, Value: 8},
	}, samples)
}
