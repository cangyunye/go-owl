package ssh

import (
	"sync"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/node"
)

// poolEntryCount 统计池内条目数（仅测试可见，同包）。
func poolEntryCount(p *ConnectionPool) int {
	n := 0
	p.pool.Range(func(key, value interface{}) bool {
		n++
		return true
	})
	return n
}

// localNode 构造走 LocalNodeExecutor（127.0.0.1，无真实网络）的节点信息。
func localNode(id string) *node.ResolvedNode {
	return &node.ResolvedNode{ID: id, Address: "127.0.0.1", Port: 22, User: "test"}
}

// TestConnectionPool_CloseStopsCleanup 验证 Close 后 cleanup goroutine 退出
// （done channel 关闭），修复泄漏的常驻 goroutine；Close 幂等。
func TestConnectionPool_CloseStopsCleanup(t *testing.T) {
	p := NewConnectionPool(2, time.Minute)

	select {
	case <-p.done:
		t.Fatal("Close 前 done 不应关闭（cleanup 应在运行）")
	default:
	}

	p.Close()

	select {
	case <-p.done:
		// cleanup 已退出
	default:
		t.Fatal("Close 后 done 应关闭（cleanup goroutine 应退出）")
	}

	// 幂等：重复 Close 不 panic
	p.Close()

	// Close 后池清空
	if n := poolEntryCount(p); n != 0 {
		t.Fatalf("Close 后池应为空, got %d entries", n)
	}
}

// TestConnectionPool_PutEvictsOldestBeyondMaxIdle 验证空闲条目超过 maxIdle
// 时按最久未使用淘汰（此前 maxIdle 只存不用，条目无上限）。
func TestConnectionPool_PutEvictsOldestBeyondMaxIdle(t *testing.T) {
	const maxIdle = 2
	p := NewConnectionPool(maxIdle, time.Minute)
	defer p.Close()

	// 依次借用并归还 3 个节点，条目 lastUsed 递增
	for _, id := range []string{"n1", "n2", "n3"} {
		if _, err := p.Get(localNode(id)); err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		time.Sleep(20 * time.Millisecond) // 保证 lastUsed 可区分先后
		p.Put(id)
	}

	// 空闲条目应被淘汰到 maxIdle 以内，且淘汰的是最旧的 n1
	if n := poolEntryCount(p); n > maxIdle {
		t.Fatalf("空闲条目 %d 超过 maxIdle=%d", n, maxIdle)
	}
	if _, ok := p.pool.Load("n1"); ok {
		t.Fatal("最久未用的 n1 应被淘汰")
	}
	for _, id := range []string{"n2", "n3"} {
		if _, ok := p.pool.Load(id); !ok {
			t.Fatalf("最近使用的 %s 不应被淘汰", id)
		}
	}
}

// TestConnectionPool_CleanupEvictsExpiredIdle 验证 cleanup 只清理
// 空闲超期条目，被引用（refCount>0）条目不清理。
func TestConnectionPool_CleanupEvictsExpiredIdle(t *testing.T) {
	p := NewConnectionPool(10, 30*time.Millisecond)
	defer p.Close()

	exec, err := p.Get(localNode("n1"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	p.Put("n1")

	// 等待 cleanup（tick = idleTimeout/2）至少跑几轮
	time.Sleep(120 * time.Millisecond)

	if _, ok := p.pool.Load("n1"); ok {
		t.Fatal("空闲超期条目应被 cleanup 清理")
	}

	// 被引用条目即使超期也不清理
	if _, err := p.Get(localNode("n2")); err != nil {
		t.Fatalf("Get n2: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	if _, ok := p.pool.Load("n2"); !ok {
		t.Fatal("refCount>0 的条目不应被 cleanup 清理")
	}
	_ = exec
}

// TestConnectionPool_ReuseIdleThenBusyCreatesNew 验证：
// 归还后复用同一条目；条目使用中（refCount>0）时新建条目且不 Delete 被引用条目。
// 注：LocalNodeExecutor 是零大小结构体，指针比较恒等，故以池条目与 refCount 行为断言。
func TestConnectionPool_ReuseIdleThenBusyCreatesNew(t *testing.T) {
	p := NewConnectionPool(10, time.Minute)
	defer p.Close()

	if _, err := p.Get(localNode("n1")); err != nil {
		t.Fatalf("Get 1: %v", err)
	}
	p.Put("n1")

	entryBefore, ok := p.pool.Load("n1")
	if !ok {
		t.Fatal("n1 条目应存在")
	}
	if _, err := p.Get(localNode("n1")); err != nil {
		t.Fatalf("Get 2: %v", err)
	}
	// 空闲复用：条目不替换，refCount 0 → 1
	entryAfterReuse, _ := p.pool.Load("n1")
	if entryAfterReuse != entryBefore {
		t.Fatal("空闲复用不应替换条目")
	}
	if refCount := entryAfterReuse.(*poolEntry).refCount; refCount != 1 {
		t.Fatalf("空闲复用后 refCount 应为 1, got %d", refCount)
	}

	// 条目使用中：第二次 Get 新建条目（不 Delete 被引用条目）
	if _, err := p.Get(localNode("n1")); err != nil {
		t.Fatalf("Get 3: %v", err)
	}
	entryAfterBusy, _ := p.pool.Load("n1")
	if entryAfterBusy == entryBefore {
		t.Fatal("使用中的条目不应被复用，应新建条目替换")
	}
	if refCount := entryAfterBusy.(*poolEntry).refCount; refCount != 1 {
		t.Fatalf("替换后的新条目 refCount 应为 1, got %d", refCount)
	}

	// 两次 Put（新旧条目各一）后 refCount 不出现负数
	p.Put("n1")
	p.Put("n1")
	if refCount := entryAfterBusy.(*poolEntry).refCount; refCount != 0 {
		t.Fatalf("两次 Put 后 refCount 应为 0, got %d", refCount)
	}
}

// TestConnectionPool_ConcurrentGetPutNoNegativeRefCount 并发 Get/Put 压力：
// 配合 -race 运行，断言任意条目 refCount 永不为负、无 panic。
func TestConnectionPool_ConcurrentGetPutNoNegativeRefCount(t *testing.T) {
	p := NewConnectionPool(4, time.Minute)
	defer p.Close()

	ids := []string{"n1", "n2", "n3", "n4", "n5"}
	const workers = 32
	const loops = 50

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < loops; i++ {
				id := ids[(w+i)%len(ids)]
				if _, err := p.Get(localNode(id)); err != nil {
					t.Errorf("Get %s: %v", id, err)
					return
				}
				p.Put(id)
			}
		}(w)
	}
	wg.Wait()

	p.pool.Range(func(key, value interface{}) bool {
		e := value.(*poolEntry)
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.refCount < 0 {
			t.Errorf("节点 %v refCount 为负: %d", key, e.refCount)
		}
		return true
	})
}
