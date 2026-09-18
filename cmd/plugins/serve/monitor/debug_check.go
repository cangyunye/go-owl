// 告警规则调试执行：对配置的检查命令立即执行一次，返回输出、解析值与
// 是否会触发阈值的判定（不落库、不发通知）。
package monitor

import (
	"fmt"
	"time"

	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
)

// DebugCheckResult 一次调试执行的完整结果。
type DebugCheckResult struct {
	NodeID       string   `json:"node_id"`
	Command      string   `json:"command"`
	Output       string   `json:"output"`
	ExitCode     int      `json:"exit_code"`
	Value        *float64 `json:"value"`
	ParseErr     string   `json:"parse_err,omitempty"`
	Metric       string   `json:"metric"`
	Op           string   `json:"op"`
	Threshold    float64  `json:"threshold"`
	WouldTrigger bool     `json:"would_trigger"`
	DurationMs   int64    `json:"duration_ms"`
}

// DebugCheck 在指定节点上执行一次规则的检查命令并给出阈值判定。
func (s *Service) DebugCheck(nodeID string, at owlmonitor.AlertType) (*DebugCheckResult, error) {
	if at.CheckCmd == "" {
		return nil, fmt.Errorf("该规则没有自定义检查命令（内置规则随每轮采集自动评估）")
	}
	if s.debugExec == nil {
		return nil, fmt.Errorf("调试执行器未就绪")
	}
	if nodeID == "" {
		return nil, fmt.Errorf("缺少目标节点")
	}

	start := time.Now()
	out, code, err := s.debugExec(nodeID, at.CheckCmd, owlmonitor.CustomCheckTimeout)
	res := &DebugCheckResult{
		NodeID:     nodeID,
		Command:    at.CheckCmd,
		Output:     out,
		ExitCode:   code,
		Metric:     owlmonitor.CustomMetricID(at.ID),
		Op:         at.DefaultParams.Op,
		Threshold:  at.DefaultParams.Value,
		DurationMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		res.ParseErr = fmt.Sprintf("执行失败: %v", err)
		return res, nil
	}
	mode := at.CheckMode
	if mode == "" {
		mode = "value"
	}
	v, perr := owlmonitor.ParseCheckOutput(mode, at.CheckPattern, out, code)
	if perr != nil {
		res.ParseErr = perr.Error()
		return res, nil
	}
	res.Value = &v
	res.WouldTrigger = compareThreshold(v, at.DefaultParams.Op, at.DefaultParams.Value)
	return res, nil
}

// compareThreshold 按规则操作符比较指标值与阈值。
func compareThreshold(v float64, op string, threshold float64) bool {
	switch op {
	case "<":
		return v < threshold
	case ">=":
		return v >= threshold
	case "<=":
		return v <= threshold
	case "==":
		return v == threshold
	case "!=":
		return v != threshold
	default: // >
		return v > threshold
	}
}
