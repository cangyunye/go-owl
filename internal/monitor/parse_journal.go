package monitor

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseJournalCount 解析计数类命令输出（wc -l / grep -c 风格，单个整数），
// 产出指定指标的一个采样。供 err.journal_errors（近 5 分钟错误日志条数）
// 与 err.oom（近 5 分钟内核 OOM 事件数）使用。
func ParseJournalCount(raw, nodeID, metric string, ts int64) ([]Sample, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return nil, fmt.Errorf("journal: 计数解析失败: %q", strings.TrimSpace(raw))
	}
	return []Sample{{NodeID: nodeID, Metric: metric, TS: ts, Value: v}}, nil
}
