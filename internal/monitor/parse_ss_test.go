package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseSS 验证 ss -s 输出解析为 TCP estab/timewait 连接数。
func TestParseSS(t *testing.T) {
	raw := `Total: 128 (kernel 96)
TCP:   12 (estab 4, closed 3, orphaned 0, timewait 5, transports 12), 
Transport Total     IP        IPv6
*	  12        -         -
TCP	  12        11        1
`

	samples, err := ParseSS(raw, "node-a", 1750000000)
	require.NoError(t, err)
	require.Equal(t, []Sample{
		{NodeID: "node-a", Metric: "net.tcp_estab", TS: 1750000000, Value: 4},
		{NodeID: "node-a", Metric: "net.tcp_timewait", TS: 1750000000, Value: 5},
	}, samples)
}

// TestParseSS_NoTCP 验证无 TCP 行时返回空而非报错。
func TestParseSS_NoTCP(t *testing.T) {
	samples, err := ParseSS("Total: 10 (kernel 10)\nUDP: 2 (estab 1)\n", "node-a", 0)
	require.NoError(t, err)
	require.Empty(t, samples)
}
