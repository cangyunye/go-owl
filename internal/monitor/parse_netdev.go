package monitor

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseNetDev 解析 /proc/net/dev 输出，产出每网卡的累计收发字节计数
// （net.rx_bytes.<iface> / net.tx_bytes.<iface>）。速率由引擎按相邻
// 两次采样差值计算，这里只保留原始计数。
func ParseNetDev(raw, nodeID string, ts int64) ([]Sample, error) {
	samples := make([]Sample, 0, 8)
	for _, line := range strings.Split(raw, "\n") {
		if !strings.Contains(line, ":") {
			continue // 表头两行
		}
		parts := strings.SplitN(line, ":", 2)
		iface := strings.TrimSpace(parts[0])
		if iface == "" || iface == "face" {
			continue
		}
		fields := strings.Fields(parts[1])
		// rx_bytes=fields[0]，tx_bytes=fields[8]
		if len(fields) < 9 {
			return nil, fmt.Errorf("netdev: 网卡 %s 列数不足: %q", iface, line)
		}
		rx, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return nil, fmt.Errorf("netdev: 解析 %s rx_bytes 失败: %w", iface, err)
		}
		tx, err := strconv.ParseFloat(fields[8], 64)
		if err != nil {
			return nil, fmt.Errorf("netdev: 解析 %s tx_bytes 失败: %w", iface, err)
		}
		samples = append(samples,
			Sample{NodeID: nodeID, Metric: "net.rx_bytes." + iface, TS: ts, Value: rx},
			Sample{NodeID: nodeID, Metric: "net.tx_bytes." + iface, TS: ts, Value: tx},
		)
	}
	return samples, nil
}
