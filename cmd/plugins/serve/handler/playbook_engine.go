package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	webmodel "github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	commonmodel "github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	controlnode "github.com/cangyunye/go-owl/internal/control/node"
	pbexec "github.com/cangyunye/go-owl/internal/control/playbook"
	"github.com/cangyunye/go-owl/internal/control/task"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/node"
	owlssh "github.com/cangyunye/go-owl/internal/ssh"
)

// playbookHub 抽象 WS 广播能力；测试注入 fake 捕获每条推送，
// 防止引擎改版时再次悄悄丢掉逐步广播（V1→V2 的教训）。
type playbookHub interface {
	Broadcast(WSMessage)
	BroadcastHistoryUpdate()
}

// playbookSSHRunner 抽象远端命令执行；测试注入 fake 控制退出码/延迟/挂死。
type playbookSSHRunner interface {
	Execute(ctx context.Context, nodeID, command string) (string, int, error)
	getNodeInfo(nodeID string) (*nodeSSHInfo, error)
}

type webCommandExecutor struct {
	ssh   playbookSSHRunner
	check *blacklist.Checker
	force bool
	// parentCtx 为该 run 的取消上下文，每步命令从中派生超时 ctx；
	// 取消运行即中断所有在跑命令。
	parentCtx context.Context
}

func (e *webCommandExecutor) Execute(tk *task.Task, nodeMgr controlnode.Manager) error {
	return fmt.Errorf("not supported")
}

func (e *webCommandExecutor) ExecuteOnNode(nodeID string, cmd string, timeout time.Duration) (*task.TaskResult, error) {
	if e.check != nil {
		var user string
		if info, err := e.ssh.getNodeInfo(nodeID); err == nil {
			user = info.User
		}
		if _, err := e.check.CheckForExec(user, cmd, e.force); err != nil {
			now := time.Now()
			return &task.TaskResult{
				NodeID: nodeID, ExitCode: -1, Error: err,
				Output: err.Error(), StartTime: now, EndTime: now,
			}, err
		}
	}

	base := e.parentCtx
	if base == nil {
		base = context.Background()
	}
	ctx := base
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(base, timeout)
		defer cancel()
	}

	start := time.Now()
	output, exitCode, err := e.ssh.Execute(ctx, nodeID, cmd)
	end := time.Now()

	// 非零退出码必须以 error 暴露：执行器的 pipeline 快停语义完全依赖
	// err 非 nil。曾因返回 err=nil 导致 Web 端 pipeline 对退出码失败
	// "假装没失败"，与 CLI 行为分裂。
	if err == nil && exitCode != 0 {
		err = fmt.Errorf("exit code %d", exitCode)
	}

	result := &task.TaskResult{
		NodeID:    nodeID,
		ExitCode:  exitCode,
		Output:    output,
		Error:     err,
		StartTime: start,
		EndTime:   end,
	}
	return result, err
}

// ExecuteOnNodeWithConfig 实现 CommandExecutor 接口；Web 侧无独立的连接阶段，
// 以命令超时为准执行。
func (e *webCommandExecutor) ExecuteOnNodeWithConfig(nodeID string, cmd string, config *owlssh.TimeoutConfig) (*task.TaskResult, error) {
	timeout := 30 * time.Second
	if config != nil && config.CommandTimeout > 0 {
		timeout = config.CommandTimeout
	}
	return e.ExecuteOnNode(nodeID, cmd, timeout)
}

type webNodeManager struct {
	db    *sql.DB
	nodes map[string]*commonmodel.Node
}

func newWebNodeManager(db *sql.DB, nodeIDs []string) *webNodeManager {
	m := &webNodeManager{
		db:    db,
		nodes: make(map[string]*commonmodel.Node),
	}
	for _, id := range nodeIDs {
		n, err := store.ScanNodeRow(db.QueryRow(
			`SELECT `+store.NodeRowColumns+` FROM nodes WHERE id = ?`, id,
		))
		if err != nil {
			continue
		}
		m.nodes[n.ID] = n
	}
	return m
}

func (m *webNodeManager) Register(node *commonmodel.Node) error                       { return nil }
func (m *webNodeManager) Unregister(id string) error                                  { return nil }
func (m *webNodeManager) UpdateStatus(id string, status commonmodel.NodeStatus) error { return nil }

func (m *webNodeManager) GetByID(id string) (*commonmodel.Node, error) {
	if n, ok := m.nodes[id]; ok {
		return n, nil
	}
	return nil, fmt.Errorf("node %s not found", id)
}

func (m *webNodeManager) List() []*commonmodel.Node {
	nodes := make([]*commonmodel.Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

func (m *webNodeManager) GetByGroup(group string) []*commonmodel.Node {
	var result []*commonmodel.Node
	for _, n := range m.nodes {
		for _, g := range n.Groups {
			if g == group {
				result = append(result, n)
				break
			}
		}
	}
	return result
}

func (m *webNodeManager) GetByLabels(labels map[string]string) []*commonmodel.Node {
	var result []*commonmodel.Node
	for _, n := range m.nodes {
		match := true
		for k, v := range labels {
			if nv, ok := n.Labels[k]; !ok || nv != v {
				match = false
				break
			}
		}
		if match {
			result = append(result, n)
		}
	}
	return result
}

func (m *webNodeManager) GetOnlineNodes() []*commonmodel.Node { return m.List() }
func (m *webNodeManager) Count() int                          { return len(m.nodes) }

func (m *webNodeManager) SearchByName(pattern string) []*commonmodel.Node {
	return nil
}

func (m *webNodeManager) SearchByAddress(pattern string) []*commonmodel.Node {
	return nil
}

func (h *PlaybookHandler) executePlaybookRunV2(runID string) {
	ctx := context.Background()

	run, err := h.runs.Get(ctx, runID)
	if err != nil {
		return
	}

	if run.Status == webmodel.RunStatusCancelled {
		return
	}

	// runCtx 只承载 SSH 执行；库操作始终用 ctx（后台），
	// 否则取消后连终态都写不进去。运行中取消靠 ctx 传播：
	// 引擎只在启动时检查一次数据库状态，改库是拦不住在跑命令的。
	runCtx, cancelRunCtx := context.WithCancel(context.Background())
	cleanup := h.registerRunCancel(runID, cancelRunCtx)
	defer cleanup()

	h.runs.UpdateStatus(ctx, runID, webmodel.RunStatusRunning, "")
	run.Status = webmodel.RunStatusRunning
	if h.hub != nil {
		h.hub.Broadcast(WSMessage{Type: "playbook_run_update", Data: run})
	}

	parser := pbexec.NewParser()
	parsedPlaybook, err := parser.ParseFromFile(run.PlaybookFile)
	if err != nil {
		h.runs.UpdateStatus(ctx, runID, webmodel.RunStatusFailed, "parse playbook failed: "+err.Error())
		h.broadcastRunUpdate(ctx, runID)
		return
	}

	nodeMgr := newWebNodeManager(h.db, run.TargetNodes)
	sshRunner := h.sshRunner
	if sshRunner == nil {
		sshRunner = &sshExecutor{db: h.db}
	}
	cmdExec := &webCommandExecutor{
		parentCtx: runCtx,
		ssh:       sshRunner,
		check:     h.checker,
		force:     run.DangerConfirmed,
	}
	nodeResolver := node.NewNodeResolver()

	pbExecutor := pbexec.NewExecutorWithOptions(nodeMgr, cmdExec, nil, nodeResolver, &pbexec.PlaybookOptions{})
	if bds, ok := pbExecutor.(interface{ SetPlaybookBaseDir(string) }); ok {
		bds.SetPlaybookBaseDir(filepath.Dir(run.PlaybookFile))
	}
	if dds, ok := pbExecutor.(interface{ SetDownloadBaseDir(string) }); ok {
		stagingDir := stagingDirFromDB(h.db)
		if err := os.MkdirAll(stagingDir, 0755); err == nil {
			dds.SetDownloadBaseDir(stagingDir)
		}
	}
	// script 动作由 ScriptExecutor 自行拨 SSH，绕过 webCommandExecutor
	// 检查点，必须在这里单独接黑名单与取消上下文。
	if h.checker != nil {
		if sc, ok := pbExecutor.(interface {
			SetScriptCheckFunc(fn pbexec.ScriptCheckFunc)
		}); ok {
			checker, sshRunnerRef, force := h.checker, sshRunner, run.DangerConfirmed
			sc.SetScriptCheckFunc(func(nodeID, scriptContent string) error {
				var user string
				if info, err := sshRunnerRef.getNodeInfo(nodeID); err == nil {
					user = info.User
				}
				_, err := checker.CheckForExec(user, scriptContent, force)
				return err
			})
		}
	}
	if bc, ok := pbExecutor.(interface{ SetBaseContext(ctx context.Context) }); ok {
		bc.SetBaseContext(runCtx)
	}

	var targetNodes []*commonmodel.Node
	for _, n := range nodeMgr.List() {
		targetNodes = append(targetNodes, n)
	}

	extraVars := make(map[string]interface{})
	for k, v := range run.ExtraVars {
		extraVars[k] = v
	}

	pbContent, _ := os.ReadFile(run.PlaybookFile)
	pbHash := history.ComputePlaybookHash(string(pbContent), run.TargetNodes)
	taskCount := len(parsedPlaybook.PreTasks) + len(parsedPlaybook.Tasks) + len(parsedPlaybook.PostTasks)
	history.CreatePlaybookRun(&history.PlaybookRun{
		ID:           runID,
		PlaybookName: run.PlaybookName,
		PlaybookHash: pbHash,
		Nodes:        run.TargetNodes,
		Status:       "running",
		StartedAt:    time.Now(),
		TotalSteps:   taskCount,
	})

	// 运行前下发预估总步数（任务数×节点数），前端据此算进度百分比
	if err := h.runs.SetTotalSteps(ctx, runID, taskCount*len(run.TargetNodes)); err != nil {
		log.Printf("set total steps: %v", err)
	}

	// 逐步进度：每个节点步骤完成（含失败）即写库并广播。
	// V1→V2 改版曾把逐步推送弄丢，运行中前端什么都看不到——
	// 回归测试 TestExecutePlaybookRunV2_BroadcastsPerStep 锁死此行为。
	taskByName := make(map[string]*pbexec.ParsedTask)
	for _, t := range allParsedTasks(parsedPlaybook) {
		taskByName[t.Name] = t
	}
	var progressMu sync.Mutex
	if setter, ok := pbExecutor.(interface {
		SetProgressFunc(func(*pbexec.TaskResult))
	}); ok {
		setter.SetProgressFunc(func(r *pbexec.TaskResult) {
			t := taskByName[r.TaskName]
			if t == nil {
				return
			}
			// 多节点并发执行时回调来自不同 goroutine，
			// AppendResult 是读-改-写，必须串行化防丢结果。
			progressMu.Lock()
			defer progressMu.Unlock()
			step := webStepResultFromTask(t, r)
			if err := h.runs.AppendResult(ctx, runID, step); err != nil {
				log.Printf("append step result: %v", err)
				return
			}
			ce := &store.CommandExecution{
				TaskID: runID, NodeID: step.NodeID,
				Command:  r.Command,
				ExitCode: step.ExitCode, Stdout: step.Output, Stderr: step.Error,
				DurationMs: step.DurationMs, Success: step.ExitCode == 0, CreatedAt: time.Now().UTC(),
			}
			if ce.Command == "" {
				ce.Command = t.Name
			}
			if e := h.History.RecordCommandExecution(ctx, ce); e != nil {
				log.Printf("record command execution: %v", e)
			}
			h.broadcastRunUpdate(ctx, runID)
		})
	}

	execution, execErr := pbExecutor.Execute(parsedPlaybook, targetNodes, extraVars)

	recordWebStepStates(runID, parsedPlaybook, execution)

	failed := execution.FailureCount()
	success := execution.SuccessCount()
	finalStatus := webmodel.RunStatusCompleted
	opStatus := "completed"
	histStatus := "completed"
	if failed > 0 {
		finalStatus = webmodel.RunStatusFailed
		opStatus = "failed"
		histStatus = "failed"
		if success > 0 {
			histStatus = "partial_failure"
		}
	}
	if execErr != nil && finalStatus == webmodel.RunStatusCompleted {
		finalStatus = webmodel.RunStatusFailed
		opStatus = "failed"
		histStatus = "failed"
	}

	// 失败原因必须写进 run.error：中止错误优先，其次失败步骤计数。
	// 传空字符串会让 run 变 failed 却无任何解释（"静默失败"）。
	errMsg := ""
	switch {
	case execErr != nil:
		errMsg = execErr.Error()
	case failed > 0:
		errMsg = fmt.Sprintf("%d/%d 个步骤执行失败", failed, failed+success)
	}

	// 用户取消优先于失败：被取消的运行不得误标为 failed/running
	if runCtx.Err() != nil {
		finalStatus = webmodel.RunStatusCancelled
		opStatus = "cancelled"
		histStatus = "cancelled"
		errMsg = "运行已被用户取消"
	}

	h.runs.UpdateStatus(ctx, runID, finalStatus, errMsg)
	if err := h.History.UpdateOperationStatus(ctx, runID, opStatus); err != nil {
		log.Printf("update op status: %v", err)
	}
	history.FinishPlaybookRun(runID, histStatus, success, failed)

	if h.hub != nil {
		h.hub.BroadcastHistoryUpdate()
	}
	h.broadcastRunUpdate(ctx, runID)
}

func (h *PlaybookHandler) broadcastRunUpdate(ctx context.Context, runID string) {
	if h.hub == nil {
		return
	}
	run, err := h.runs.Get(ctx, runID)
	if err == nil {
		h.hub.Broadcast(WSMessage{Type: "playbook_run_update", Data: run})
	}
}

func toWebStepResults(pb *pbexec.ParsedPlaybook, exec *pbexec.PlaybookExecution) []*webmodel.StepResult {
	if exec == nil {
		return nil
	}

	var results []*webmodel.StepResult
	for _, t := range allParsedTasks(pb) {
		for _, r := range exec.Results[t.Name] {
			results = append(results, webStepResultFromTask(t, r))
		}
	}
	return results
}

func allParsedTasks(pb *pbexec.ParsedPlaybook) []*pbexec.ParsedTask {
	allTasks := make([]*pbexec.ParsedTask, 0, len(pb.PreTasks)+len(pb.Tasks)+len(pb.PostTasks))
	allTasks = append(allTasks, pb.PreTasks...)
	allTasks = append(allTasks, pb.Tasks...)
	allTasks = append(allTasks, pb.PostTasks...)
	return allTasks
}

func webStepResultFromTask(t *pbexec.ParsedTask, r *pbexec.TaskResult) *webmodel.StepResult {
	status := "completed"
	errMsg := ""
	if r.Error != nil {
		status = "failed"
		errMsg = r.Error.Error()
	} else if r.ExitCode != 0 {
		status = "failed"
		errMsg = fmt.Sprintf("exit code %d", r.ExitCode)
	}
	output := r.Output
	if len(output) > 4096 {
		output = output[:4093] + "..."
	}
	return &webmodel.StepResult{
		TaskName:   t.Name,
		NodeID:     r.NodeID,
		Action:     t.Action,
		Status:     status,
		ExitCode:   r.ExitCode,
		Output:     output,
		Error:      errMsg,
		DurationMs: r.EndTime.Sub(r.StartTime).Milliseconds(),
	}
}

func recordWebStepStates(runID string, pb *pbexec.ParsedPlaybook, exec *pbexec.PlaybookExecution) {
	if exec == nil {
		return
	}

	allTasks := make([]*pbexec.ParsedTask, 0, len(pb.PreTasks)+len(pb.Tasks)+len(pb.PostTasks))
	allTasks = append(allTasks, pb.PreTasks...)
	allTasks = append(allTasks, pb.Tasks...)
	allTasks = append(allTasks, pb.PostTasks...)

	for stepIndex, t := range allTasks {
		taskResults, ok := exec.Results[t.Name]
		if !ok {
			continue
		}
		for _, r := range taskResults {
			status := "completed"
			errMsg := ""
			if r.Error != nil {
				status = "failed"
				errMsg = r.Error.Error()
			} else if r.ExitCode != 0 {
				status = "failed"
				errMsg = fmt.Sprintf("exit code %d", r.ExitCode)
			}
			stdout := r.Output
			if len(stdout) > 4096 {
				stdout = stdout[:4093] + "..."
			}
			startedAt := r.StartTime
			finishedAt := r.EndTime
			history.UpsertStepState(&history.PlaybookStepState{
				RunID:      runID,
				NodeID:     r.NodeID,
				StepIndex:  stepIndex,
				StepName:   t.Name,
				Action:     t.Action,
				Status:     status,
				StartedAt:  &startedAt,
				FinishedAt: &finishedAt,
				DurationMs: r.EndTime.Sub(r.StartTime).Milliseconds(),
				ExitCode:   r.ExitCode,
				Stdout:     stdout,
				Stderr:     errMsg,
				Error:      errMsg,
			})
		}
	}
}
