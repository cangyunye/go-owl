package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAIRateLimiter_Allow(t *testing.T) {
	rl := newAIRateLimiter()
	rl.now = func() time.Time { return time.Unix(1000, 0) }

	// 限额 3/min：前 3 次放行，第 4 次拒绝
	for i := 0; i < 3; i++ {
		retryAfter, ok := rl.Allow("alice", 3, time.Minute)
		require.True(t, ok, "call %d should pass", i+1)
		require.Zero(t, retryAfter)
	}
	retryAfter, ok := rl.Allow("alice", 3, time.Minute)
	require.False(t, ok)
	require.Positive(t, retryAfter)

	// 不同用户互不影响
	_, ok = rl.Allow("bob", 3, time.Minute)
	require.True(t, ok)
}

func TestAIRateLimiter_WindowSlide(t *testing.T) {
	now := time.Unix(1000, 0)
	rl := newAIRateLimiter()
	rl.now = func() time.Time { return now }

	for i := 0; i < 5; i++ {
		rl.Allow("alice", 5, time.Minute)
	}
	_, ok := rl.Allow("alice", 5, time.Minute)
	require.False(t, ok, "窗口内超限")

	// 时间推进 61s：最早记录滑出窗口，放行
	now = now.Add(61 * time.Second)
	_, ok = rl.Allow("alice", 5, time.Minute)
	require.True(t, ok, "滑出窗口后应放行")
}

func TestAIRateLimiter_ZeroLimitUnlimited(t *testing.T) {
	rl := newAIRateLimiter()
	// limit=0 表示不限流
	for i := 0; i < 100; i++ {
		_, ok := rl.Allow("alice", 0, time.Minute)
		require.True(t, ok)
	}
}
