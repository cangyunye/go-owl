package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExecutionsBatch 单个执行批次的日志占用。
type ExecutionsBatch struct {
	OpID      string    `json:"op_id"`
	Files     int       `json:"files"`
	SizeBytes int64     `json:"size_bytes"`
	ModTime   time.Time `json:"mod_time"`
}

// ExecutionsSummary 全部执行日志批次的占用汇总。
type ExecutionsSummary struct {
	Batches        []ExecutionsBatch `json:"batches"`
	TotalBatches   int               `json:"total_batches"`
	TotalSizeBytes int64             `json:"total_size_bytes"`
}

// ExecutionsSummary 汇总执行日志根目录下所有批次的文件数与字节占用。
func SummarizeExecutions() (ExecutionsSummary, error) {
	var sum ExecutionsSummary
	entries, err := os.ReadDir(ExecutionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return sum, nil
		}
		return sum, err
	}
	for _, e := range entries {
		if !e.IsDir() || !isBatchDirName(e.Name()) {
			continue
		}
		batch, err := summarizeBatch(filepath.Join(ExecutionsDir(), e.Name()), e.Name())
		if err != nil {
			continue // 读不到的批次跳过，不影响整体汇总
		}
		sum.Batches = append(sum.Batches, batch)
		sum.TotalSizeBytes += batch.SizeBytes
	}
	sum.TotalBatches = len(sum.Batches)
	return sum, nil
}

func summarizeBatch(dir, opID string) (ExecutionsBatch, error) {
	batch := ExecutionsBatch{OpID: opID}
	fi, err := os.Stat(dir)
	if err != nil {
		return batch, err
	}
	batch.ModTime = fi.ModTime()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return batch, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if e.Name() != manifestName && filepath.Ext(e.Name()) == ".log" {
			batch.Files++ // Files 只数节点日志，manifest 属元数据
		}
		batch.SizeBytes += info.Size()
	}
	return batch, nil
}

// CleanupExecutions 删除最后修改时间早于 olderThan 的批次目录，
// 返回删除的批次数与回收字节数。目录名不是合法批次 ID 的一律不触碰；
// 根目录不存在视为空，幂等。
func CleanupExecutions(olderThan time.Duration) (removed int, reclaimedBytes int64, err error) {
	entries, err := os.ReadDir(ExecutionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	cutoff := time.Now().Add(-olderThan)
	for _, e := range entries {
		if !e.IsDir() || !isBatchDirName(e.Name()) {
			continue
		}
		dir := filepath.Join(ExecutionsDir(), e.Name())
		fi, err := os.Stat(dir)
		if err != nil {
			continue
		}
		if fi.ModTime().After(cutoff) {
			continue
		}
		batch, err := summarizeBatch(dir, e.Name())
		if err != nil {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return removed, reclaimedBytes, fmt.Errorf("删除执行日志批次 %s 失败: %w", e.Name(), err)
		}
		removed++
		reclaimedBytes += batch.SizeBytes
	}
	return removed, reclaimedBytes, nil
}

// isBatchDirName 批次目录名与 sanitizeID 产出的字符集一致（字母/数字/-）。
func isBatchDirName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}
