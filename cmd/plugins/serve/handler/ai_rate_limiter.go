package handler

import (
	"sync"
	"time"
)

// aiRateLimiter 是 AI 聊天接口的每用户内存滑动窗口限流器。
// 仅存于进程内存（重启清零），零配置默认不限流。
type aiRateLimiter struct {
	mu    sync.Mutex
	calls map[string][]time.Time
	now   func() time.Time
}

func newAIRateLimiter() *aiRateLimiter {
	return &aiRateLimiter{
		calls: make(map[string][]time.Time),
		now:   time.Now,
	}
}

// Allow 判定用户本次调用是否放行。limit<=0 表示不限流。
// 返回 (被拒绝时的建议等待时长, 是否放行)。
func (l *aiRateLimiter) Allow(user string, limit int, window time.Duration) (time.Duration, bool) {
	if limit <= 0 {
		return 0, true
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// 清理窗口外记录
	recent := l.calls[user][:0]
	for _, t := range l.calls[user] {
		if now.Sub(t) < window {
			recent = append(recent, t)
		}
	}
	l.calls[user] = recent

	if len(recent) >= limit {
		// 最早一条记录滑出窗口所需的时间即建议等待时长
		retryAfter := window - now.Sub(recent[0])
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return retryAfter, false
	}
	l.calls[user] = append(l.calls[user], now)
	return 0, true
}
