package monitor

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseFree 解析 free -m 输出，产出 mem.used_pct 与 mem.swap_pct 两个指标。
// 优先使用 available 列计算内存使用率；无 available 时回退
// (total-free-buff/cache)/total。
func ParseFree(raw, nodeID string, ts int64) ([]Sample, error) {
	samples := make([]Sample, 0, 2)

	// 先识别表头是否含 available 列（procps-ng 新格式），
	// 旧格式为 buffers/cached 两列且无 available。
	hasAvailable := false
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(line, "total") && strings.Contains(line, "used") {
			for _, f := range strings.Fields(line) {
				if f == "available" {
					hasAvailable = true
					break
				}
			}
			break
		}
	}

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || !strings.HasSuffix(fields[0], ":") {
			continue
		}
		switch fields[0] {
		case "Mem:":
			// total used free shared buff/cache [available]
			total, err := strconv.ParseFloat(fields[1], 64)
			if err != nil || total <= 0 {
				return nil, fmt.Errorf("free: 解析 Mem.total 失败: %q", line)
			}
			var usedPct float64
			if hasAvailable && len(fields) >= 7 {
				available, err := strconv.ParseFloat(fields[6], 64)
				if err != nil {
					return nil, fmt.Errorf("free: 解析 Mem.available 失败: %q", line)
				}
				usedPct = (total - available) / total * 100
			} else {
				free, err := strconv.ParseFloat(fields[3], 64)
				if err != nil {
					return nil, fmt.Errorf("free: 解析 Mem.free 失败: %q", line)
				}
				buffers := 0.0
				cached := 0.0
				if len(fields) >= 6 {
					buffers, _ = strconv.ParseFloat(fields[5], 64)
				}
				if len(fields) >= 7 {
					cached, _ = strconv.ParseFloat(fields[6], 64)
				}
				usedPct = (total - free - buffers - cached) / total * 100
			}
			samples = append(samples, Sample{NodeID: nodeID, Metric: "mem.used_pct", TS: ts, Value: usedPct})
		case "Swap:":
			total, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return nil, fmt.Errorf("free: 解析 Swap.total 失败: %q", line)
			}
			used, err := strconv.ParseFloat(fields[2], 64)
			if err != nil {
				return nil, fmt.Errorf("free: 解析 Swap.used 失败: %q", line)
			}
			swapPct := 0.0
			if total > 0 {
				swapPct = used / total * 100
			}
			samples = append(samples, Sample{NodeID: nodeID, Metric: "mem.swap_pct", TS: ts, Value: swapPct})
		}
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("free: 未解析到 Mem/Swap 行")
	}
	return samples, nil
}
