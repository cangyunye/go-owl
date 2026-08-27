package monitor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reEstab    = regexp.MustCompile(`estab\s+(\d+)`)
	reTimewait = regexp.MustCompile(`timewait\s+(\d+)`)
)

// ParseSS 解析 ss -s 输出，产出 net.tcp_estab 与 net.tcp_timewait。
// 只取 "TCP:   N (estab X, ..., timewait Y, ...)" 行内的计数。
func ParseSS(raw, nodeID string, ts int64) ([]Sample, error) {
	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "TCP:") {
			continue
		}
		values := make([]float64, 2)
		names := []string{"net.tcp_estab", "net.tcp_timewait"}
		pairs := []struct {
			re  *regexp.Regexp
			idx int
		}{{reEstab, 0}, {reTimewait, 1}}
		for _, p := range pairs {
			m := p.re.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("ss: TCP 行缺少 %s 计数: %q", names[p.idx], line)
			}
			v, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				return nil, fmt.Errorf("ss: 解析 %s 失败: %w", names[p.idx], err)
			}
			values[p.idx] = v
		}
		samples := []Sample{
			{NodeID: nodeID, Metric: names[0], TS: ts, Value: values[0]},
			{NodeID: nodeID, Metric: names[1], TS: ts, Value: values[1]},
		}
		return samples, nil
	}
	return nil, nil
}
