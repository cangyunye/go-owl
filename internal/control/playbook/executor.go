package playbook

import (
	"fmt"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/control/task"
)

type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusAborted   ExecutionStatus = "aborted"
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

type Executor interface {
	Execute(playbook *ParsedPlaybook, targets []*model.Node, extraVars map[string]interface{}) (*PlaybookExecution, error)
	ExecuteTask(exec *PlaybookExecution, task *ParsedTask) ([]*TaskResult, error)
	Stop(execID string) error
}

type playbookExecutor struct {
	nodeMgr   node.Manager
	cmdExec   command.CommandExecutor
	taskSched task.Scheduler
	runner    ActionRunner
}

func NewExecutor(nodeMgr node.Manager, cmdExec command.CommandExecutor, taskSched task.Scheduler, runner ActionRunner) Executor {
	return &playbookExecutor{
		nodeMgr:   nodeMgr,
		cmdExec:   cmdExec,
		taskSched: taskSched,
		runner:    runner,
	}
}

type ActionRunner interface {
	RunAction(action string, args map[string]interface{}, nodeID string, vars map[string]interface{}) (*TaskResult, error)
}

type defaultActionRunner struct {
	cmdExec command.CommandExecutor
}

func NewDefaultActionRunner(cmdExec command.CommandExecutor) *defaultActionRunner {
	return &defaultActionRunner{cmdExec: cmdExec}
}

func (r *defaultActionRunner) RunAction(action string, args map[string]interface{}, nodeID string, vars map[string]interface{}) (*TaskResult, error) {
	result := &TaskResult{
		TaskName:  action,
		NodeID:    nodeID,
		Action:    action,
		StartTime: time.Now(),
	}

	var cmd string
	if c, ok := args["cmd"]; ok {
		cmd = fmt.Sprintf("%v", c)
	} else if c, ok := args["command"]; ok {
		cmd = fmt.Sprintf("%v", c)
	} else if c, ok := args["script"]; ok {
		cmd = fmt.Sprintf("bash %v", c)
	} else {
		cmd = fmt.Sprintf("echo 'Action: %s, Args: %v'", action, args)
	}

	if r.cmdExec != nil {
		taskResult, err := r.cmdExec.ExecuteOnNode(nodeID, cmd, 5*time.Minute)
		if err != nil {
			result.Error = err
			result.EndTime = time.Now()
			return result, err
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
		preTask := playbook.PreTasks[i]
		results, err := e.executeTaskInternal(exec, preTask)
		if err != nil {
			if !preTask.Options.IgnoreErrors {
				exec.Status = ExecutionStatusFailed
				exec.Error = err.Error()
				return exec, err
			}
		}
		exec.Results[preTask.Name] = append(exec.Results[preTask.Name], results...)
	}

	for i := range playbook.Tasks {
		mainTask := playbook.Tasks[i]
		shouldContinue := e.shouldContinueExecution(exec)
		if !shouldContinue && exec.Status == ExecutionStatusAborted {
			break
		}

		results, err := e.executeTaskInternal(exec, mainTask)
		if err != nil {
			if !mainTask.Options.IgnoreErrors {
				if mainTask.Options.AnyErrorsFatal {
					exec.Status = ExecutionStatusFailed
					exec.Error = err.Error()
					return exec, err
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
		postTask := playbook.PostTasks[i]
		results, err := e.executeTaskInternal(exec, postTask)
		if err != nil {
			if !postTask.Options.IgnoreErrors {
				exec.Status = ExecutionStatusFailed
				exec.Error = err.Error()
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

	if task.Loop != nil {
		for _, item := range task.Loop.Items {
			itemResults, err := e.executeTaskForNode(exec, task, "", item)
			results = append(results, itemResults...)
			if err != nil && !task.Options.IgnoreErrors {
				return results, err
			}
		}
	} else {
		for _, target := range exec.TargetNodes {
			itemResults, err := e.executeTaskForNode(exec, task, target.ID, nil)
			results = append(results, itemResults...)
			if err != nil && !task.Options.IgnoreErrors {
				if task.Options.AnyErrorsFatal {
					return results, err
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
		e.runner = NewDefaultActionRunner(e.cmdExec)
	}

	result, err := e.runner.RunAction(task.Action, task.Args, nodeID, taskVars)
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
