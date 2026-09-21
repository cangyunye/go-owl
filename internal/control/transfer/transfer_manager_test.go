package transfer

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/node"
	"github.com/stretchr/testify/require"
)

// peakGauge 记录并发执行峰值（-race 安全）。
type peakGauge struct {
	cur  atomic.Int64
	peak atomic.Int64
}

func (g *peakGauge) enter() func() {
	cur := g.cur.Add(1)
	for {
		peak := g.peak.Load()
		if cur <= peak || g.peak.CompareAndSwap(peak, cur) {
			break
		}
	}
	return func() { g.cur.Add(-1) }
}

// TestUploadEffectiveConcurrency 验证上传并发上限取值规则：
// 未设置/0/负数 → 默认 10；显式设置 → 使用设置值。
func TestUploadEffectiveConcurrency(t *testing.T) {
	require.Equal(t, defaultMaxConcurrency, effectiveUploadConcurrency(nil))
	require.Equal(t, 10, effectiveUploadConcurrency(&UploadOptions{}))
	require.Equal(t, 10, effectiveUploadConcurrency(&UploadOptions{MaxConcurrency: 0}))
	require.Equal(t, 10, effectiveUploadConcurrency(&UploadOptions{MaxConcurrency: -1}))
	require.Equal(t, 3, effectiveUploadConcurrency(&UploadOptions{MaxConcurrency: 3}))
}

// TestParallelLimited_RespectsLimitAndOrder 验证受限并发执行：
// 峰值并发 ≤ 上限、结果按输入顺序一一对应（配合 -race）。
func TestParallelLimited_RespectsLimitAndOrder(t *testing.T) {
	const limit = 3
	ids := make([]string, 40)
	for i := range ids {
		ids[i] = "node-" + strconv.Itoa(i)
	}

	g := &peakGauge{}
	var mu sync.Mutex
	counts := map[string]int{}

	got := parallelLimited(context.Background(), ids, limit, func(ctx context.Context, id string) string {
		defer g.enter()()
		time.Sleep(time.Millisecond)
		mu.Lock()
		counts[id]++
		mu.Unlock()
		return id
	})

	require.Equal(t, ids, got, "结果必须按输入顺序与输入一一对应")
	for id, n := range counts {
		require.Equal(t, 1, n, "节点 %s 应恰好执行一次", id)
	}
	require.LessOrEqual(t, g.peak.Load(), int64(limit), "并发峰值不得超过上限")
	require.Positive(t, g.peak.Load(), "应有实际并发发生")
}

// TestUpload_AllNodesComplete 端到端验证：并行上传为每个节点产出一条结果
// 且顺序与输入一致。未知节点走解析失败路径（不依赖真实网络），
// 与现有调用方（不设置 MaxConcurrency）兼容 —— 使用默认上限。
func TestUpload_AllNodesComplete(t *testing.T) {
	tm := NewTransferManager(node.NewNodeResolver())
	defer tm.Close()

	ids := make([]string, 20)
	for i := range ids {
		ids[i] = "node-" + strconv.Itoa(i)
	}

	// 并行（默认并发上限）
	results := tm.Upload(context.Background(), ids, "/tmp/local-file", "/tmp/remote-file", &UploadOptions{Parallel: true})
	require.Len(t, results, len(ids))
	for i, r := range results {
		require.Equal(t, ids[i], r.NodeID, "结果必须按输入顺序排列")
		require.Error(t, r.Error, "未知节点应返回解析失败错误")
	}

	// 顺序路径行为不变
	results = tm.Upload(context.Background(), ids, "/tmp/local-file", "/tmp/remote-file", &UploadOptions{Parallel: false})
	require.Len(t, results, len(ids))
	for i, r := range results {
		require.Equal(t, ids[i], r.NodeID)
		require.Error(t, r.Error)
	}
}
