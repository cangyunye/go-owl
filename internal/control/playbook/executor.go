package playbook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/control/script"
	controlnode "github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/control/task"
	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
)

type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning  ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed   ExecutionStatus = "failed"
	ExecutionStatusAborted  ExecutionStatus = "aborted"
)

type ExecutionMode string

const (
	ExecutionModeFailContinue ExecutionMode = "fail_continue"
	ExecutionModePipeline     ExecutionMode = "pipeline"
)

type TaskResult struct {
	TaskName  string
	NodeID    string
	Action    string
	ExitCode  int
	Output    string
	Error     error
	Changed   bool
	StartTime time.Time
	EndTime   time.Time
}

type PlaybookExecution struct {
	ID          string
	Playbook    *ParsedPlaybook
	TargetNodes []*model.Node
	Status      ExecutionStatus
	Results     map[string][]*TaskResult
	Vars        map[string]interface{}
	Error       string
	StartTime   time.Time
	EndTime     *time.Time
}

type PlaybookOptions struct {
	TimeoutConfig *ssh.TimeoutConfig
	RetryConfig   *command.RetryConfig
}

type Executor interface {
	Execute(playbook *ParsedPlaybook, targets []*model.Node, extraVars map[string]interface{}) (*PlaybookExecution, error)
	ExecuteTask(exec *PlaybookExecution, task *ParsedTask) ([]*TaskResult, error)
	Stop(execID string) error
}

type checkpoint struct {
	Phase string // pre_tasks / tasks / post_tasks
	Index int    // task index
}

type playbookExecutor struct {
	nodeMgr      controlnode.Manager
	cmdExec      command.CommandExecutor
	taskSched    task.Scheduler
	runner       ActionRunner
	options      *PlaybookOptions
	nodeResolver *node.NodeResolver

	// 断点续跑
	resumeFrom     *checkpoint // 非 nil 时从此处跳过已执行任务
	checkpointFunc func(phase string, index int) // 保存 checkpoint 的回调
}

// SetResumeFrom 设置断点续跑的起始位置
func (e *playbookExecutor) SetResumeFrom(phase string, index int) {
	e.resumeFrom = &checkpoint{Phase: phase, Index: index}
}

// SetCheckpointFunc 设置 checkpoint 保存回调
func (e *playbookExecutor) SetCheckpointFunc(fn func(phase string, index int)) {
	e.checkpointFunc = fn
}

func NewExecutor(nodeMgr controlnode.Manager, cmdExec command.CommandExecutor, taskSched task.Scheduler, nodeResolver *node.NodeResolver) Executor {
	runner := NewDefaultActionRunner(cmdExec, nodeResolver)
	return &playbookExecutor{
		nodeMgr:      nodeMgr,
		cmdExec:      cmdExec,
		taskSched:    taskSched,
		runner:       runner,
		nodeResolver: nodeResolver,
	}
}

func NewExecutorWithOptions(nodeMgr controlnode.Manager, cmdExec command.CommandExecutor, taskSched task.Scheduler, nodeResolver *node.NodeResolver, opts *PlaybookOptions) Executor {
	runner := NewDefaultActionRunnerWithOptions(cmdExec, nodeResolver, opts)
	return &playbookExecutor{
		nodeMgr:      nodeMgr,
		cmdExec:      cmdExec,
		taskSched:    taskSched,
		runner:       runner,
		options:      opts,
		nodeResolver: nodeResolver,
	}
}

// SetPlaybookBaseDir 设置 Playbook 基础目录，用于解析相对路径
func (e *playbookExecutor) SetPlaybookBaseDir(path string) {
	if r, ok := e.runner.(*defaultActionRunner); ok {
		r.SetPlaybookBaseDir(path)
	}
}

type ActionRunner interface {
	RunAction(action string, args map[string]interface{}, nodeID string, vars map[string]interface{}, actionOpts *ActionOptions) (*TaskResult, error)
}

type defaultActionRunner struct {
	cmdExec       command.CommandExecutor
	nodeResolver  *node.NodeResolver
	transferMgr   *transfer.TransferManager
	opts          *PlaybookOptions
	playbookBaseDir string
}

func NewDefaultActionRunner(cmdExec command.CommandExecutor, nodeResolver *node.NodeResolver) *defaultActionRunner {
	return &defaultActionRunner{
		cmdExec:       cmdExec,
		nodeResolver:  nodeResolver,
		transferMgr:   transfer.NewTransferManager(nodeResolver),
	}
}

func NewDefaultActionRunnerWithOptions(cmdExec command.CommandExecutor, nodeResolver *node.NodeResolver, opts *PlaybookOptions) *defaultActionRunner {
	return &defaultActionRunner{
		cmdExec:       cmdExec,
		nodeResolver:  nodeResolver,
		transferMgr:   transfer.NewTransferManager(nodeResolver),
		opts:          opts,
	}
}

// SetPlaybookBaseDir 设置 Playbook 所在的基础目录，用于解析相对路径
func (r *defaultActionRunner) SetPlaybookBaseDir(path string) {
	r.playbookBaseDir = path
}

// resolvePath 相对于 Playbook 目录解析路径
func (r *defaultActionRunner) resolvePath(path string) string {
	if r.playbookBaseDir != "" && !filepath.IsAbs(path) {
		return filepath.Join(r.playbookBaseDir, path)
	}
	return path
}

func (r *defaultActionRunner) RunAction(action string, args map[string]interface{}, nodeID string, vars map[string]interface{}, actionOpts *ActionOptions) (*TaskResult, error) {
	result := &TaskResult{
		TaskName:  action,
		NodeID:    nodeID,
		Action:    action,
		StartTime: time.Now(),
	}

	// 根据 action 类型执行不同的操作
	switch strings.ToLower(action) {
	case "script":
		return r.runScript(result, args, nodeID, vars, actionOpts)
	case "upload":
		return r.runUpload(result, args, nodeID, vars, actionOpts)
	case "download":
		return r.runDownload(result, args, nodeID, vars, actionOpts)
	case "command", "cmd", "shell":
		fallthrough
	default:
		return r.runCommand(result, args, nodeID, vars, actionOpts)
	}
}

// runCommand 执行命令类型的动作
func (r *defaultActionRunner) runCommand(result *TaskResult, args map[string]interface{}, nodeID string, vars map[string]interface{}, actionOpts *ActionOptions) (*TaskResult, error) {
	var cmd string
	if c, ok := args["cmd"]; ok {
		cmd = fmt.Sprintf("%v", c)
	} else if c, ok := args["command"]; ok {
		cmd = fmt.Sprintf("%v", c)
	} else if c, ok := args["script"]; ok {
		cmd = fmt.Sprintf("bash %v", c)
	} else {
		cmd = fmt.Sprintf("echo 'Action: %s, Args: %v'", result.Action, args)
	}

	// 替换变量
	cmd = r.interpolateVariables(cmd, vars)

	mergedOpts := MergeActionOptions(actionOpts, r.getGlobalDefaults())

	if r.cmdExec != nil {
		timeout := mergedOpts.GetTimeout()
		taskResult, err := r.cmdExec.ExecuteOnNode(nodeID, cmd, timeout)
		if err != nil {
			if mergedOpts.ShouldRetry() {
				taskResult, err = r.executeWithRetry(nodeID, cmd, timeout, mergedOpts.GetRetryConfig())
			}
			if err != nil {
				result.Error = err
				result.EndTime = time.Now()
				return result, err
			}
		}
		result.ExitCode = taskResult.ExitCode
		result.Output = taskResult.Output
		result.Changed = taskResult.ExitCode != 0
	} else {
		result.ExitCode = 0
		result.Output = fmt.Sprintf("Mock: %s", cmd)
	}
	result.EndTime = time.Now()

	return result, nil
}

// runScript 执行脚本类型的动作
func (r *defaultActionRunner) runScript(result *TaskResult, args map[string]interface{}, nodeID string, vars map[string]interface{}, actionOpts *ActionOptions) (*TaskResult, error) {
	scriptPath, ok := args["script"].(string)
	if !ok {
		result.Error = fmt.Errorf("script action requires 'script' argument")
		result.EndTime = time.Now()
		return result, result.Error
	}

	// 解析路径和替换变量
	scriptPath = r.resolvePath(r.interpolateVariables(scriptPath, vars))

	// 检查脚本文件是否存在
	if !(len(scriptPath) > 8 && (scriptPath[:7] == "http://" || scriptPath[:8] == "https://")) {
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			result.Error = fmt.Errorf("script file not found: %s", scriptPath)
			result.EndTime = time.Now()
			return result, result.Error
		}
	}

	// 读取其他参数
	opts := &script.ScriptExecutionOptions{
		DestDir: "/tmp",
	}

	if v, ok := args["dest"].(string); ok {
		opts.DestDir = r.interpolateVariables(v, vars)
	}
	if v, ok := args["args"].(string); ok {
		opts.Args = r.interpolateVariables(v, vars)
	}
	if v, ok := args["inline"].(bool); ok {
		opts.Inline = v
	}
	if v, ok := args["keep"].(bool); ok {
		opts.Keep = v
	}

	mergedOpts := MergeActionOptions(actionOpts, r.getGlobalDefaults())
	opts.Timeout = mergedOpts.GetTimeout()

	// 创建 script executor
	scriptExec := script.NewScriptExecutor(r.nodeResolver, r.transferMgr)

	// 执行脚本
	results, err := scriptExec.ExecuteScript(scriptPath, []string{nodeID}, opts)
	if err != nil {
		result.Error = err
		result.EndTime = time.Now()
		return result, err
	}

	if len(results) > 0 {
		scriptResult := results[0]
		result.ExitCode = scriptResult.ExitCode
		result.Output = scriptResult.Output
		result.Error = scriptResult.Error
		result.Changed = scriptResult.ExitCode != 0
	}

	result.EndTime = time.Now()
	return result, result.Error
}

// runUpload 执行上传动作
func (r *defaultActionRunner) runUpload(result *TaskResult, args map[string]interface{}, nodeID string, vars map[string]interface{}, actionOpts *ActionOptions) (*TaskResult, error) {
	src, ok := args["src"].(string)
	if !ok {
		result.Error = fmt.Errorf("upload requires 'src' argument")
		result.EndTime = time.Now()
		return result, result.Error
	}

	dest, ok := args["dest"].(string)
	if !ok {
		result.Error = fmt.Errorf("upload requires 'dest' argument")
		result.EndTime = time.Now()
		return result, result.Error
	}

	// 解析路径和替换变量
	src = r.resolvePath(r.interpolateVariables(src, vars))
	dest = r.interpolateVariables(dest, vars)

	// 检查 dest 是否以 / 结尾，如果是则拼接原文件名
	if len(dest) > 0 && dest[len(dest)-1] == '/' {
		// 获取原文件名
		fileName := getFileNameFromPath(src)
		if fileName != "" {
			dest = dest + fileName
		}
	}

	// 构建上传选项
	opts := &transfer.UploadOptions{
		Parallel:  true,
		Resume:    true,
		Overwrite: true,
	}

	if v, ok := args["overwrite"].(bool); ok {
		opts.Overwrite = v
	}
	if v, ok := args["no-overwrite"].(bool); ok {
		opts.NoOverwrite = v
	}
	if v, ok := args["resume"].(bool); ok {
		opts.Resume = v
	}

	// 执行上传
	ctx := context.Background()
	results := r.transferMgr.Upload(ctx, []string{nodeID}, src, dest, opts)

	if len(results) > 0 {
		transferResult := results[0]
		if transferResult.Error != nil {
			result.Error = transferResult.Error
			result.ExitCode = 1
		} else {
			result.ExitCode = 0
			result.Output = fmt.Sprintf("Uploaded %s to %s (method: %s)", src, transferResult.Path, transferResult.Method)
			result.Changed = true
		}
	}

	result.EndTime = time.Now()
	return result, result.Error
}

// runDownload 执行下载动作
func (r *defaultActionRunner) runDownload(result *TaskResult, args map[string]interface{}, nodeID string, vars map[string]interface{}, actionOpts *ActionOptions) (*TaskResult, error) {
	src, ok := args["src"].(string)
	if !ok {
		result.Error = fmt.Errorf("download requires 'src' argument")
		result.EndTime = time.Now()
		return result, result.Error
	}

	dest, ok := args["dest"].(string)
	if !ok {
		result.Error = fmt.Errorf("download requires 'dest' argument")
		result.EndTime = time.Now()
		return result, result.Error
	}

	// 解析路径和替换变量
	src = r.interpolateVariables(src, vars)
	dest = r.resolvePath(r.interpolateVariables(dest, vars))

	// 构建下载选项
	opts := &transfer.DownloadOptions{
		Parallel: true,
		Resume:   true,
	}

	if v, ok := args["subdir"].(bool); ok {
		opts.Subdir = v
	}
	if v, ok := args["name-format"].(string); ok {
		opts.NameFormat = v
	}
	if v, ok := args["resume"].(bool); ok {
		opts.Resume = v
	}

	// 执行下载
	ctx := context.Background()
	results := r.transferMgr.Download(ctx, []string{nodeID}, src, dest, opts)

	if len(results) > 0 {
		transferResult := results[0]
		if transferResult.Error != nil {
			result.Error = transferResult.Error
			result.ExitCode = 1
		} else {
			result.ExitCode = 0
			result.Output = fmt.Sprintf("Downloaded %s to %s (method: %s)", src, transferResult.Path, transferResult.Method)
			result.Changed = true
		}
	}

	result.EndTime = time.Now()
	return result, result.Error
}

// interpolateVariables 简单的变量插值函数
func (r *defaultActionRunner) interpolateVariables(s string, vars map[string]interface{}) string {
	// 这里使用简单的变量替换，实际可以使用 TemplateEngine
	for k, v := range vars {
		placeholder := fmt.Sprintf("{{%s}}", k)
		s = strings.ReplaceAll(s, placeholder, fmt.Sprintf("%v", v))
	}
	// 添加 PLAYBOOK_DIR 变量支持
	playbookDirPlaceholder := "{{PLAYBOOK_DIR}}"
	s = strings.ReplaceAll(s, playbookDirPlaceholder, r.playbookBaseDir)
	// 也支持 ${PLAYBOOK_DIR} 格式
	playbookDirPlaceholder2 := "${PLAYBOOK_DIR}"
	s = strings.ReplaceAll(s, playbookDirPlaceholder2, r.playbookBaseDir)
	return s
}

func (r *defaultActionRunner) getTimeout() time.Duration {
	if r.opts != nil && r.opts.TimeoutConfig != nil {
		return r.opts.TimeoutConfig.CommandTimeout
	}
	return 5 * time.Minute
}

func (r *defaultActionRunner) getGlobalDefaults() *PlaybookDefaults {
	if r.opts == nil {
		return DefaultPlaybookDefaults()
	}
	return &PlaybookDefaults{
		TimeoutConfig: r.opts.TimeoutConfig,
		RetryConfig:   r.opts.RetryConfig,
	}
}

func (r *defaultActionRunner) executeWithRetry(nodeID, cmd string, timeout time.Duration, retryConfig *command.RetryConfig) (*task.TaskResult, error) {
	maxRetries := retryConfig.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := r.cmdExec.ExecuteOnNode(nodeID, cmd, timeout)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if attempt < maxRetries && command.IsRetryable(err, retryConfig) {
			interval := r.calculateRetryInterval(attempt, retryConfig)
			time.Sleep(interval)
			continue
		}
		break
	}

	return nil, lastErr
}

func (r *defaultActionRunner) calculateRetryInterval(attempt int, config *command.RetryConfig) time.Duration {
	interval := config.InitialInterval
	for i := 0; i < attempt; i++ {
		interval *= 2
		if interval > config.MaxInterval {
			interval = config.MaxInterval
			break
		}
	}
	return interval
}

type TaskContext struct {
	Execution         *PlaybookExecution
	Task              *ParsedTask
	NodeID            string
	Item              interface{}
	Vars              map[string]interface{}
	RegisteredResults map[string]interface{}
}

func (e *playbookExecutor) Execute(playbook *ParsedPlaybook, targets []*model.Node, extraVars map[string]interface{}) (*PlaybookExecution, error) {
	exec := &PlaybookExecution{
		ID:          fmt.Sprintf("exec-%d", time.Now().UnixNano()),
		Playbook:    playbook,
		TargetNodes: targets,
		Status:      ExecutionStatusRunning,
		Results:     make(map[string][]*TaskResult),
		Vars:        make(map[string]interface{}),
		StartTime:   time.Now(),
	}

	for k, v := range playbook.Variables {
		exec.Vars[k] = v
	}
	for k, v := range extraVars {
		exec.Vars[k] = v
	}

	for i := range playbook.PreTasks {
		if e.resumeFrom != nil && e.resumeFrom.Phase == "pre_tasks" && i < e.resumeFrom.Index {
			continue
		}
		preTask := playbook.PreTasks[i]
		results, err := e.executeTaskInternal(exec, preTask)
		if err != nil {
			if !preTask.Options.IgnoreErrors {
				exec.Status = ExecutionStatusFailed
				exec.Error = err.Error()
				if e.checkpointFunc != nil {
					e.checkpointFunc("pre_tasks", i)
				}
				return exec, err
			}
		}
		exec.Results[preTask.Name] = append(exec.Results[preTask.Name], results...)
	}

	for i := range playbook.Tasks {
		if e.resumeFrom != nil && e.resumeFrom.Phase == "tasks" && i < e.resumeFrom.Index {
			continue
		}
		mainTask := playbook.Tasks[i]
		shouldContinue := e.shouldContinueExecution(exec)
		if !shouldContinue && exec.Status == ExecutionStatusAborted {
			break
		}

		results, err := e.executeTaskInternal(exec, mainTask)
		if err != nil {
			if !mainTask.Options.IgnoreErrors {
				if playbook.ExecutionMode == ExecutionModePipeline || mainTask.Options.AnyErrorsFatal {
					exec.Status = ExecutionStatusFailed
					exec.Error = err.Error()
					if e.checkpointFunc != nil {
						e.checkpointFunc("tasks", i)
					}
					break
				}
			}
		}
		exec.Results[mainTask.Name] = append(exec.Results[mainTask.Name], results...)

		for _, result := range results {
			if mainTask.Options.Register != "" {
				exec.Vars[mainTask.Options.Register] = result
			}
		}
	}

	for i := range playbook.PostTasks {
		if e.resumeFrom != nil && e.resumeFrom.Phase == "post_tasks" && i < e.resumeFrom.Index {
			continue
		}
		postTask := playbook.PostTasks[i]
		results, err := e.executeTaskInternal(exec, postTask)
		if err != nil {
			if !postTask.Options.IgnoreErrors {
				exec.Status = ExecutionStatusFailed
				exec.Error = err.Error()
				if e.checkpointFunc != nil {
					e.checkpointFunc("post_tasks", i)
				}
				return exec, err
			}
		}
		exec.Results[postTask.Name] = append(exec.Results[postTask.Name], results...)
	}

	now := time.Now()
	exec.EndTime = &now

	if exec.Status == ExecutionStatusRunning {
		hasFailure := false
		for _, results := range exec.Results {
			for _, result := range results {
				if result.ExitCode != 0 && result.Error != nil {
					hasFailure = true
					break
				}
			}
		}
		if hasFailure {
			exec.Status = ExecutionStatusFailed
		} else {
			exec.Status = ExecutionStatusCompleted
		}
	}

	return exec, nil
}

func (e *playbookExecutor) executeTaskInternal(exec *PlaybookExecution, task *ParsedTask) ([]*TaskResult, error) {
	if task.Condition != nil {
		evaluator := NewConditionEvaluator(exec.Vars)
		passes, err := evaluator.Evaluate(task.Condition)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate condition: %w", err)
		}
		if !passes {
			return []*TaskResult{}, nil
		}
	}

	var results []*TaskResult

	isPipeline := exec.Playbook != nil && exec.Playbook.ExecutionMode == ExecutionModePipeline

	if task.Loop != nil {
		for _, item := range task.Loop.Items {
			itemResults, err := e.executeTaskForNode(exec, task, "", item)
			results = append(results, itemResults...)
			if err != nil && !task.Options.IgnoreErrors {
				if isPipeline || task.Options.AnyErrorsFatal {
					return results, err
				}
			}
		}
	} else {
		if len(exec.TargetNodes) == 1 {
			for _, target := range exec.TargetNodes {
				itemResults, err := e.executeTaskForNode(exec, task, target.ID, nil)
				results = append(results, itemResults...)
				if err != nil && !task.Options.IgnoreErrors {
					if isPipeline || task.Options.AnyErrorsFatal {
						return results, err
					}
				}
			}
		} else {
			nodeCount := len(exec.TargetNodes)
			resultsChan := make(chan *TaskResult, nodeCount)
			errChan := make(chan error, nodeCount)
			var wg sync.WaitGroup
			wg.Add(nodeCount)

			for _, target := range exec.TargetNodes {
				go func(nodeID string) {
					defer wg.Done()
					itemResults, err := e.executeTaskForNode(exec, task, nodeID, nil)
					if len(itemResults) > 0 {
						resultsChan <- itemResults[0]
					}
					if err != nil {
						errChan <- err
					}
				}(target.ID)
			}

			go func() {
				wg.Wait()
				close(resultsChan)
				close(errChan)
			}()

			for result := range resultsChan {
				results = append(results, result)
			}

			for err := range errChan {
				if !task.Options.IgnoreErrors {
					if isPipeline || task.Options.AnyErrorsFatal {
						return results, err
					}
				}
			}
		}
	}

	return results, nil
}

func (e *playbookExecutor) executeTaskForNode(exec *PlaybookExecution, task *ParsedTask, nodeID string, item interface{}) ([]*TaskResult, error) {
	taskVars := make(map[string]interface{})
	for k, v := range exec.Vars {
		taskVars[k] = v
	}
	if item != nil {
		taskVars["item"] = item
	}

	if e.runner == nil {
		if e.nodeResolver == nil {
			e.nodeResolver = node.NewNodeResolver()
		}
		e.runner = NewDefaultActionRunnerWithOptions(e.cmdExec, e.nodeResolver, e.options)
	}

	result, err := e.runner.RunAction(task.Action, task.Args, nodeID, taskVars, task.ActionOpts)
	result.TaskName = task.Name

	if err != nil && task.Options.FailedWhen != "" {
		evaluator := NewConditionEvaluator(taskVars)
		failed, _ := evaluator.Evaluate(&Condition{Expression: task.Options.FailedWhen})
		if !failed {
			result.ExitCode = 0
			err = nil
		}
	}

	if err != nil {
		return []*TaskResult{result}, err
	}

	return []*TaskResult{result}, nil
}

func (e *playbookExecutor) shouldContinueExecution(exec *PlaybookExecution) bool {
	return exec.Status == ExecutionStatusRunning
}

func (e *playbookExecutor) ExecuteTask(exec *PlaybookExecution, task *ParsedTask) ([]*TaskResult, error) {
	return e.executeTaskInternal(exec, task)
}

func (e *playbookExecutor) Stop(execID string) error {
	return fmt.Errorf("stop functionality not implemented yet")
}

type ParallelExecutor struct {
	executor       Executor
	maxParallelism int
	mu             sync.Mutex
	executions     map[string]*PlaybookExecution
}

func NewParallelExecutor(executor Executor, maxParallelism int) *ParallelExecutor {
	if maxParallelism <= 0 {
		maxParallelism = 10
	}
	return &ParallelExecutor{
		executor:       executor,
		maxParallelism: maxParallelism,
		executions:     make(map[string]*PlaybookExecution),
	}
}

func (p *ParallelExecutor) ExecuteAsync(playbook *ParsedPlaybook, targets []*model.Node, extraVars map[string]interface{}) (string, error) {
	execID := fmt.Sprintf("parallel-exec-%d", time.Now().UnixNano())

	go func() {
		exec, err := p.executor.Execute(playbook, targets, extraVars)
		p.mu.Lock()
		if err == nil {
			p.executions[execID] = exec
		}
		p.mu.Unlock()
	}()

	return execID, nil
}

func (p *ParallelExecutor) GetExecution(execID string) (*PlaybookExecution, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	exec, ok := p.executions[execID]
	return exec, ok
}

func (p *ParallelExecutor) ListExecutions() []*PlaybookExecution {
	p.mu.Lock()
	defer p.mu.Unlock()
	execs := make([]*PlaybookExecution, 0, len(p.executions))
	for _, exec := range p.executions {
		execs = append(execs, exec)
	}
	return execs
}

func (e *PlaybookExecution) GetTaskResult(taskName string) []*TaskResult {
	return e.Results[taskName]
}

func (e *PlaybookExecution) GetAllResults() []*TaskResult {
	var all []*TaskResult
	for _, results := range e.Results {
		all = append(all, results...)
	}
	return all
}

func (e *PlaybookExecution) SuccessCount() int {
	count := 0
	for _, results := range e.Results {
		for _, result := range results {
			if result.ExitCode == 0 {
				count++
			}
		}
	}
	return count
}

func (e *PlaybookExecution) FailureCount() int {
	count := 0
	for _, results := range e.Results {
		for _, result := range results {
			if result.ExitCode != 0 {
				count++
			}
		}
	}
	return count
}

func (e *PlaybookExecution) Duration() time.Duration {
	if e.StartTime.IsZero() {
		return 0
	}
	end := e.EndTime
	if end == nil {
		end = &time.Time{}
		*end = time.Now()
	}
	return end.Sub(e.StartTime)
}

func getFileNameFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
