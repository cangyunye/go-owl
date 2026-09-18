// 告警绑定执行器：按告警 ID 绑定的 playbook / 脚本指令的编排与状态回写。
// 手动触发（handler）与告警打开自动执行（Engine.OnAlertOpened 钩子）共用；
// playbook 走 PlaybookRunner（handler 的剧本运行链路），script 走单步
// 处置计划（复用黑名单闸门与执行审计）。
package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/cangyunye/go-owl/internal/logger"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
)

// PlaybookRunner 剧本执行入口（由 handler.PlaybookHandler 实现）。
type PlaybookRunner interface {
	// RunForAlert 以告警节点为目标发起剧本运行，返回 playbook_run ID。
	RunForAlert(ctx context.Context, playbookID, nodeID, createdBy string) (string, error)
	// PlaybookRunFinished 查询运行是否已到终态（completed/failed/cancelled）。
	PlaybookRunFinished(ctx context.Context, runID string) (done bool, status string, err error)
}

const (
	bindingPollInterval = 2 * time.Second
	bindingItemTimeout  = 30 * time.Minute
)

// SetPlaybookRunner 注入剧本执行入口（server 装配时调用）。
func (s *Service) SetPlaybookRunner(r PlaybookRunner) {
	s.playbookRunner = r
}

// StartScriptRun 创建单步脚本处置计划并异步执行（复用黑名单闸门与审计），
// 返回 remedy_run ID。
func (s *Service) StartScriptRun(alertID, name, content, nodeID, createdBy string) (string, error) {
	now := time.Now()
	run := &owlmonitor.RemedyRun{
		ID:          fmt.Sprintf("RUN-%d", now.UnixNano()),
		AlertID:     alertID,
		NodeID:      nodeID,
		Status:      owlmonitor.RunPending,
		StopOnError: true,
		CreatedBy:   createdBy,
		CreatedAt:   now.Unix(),
		UpdatedAt:   now.Unix(),
		Steps: []owlmonitor.RemedyStep{{
			Order:   0,
			Name:    name,
			Kind:    "script",
			Content: content,
			Status:  owlmonitor.StepPending,
			NodeID:  nodeID,
		}},
	}
	if err := s.Store.CreateRemedyRun(run); err != nil {
		return "", err
	}
	s.ExecuteRemedyRun(run.ID)
	return run.ID, nil
}

// handleAlertOpened 告警打开/重开钩子：执行 auto_exec 绑定（Engine 异步调用）。
func (s *Service) handleAlertOpened(ev owlmonitor.AlertEvent, t owlmonitor.Target) {
	bindings, err := s.Store.ListAutoAlertBindings(ev.Alert.ID)
	if err != nil {
		logger.Warn("读取告警自动执行绑定失败", logger.WithOperation("alert_bindings"),
			logger.WithField("alert_id", ev.Alert.ID), logger.WithError(err))
		return
	}
	if len(bindings) == 0 {
		return
	}
	if _, err := s.RunAlertBindings(ev.Alert.ID, bindings, "", t.ID, "auto"); err != nil {
		logger.Warn("告警绑定自动执行失败", logger.WithOperation("alert_bindings"),
			logger.WithField("alert_id", ev.Alert.ID), logger.WithError(err))
	}
}

// RunAlertBindings 执行一组绑定指令（手动触发按用户给定顺序；mode 为空时
// 按各绑定 exec_mode 分组——自动执行场景）。返回创建的执行记录。
// 绑定必须属于该告警（防跨告警携带他人绑定执行）。
func (s *Service) RunAlertBindings(alertID string, bindings []owlmonitor.AlertBinding, mode, nodeID, createdBy string) ([]owlmonitor.AlertBindingRun, error) {
	if len(bindings) == 0 {
		return nil, fmt.Errorf("未选择指令")
	}
	for _, b := range bindings {
		if b.AlertID != alertID {
			return nil, fmt.Errorf("绑定 %s 不属于告警 %s", b.ID, alertID)
		}
	}
	now := time.Now()
	recs := make([]owlmonitor.AlertBindingRun, 0, len(bindings))
	for i, b := range bindings {
		m := mode
		if m == "" {
			m = b.ExecMode
		}
		if m != "concurrent" {
			m = "sequential"
		}
		rec := owlmonitor.AlertBindingRun{
			ID:        fmt.Sprintf("ABR-%d-%d", now.UnixNano(), i),
			AlertID:   alertID,
			BindingID: b.ID,
			Kind:      b.Kind,
			Mode:      m,
			Status:    "pending",
			CreatedBy: createdBy,
			CreatedAt: now.Unix(),
		}
		if err := s.Store.CreateAlertBindingRun(&rec); err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}

	if mode == "concurrent" {
		for i := range recs {
			go s.execBindingItem(&recs[i], bindings[i], alertID, nodeID, createdBy)
		}
		return recs, nil
	}
	if mode == "sequential" {
		go s.execBindingChain(recs, bindings, alertID, nodeID, createdBy)
		return recs, nil
	}
	// 自动执行：并发组立即启动，串行组按 seq 链式执行
	var chainRecs []owlmonitor.AlertBindingRun
	var chainBinds []owlmonitor.AlertBinding
	for i := range recs {
		if recs[i].Mode == "concurrent" {
			go s.execBindingItem(&recs[i], bindings[i], alertID, nodeID, createdBy)
		} else {
			chainRecs = append(chainRecs, recs[i])
			chainBinds = append(chainBinds, bindings[i])
		}
	}
	if len(chainRecs) > 0 {
		go s.execBindingChain(chainRecs, chainBinds, alertID, nodeID, createdBy)
	}
	return recs, nil
}

// execBindingChain 顺序串行：前一条到终态后才启动下一条。
func (s *Service) execBindingChain(recs []owlmonitor.AlertBindingRun, bindings []owlmonitor.AlertBinding, alertID, nodeID, createdBy string) {
	for i := range recs {
		s.execBindingItem(&recs[i], bindings[i], alertID, nodeID, createdBy)
	}
}

// execBindingItem 执行单条绑定并回写状态：start → 轮询关联运行至终态。
func (s *Service) execBindingItem(rec *owlmonitor.AlertBindingRun, b owlmonitor.AlertBinding, alertID, nodeID, createdBy string) {
	finish := func(status, msg string) {
		if err := s.Store.FinishAlertBindingRun(rec.ID, status, msg); err != nil {
			logger.Warn("回写绑定执行终态失败", logger.WithOperation("alert_bindings"),
				logger.WithField("binding_id", b.ID), logger.WithError(err))
		}
	}

	refID, err := s.startBinding(b, alertID, nodeID, createdBy)
	if err != nil {
		logger.Warn("告警绑定启动失败", logger.WithOperation("alert_bindings"),
			logger.WithField("binding_id", b.ID), logger.WithError(err))
		finish("failed", err.Error())
		return
	}
	if err := s.Store.UpdateAlertBindingRunRef(rec.ID, refID); err != nil {
		logger.Warn("回写绑定运行关联失败", logger.WithOperation("alert_bindings"),
			logger.WithField("binding_id", b.ID), logger.WithError(err))
	}

	deadline := time.Now().Add(bindingItemTimeout)
	for {
		time.Sleep(bindingPollInterval)
		done, success, status, pollErr := s.bindingItemDone(b.Kind, refID)
		if pollErr != nil {
			finish("failed", pollErr.Error())
			return
		}
		if done {
			if success {
				finish("success", "")
			} else {
				finish("failed", "运行结束： "+status)
			}
			return
		}
		if time.Now().After(deadline) {
			finish("failed", "执行超时（30 分钟）")
			return
		}
	}
}

// startBinding 按类型启动：playbook 走剧本运行链路，script 走单步处置计划。
func (s *Service) startBinding(b owlmonitor.AlertBinding, alertID, nodeID, createdBy string) (string, error) {
	switch b.Kind {
	case "playbook":
		if s.playbookRunner == nil {
			return "", fmt.Errorf("剧本执行器未就绪")
		}
		return s.playbookRunner.RunForAlert(context.Background(), b.Content, nodeID, createdBy)
	case "script":
		return s.StartScriptRun(alertID, b.Name, b.Content, nodeID, createdBy)
	default:
		return "", fmt.Errorf("未知指令类型 %s", b.Kind)
	}
}

// bindingItemDone 查询关联运行是否到终态及其成败。
func (s *Service) bindingItemDone(kind, refID string) (done, success bool, status string, err error) {
	if kind == "playbook" {
		if s.playbookRunner == nil {
			return true, false, "", fmt.Errorf("剧本执行器未就绪")
		}
		done, status, err = s.playbookRunner.PlaybookRunFinished(context.Background(), refID)
		if err != nil {
			return true, false, "", err
		}
		return done, status == "completed", status, nil
	}
	// script：轮询 remedy_run
	run, exists, err := s.Store.GetRemedyRun(refID)
	if err != nil {
		return true, false, "", err
	}
	if !exists {
		return true, false, "", fmt.Errorf("处置计划 %s 不存在", refID)
	}
	switch run.Status {
	case owlmonitor.RunDone:
		return true, true, string(run.Status), nil
	case owlmonitor.RunFailed, owlmonitor.RunStopped:
		return true, false, string(run.Status), nil
	default:
		return false, false, string(run.Status), nil
	}
}
