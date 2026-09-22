package command

import (
	"context"
	"fmt"
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

// enter 进入工作区，返回退出函数。
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

// TestEffectiveMaxConcurrency 验证并发上限取值规则：
// 未设置/0/负数 → 默认 10；显式设置 → 使用设置值。
func TestEffectiveMaxConcurrency(t *testing.T) {
	require.Equal(t, defaultMaxConcurrency, effectiveConcurrency(nil))
	require.Equal(t, 10, effectiveConcurrency(&ExecuteOptions{}))
	require.Equal(t, 10, effectiveConcurrency(&ExecuteOptions{MaxConcurrency: 0}))
	require.Equal(t, 10, effectiveConcurrency(&ExecuteOptions{MaxConcurrency: -3}))
	require.Equal(t, 5, effectiveConcurrency(&ExecuteOptions{MaxConcurrency: 5}))
	require.Equal(t, 1, effectiveConcurrency(&ExecuteOptions{MaxConcurrency: 1}))
}

// TestRunParallelLimited_RespectsLimit 验证完成序并发执行：
// 峰值并发 ≤ 上限、全部任务恰好执行一次（配合 -race）。
func TestRunParallelLimited_RespectsLimit(t *testing.T) {
	const limit = 3
	const total = 50

	g := &peakGauge{}
	var mu sync.Mutex
	executed := make(map[string]int)

	got := runParallelLimited(context.Background(), nodeIDs(total), limit,
		func(ctx context.Context, id string) int {
			defer g.enter()()
			time.Sleep(time.Millisecond) // 放大并发窗口
			mu.Lock()
			executed[id]++
			mu.Unlock()
			return len(id)
		})

	require.Len(t, got, total, "全部任务的结果都应被聚合")
	for id, n := range executed {
		require.Equal(t, 1, n, "节点 %s 应恰好执行一次", id)
	}
	require.LessOrEqual(t, g.peak.Load(), int64(limit), "并发峰值不得超过上限")
	require.Positive(t, g.peak.Load(), "应有实际并发发生")
}

// TestRunParallelLimited_CancelledContext 验证 ctx 已取消时不死锁、
// 快速返回且不产生结果（与原实现的跳过语义一致）。
func TestRunParallelLimited_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan []int, 1)
	go func() {
		done <- runParallelLimited(ctx, []string{"a", "b", "c"}, 2,
			func(ctx context.Context, id string) int { return 1 })
	}()

	select {
	case got := <-done:
		require.Empty(t, got, "已取消的 ctx 不应产出结果")
	case <-time.After(2 * time.Second):
		t.Fatal("取消后发生死锁")
	}
}

// TestRunIndexedLimited_KeepsOrderAndLimit 验证输入序并发执行：
// 结果按输入顺序排列、峰值并发 ≤ 上限。
func TestRunIndexedLimited_KeepsOrderAndLimit(t *testing.T) {
	const limit = 2
	ids := nodeIDs(30)

	g := &peakGauge{}
	got := runIndexedLimited(context.Background(), ids, limit,
		func(ctx context.Context, id string) string {
			defer g.enter()()
			time.Sleep(time.Millisecond)
			return id
		})

	require.Equal(t, ids, got, "结果必须按输入顺序排列")
	require.LessOrEqual(t, g.peak.Load(), int64(limit), "并发峰值不得超过上限")
	require.Positive(t, g.peak.Load(), "应有实际并发发生")
}

// TestRunParallel_AllNodesComplete 端到端验证：批量节点并行执行全部完成，
// 结果与节点一一对应（未知节点走解析失败路径，不依赖真实 SSH）。
func TestRunParallel_AllNodesComplete(t *testing.T) {
	e := NewExecutor(node.NewNodeResolver())
	defer e.Close()

	const total = 25
	ids := nodeIDs(total)
	opts := &ExecuteOptions{Parallel: true, Timeout: 5 * time.Second}

	results := e.Run(context.Background(), ids, "echo hi", opts)

	require.Len(t, results, total, "每个节点都应有结果")
	got := make(map[string]bool, total)
	for _, r := range results {
		got[r.NodeID] = true
	}
	for _, id := range ids {
		require.True(t, got[id], "节点 %s 缺少结果", id)
	}
}

func nodeIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("node-%03d", i)
	}
	return ids
}
