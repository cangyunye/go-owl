package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
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

type webCommandExecutor struct {
	ssh   *sshExecutor
	check *blacklist.Checker
	force bool
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

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	start := time.Now()
	output, exitCode, err := e.ssh.Execute(ctx, nodeID, cmd)
	end := time.Now()

	result := &task.TaskResult{
		NodeID:    nodeID,
		ExitCode:  exitCode,
		Output:    output,
		StartTime: start,
		EndTime:   end,
	}
	if err != nil {
		result.Error = err
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
		var n commonmodel.Node
		var groupsJSON, labelsJSON string
		err := db.QueryRow(
			`SELECT id, name, address, port, user, status, COALESCE(groups,'[]'), COALESCE(labels,'{}') FROM nodes WHERE id = ?`, id,
		).Scan(&n.ID, &n.Name, &n.Address, &n.Port, &n.User, &n.Status, &groupsJSON, &labelsJSON)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(groupsJSON), &n.Groups)
		json.Unmarshal([]byte(labelsJSON), &n.Labels)
		m.nodes[n.ID] = &n
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
	cmdExec := &webCommandExecutor{
		ssh:   &sshExecutor{db: h.db},
		check: h.checker,
		force: run.DangerConfirmed,
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
	totalSteps := len(parsedPlaybook.PreTasks) + len(parsedPlaybook.Tasks) + len(parsedPlaybook.PostTasks)
	history.CreatePlaybookRun(&history.PlaybookRun{
		ID:           runID,
		PlaybookName: run.PlaybookName,
		PlaybookHash: pbHash,
		Nodes:        run.TargetNodes,
		Status:       "running",
		StartedAt:    time.Now(),
		TotalSteps:   totalSteps,
	})

	execution, execErr := pbExecutor.Execute(parsedPlaybook, targetNodes, extraVars)

	for _, step := range toWebStepResults(parsedPlaybook, execution) {
		h.runs.AppendResult(ctx, runID, step)
		ce := &store.CommandExecution{
			TaskID: runID, NodeID: step.NodeID, Command: step.TaskName,
			ExitCode: step.ExitCode, Stdout: step.Output, Stderr: step.Error,
			DurationMs: step.DurationMs, Success: step.ExitCode == 0, CreatedAt: time.Now().UTC(),
		}
		if e := h.History.RecordCommandExecution(ctx, ce); e != nil {
			log.Printf("record command execution: %v", e)
		}
	}

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

	h.runs.UpdateStatus(ctx, runID, finalStatus, "")
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

	allTasks := make([]*pbexec.ParsedTask, 0, len(pb.PreTasks)+len(pb.Tasks)+len(pb.PostTasks))
	allTasks = append(allTasks, pb.PreTasks...)
	allTasks = append(allTasks, pb.Tasks...)
	allTasks = append(allTasks, pb.PostTasks...)

	var results []*webmodel.StepResult
	for _, t := range allTasks {
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
			output := r.Output
			if len(output) > 4096 {
				output = output[:4093] + "..."
			}
			results = append(results, &webmodel.StepResult{
				TaskName:   t.Name,
				NodeID:     r.NodeID,
				Action:     t.Action,
				Status:     status,
				ExitCode:   r.ExitCode,
				Output:     output,
				Error:      errMsg,
				DurationMs: r.EndTime.Sub(r.StartTime).Milliseconds(),
			})
		}
	}
	return results
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
