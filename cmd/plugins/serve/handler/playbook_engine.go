package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
	"unicode/utf8"

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

	// nodeUserCache 缓存节点连接用户（每 run 一个执行器实例，节点集合
	// 固定）：多节点多步骤时避免每步重复查库并解密凭据（R7）。
	userMu    sync.Mutex
	userCache map[string]string
}

// nodeUser 返回节点连接用户；解析失败必须 fail-closed（S15）：静默
// 置空会让 root 规则组整体失效，root 危险命令被无提示放行。
func (e *webCommandExecutor) nodeUser(nodeID string) (string, error) {
	e.userMu.Lock()
	defer e.userMu.Unlock()
	if u, ok := e.userCache[nodeID]; ok {
		return u, nil
	}
	info, err := e.ssh.getNodeInfo(nodeID)
	if err != nil {
		return "", err
	}
	if e.userCache == nil {
		e.userCache = make(map[string]string)
	}
	e.userCache[nodeID] = info.User
	return info.User, nil
}

func (e *webCommandExecutor) Execute(tk *task.Task, nodeMgr controlnode.Manager) error {
	return fmt.Errorf("not supported")
}

func (e *webCommandExecutor) ExecuteOnNode(nodeID string, cmd string, timeout time.Duration) (*task.TaskResult, error) {
	base := e.parentCtx
	if base == nil {
		base = context.Background()
	}
	ctx := base
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(base, timeout)
	defer cancel()

	// 外层硬超时：dial、NewSession、执行器内部任何不响应 ctx 的阻塞点
	// 都无法超出本截止时间。曾出现单步卡住数小时把整个 run 拖死的案例
	// （fail_continue 与 pipeline 均中招）——任何步骤都不允许无限等待。
	type execOutcome struct {
		result *task.TaskResult
		err    error
	}
	ch := make(chan execOutcome, 1)
	go func() {
		result, err := e.executeOnNode(ctx, nodeID, cmd)
		ch <- execOutcome{result: result, err: err}
	}()

	select {
	case out := <-ch:
		return out.result, out.err
	case <-ctx.Done():
		now := time.Now()
		err := fmt.Errorf("命令执行超时(%s)，步骤被强制中止（外层硬超时）", timeout)
		return &task.TaskResult{
			NodeID: nodeID, ExitCode: -1, Error: err,
			Output: err.Error(), StartTime: now, EndTime: now,
		}, err
	}
}

// executeOnNode 执行单步：黑名单检查 + SSH 执行。始终在外层硬超时的
// goroutine 内运行。
func (e *webCommandExecutor) executeOnNode(ctx context.Context, nodeID string, cmd string) (*task.TaskResult, error) {
	if e.check != nil {
		user, userErr := e.nodeUser(nodeID)
		if userErr != nil {
			now := time.Now()
			err := fmt.Errorf("黑名单检查前解析节点失败（fail-closed）: %w", userErr)
			return &task.TaskResult{
				NodeID: nodeID, ExitCode: -1, Error: err,
				Output: err.Error(), StartTime: now, EndTime: now,
			}, err
		}
		if _, err := e.check.CheckForExec(user, cmd, e.force); err != nil {
			now := time.Now()
			return &task.TaskResult{
				NodeID: nodeID, ExitCode: -1, Error: err,
				Output: err.Error(), StartTime: now, EndTime: now,
			}, err
		}
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

// defaultWebCommandTimeout 为 Web 命令执行器在调用方未给超时时的兜底值
// （与剧本动作的 DefaultCommandTimeout 语义不同，勿混用）。
const defaultWebCommandTimeout = 30 * time.Second

// ExecuteOnNodeWithConfig 实现 CommandExecutor 接口；Web 侧无独立的连接阶段，
// 以命令超时为准执行。
func (e *webCommandExecutor) ExecuteOnNodeWithConfig(nodeID string, cmd string, config *owlssh.TimeoutConfig) (*task.TaskResult, error) {
	timeout := defaultWebCommandTimeout
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

	// panic 防护：引擎 goroutine 一旦 panic，进程可能被带崩或 run 永远
	// 停在 running。兜底把 run 标记为 failed 并广播，症状不再无声。
	defer func() {
		if r := recover(); r != nil {
			log.Printf("playbook run %s panicked: %v\n%s", runID, r, debug.Stack())
			errMsg := fmt.Sprintf("执行引擎内部错误(panic): %v", r)
			if err := h.runs.UpdateStatus(ctx, runID, webmodel.RunStatusFailed, errMsg); err == nil {
				if h.History != nil {
					_ = h.History.UpdateOperationStatus(ctx, runID, "failed")
				}
				history.FinishPlaybookRun(runID, "failed", 0, 0)
				h.clearRunningSteps(runID)
				h.broadcastRunUpdate(ctx, runID)
			}
		}
	}()

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
	defer h.clearRunningSteps(runID)

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
			// 脚本检查与命令检查共用同一个 webCommandExecutor：
			// 同一 fail-closed 语义 + 同一份用户缓存。
			checker, cmdExecRef, force := h.checker, cmdExec, run.DangerConfirmed
			sc.SetScriptCheckFunc(func(nodeID, scriptContent string) error {
				user, userErr := cmdExecRef.nodeUser(nodeID)
				if userErr != nil {
					return fmt.Errorf("黑名单检查前解析节点失败（fail-closed）: %w", userErr)
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

	// 逐步进度：步骤开始/完成事件先进内存队列，由专职 writer 串行写库并
	// 广播。执行器回调只做非阻塞入队——DB 争用（监控长事务/VACUUM 等）
	// 时执行器绝不被拖住，避免"单步卡住数小时"级别的事故。
	// V1→V2 改版曾把逐步推送弄丢，运行中前端什么都看不到——
	// 回归测试 TestExecutePlaybookRunV2_BroadcastsPerStep 锁死此行为。
	taskByName := make(map[string]*pbexec.ParsedTask)
	for _, t := range allParsedTasks(parsedPlaybook) {
		taskByName[t.Name] = t
	}
	type pbStepEvent struct {
		start *struct {
			taskName, nodeID string
		}
		done *pbexec.TaskResult
	}
	stepCh := make(chan pbStepEvent, 1024)
	var closeOnce sync.Once
	defer closeOnce.Do(func() { close(stepCh) })
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for ev := range stepCh {
			switch {
			case ev.start != nil:
				h.addRunningStep(runID, &webmodel.StepResult{
					TaskName: ev.start.taskName,
					NodeID:   ev.start.nodeID,
					Action:   taskByName[ev.start.taskName].Action,
					Status:   "running",
				})
				h.broadcastRunUpdate(ctx, runID)
			case ev.done != nil:
				r, t := ev.done, taskByName[ev.done.TaskName]
				if t == nil {
					continue
				}
				h.removeRunningStep(runID, r.TaskName, r.NodeID)
				step := webStepResultFromTask(t, r)
				if err := h.runs.AppendResult(ctx, runID, step); err != nil {
					// 写库失败也必须广播：前端至少能看到步骤开始/完成的实时状态
					log.Printf("append step result: %v", err)
				} else {
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
				}
				h.broadcastRunUpdate(ctx, runID)
			}
		}
	}()

	enqueue := func(ev pbStepEvent) {
		// 队列满时阻塞入队：1024 个在途步骤远超正常规模，宁可等也不丢结果；
		// writer 正常情况下消化极快，仅 DB 严重争用时短暂等待。
		stepCh <- ev
	}
	if setter, ok := pbExecutor.(interface {
		SetProgressFunc(func(*pbexec.TaskResult))
	}); ok {
		setter.SetProgressFunc(func(r *pbexec.TaskResult) {
			enqueue(pbStepEvent{done: r})
		})
	}
	if setter, ok := pbExecutor.(interface {
		SetStepStartFunc(func(taskName, nodeID string))
	}); ok {
		setter.SetStepStartFunc(func(taskName, nodeID string) {
			enqueue(pbStepEvent{start: &struct {
				taskName, nodeID string
			}{taskName, nodeID}})
		})
	}

	execution, execErr := pbExecutor.Execute(parsedPlaybook, targetNodes, extraVars)

	// 执行结束：关闭事件队列并等待 writer 把剩余步骤全部落库，
	// 之后才能写终态，否则 results 会缺最后几步。
	closeOnce.Do(func() { close(stepCh) })
	<-writerDone

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
		// 附带执行中步骤快照（仅内存）：前端在长任务执行期间也能看到
		// "当前卡在哪一步、哪个节点"
		run.RunningSteps = h.snapshotRunningSteps(runID)
		h.hub.Broadcast(WSMessage{Type: "playbook_run_update", Data: run})
	}
}

func allParsedTasks(pb *pbexec.ParsedPlaybook) []*pbexec.ParsedTask {
	allTasks := make([]*pbexec.ParsedTask, 0, len(pb.PreTasks)+len(pb.Tasks)+len(pb.PostTasks))
	allTasks = append(allTasks, pb.PreTasks...)
	allTasks = append(allTasks, pb.Tasks...)
	allTasks = append(allTasks, pb.PostTasks...)
	return allTasks
}

// stepStatusOf 步骤状态与错误信息的唯一判定口径：
// Error 优先（拦截/连接类失败），其次非零退出码。
func stepStatusOf(r *pbexec.TaskResult) (status, errMsg string) {
	if r.Error != nil {
		return "failed", r.Error.Error()
	}
	if r.ExitCode != 0 {
		return "failed", fmt.Sprintf("exit code %d", r.ExitCode)
	}
	return "completed", ""
}

func webStepResultFromTask(t *pbexec.ParsedTask, r *pbexec.TaskResult) *webmodel.StepResult {
	status, errMsg := stepStatusOf(r)
	output := truncateRunes(r.Output, stepOutputLimit)
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

// stepOutputLimit 单条步骤输出的存储/广播上限（字节）。
const stepOutputLimit = 4096

// truncateRunes 超限时在 rune 边界回退后截断并追加省略号，
// 避免把多字节 UTF-8 字符切成非法序列（乱码入库/广播）。
func truncateRunes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit - 3
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
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
			status, errMsg := stepStatusOf(r)
			stdout := truncateRunes(r.Output, stepOutputLimit)
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
