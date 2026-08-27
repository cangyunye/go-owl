package monitor

import (
	"context"
	"errors"
	"fmt"
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
	WebURL        string        // 告警处理入口链接前缀
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
	lastCounters map[string]counterPoint // node|metric → 上次累计计数（网卡速率）
	mu           sync.Mutex
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
func (e *Engine) TickOnce(ctx context.Context) error {
	targets, err := e.source.ListTargets()
	if err != nil {
		return fmt.Errorf("monitor: 获取采集目标失败: %w", err)
	}
	types, err := e.store.ListEnabledAlertTypes()
	if err != nil {
		return fmt.Errorf("monitor: 读取告警类型失败: %w", err)
	}
	e.manager.SilentUntil = e.cfg.SilenceUntil()

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
	events, merr := e.manager.MarkCollectOK(t.ID)
	if merr != nil {
		return merr
	}
	e.dispatch(ctx, events, types, t)

	// 合成网卡速率并入库
	all := e.computeNetRates(t.ID, samples)
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
		webURL := e.cfg.WebURL + "/alerts/" + ev.Alert.ID
		if errs := e.dispatcher.Notify(ctx, ev, at, t.Name, webURL); len(errs) > 0 {
			for _, err := range errs {
				logger.Warn("告警通知失败", logger.WithOperation("monitor_notify"),
					logger.WithField("alert_id", ev.Alert.ID), logger.WithError(err))
			}
		}
	}
}

// CleanupOnce 执行一次保留期清理（幂等，可每日调用）。
func (e *Engine) CleanupOnce() error {
	return e.store.Cleanup(e.cfg.RetentionDays)
}

func (e *Engine) isSilenced() bool {
	until := e.cfg.SilenceUntil()
	return until > 0 && time.Now().Unix() < until
}
