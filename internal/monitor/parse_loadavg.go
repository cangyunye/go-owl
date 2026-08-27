package monitor

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseLoadavg 解析 /proc/loadavg 输出，产出 load.load1/load5/load15 三个指标。
// 输入形如 "0.52 0.47 0.41 2/345 12345"。
func ParseLoadavg(raw, nodeID string, ts int64) ([]Sample, error) {
	fields := strings.Fields(raw)
	if len(fields) < 3 {
		return nil, fmt.Errorf("loadavg: 字段不足: %q", strings.TrimSpace(raw))
	}
	values := make([]float64, 3)
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return nil, fmt.Errorf("loadavg: 解析第 %d 个字段失败: %w", i+1, err)
		}
		values[i] = v
	}
	names := []string{"load.load1", "load.load5", "load.load15"}
	samples := make([]Sample, 3)
	for i, name := range names {
		samples[i] = Sample{NodeID: nodeID, Metric: name, TS: ts, Value: values[i]}
	}
	return samples, nil
}
