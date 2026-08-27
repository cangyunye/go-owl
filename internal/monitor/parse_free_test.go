package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseFree 验证 free -m 输出解析为内存/swap 使用率。
// mem.used_pct 基于 available（可用内存）计算，更贴近真实占用。
func TestParseFree(t *testing.T) {
	raw := `              total        used        free      shared  buff/cache   available
Mem:          15891        2352        2419         189       11120       12902
Swap:          2047           0        2047
`

	samples, err := ParseFree(raw, "node-a", 1750000000)
	require.NoError(t, err)
	require.Len(t, samples, 2)

	byName := map[string]float64{}
	for _, s := range samples {
		byName[s.Metric] = s.Value
	}
	// (15891-12902)/15891 = 18.81%
	require.InDelta(t, 18.81, byName["mem.used_pct"], 0.01)
	require.InDelta(t, 0, byName["mem.swap_pct"], 0.01)
}

// TestParseFree_NoAvailable 验证无 available 列时回退到 (total-free-buff/cache)/total。
func TestParseFree_NoAvailable(t *testing.T) {
	raw := `             total       used       free     shared    buffers     cached
Mem:          15891      13522       2369          0        200       11000
Swap:          2047       1024       1023
`

	samples, err := ParseFree(raw, "node-a", 1750000000)
	require.NoError(t, err)
	byName := map[string]float64{}
	for _, s := range samples {
		byName[s.Metric] = s.Value
	}
	// (15891-2369-200-11000)/15891 = 14.61%
	require.InDelta(t, 14.61, byName["mem.used_pct"], 0.01)
	// 1024/2047 = 50.02%
	require.InDelta(t, 50.02, byName["mem.swap_pct"], 0.01)
}
