package monitor

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/internal/control/blacklist"
)

// TargetResolver 按节点 ID 解析执行目标（serve 中查询 nodes 表获取凭据）。
type TargetResolver func(nodeID string) (*Target, error)

// RunExecutor 串行执行处置计划：脚本步骤经黑名单校验后通过 SSH 执行，
// sop 人工步骤自动跳过，失败按 StopOnError 决定是否继续，支持中途停止。
type RunExecutor struct {
	factory   ExecerFactory
	resolve   TargetResolver
	checker   *blacklist.Checker
	timeout   time.Duration
	stepDelay time.Duration // 测试用：步骤间停顿，便于并发停止
	now       func() int64
}

// NewRunExecutor 创建处置执行器，默认单步超时 60s。
func NewRunExecutor(factory ExecerFactory, resolve TargetResolver) *RunExecutor {
	return &RunExecutor{
		factory: factory,
		resolve: resolve,
		checker: blacklist.NewDefaultChecker(),
		timeout: 60 * time.Second,
		now:     func() int64 { return time.Now().Unix() },
	}
}

// ExecuteRun 执行计划中未完成步骤并实时回写存储。幂等：已结束的计划直接返回。
func (e *RunExecutor) ExecuteRun(ctx context.Context, runID string, store *Store) error {
	run, exists, err := store.GetRemedyRun(runID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("monitor: 执行计划 %s 不存在", runID)
	}
	if run.IsTerminal() {
		return nil
	}
	if run.Status == RunPending {
		if err := store.UpdateRemedyRunStatus(runID, RunRunning); err != nil {
			return err
		}
		run.Status = RunRunning
	}

	for i := range run.Steps {
		// 每步前重查库状态：避免本地副本过期（stopOnError 失败时库内已跳过剩余）
		current, _, err := store.GetRemedyRun(runID)
		if err != nil {
			return err
		}
		if current.IsTerminal() {
			// 幂等收尾：终态前若有遗漏的 pending 步骤（如执行中被停止），补标 skipped
			return e.skipRemaining(runID, current.Status, store)
		}
		st := &current.Steps[i]
		// 已结束步骤（恢复执行场景）跳过
		if st.Status == StepSuccess || st.Status == StepSkipped {
			continue
		}
		if err := e.stepGate(ctx, current, st, store); err != nil {
			return err
		}
	}

	// 收尾：刷新最终状态
	final, _, err := store.GetRemedyRun(runID)
	if err != nil {
		return err
	}
	if final.Status == RunRunning || final.Status == RunPending {
		return store.UpdateRemedyRunStatus(runID, RunDone)
	}
	return nil
}

// stepGate 处理单步：检查停止/取消 → 执行或跳过 → 失败处理。
func (e *RunExecutor) stepGate(ctx context.Context, run *RemedyRun, st *RemedyStep, store *Store) error {
	// 停止/取消检查：用户 Stop 或 ctx 取消
	if err := ctx.Err(); err != nil {
		return e.finishAborted(run.ID, RunStopped, store)
	}
	current, _, err := store.GetRemedyRun(run.ID)
	if err != nil {
		return err
	}
	if current.Status == RunStopped {
		return e.skipRemaining(run.ID, RunStopped, store)
	}

	switch st.Kind {
	case "sop":
		// 人工指引步骤：不自动执行
		return e.markSkipped(run.ID, st, "人工指引步骤，请在告警详情查看并手动执行", store)
	case "playbook":
		return e.markSkipped(run.ID, st, "剧本类对策暂不支持自动执行，请在剧本管理页手动运行", store)
	case "script":
		return e.executeScript(ctx, run, st, store)
	default:
		return e.markSkipped(run.ID, st, "未知对策类型 "+st.Kind, store)
	}
}

// executeScript 黑名单校验后通过 SSH 执行脚本内容。
func (e *RunExecutor) executeScript(ctx context.Context, run *RemedyRun, st *RemedyStep, store *Store) error {
	start := e.now()
	if err := store.UpdateRemedyStep(run.ID, st.Order, func(s *RemedyStep) {
		s.Status = StepRunning
		s.StartedAt = start
	}); err != nil {
		return err
	}

	// 黑名单闸门：命中危险命令不执行
	if res := e.checker.Check(run.CreatedBy, st.Content); res.Blocked {
		patterns := make([]string, 0, len(res.Matches))
		for _, m := range res.Matches {
			patterns = append(patterns, m.Pattern)
		}
		return e.finishStep(run, st, StepFailed, -1,
			fmt.Sprintf("命中危险命令黑名单（%s），已拦截未执行", strings.Join(patterns, ", ")), store, true)
	}

	// 解析执行目标
	target, err := e.resolve(run.NodeID)
	if err != nil {
		return e.finishStep(run, st, StepFailed, -1, "解析执行节点失败: "+err.Error(), store, true)
	}
	exec, err := e.factory.NewExecer(target)
	if err != nil {
		return e.finishStep(run, st, StepFailed, -1, "创建执行器失败: "+err.Error(), store, true)
	}

	// base64 管道执行，避免转义问题
	cmd := fmt.Sprintf("echo '%s' | base64 -d | bash", base64.StdEncoding.EncodeToString([]byte(st.Content)))
	if e.stepDelay > 0 {
		time.Sleep(e.stepDelay)
	}
	if err := ctx.Err(); err != nil {
		return e.finishAborted(run.ID, RunStopped, store)
	}
	code, output, execErr := exec.Execute(cmd, e.timeout)
	success := execErr == nil && code == 0
	msg := output
	if execErr != nil {
		msg = execErr.Error()
	} else if !success {
		msg = fmt.Sprintf("退出码 %d\n%s", code, output)
	}
	return e.finishStep(run, st, boolStatus(success), code, msg, store, !success)
}

// finishStep 写步骤结果 + 反馈对策库 + 按 StopOnError 处理失败。
func (e *RunExecutor) finishStep(run *RemedyRun, st *RemedyStep, status RemedyStepStatus, exitCode int, output string, store *Store, failed bool) error {
	now := e.now()
	if err := store.UpdateRemedyStep(run.ID, st.Order, func(s *RemedyStep) {
		s.Status = status
		s.ExitCode = exitCode
		s.Output = output
		s.FinishedAt = now
		s.NodeID = run.NodeID
	}); err != nil {
		return err
	}
	// 反馈闭环（脚本类才计数）
	if st.Kind == "script" {
		_ = store.RecordExecution(st.RemedyID, status == StepSuccess)
	}
	if failed && run.StopOnError {
		return e.skipRemaining(run.ID, RunFailed, store)
	}
	return nil
}

func boolStatus(ok bool) RemedyStepStatus {
	if ok {
		return StepSuccess
	}
	return StepFailed
}

// markSkipped 标记步骤跳过（人工/不支持类型）。
func (e *RunExecutor) markSkipped(runID string, st *RemedyStep, reason string, store *Store) error {
	return store.UpdateRemedyStep(runID, st.Order, func(s *RemedyStep) {
		s.Status = StepSkipped
		s.Output = reason
		s.FinishedAt = e.now()
	})
}

// skipRemaining 停止/失败：剩余 pending 步骤全部标记 skipped，整单置终态。
func (e *RunExecutor) skipRemaining(runID string, status RemedyRunStatus, store *Store) error {
	run, _, err := store.GetRemedyRun(runID)
	if err != nil {
		return err
	}
	for _, st := range run.Steps {
		if st.Status != StepPending && st.Status != StepRunning {
			continue
		}
		reason := "整单中止"
		if status == RunFailed {
			reason = "前置步骤失败"
		}
		_ = store.UpdateRemedyStep(runID, st.Order, func(s *RemedyStep) {
			s.Status = StepSkipped
			s.Output = reason
			s.FinishedAt = e.now()
		})
	}
	return store.UpdateRemedyRunStatus(runID, status)
}

func (e *RunExecutor) finishAborted(runID string, status RemedyRunStatus, store *Store) error {
	return e.skipRemaining(runID, status, store)
}
