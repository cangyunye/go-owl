package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// 验证结论状态
const (
	VerifyRecovered    = "recovered"     // 定向采集未再命中规则（恢复迹象）
	VerifyNotRecovered = "not_recovered" // 指标仍命中规则（告警仍在）
	VerifyInconclusive = "inconclusive"  // 采集失败，无法判定（与恢复严格区分）
)

// RunVerification 是一次处置疗效的定向核查记录。
// 正式 resolved 仍由引擎周期采集（连续未命中）判定，本记录只做即时核查与留痕。
type RunVerification struct {
	RunID       string  `json:"run_id"`
	AlertID     string  `json:"alert_id"`
	NodeID      string  `json:"node_id"`
	AlertTypeID string  `json:"alert_type_id"`
	Status      string  `json:"status"`
	Metric      string  `json:"metric,omitempty"`
	Value       float64 `json:"value,omitempty"`
	Message     string  `json:"message,omitempty"`
	VerifiedAt  int64   `json:"verified_at"`
}

// SaveVerification 写入验证记录（同 run 覆盖）。
func (s *Store) SaveVerification(v *RunVerification) error {
	_, err := s.db.Exec(`
		INSERT INTO remedy_run_verifications (run_id, alert_id, node_id, alert_type_id, status, metric, value, message, verified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			status = excluded.status,
			metric = excluded.metric,
			value = excluded.value,
			message = excluded.message,
			verified_at = excluded.verified_at
	`, v.RunID, v.AlertID, v.NodeID, v.AlertTypeID, v.Status, v.Metric, v.Value, v.Message, v.VerifiedAt)
	if err != nil {
		return fmt.Errorf("monitor: 保存验证记录失败: %w", err)
	}
	return nil
}

// GetVerificationByRun 读取执行计划的验证记录。
func (s *Store) GetVerificationByRun(runID string) (*RunVerification, error) {
	var v RunVerification
	var metric, message *string
	var value *float64
	err := s.db.QueryRow(`
		SELECT run_id, alert_id, node_id, alert_type_id, status, metric, value, message, verified_at
		FROM remedy_run_verifications WHERE run_id = ?`, runID).
		Scan(&v.RunID, &v.AlertID, &v.NodeID, &v.AlertTypeID, &v.Status, &metric, &value, &message, &v.VerifiedAt)
	if err != nil {
		return nil, fmt.Errorf("monitor: 查询验证记录失败: %w", err)
	}
	if metric != nil {
		v.Metric = *metric
	}
	if message != nil {
		v.Message = *message
	}
	if value != nil {
		v.Value = *value
	}
	return &v, nil
}

// ListVerificationsByAlert 按告警列出验证记录（时间倒序）。
func (s *Store) ListVerificationsByAlert(alertID string) ([]*RunVerification, error) {
	rows, err := s.db.Query(`
		SELECT run_id, alert_id, node_id, alert_type_id, status, metric, value, message, verified_at
		FROM remedy_run_verifications WHERE alert_id = ? ORDER BY verified_at DESC`, alertID)
	if err != nil {
		return nil, fmt.Errorf("monitor: 列出验证记录失败: %w", err)
	}
	defer rows.Close()

	var out []*RunVerification
	for rows.Next() {
		var v RunVerification
		var metric, message *string
		var value *float64
		if err := rows.Scan(&v.RunID, &v.AlertID, &v.NodeID, &v.AlertTypeID, &v.Status, &metric, &value, &message, &v.VerifiedAt); err != nil {
			continue
		}
		if metric != nil {
			v.Metric = *metric
		}
		if message != nil {
			v.Message = *message
		}
		if value != nil {
			v.Value = *value
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// RunVerifier 处置疗效定向核查：RemedyRun 完成后等待生效间隔，
// 对目标节点立即采集一轮指标并用告警规则重新评估。
type RunVerifier struct {
	collector *Collector
	resolve   TargetResolver
	store     *Store
	// delay 处置生效等待（脚本执行完指标往往不会立刻回落）
	delay time.Duration
	now   func() time.Time
}

// NewRunVerifier 创建验证器。
func NewRunVerifier(collector *Collector, resolve TargetResolver, store *Store) *RunVerifier {
	return &RunVerifier{
		collector: collector,
		resolve:   resolve,
		store:     store,
		delay:     30 * time.Second,
		now:       func() time.Time { return time.Now() },
	}
}

// SetDelay 覆盖处置生效等待时长。
func (v *RunVerifier) SetDelay(d time.Duration) {
	if d >= 0 {
		v.delay = d
	}
}

// VerifyRun 对一个已完成（RunDone）的执行计划做定向核查并落库。
// 非终态/失败单返回错误（失败单无疗效可言）。
func (v *RunVerifier) VerifyRun(ctx context.Context, runID string) (*RunVerification, error) {
	run, exists, err := v.store.GetRemedyRun(runID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("monitor: 执行计划 %s 不存在", runID)
	}
	if run.Status != RunDone {
		return nil, fmt.Errorf("monitor: 执行计划 %s 状态为 %s，仅完成的计划可验证", runID, run.Status)
	}
	alert, exists, err := v.store.GetAlert(run.AlertID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("monitor: 关联告警 %s 不存在", run.AlertID)
	}
	at, exists, err := v.store.GetAlertType(alert.AlertTypeID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("monitor: 告警类型 %s 不存在", alert.AlertTypeID)
	}

	verification := &RunVerification{
		RunID: run.ID, AlertID: alert.ID, NodeID: run.NodeID,
		AlertTypeID: at.ID, VerifiedAt: v.now().Unix(),
	}

	if v.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(v.delay):
		}
	}

	target, err := v.resolve(run.NodeID)
	if err != nil {
		verification.Status = VerifyInconclusive
		verification.Message = "解析采集节点失败: " + err.Error()
		_ = v.store.SaveVerification(verification)
		return verification, nil
	}
	samples, err := v.collector.Collect(ctx, target)
	if err != nil {
		verification.Status = VerifyInconclusive
		verification.Message = "定向采集失败: " + err.Error()
		_ = v.store.SaveVerification(verification)
		return verification, nil
	}

	result, err := at.DefaultParams.Evaluate(samples, run.NodeID)
	if err != nil {
		verification.Status = VerifyInconclusive
		verification.Message = "规则评估失败: " + err.Error()
		_ = v.store.SaveVerification(verification)
		return verification, nil
	}

	verification.Metric = strings.TrimSuffix(at.DefaultParams.Metric, ".") + "（当前值）"
	verification.Value = result.Value
	if result.Matched {
		verification.Status = VerifyNotRecovered
		verification.Message = fmt.Sprintf("处置后定向采集：指标仍命中告警规则（当前 %.2f，阈值 %g %s），告警尚未消除",
			result.Value, at.DefaultParams.Value, at.DefaultParams.Op)
	} else {
		verification.Status = VerifyRecovered
		verification.Message = "处置后定向采集：指标已回落到阈值内，等待引擎确认恢复"
	}
	if err := v.store.SaveVerification(verification); err != nil {
		return verification, err
	}
	return verification, nil
}
