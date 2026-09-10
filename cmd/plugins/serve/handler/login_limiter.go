package handler

import (
	"sync"
	"time"
)

const (
	loginFailThreshold = 5
	loginBaseBackoff   = 2 * time.Second
	loginMaxBackoff    = 5 * time.Minute
)

// loginLimiter 按"用户名+来源 IP"限制连续失败登录：达到阈值后指数退避，
// 登录成功即清零。进程内状态，重启后需重新累计。
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
	now      func() time.Time
}

type loginAttempt struct {
	fails int
	until time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]*loginAttempt), now: time.Now}
}

// RetryAfter 返回仍需等待的时长；0 表示可以尝试。
func (l *loginLimiter) RetryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		return 0
	}
	if wait := a.until.Sub(l.now()); wait > 0 {
		return wait
	}
	return 0
}

// Fail 记录一次失败：达到阈值后按 2^n 退避，上限 loginMaxBackoff。
func (l *loginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		a = &loginAttempt{}
		l.attempts[key] = a
	}
	a.fails++
	if a.fails < loginFailThreshold {
		return
	}
	backoff := loginBaseBackoff << uint(a.fails-loginFailThreshold)
	if backoff <= 0 || backoff > loginMaxBackoff {
		backoff = loginMaxBackoff
	}
	a.until = l.now().Add(backoff)
}

// Reset 登录成功后清除该 key 的失败记录。
func (l *loginLimiter) Reset(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}
