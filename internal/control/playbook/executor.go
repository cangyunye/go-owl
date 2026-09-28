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
	controlnode "github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/control/script"
	"github.com/cangyunye/go-owl/internal/control/task"
	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
)

type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusAborted   ExecutionStatus = "aborted"
)

type ExecutionMode string

const (
	ExecutionModeFailContinue ExecutionMode = "fail_continue"
	ExecutionModePipeline     ExecutionMode = "pipeline"
)

type TaskResult struct {
	TaskName string
	NodeID   string
	Action   string
	// Command 是插值后实际下发执行的命令/资源描述（command/shell/script 等），
	// 供执行日志与历史的 command 列使用；动作未产生命令串时为空。
	Command   string
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
	CheckMode     bool
}

type Executor interface {
	Execute(playbook *ParsedPlaybook, targets []*model.Node, extraVars map[string]interface{}) (*PlaybookExecution, error)
	ExecuteTask(exec *PlaybookExecution, task *ParsedTask) ([]*TaskResult, error)
	// 中断经上层 ctx 传播（Web 端 runCancels 注册表 + parentCtx），
	// 引擎级 Stop 从未接线，已从接口移除。
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
	resumeFrom     *checkpoint                   // 非 nil 时从此处跳过已执行任务
	checkpointFunc func(phase string, index int) // 保存 checkpoint 的回调
	// progressFunc 每个节点步骤完成（含失败）即回调，供上层实时推送进度
	progressFunc func(*TaskResult)
}

// SetResumeFrom 设置断点续跑的起始位置
func (e *playbookExecutor) SetResumeFrom(phase string, index int) {
	e.resumeFrom = &checkpoint{Phase: phase, Index: index}
}

// SetCheckpointFunc 设置 checkpoint 保存回调
func (e *playbookExecutor) SetCheckpointFunc(fn func(phase string, index int)) {
	e.checkpointFunc = fn
}

// SetProgressFunc 设置步骤进度回调：每个节点步骤完成（含失败）即触发，
// 上层用于逐步写库与广播，运行中才有可见进度。
func (e *playbookExecutor) SetProgressFunc(fn func(*TaskResult)) {
	e.progressFunc = fn
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

// SetScriptCheckFunc 注入脚本内容黑名单检查（透传给动作执行器）。
func (e *playbookExecutor) SetScriptCheckFunc(fn ScriptCheckFunc) {
	if r, ok := e.runner.(*defaultActionRunner); ok {
		r.SetScriptCheckFunc(fn)
	}
}

// SetBaseContext 设置运行取消上下文（透传给动作执行器）。
func (e *playbookExecutor) SetBaseContext(ctx context.Context) {
	if r, ok := e.runner.(*defaultActionRunner); ok {
		r.SetBaseContext(ctx)
	}
}

type ActionRunner interface {
	RunAction(action string, args map[string]interface{}, nodeID string, vars map[string]interface{}, actionOpts *ActionOptions) (*TaskResult, error)
}

// ScriptCheckFunc 校验脚本动作内容是否允许在节点上执行（可选注入；
// nil 表示不检查）。返回非 nil error 表示拦截（如命中危险命令黑名单
// 且未被确认）。command/cmd/shell 动作经 CommandExecutor 内部的检查点，
// script 动作由 ScriptExecutor 自行拨 SSH，必须在此单独接检查。
type ScriptCheckFunc func(nodeID, scriptContent string) error

type defaultActionRunner struct {
	cmdExec         command.CommandExecutor
	nodeResolver    *node.NodeResolver
	transferMgr     *transfer.TransferManager
	opts            *PlaybookOptions
	playbookBaseDir string
	downloadBaseDir string
	scriptCheck     ScriptCheckFunc
	// baseCtx 为该次运行的取消上下文，脚本动作从中派生；
	// 取消运行时未开始的脚本步骤不再发起新的 SSH 执行。
	baseCtx context.Context
}

func NewDefaultActionRunnerWithOptions(cmdExec command.CommandExecutor, nodeResolver *node.NodeResolver, opts *PlaybookOptions) *defaultActionRunner {
	return &defaultActionRunner{
		cmdExec:      cmdExec,
		nodeResolver: nodeResolver,
		transferMgr:  transfer.NewTransferManager(nodeResolver),
		opts:         opts,
		baseCtx:      context.Background(),
	}
}

// SetScriptCheckFunc 注入脚本内容检查器（Web 端接黑名单；CLI 未注入时
// 走自身交互确认，行为不变）。
func (r *defaultActionRunner) SetScriptCheckFunc(fn ScriptCheckFunc) {
	r.scriptCheck = fn
}

// SetBaseContext 设置运行取消上下文，脚本执行从中派生。
func (r *defaultActionRunner) SetBaseContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.baseCtx = ctx
}

// SetPlaybookBaseDir 设置 Playbook 所在的基础目录，用于解析相对路径
func (r *defaultActionRunner) SetPlaybookBaseDir(path string) {
	r.playbookBaseDir = path
}

// SetDownloadBaseDir 设置 download 动作相对路径的落盘目录（未设置时回退为 Playbook 目录）
func (r *defaultActionRunner) SetDownloadBaseDir(path string) {
	r.downloadBaseDir = path
}

// resolvePath 相对于 Playbook 目录解析路径
func (r *defaultActionRunner) resolvePath(path string) string {
	if r.playbookBaseDir != "" && !filepath.IsAbs(path) {
		return filepath.Join(r.playbookBaseDir, path)
	}
	return path
}

// resolveDownloadDest 解析 download 动作的本地目标路径：
// 绝对路径保持原样；相对路径优先落入 downloadBaseDir（如中转站目录），未设置时回退 Playbook 目录
func (r *defaultActionRunner) resolveDownloadDest(dest string, vars map[string]interface{}) string {
	dest = r.interpolateVariables(dest, vars)
	if isAbsPath(dest) {
		return dest
	}
	if r.downloadBaseDir != "" {
		return filepath.Join(r.downloadBaseDir, dest)
	}
	return r.resolvePath(dest)
}

// isAbsPath 同时识别 Windows 盘符路径与 Unix 风格绝对路径
func isAbsPath(p string) bool {
	return filepath.IsAbs(p) || strings.HasPrefix(p, "/")
}

func (r *defaultActionRunner) RunAction(action string, args map[string]interface{}, nodeID string, vars map[string]interface{}, actionOpts *ActionOptions) (*TaskResult, error) {
	result := &TaskResult{
		TaskName:  action,
		NodeID:    nodeID,
		Action:    action,
		StartTime: time.Now(),
	}

	if r.opts != nil && r.opts.CheckMode {
		result.ExitCode = 0
		result.Output = fmt.Sprintf("[check mode] would execute %s on %s", action, nodeID)
		result.EndTime = time.Now()
		return result, nil
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
	result.Command = cmd

	mergedOpts := MergeActionOptions(actionOpts, r.getGlobalDefaults())

	if r.cmdExec != nil {
		taskResult, err := executeCommandOnNode(r.cmdExec, nodeID, cmd, mergedOpts)
		if err != nil {
			// 出错也要保留底层结果里的退出码/输出：FailureCount 与
			// 前端展示都依赖 ExitCode，丢了会把失败误判成成功。
			if taskResult != nil {
				result.ExitCode = taskResult.ExitCode
				if result.Output == "" {
					result.Output = taskResult.Output
				}
			}
			if mergedOpts.ShouldRetry() {
				taskResult, err = r.executeWithRetry(nodeID, cmd, mergedOpts)
			}
			if err != nil {
				if taskResult != nil && result.ExitCode == 0 {
					result.ExitCode = taskResult.ExitCode
				}
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

	// 读取其他参数（先于存在性检查：inline 模式下 script 参数是
	// 脚本内容而非路径，不能按文件 stat）
	opts := &script.ScriptExecutionOptions{
		DestDir: "/tmp",
	}
	if v, ok := args["inline"].(bool); ok {
		opts.Inline = v
	}
	if v, ok := args["keep"].(bool); ok {
		opts.Keep = v
	}

	// 解析路径和替换变量
	scriptPath = r.resolvePath(r.interpolateVariables(scriptPath, vars))
	result.Command = "bash " + scriptPath

	// 检查脚本文件是否存在（inline 与 URL 除外）
	isURL := len(scriptPath) > 8 && (scriptPath[:7] == "http://" || scriptPath[:8] == "https://")
	if !opts.Inline && !isURL {
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			result.Error = fmt.Errorf("script file not found: %s", scriptPath)
			result.EndTime = time.Now()
			return result, result.Error
		}
	}

	if v, ok := args["dest"].(string); ok {
		opts.DestDir = r.interpolateVariables(v, vars)
	}
	if v, ok := args["args"].(string); ok {
		opts.Args = r.interpolateVariables(v, vars)
	}

	mergedOpts := MergeActionOptions(actionOpts, r.getGlobalDefaults())
	opts.Timeout = mergedOpts.GetTimeout()
	opts.Ctx = r.baseCtx

	// 脚本内容与命令一视同仁地过黑名单：inline 的内容即参数本身，
	// 文件脚本读取内容检查（执行器随后也会读它）。
	if r.scriptCheck != nil {
		content := scriptPath
		if !opts.Inline {
			data, readErr := os.ReadFile(scriptPath)
			if readErr != nil {
				result.Error = fmt.Errorf("读取脚本文件失败: %w", readErr)
				result.EndTime = time.Now()
				return result, result.Error
			}
			content = string(data)
		}
		if checkErr := r.scriptCheck(nodeID, content); checkErr != nil {
			result.ExitCode = -1
			result.Error = checkErr
			result.Output = checkErr.Error()
			result.EndTime = time.Now()
			return result, checkErr
		}
	}

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

	result.Command = fmt.Sprintf("upload %s -> %s", src, dest)

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
	dest = r.resolveDownloadDest(dest, vars)

	result.Command = fmt.Sprintf("download %s -> %s", src, dest)

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

func (r *defaultActionRunner) getGlobalDefaults() *PlaybookDefaults {
	if r.opts == nil {
		return DefaultPlaybookDefaults()
	}
	return &PlaybookDefaults{
		TimeoutConfig: r.opts.TimeoutConfig,
		RetryConfig:   r.opts.RetryConfig,
	}
}

// executeCommandOnNode 通过命令执行器运行命令，优先使用带连接/命令超时区分的配置。
func executeCommandOnNode(cmdExec command.CommandExecutor, nodeID, cmd string, opts *ActionOptions) (*task.TaskResult, error) {
	config := opts.GetTimeoutConfig()
	if config.ConnectTimeout <= 0 && config.CommandTimeout <= 0 {
		return cmdExec.ExecuteOnNode(nodeID, cmd, 5*time.Minute)
	}
	return cmdExec.ExecuteOnNodeWithConfig(nodeID, cmd, config)
}

func (r *defaultActionRunner) executeWithRetry(nodeID, cmd string, opts *ActionOptions) (*task.TaskResult, error) {
	retryConfig := opts.GetRetryConfig()
	if retryConfig == nil {
		retryConfig = &command.RetryConfig{MaxRetries: 3}
	}
	maxRetries := retryConfig.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := executeCommandOnNode(r.cmdExec, nodeID, cmd, opts)
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
		// 先保留结果再处理错误：中止时失败步骤自身的执行记录不能丢，
		// 否则前端只能看到失败之前的步骤，失败原因无处可查。
		exec.Results[preTask.Name] = append(exec.Results[preTask.Name], results...)
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
		// 先保留结果再处理错误：pipeline 快停时失败步骤自身的执行记录不能丢。
		exec.Results[mainTask.Name] = append(exec.Results[mainTask.Name], results...)
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
		exec.Results[postTask.Name] = append(exec.Results[postTask.Name], results...)
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
	}

	now := time.Now()
	exec.EndTime = &now
	exec.applyTerminalStatus()

	// 中止失败（pipeline 快停走 break 到这里）必须把错误返回给调用方：
	// handler 靠它写 run.error，返回 nil 会表现为"静默失败"。
	if exec.Status == ExecutionStatusFailed && exec.Error != "" {
		return exec, fmt.Errorf("%s", exec.Error)
	}
	return exec, nil
}

// IsFailedResult 单一失败判定口径：非零退出码即失败（shell 语义），
// Error 是补充信息。历史上终态判定（ExitCode!=0 && Error!=nil）与
// FailureCount（ExitCode!=0）两套口径并存，是 946542b 修复的
// 「误判成功」类缺陷的温床，现统一由此谓词判定。
func IsFailedResult(r *TaskResult) bool {
	return r.ExitCode != 0
}

// applyTerminalStatus 依据结果集推导执行终态（仅当仍在 running 时生效，
// 已被取消/中止标记的执行不覆盖）。
func (e *PlaybookExecution) applyTerminalStatus() {
	if e.Status != ExecutionStatusRunning {
		return
	}
	hasFailure := false
	for _, results := range e.Results {
		for _, result := range results {
			if IsFailedResult(result) {
				hasFailure = true
				break
			}
		}
	}
	if hasFailure {
		e.Status = ExecutionStatusFailed
	} else {
		e.Status = ExecutionStatusCompleted
	}
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

	if e.progressFunc != nil {
		e.progressFunc(result)
	}

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
			if !IsFailedResult(result) {
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
			if IsFailedResult(result) {
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
