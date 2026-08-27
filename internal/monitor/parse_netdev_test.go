package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseNetDev 验证 /proc/net/dev 输出解析为每网卡收发字节计数。
func TestParseNetDev(t *testing.T) {
	raw := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 12345678   12345    0    0    0     0          0         0 12345678   12345    0    0    0     0       0          0
  eth0: 1000000000  500000    0    0    0     0          0         0  50000000  250000    0    0    0     0       0          0
`

	samples, err := ParseNetDev(raw, "node-a", 1750000000)
	require.NoError(t, err)
	require.Equal(t, []Sample{
		{NodeID: "node-a", Metric: "net.rx_bytes.lo", TS: 1750000000, Value: 12345678},
		{NodeID: "node-a", Metric: "net.tx_bytes.lo", TS: 1750000000, Value: 12345678},
		{NodeID: "node-a", Metric: "net.rx_bytes.eth0", TS: 1750000000, Value: 1000000000},
		{NodeID: "node-a", Metric: "net.tx_bytes.eth0", TS: 1750000000, Value: 50000000},
	}, samples)
}

// TestParseNetDev_NoData 验证无网卡数据时不报错、返回空。
func TestParseNetDev_NoData(t *testing.T) {
	samples, err := ParseNetDev("Inter-|   Receive   |  Transmit\n face |bytes    |bytes\n", "node-a", 0)
	require.NoError(t, err)
	require.Empty(t, samples)
}
