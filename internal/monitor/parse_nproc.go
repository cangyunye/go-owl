package monitor

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseNproc 解析 nproc 输出为 CPU 核数（sys.cores），供负载类规则动态阈值。
func ParseNproc(raw, nodeID string, ts int64) ([]Sample, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return nil, fmt.Errorf("nproc: 解析失败: %w", err)
	}
	return []Sample{{NodeID: nodeID, Metric: "sys.cores", TS: ts, Value: v}}, nil
}
