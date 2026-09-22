package monitor

import (
	"testing"
	"time"
)

func ts(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, time.Local)
}

func TestNextCleanupAfter_Daily(t *testing.T) {
	cases := []struct {
		now, want time.Time
	}{
		// 下午 15:00 → 次日凌晨 02:00
		{ts(2026, 9, 21, 15, 0), ts(2026, 9, 22, 2, 0)},
		// 当天 01:30 → 当天 02:00（尚未执行过今天的时间点）
		{ts(2026, 9, 21, 1, 30), ts(2026, 9, 21, 2, 0)},
		// 恰好 02:00 → 明天 02:00（刚执行过，严格晚于 now）
		{ts(2026, 9, 21, 2, 0), ts(2026, 9, 22, 2, 0)},
	}
	for _, c := range cases {
		if got := nextCleanupAfter(c.now, "daily"); !got.Equal(c.want) {
			t.Errorf("daily from %v: got %v, want %v", c.now, got, c.want)
		}
	}
}

func TestNextCleanupAfter_Weekly(t *testing.T) {
	cases := []struct {
		now, want time.Time
	}{
		// 2026-09-21 是周一：周一 15:00 → 下周一 02:00
		{ts(2026, 9, 21, 15, 0), ts(2026, 9, 28, 2, 0)},
		// 周一 01:00 → 当天 02:00
		{ts(2026, 9, 21, 1, 0), ts(2026, 9, 21, 2, 0)},
		// 周日 → 明天（周一）02:00
		{ts(2026, 9, 20, 10, 0), ts(2026, 9, 21, 2, 0)},
	}
	for _, c := range cases {
		if got := nextCleanupAfter(c.now, "weekly"); !got.Equal(c.want) {
			t.Errorf("weekly from %v: got %v, want %v", c.now, got, c.want)
		}
	}
}

func TestNextCleanupAfter_Monthly(t *testing.T) {
	cases := []struct {
		now, want time.Time
	}{
		// 9 月中旬 → 10 月 1 日 02:00
		{ts(2026, 9, 21, 15, 0), ts(2026, 10, 1, 2, 0)},
		// 10 月 1 日 01:00 → 当天 02:00
		{ts(2026, 10, 1, 1, 0), ts(2026, 10, 1, 2, 0)},
		// 年末 12 月 → 次年 1 月 1 日
		{ts(2026, 12, 15, 8, 0), ts(2027, 1, 1, 2, 0)},
	}
	for _, c := range cases {
		if got := nextCleanupAfter(c.now, "monthly"); !got.Equal(c.want) {
			t.Errorf("monthly from %v: got %v, want %v", c.now, got, c.want)
		}
	}
}

// 非法/未知档位按每日执行，不阻塞清理。
func TestNextCleanupAfter_InvalidFallsBackToDaily(t *testing.T) {
	now := ts(2026, 9, 21, 15, 0)
	want := ts(2026, 9, 22, 2, 0)
	if got := nextCleanupAfter(now, "hourly!!"); !got.Equal(want) {
		t.Errorf("invalid schedule: got %v, want %v", got, want)
	}
	if got := nextCleanupAfter(now, ""); !got.Equal(want) {
		t.Errorf("empty schedule: got %v, want %v", got, want)
	}
}

// RetentionDaysFn 优先于静态 RetentionDays（运行期经 settings 调整）。
func TestEngine_CleanupOnce_RetentionDaysFnOverride(t *testing.T) {
	eng, s := newTestEngine(t, nil, sampleOutputs())
	eng.cfg.RetentionDays = 30
	eng.cfg.RetentionDaysFn = func() int { return 1 }

	now := time.Now().Unix()
	if err := s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "load.load1", TS: now - 20*86400, Value: 1},
		{NodeID: "n1", Metric: "load.load1", TS: now - 3600, Value: 2},
	}); err != nil {
		t.Fatal(err)
	}

	if err := eng.CleanupOnce(); err != nil {
		t.Fatalf("cleanup once: %v", err)
	}
	rows, err := s.QuerySamples("n1", "load.load1", 0, 1<<62)
	if err != nil {
		t.Fatal(err)
	}
	// Fn=1 生效：20 天前的行被删（静态 30 天会保留）
	if len(rows) != 1 || rows[0].Value != 2 {
		t.Fatalf("rows after cleanup = %+v, want only the recent sample", rows)
	}
}

// collectInterval：IntervalFn 优先；未设置或非法回退静态 Interval。
func TestEngine_CollectInterval(t *testing.T) {
	eng, _ := newTestEngine(t, nil, sampleOutputs())
	eng.cfg.Interval = time.Minute

	if got := eng.collectInterval(); got != time.Minute {
		t.Fatalf("default interval = %v, want 1m", got)
	}
	eng.cfg.IntervalFn = func() time.Duration { return 5 * time.Second }
	if got := eng.collectInterval(); got != 5*time.Second {
		t.Fatalf("fn interval = %v, want 5s", got)
	}
	eng.cfg.IntervalFn = func() time.Duration { return 0 }
	if got := eng.collectInterval(); got != time.Minute {
		t.Fatalf("invalid fn interval = %v, want fallback 1m", got)
	}
}

// cleanupSchedule：Fn 缺省或非法值归一为 daily。
func TestEngine_CleanupSchedule(t *testing.T) {
	eng, _ := newTestEngine(t, nil, sampleOutputs())
	if got := eng.cleanupSchedule(); got != "daily" {
		t.Fatalf("default schedule = %q, want daily", got)
	}
	eng.cfg.CleanupScheduleFn = func() string { return " weekly " }
	if got := eng.cleanupSchedule(); got != "weekly" {
		t.Fatalf("schedule = %q, want weekly", got)
	}
	eng.cfg.CleanupScheduleFn = func() string { return "nope" }
	if got := eng.cleanupSchedule(); got != "daily" {
		t.Fatalf("invalid schedule = %q, want fallback daily", got)
	}
}
