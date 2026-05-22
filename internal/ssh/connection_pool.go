package ssh

import (
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/node"
)

type ConnectionPool struct {
	pool        sync.Map // nodeID -> *poolEntry
	maxIdle     int
	idleTimeout time.Duration
	mu          sync.Mutex
	factory     *NodeExecutorFactory
}

type poolEntry struct {
	executor NodeExecutor
	nodeInfo *node.ResolvedNode
	lastUsed time.Time
	refCount int
	mu       sync.Mutex
}

func NewConnectionPool(maxIdle int, idleTimeout time.Duration) *ConnectionPool {
	pool := &ConnectionPool{
		maxIdle:     maxIdle,
		idleTimeout: idleTimeout,
		factory:     NewNodeExecutorFactory(),
	}

	go pool.cleanup()

	return pool
}

func (p *ConnectionPool) Get(nodeInfo *node.ResolvedNode) (NodeExecutor, error) {
	key := nodeInfo.ID

	if entry, ok := p.pool.Load(key); ok {
		e := entry.(*poolEntry)
		e.mu.Lock()
		defer e.mu.Unlock()

		if time.Since(e.lastUsed) < p.idleTimeout && e.refCount >= 0 {
			e.lastUsed = time.Now()
			e.refCount++
			return e.executor, nil
		}

		p.pool.Delete(key)
	}

	executor, err := p.factory.GetExecutorForNode(
		nodeInfo.ID,
		nodeInfo.Address,
		nodeInfo.Port,
		nodeInfo.User,
		nodeInfo.SSHKey,
		nodeInfo.SSHPassword,
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
		e.refCount--
		if e.refCount <= 0 {
			e.executor = nil
			p.pool.Delete(nodeID)
		}
		e.mu.Unlock()
	}
}

func (p *ConnectionPool) cleanup() {
	ticker := time.NewTicker(p.idleTimeout / 2)
	defer ticker.Stop()

	for range ticker.C {
		p.pool.Range(func(key, value interface{}) bool {
			entry := value.(*poolEntry)
			entry.mu.Lock()
			if time.Since(entry.lastUsed) >= p.idleTimeout && entry.refCount <= 0 {
				entry.executor = nil
				p.pool.Delete(key)
			}
			entry.mu.Unlock()
			return true
		})
	}
}

func (p *ConnectionPool) Close() {
	p.pool.Range(func(key, value interface{}) bool {
		entry := value.(*poolEntry)
		entry.mu.Lock()
		entry.executor = nil
		entry.refCount = -1
		entry.mu.Unlock()
		p.pool.Delete(key)
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
