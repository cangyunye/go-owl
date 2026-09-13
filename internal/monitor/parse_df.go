package monitor

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseDF 解析 df 输出（-P 固定列宽 / -Pi inode 模式），产出每挂载点使用率。
// prefix 为指标前缀：disk.usage 或 disk.inodes；指标名为 prefix + 挂载点
// （如 disk.usage./ 、disk.inodes./boot/efi）。
func ParseDF(raw, nodeID string, ts int64, prefix string) ([]Sample, error) {
	lines := strings.Split(raw, "\n")
	samples := make([]Sample, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Filesystem") {
			continue
		}
		fields := strings.Fields(line)
		// df -P: Filesystem 1024-blocks Used Available Capacity Mounted on（6 列）
		// df -Pi: Filesystem Inodes IUsed IFree IUse% Mounted on（6 列）
		if len(fields) < 6 {
			return nil, fmt.Errorf("df: 第 %d 行列数不足: %q", i+1, line)
		}
		pctStr := strings.TrimSuffix(fields[4], "%")
		pct, err := strconv.ParseFloat(pctStr, 64)
		if err != nil {
			// 虚拟文件系统（如 WSL drivers 挂载）的 IUse% 可能为 "-"：
			// 跳过该行，不因单行坏数据丢弃其余挂载点的指标
			continue
		}
		mount := fields[5]
		samples = append(samples, Sample{
			NodeID: nodeID,
			Metric: prefix + "." + mount,
			TS:     ts,
			Value:  pct,
		})
	}
	return samples, nil
}
