package monitor

import (
	"fmt"
	"strings"
	"sync"
)

// EvalResult 单条规则对一批采样的评估结果。
type EvalResult struct {
	AlertTypeID string
	Metric      string // 实际触发/最坏的指标名
	Value       float64
	Matched     bool
}

// Evaluate 用规则参数评估一批采样（不含持续窗口计数）。
// - Metric 为空（如 OWL-OSS-001 引擎信号类）→ 不参与指标评估，返回未匹配
// - 前缀匹配（metric 以 "." 结尾）取最坏值（> 类取最大、< 类取最小）
// - PerCore > 0 时阈值 = sys.cores × PerCore（负载类规则）
func (p RuleParams) Evaluate(samples []Sample, nodeID string) (EvalResult, error) {
	res := EvalResult{AlertTypeID: "", Matched: false}
	if p.Metric == "" {
		return res, nil
	}
	prefix := strings.HasSuffix(p.Metric, ".")

	// 计算阈值
	threshold := p.Value
	if p.PerCore > 0 {
		cores, ok := findMetric(samples, nodeID, "sys.cores")
		if !ok {
			return res, nil // 缺核数信息无法判定
		}
		threshold = cores * p.PerCore
	}

	// 找出匹配指标中「最坏」的一个
	var best *Sample
	for i := range samples {
		s := &samples[i]
		if s.NodeID != nodeID {
			continue
		}
		match := s.Metric == p.Metric || (prefix && strings.HasPrefix(s.Metric, p.Metric))
		if !match {
			continue
		}
		if best == nil || worse(p.Op, s.Value, best.Value) {
			best = s
		}
	}
	if best == nil {
		return res, nil
	}

	matched, err := compare(p.Op, best.Value, threshold)
	if err != nil {
		return res, err
	}
	return EvalResult{Metric: best.Metric, Value: best.Value, Matched: matched}, nil
}

// findMetric 在批中查找指定指标。
func findMetric(samples []Sample, nodeID, metric string) (float64, bool) {
	for _, s := range samples {
		if s.NodeID == nodeID && s.Metric == metric {
			return s.Value, true
		}
	}
	return 0, false
}

// worse 判断新值是否比当前最坏值更坏（> 类取大、< 类取小）。
func worse(op string, newVal, best float64) bool {
	switch op {
	case "<", "<=":
		return newVal < best
	default:
		return newVal > best
	}
}

func compare(op string, value, threshold float64) (bool, error) {
	switch op {
	case ">":
		return value > threshold, nil
	case ">=":
		return value >= threshold, nil
	case "<":
		return value < threshold, nil
	case "<=":
		return value <= threshold, nil
	default:
		return false, fmt.Errorf("monitor: 不支持的规则操作符 %q", op)
	}
}

// TickOutcome 一轮规则评估的输出：达到持续窗口的触发项 + 当前满足条件的类型。
type TickOutcome struct {
	Triggered []EvalResult    // 达到持续窗口（新触发或持续触发）
	Matched   map[string]bool // 当前满足条件的告警类型 ID（供恢复检测）
}

// RuleEngine 状态化规则引擎：维护每 (node, alert_type) 的连续命中计数。
// RuleEngine 逐节点的规则持续计数。counts 在并发采集下被多个 goroutine
// 同时读写，必须持 mu 访问。
type RuleEngine struct {
	mu     sync.Mutex
	counts map[string]int
}

// NewRuleEngine 创建规则引擎。
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{counts: make(map[string]int)}
}

func ruleKey(nodeID, typeID string) string { return nodeID + "|" + typeID }

// Tick 对单节点一批采样推进一轮评估：
// 命中规则计数 +1，未命中清零；计数达到规则 Duration 时进入 Triggered。
func (e *RuleEngine) Tick(nodeID string, samples []Sample, types []AlertType) TickOutcome {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := TickOutcome{Matched: make(map[string]bool)}
	for _, at := range types {
		if !at.Enabled {
			continue
		}
		res, err := at.DefaultParams.Evaluate(samples, nodeID)
		if err != nil {
			continue // 规则参数错误：跳过，不中断整体评估
		}
		key := ruleKey(nodeID, at.ID)
		if !res.Matched {
			delete(e.counts, key)
			continue
		}
		out.Matched[at.ID] = true
		e.counts[key]++
		res.AlertTypeID = at.ID
		if e.counts[key] >= at.DefaultParams.Duration {
			out.Triggered = append(out.Triggered, res)
		}
	}
	return out
}

// Reset 清空某节点的全部计数（节点失联/恢复后调用）。
func (e *RuleEngine) Reset(nodeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for k := range e.counts {
		if strings.HasPrefix(k, nodeID+"|") {
			delete(e.counts, k)
		}
	}
}
