package ssh

import (
	"sort"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/node"
)

// ConnectionPool 按节点复用执行器。
// 注意：NodeExecutor（NativeNodeExecutor）本身无持久连接（每次执行独立拨号），
// 池复用的是已解析的执行器对象；条目释放即 drop 引用，无需额外关闭资源。
type ConnectionPool struct {
	pool        sync.Map // nodeID -> *poolEntry
	maxIdle     int      // 空闲条目上限（>0 时生效；<=0 = 不限制）
	idleTimeout time.Duration
	mu          sync.Mutex
	factory     *NodeExecutorFactory
	// done 关闭后 cleanup goroutine 退出（修复 Close 不停 cleanup 的泄漏）
	done      chan struct{}
	closeOnce sync.Once
}

type poolEntry struct {
	executor NodeExecutor
	nodeInfo *node.ResolvedNode
	lastUsed time.Time
	refCount int
	mu       sync.Mutex
}

func NewConnectionPool(maxIdle int, idleTimeout time.Duration) *ConnectionPool {
	if idleTimeout <= 0 {
		idleTimeout = 5 * time.Minute
	}
	pool := &ConnectionPool{
		maxIdle:     maxIdle,
		idleTimeout: idleTimeout,
		factory:     NewNodeExecutorFactory(),
		done:        make(chan struct{}),
	}

	go pool.cleanup()

	return pool
}

func (p *ConnectionPool) Get(nodeInfo *node.ResolvedNode) (NodeExecutor, error) {
	key := nodeInfo.ID

	if entry, ok := p.pool.Load(key); ok {
		e := entry.(*poolEntry)
		e.mu.Lock()
		// 仅复用完全空闲且未超期的条目
		if e.refCount == 0 && time.Since(e.lastUsed) < p.idleTimeout {
			e.lastUsed = time.Now()
			e.refCount++
			executor := e.executor
			e.mu.Unlock()
			return executor, nil
		}
		e.mu.Unlock()

		// 条目使用中（refCount>0）或空闲超期：新建执行器替换条目。
		// 不对被引用条目做 Delete（其持有者仍在使用返回的执行器值），
		// 旧条目由引用归零后的 Replace/Put 防护兜底。
	}

	executor, err := p.factory.GetExecutorForNode(
		nodeInfo.ID,
		nodeInfo.Address,
		nodeInfo.Port,
		nodeInfo.User,
		nodeInfo.SSHKey,
		nodeInfo.SSHPassword,
		nodeInfo.ProxyJump,
	)
	if err != nil {
		return nil, err
	}

	entry := &poolEntry{
		executor: executor,
		nodeInfo: nodeInfo,
		lastUsed: time.Now(),
		refCount: 1,
	}

	p.pool.Store(key, entry)

	return executor, nil
}

func (p *ConnectionPool) Put(nodeID string) {
	if entry, ok := p.pool.Load(nodeID); ok {
		e := entry.(*poolEntry)
		e.mu.Lock()
		// 防护：refCount 归零后不再递减（Get 替换条目等边界下避免负数）
		if e.refCount > 0 {
			e.refCount--
			if e.refCount == 0 {
				e.lastUsed = time.Now()
			}
		}
		e.mu.Unlock()
	}
	p.enforceMaxIdle()
}

func (p *ConnectionPool) cleanup() {
	ticker := time.NewTicker(p.idleTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.evictIdle()
		}
	}
}

// evictIdle 清理空闲超期且无引用的条目（refCount 归零才清理）。
func (p *ConnectionPool) evictIdle() {
	now := time.Now()
	p.pool.Range(func(key, value interface{}) bool {
		entry := value.(*poolEntry)
		entry.mu.Lock()
		idle := entry.refCount <= 0 && now.Sub(entry.lastUsed) >= p.idleTimeout
		entry.mu.Unlock()
		if idle {
			// 条目可能刚被 Get 复用，CompareAndDelete 保证只删原值
			p.pool.CompareAndDelete(key, value)
		}
		return true
	})
	p.enforceMaxIdle()
}

// enforceMaxIdle 空闲条目数超过 maxIdle 时按最久未使用淘汰最旧的条目。
func (p *ConnectionPool) enforceMaxIdle() {
	if p.maxIdle <= 0 {
		return
	}
	type idleKey struct {
		key      interface{}
		entry    *poolEntry
		lastUsed time.Time
	}
	var idles []idleKey
	idleCount := 0
	p.pool.Range(func(key, value interface{}) bool {
		entry := value.(*poolEntry)
		entry.mu.Lock()
		if entry.refCount <= 0 {
			idleCount++
			idles = append(idles, idleKey{key: key, entry: entry, lastUsed: entry.lastUsed})
		}
		entry.mu.Unlock()
		return true
	})
	if idleCount <= p.maxIdle {
		return
	}
	// 最久未用优先淘汰
	sort.Slice(idles, func(i, j int) bool { return idles[i].lastUsed.Before(idles[j].lastUsed) })
	for _, ik := range idles[:idleCount-p.maxIdle] {
		ik.entry.mu.Lock()
		referenced := ik.entry.refCount > 0
		ik.entry.mu.Unlock()
		if referenced {
			continue
		}
		p.pool.CompareAndDelete(ik.key, ik.entry)
	}
}

// Close 停止 cleanup goroutine 并清空池。幂等，可安全多次调用。
func (p *ConnectionPool) Close() {
	p.closeOnce.Do(func() {
		close(p.done)
	})
	p.pool.Range(func(key, value interface{}) bool {
		p.pool.CompareAndDelete(key, value)
		return true
	})
}

func (p *ConnectionPool) Stats() map[string]interface{} {
	var active, idle int
	p.pool.Range(func(key, value interface{}) bool {
		entry := value.(*poolEntry)
		entry.mu.Lock()
		if entry.refCount > 0 {
			active++
		} else {
			idle++
		}
		entry.mu.Unlock()
		return true
	})

	return map[string]interface{}{
		"active_connections": active,
		"idle_connections":   idle,
		"max_idle":           p.maxIdle,
		"idle_timeout":       p.idleTimeout.String(),
	}
}
