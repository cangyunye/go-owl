package monitor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/logger"
)

// TargetSource 采集目标来源（serve 中为 nodes 表，测试可注入 fake）。
type TargetSource interface {
	ListTargets() ([]Target, error)
}

// EngineConfig 监控引擎配置。
type EngineConfig struct {
	Interval      time.Duration // 采集间隔，默认 60s
	RetentionDays int           // 指标/告警保留天数，默认 30
	Concurrency   int           // 并发采集节点数，默认 10
	SilenceUntil  func() int64  // 全局静默截止时间戳（0 = 不静默）
	// EscalateAfter 返回 warn 未处理升级为 critical 的时长（nil 或 <=0 =
	// 保持默认 1h）；每轮采集前求值，支持运行期经 settings 调整
	EscalateAfter func() time.Duration
	// RealertWindow 返回重复告警合并窗口（nil = 保持默认 24h；0 = 关闭合并）；
	// 每轮采集前求值，支持运行期经 settings 调整
	RealertWindow func() time.Duration
	// Enabled 返回监控总开关（nil = 开启）；false 时整轮跳过采集与评估
	Enabled func() bool
	// CollectWindow 返回采集时段窗口 "HH:MM-HH:MM"（"" = 全天）；
	// 窗口外整轮跳过；每轮采集前求值
	CollectWindow func() string
	// AlertRetentionDays 返回告警记录保留天数（nil/0 = 不启用）；
	// 只清理已解决且解决时间超期的记录
	AlertRetentionDays func() int
	WebURL             string // 告警处理入口链接前缀
}

// Engine 监控引擎：周期采集 → 入库 → 规则评估 → 告警 → 通知，每日清理。
type Engine struct {
	cfg          EngineConfig
	store        *Store
	collector    *Collector
	source       TargetSource
	manager      *AlertManager
	dispatcher   *Dispatcher
	healer       *AutoHealer             // 自愈管线（nil = 关闭）
	// OnAlertOpened 告警打开（含合并窗口重开）时的扩展钩子（nil = 无操作）；
	// 异步调用，serve 侧用于执行告警绑定的自动处置指令
	OnAlertOpened func(ev AlertEvent, t Target)
	lastCounters map[string]counterPoint // node|metric → 上次累计计数（网卡速率）
	mu           sync.Mutex
	now          func() time.Time // 采集时段窗口判定用时钟（测试可注入）
}

// counterPoint 累计计数采样点。
type counterPoint struct {
	Value float64
	TS    int64
}

// NewEngine 创建监控引擎。
func NewEngine(cfg EngineConfig, store *Store, collector *Collector, source TargetSource, manager *AlertManager, dispatcher *Dispatcher) *Engine {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 30
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}
	if cfg.SilenceUntil == nil {
		cfg.SilenceUntil = func() int64 { return 0 }
	}
	return &Engine{
		cfg:          cfg,
		store:        store,
		collector:    collector,
		source:       source,
		manager:      manager,
		dispatcher:   dispatcher,
		lastCounters: make(map[string]counterPoint),
		now:          time.Now,
	}
}

// SetAutoHealer 挂载自愈管线（告警触发且类型放行时自动处置）。
func (e *Engine) SetAutoHealer(h *AutoHealer) {
	e.healer = h
}

// Run 阻塞运行：立即执行一轮与清理，之后按 Interval 周期执行，24h 清理一次。
// ctx 取消时优雅退出。
func (e *Engine) Run(ctx context.Context) error {
	if err := e.CleanupOnce(); err != nil {
		logger.Warn("监控清理任务失败", logger.WithOperation("monitor_cleanup"), logger.WithError(err))
	}
	_ = e.TickOnce(ctx)

	tick := time.NewTicker(e.cfg.Interval)
	defer tick.Stop()
	cleanup := time.NewTicker(24 * time.Hour)
	defer cleanup.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if err := e.TickOnce(ctx); err != nil {
				logger.Warn("监控采集轮次失败", logger.WithOperation("monitor_tick"), logger.WithError(err))
			}
		case <-cleanup.C:
			if err := e.CleanupOnce(); err != nil {
				logger.Warn("监控清理任务失败", logger.WithOperation("monitor_cleanup"), logger.WithError(err))
			}
		}
	}
}

// TickOnce 执行一轮采集与评估（可单测）。
// 监控总开关关闭或采集时段窗口之外时整轮跳过。
func (e *Engine) TickOnce(ctx context.Context) error {
	if e.cfg.Enabled != nil && !e.cfg.Enabled() {
		return nil
	}
	if e.cfg.CollectWindow != nil && !inCollectWindow(e.now(), e.cfg.CollectWindow()) {
		return nil
	}
	targets, err := e.source.ListTargets()
	if err != nil {
		return fmt.Errorf("monitor: 获取采集目标失败: %w", err)
	}
	types, err := e.store.ListEnabledAlertTypes()
	if err != nil {
		return fmt.Errorf("monitor: 读取告警类型失败: %w", err)
	}
	e.manager.SetSilentUntil(e.cfg.SilenceUntil())
	if e.cfg.EscalateAfter != nil {
		if d := e.cfg.EscalateAfter(); d > 0 {
			e.manager.SetEscalateAfter(d)
		}
	}
	if e.cfg.RealertWindow != nil {
		e.manager.SetRealertWindow(e.cfg.RealertWindow())
	}

	sem := make(chan struct{}, e.cfg.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, t := range targets {
		wg.Add(1)
		go func(t Target) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if err := e.collectNode(ctx, t, types); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(t)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// collectNode 单节点：采集 → 失联判定 → 入库 → 规则评估 → 通知。
func (e *Engine) collectNode(ctx context.Context, t Target, types []AlertType) error {
	samples, err := e.collector.Collect(ctx, &t)

	if err != nil && len(samples) == 0 {
		// 必选命令失败且无任何指标 → 视为失联
		e.clearCounters(t.ID)
		events, merr := e.manager.MarkCollectFail(t.ID)
		if merr != nil {
			return merr
		}
		e.dispatch(ctx, events, types, t)
		return err
	}

	// 采集成功（含部分成功）
	if err != nil {
		logger.Warn("节点部分采集命令失败（指标可能缺失）",
			logger.WithOperation("monitor_collect"),
			logger.WithField("node_id", t.ID), logger.WithError(err))
	}
	events, merr := e.manager.MarkCollectOK(t.ID)
	if merr != nil {
		return merr
	}
	e.dispatch(ctx, events, types, t)

	// 合成网卡速率并入库
	all := e.computeNetRates(t.ID, samples)

	// 自定义检查：check_cmd 类型的每轮采样（失败仅跳过该指标，不影响失联判定）
	if custom := e.collectCustomChecks(ctx, &t, types); len(custom) > 0 {
		all = append(all, custom...)
	}

	if err := e.store.InsertSamples(all); err != nil {
		return fmt.Errorf("monitor: 指标入库失败(node=%s): %w", t.ID, err)
	}

	// 规则评估 → 告警生命周期
	events, merr = e.manager.Tick(t.ID, all, types)
	if merr != nil {
		return merr
	}
	e.dispatch(ctx, events, types, t)
	return nil
}

// customCheckTimeout 单条自定义检查命令超时。
const customCheckTimeout = 30 * time.Second

// collectCustomChecks 对启用了 check_cmd 的告警类型逐个执行检查命令，
// 按 check_mode 解析为 custom.<小写类型ID> 数值指标。
func (e *Engine) collectCustomChecks(ctx context.Context, t *Target, types []AlertType) []Sample {
	var out []Sample
	for _, at := range types {
		if !at.Enabled || at.CheckCmd == "" {
			continue
		}
		stdout, exitCode, err := e.collector.ExecCommand(ctx, t, at.CheckCmd, customCheckTimeout)
		if err != nil {
			logger.Warn("自定义检查执行失败", logger.WithOperation("monitor_custom_check"),
				logger.WithField("type_id", at.ID), logger.WithField("node_id", t.ID),
				logger.WithError(err))
			continue
		}
		mode := at.CheckMode
		if mode == "" {
			mode = "value"
		}
		v, perr := ParseCheckOutput(mode, at.CheckPattern, stdout, exitCode)
		if perr != nil {
			logger.Warn("自定义检查输出解析失败", logger.WithOperation("monitor_custom_check"),
				logger.WithField("type_id", at.ID), logger.WithField("node_id", t.ID),
				logger.WithError(perr))
			continue
		}
		out = append(out, Sample{NodeID: t.ID, Metric: CustomMetricID(at.ID),
			TS: e.now().Unix(), Value: v})
	}
	return out
}

// computeNetRates 由累计计数合成每秒速率指标（net.rx_rate.<iface> 等），
// 并更新计数缓存；无前值（首轮/网卡新增）时跳过该指标。
func (e *Engine) computeNetRates(nodeID string, samples []Sample) []Sample {
	out := make([]Sample, 0, len(samples))
	for _, s := range samples {
		if !isCounterMetric(s.Metric) {
			out = append(out, s)
			continue
		}
		key := nodeID + "|" + s.Metric
		e.mu.Lock()
		prev, ok := e.lastCounters[key]
		e.lastCounters[key] = counterPoint{Value: s.Value, TS: s.TS}
		e.mu.Unlock()
		if !ok {
			continue // 首轮无前值，跳过速率
		}
		rate := RatePerSec(Sample{TS: prev.TS, Value: prev.Value}, s)
		out = append(out, s, Sample{
			NodeID: nodeID,
			Metric: rateMetric(s.Metric),
			TS:     s.TS,
			Value:  rate,
		})
	}
	return out
}

// isCounterMetric 是否为网卡累计字节计数。
func isCounterMetric(metric string) bool {
	return len(metric) > len("net.rx_bytes.") && (metric[:len("net.rx_bytes.")] == "net.rx_bytes." ||
		metric[:len("net.tx_bytes.")] == "net.tx_bytes.")
}

func rateMetric(counter string) string {
	prefix := "net.rx_bytes."
	if len(counter) > len(prefix) && counter[:len(prefix)] == prefix {
		return "net.rx_rate." + counter[len(prefix):]
	}
	prefix = "net.tx_bytes."
	return "net.tx_rate." + counter[len(prefix):]
}

// clearCounters 节点失联时清空其计数缓存，避免恢复后首轮速率虚高。
func (e *Engine) clearCounters(nodeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	prefix := nodeID + "|"
	for k := range e.lastCounters {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			delete(e.lastCounters, k)
		}
	}
}

// dispatch 事件分发（静默期内不通知）。
func (e *Engine) dispatch(ctx context.Context, events []AlertEvent, types []AlertType, t Target) {
	if len(events) == 0 || e.isSilenced() {
		return
	}
	typeByID := make(map[string]AlertType, len(types))
	for _, at := range types {
		typeByID[at.ID] = at
	}
	for _, ev := range events {
		if ev.Type != EventOpened && ev.Type != EventEscalated {
			continue // 恢复/刷新不通知
		}
		at, ok := typeByID[ev.Alert.AlertTypeID]
		if !ok {
			at, ok = FindAlertType(ev.Alert.AlertTypeID)
			if !ok {
				continue
			}
		}
		// 自愈：新告警且类型已放行 → 走自愈管线（异步执行获准步骤）
		if ev.Type == EventOpened && e.healer != nil && at.AutoApprove {
			go func(ev AlertEvent, at AlertType, t Target) {
				if _, err := e.healer.Heal(context.Background(), ev.Alert, at, &t); err != nil {
					logger.Warn("自愈管线失败", logger.WithOperation("monitor_autoheal"),
						logger.WithField("alert_id", ev.Alert.ID), logger.WithError(err))
				}
			}(ev, at, t)
		}
		// 告警绑定执行钩子（异步；serve 侧按 auto_exec 绑定处置指令；
		// recover 防止钩子 panic 带崩采集循环）
		if ev.Type == EventOpened && e.OnAlertOpened != nil {
			go func(ev AlertEvent, t Target) {
				defer func() {
					if r := recover(); r != nil {
						logger.Warn("告警打开钩子 panic", logger.WithOperation("monitor_hooks"),
							logger.WithField("alert_id", ev.Alert.ID),
							logger.WithField("panic", r))
					}
				}()
				e.OnAlertOpened(ev, t)
			}(ev, t)
		}
		webURL := e.cfg.WebURL + "/alerts/" + ev.Alert.ID
		if errs := e.dispatcher.Notify(ctx, ev, at, t.Name, webURL); len(errs) > 0 {
			for _, err := range errs {
				logger.Warn("告警通知失败", logger.WithOperation("monitor_notify"),
					logger.WithField("alert_id", ev.Alert.ID), logger.WithError(err))
			}
		}
	}
}

// CleanupOnce 执行一次保留期清理（幂等，可每日调用）：
// 指标按 RetentionDays 清理；告警记录按 AlertRetentionDays 清理
//（只删已解决且解决时间超期的，未解决告警永不删除）。
func (e *Engine) CleanupOnce() error {
	if err := e.store.Cleanup(e.cfg.RetentionDays); err != nil {
		return err
	}
	if e.cfg.AlertRetentionDays == nil {
		return nil
	}
	if days := e.cfg.AlertRetentionDays(); days > 0 {
		return e.store.CleanupAlerts(days)
	}
	return nil
}

// inCollectWindow 判断当前时间是否在采集时段窗口 "HH:MM-HH:MM" 内。
// 空窗口 = 全天；支持跨午夜（如 22:00-06:00）；解析失败放行（不阻塞采集）。
func inCollectWindow(now time.Time, window string) bool {
	window = strings.TrimSpace(window)
	if window == "" {
		return true
	}
	parts := strings.SplitN(window, "-", 2)
	if len(parts) != 2 {
		return true
	}
	start, err1 := parseHHMM(parts[0])
	end, err2 := parseHHMM(parts[1])
	if err1 != nil || err2 != nil {
		return true
	}
	cur := now.Hour()*60 + now.Minute()
	s := start.Hour()*60 + start.Minute()
	t := end.Hour()*60 + end.Minute()
	if s == t {
		return true // 零长度窗口视为全天
	}
	if s < t {
		return cur >= s && cur < t
	}
	return cur >= s || cur < t // 跨午夜
}

// ValidateCollectWindow 校验采集时段窗口格式（"HH:MM-HH:MM"；空 = 全天，
// 支持跨午夜）。供 settings 写入校验复用。
func ValidateCollectWindow(window string) error {
	window = strings.TrimSpace(window)
	if window == "" {
		return nil
	}
	parts := strings.SplitN(window, "-", 2)
	if len(parts) != 2 {
		return fmt.Errorf("collect window must be HH:MM-HH:MM")
	}
	if _, err := parseHHMM(parts[0]); err != nil {
		return err
	}
	if _, err := parseHHMM(parts[1]); err != nil {
		return err
	}
	return nil
}

// parseHHMM 解析 "HH:MM" 为时刻。
func parseHHMM(s string) (time.Time, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid HH:MM: %s", s)
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || h < 0 || h > 23 {
		return time.Time{}, fmt.Errorf("invalid hour in %s", s)
	}
	m, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || m < 0 || m > 59 {
		return time.Time{}, fmt.Errorf("invalid minute in %s", s)
	}
	return time.Date(0, 1, 1, h, m, 0, 0, time.UTC), nil
}

func (e *Engine) isSilenced() bool {
	until := e.cfg.SilenceUntil()
	return until > 0 && time.Now().Unix() < until
}
