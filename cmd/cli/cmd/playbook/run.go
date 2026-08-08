package playbook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/google/uuid"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/command"
	pbexec "github.com/cangyunye/go-owl/internal/control/playbook"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
)

// playbookRunFlags
var (
	pbRunNodes                   string
	pbRunGroup                   []string
	pbRunLabel                   []string
	pbRunTags                    string
	pbRunSkipTags                string
	pbRunExtraVars               []string
	pbRunCheck                   bool
	pbRunDefaultConnectTimeout   time.Duration
	pbRunDefaultCommandTimeout   time.Duration
	pbRunDefaultRetry            int
	pbRunDefaultRetryInterval    time.Duration
	pbRunDefaultRetryMaxInterval time.Duration
	pbRunResume                  bool
)

// adapterNodeManager 包装 node.NodeResolver 实现 controlnode.Manager
type adapterNodeManager struct {
	resolver *node.NodeResolver
	nodes    map[string]*model.Node
}

func newAdapterNodeManager(resolver *node.NodeResolver, resolvedNodes []*node.ResolvedNode) *adapterNodeManager {
	m := &adapterNodeManager{
		resolver: resolver,
		nodes:    make(map[string]*model.Node),
	}
	for _, rn := range resolvedNodes {
		m.nodes[rn.ID] = &model.Node{
			ID:      rn.ID,
			Name:    rn.Name,
			Address: rn.Address,
			Port:    rn.Port,
			User:    rn.User,
			Status:  model.NodeStatusOnline,
			Groups:  rn.Groups,
			Labels:  rn.Labels,
		}
	}
	return m
}

func (m *adapterNodeManager) Register(node *model.Node) error  { return nil }
func (m *adapterNodeManager) Unregister(id string) error        { return nil }
func (m *adapterNodeManager) UpdateStatus(id string, status model.NodeStatus) error { return nil }

func (m *adapterNodeManager) GetByID(id string) (*model.Node, error) {
	if n, ok := m.nodes[id]; ok {
		return n, nil
	}
	return nil, fmt.Errorf("node %s not found", id)
}

func (m *adapterNodeManager) List() []*model.Node {
	nodes := make([]*model.Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

func (m *adapterNodeManager) GetByGroup(group string) []*model.Node {
	var result []*model.Node
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

func (m *adapterNodeManager) GetByLabels(labels map[string]string) []*model.Node {
	var result []*model.Node
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

func (m *adapterNodeManager) GetOnlineNodes() []*model.Node { return m.List() }
func (m *adapterNodeManager) Count() int                    { return len(m.nodes) }

func (m *adapterNodeManager) SearchByName(pattern string) []*model.Node {
	if pattern == "" {
		return nil
	}
	var result []*model.Node
	lowerPattern := strings.ToLower(pattern)
	for _, n := range m.nodes {
		if strings.Contains(strings.ToLower(n.Name), lowerPattern) {
			result = append(result, n)
		}
	}
	return result
}

func (m *adapterNodeManager) SearchByAddress(pattern string) []*model.Node {
	if pattern == "" {
		return nil
	}
	var result []*model.Node
	lowerPattern := strings.ToLower(pattern)
	for _, n := range m.nodes {
		if strings.Contains(strings.ToLower(n.Address), lowerPattern) {
			result = append(result, n)
		}
	}
	return result
}

// NewPlaybookRunCmd 创建剧本执行命令
func NewPlaybookRunCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run <playbook-file>",
		Short: i18n.T("playbook.run.short"),
		Long:  i18n.T("playbook.run.long"),
		Args:  cobra.ExactArgs(1),
		Run:   runPlaybookRun,
	}

	runCmd.Flags().StringVarP(&pbRunNodes, "nodes", "N", "",
		i18n.T("playbook.run.flag_nodes"))
	runCmd.Flags().StringSliceVarP(&pbRunGroup, "groups", "g", nil,
		i18n.T("playbook.run.flag_groups"))
	runCmd.Flags().StringSliceVar(&pbRunGroup, "group", nil,
		i18n.T("playbook.run.flag_group_deprecated"))
	runCmd.Flags().MarkHidden("group")
	runCmd.Flags().StringSliceVarP(&pbRunLabel, "label", "l", nil,
		i18n.T("playbook.run.flag_label"))
	runCmd.Flags().StringVar(&pbRunTags, "tags", "",
		i18n.T("playbook.run.flag_tags"))
	runCmd.Flags().StringVar(&pbRunSkipTags, "skip-tags", "",
		i18n.T("playbook.run.flag_skip_tags"))
	runCmd.Flags().StringArrayVar(&pbRunExtraVars, "extra-vars", nil,
		i18n.T("playbook.run.flag_extra_vars"))
	runCmd.Flags().BoolVar(&pbRunCheck, "check", false,
		i18n.T("playbook.run.flag_check"))
	runCmd.Flags().BoolVar(&pbRunCheck, "dry-run", false,
		i18n.T("playbook.run.flag_dry_run"))
	runCmd.Flags().BoolVar(&pbRunResume, "resume", false,
		i18n.T("playbook.run.flag_resume"))
	runCmd.Flags().DurationVar(&pbRunDefaultConnectTimeout, "default-connect-timeout", 10*time.Second,
		i18n.T("playbook.run.flag_connect_timeout"))
	runCmd.Flags().DurationVar(&pbRunDefaultCommandTimeout, "default-command-timeout", 5*time.Minute,
		i18n.T("playbook.run.flag_command_timeout"))
	runCmd.Flags().IntVar(&pbRunDefaultRetry, "default-retry", 0,
		i18n.T("playbook.run.flag_retry"))
	runCmd.Flags().DurationVar(&pbRunDefaultRetryInterval, "default-retry-interval", 1*time.Second,
		i18n.T("playbook.run.flag_retry_interval"))
	runCmd.Flags().DurationVar(&pbRunDefaultRetryMaxInterval, "default-retry-max-interval", 30*time.Second,
		i18n.T("playbook.run.flag_retry_max_interval"))

	return runCmd
}

func runPlaybookRun(cmd *cobra.Command, args []string) {
	playbookFile := args[0]

	logger.Init(nil)
	defer logger.Sync()
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.run.warn_history_db", err))
	}

	nodeLogWriter := logfile.NewNodeLogWriter("")

	common.CheckNodeConflictsBeforeExec()

	nodeResolver := node.NewNodeResolver()

	// 从剧本中获取 hosts 和默认配置
	var playbookHosts []string
	var parsedPlaybook *pbexec.ParsedPlaybook
	// 检查剧本文件是否存在
	if _, err := os.Stat(playbookFile); !os.IsNotExist(err) {
		// 解析剧本文件获取 hosts 和默认配置
		parser := pbexec.NewParser()
		parsedPlaybook, err = parser.ParseFromFile(playbookFile)
		if err == nil && parsedPlaybook.Raw != nil {
			playbookHosts = parsedPlaybook.Raw.Hosts

			// 将 YAML default 配置合并到 CLI 参数中（CLI 优先）
			if parsedPlaybook.DefaultConfig != nil {
				defaultCfg := parsedPlaybook.DefaultConfig
				pbRunGroup, pbRunTags, pbRunSkipTags = ApplyDefaultConfig(
					pbRunGroup, pbRunTags, pbRunSkipTags,
					defaultCfg.Groups, defaultCfg.Tags, defaultCfg.SkipTags,
				)
			}
		}
	}

	targetNodes := selectPlaybookRunTargetNodes(nodeResolver, playbookHosts)
	if len(targetNodes) == 0 {
		fmt.Println(i18n.T("playbook.run.no_target"))
		return
	}

	extraVars := parsePlaybookRunExtraVars(pbRunExtraVars)

	fmt.Printf("Playbook: %s\n", playbookFile)
	fmt.Printf("Target: %d nodes\n", len(targetNodes))
	if pbRunTags != "" {
		fmt.Printf("Tags: %s\n", pbRunTags)
	}
	if pbRunSkipTags != "" {
		fmt.Printf("Skip tags: %s\n", pbRunSkipTags)
	}
	if len(extraVars) > 0 {
		fmt.Printf("Extra vars: %v\n", extraVars)
	}
	if pbRunCheck {
		fmt.Println("Mode: CHECK (no changes will be made)")
	}
	if pbRunResume {
		fmt.Println("Mode: RESUME (continue from last failure)")
	}

	if pbRunDefaultConnectTimeout > 0 || pbRunDefaultCommandTimeout > 0 {
		timeoutCfg := &ssh.TimeoutConfig{
			ConnectTimeout: pbRunDefaultConnectTimeout,
			CommandTimeout: pbRunDefaultCommandTimeout,
		}
		fmt.Printf("Timeout: connect=%v, command=%v\n", timeoutCfg.ConnectTimeout, timeoutCfg.CommandTimeout)
	}

	if pbRunDefaultRetry > 0 {
		retryCfg := &command.RetryConfig{
			MaxRetries:      pbRunDefaultRetry,
			InitialInterval: pbRunDefaultRetryInterval,
			MaxInterval:     pbRunDefaultRetryMaxInterval,
		}
		fmt.Printf("Retry: max=%d, interval=%v, max-interval=%v\n", retryCfg.MaxRetries, retryCfg.InitialInterval, retryCfg.MaxInterval)
	}

	// 确保剧本已解析。如果第一次解析成功则复用，否则重新解析
	if parsedPlaybook == nil || parsedPlaybook.Raw == nil {
		if _, err := os.Stat(playbookFile); os.IsNotExist(err) {
			runSamplePlaybook(targetNodes)
			return
		}
		parser := pbexec.NewParser()
		parsedPlaybook, err = parser.ParseFromFile(playbookFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.run.err_parse", err))
			os.Exit(1)
		}
	}

	// 从 YAML default 块合并 timeout/retry（如果 CLI 未指定）
	if parsedPlaybook.DefaultConfig != nil {
		defaultCfg := parsedPlaybook.DefaultConfig
		if defaultCfg.Timeout != nil {
			if !cmd.Flags().Changed("default-connect-timeout") && defaultCfg.Timeout.Connect != "" {
				if d, err := time.ParseDuration(defaultCfg.Timeout.Connect); err == nil {
					pbRunDefaultConnectTimeout = d
				}
			}
			if !cmd.Flags().Changed("default-command-timeout") && defaultCfg.Timeout.Command != "" {
				if d, err := time.ParseDuration(defaultCfg.Timeout.Command); err == nil {
					pbRunDefaultCommandTimeout = d
				}
			}
		}
		if defaultCfg.Retry != nil {
			if !cmd.Flags().Changed("default-retry") && defaultCfg.Retry.Max > 0 {
				pbRunDefaultRetry = defaultCfg.Retry.Max
			}
			if !cmd.Flags().Changed("default-retry-interval") && defaultCfg.Retry.Interval != "" {
				if d, err := time.ParseDuration(defaultCfg.Retry.Interval); err == nil {
					pbRunDefaultRetryInterval = d
				}
			}
			if !cmd.Flags().Changed("default-retry-max-interval") && defaultCfg.Retry.MaxInterval != "" {
				if d, err := time.ParseDuration(defaultCfg.Retry.MaxInterval); err == nil {
					pbRunDefaultRetryMaxInterval = d
				}
			}
		}
	}

	// 解析节点完整信息
	var resolvedNodes []*node.ResolvedNode
	for _, n := range targetNodes {
		rn, err := nodeResolver.Resolve(n.ID)
		if err == nil {
			resolvedNodes = append(resolvedNodes, rn)
		}
	}

	if len(resolvedNodes) == 0 {
		fmt.Println(i18n.T("playbook.run.no_resolved"))
		os.Exit(1)
	}

	var targetModelNodes []*model.Node
	for _, rn := range resolvedNodes {
		targetModelNodes = append(targetModelNodes, &model.Node{
			ID:      rn.ID,
			Name:    rn.Name,
			Address: rn.Address,
			Port:    rn.Port,
			User:    rn.User,
			Status:  model.NodeStatusOnline,
			Groups:  rn.Groups,
			Labels:  rn.Labels,
		})
	}

	// 设置执行器
	v2Exec := command.NewExecutor(nodeResolver)
	defer v2Exec.Close()

	cmdExec := command.CommandExecutor(v2Exec)
	nodeMgr := newAdapterNodeManager(nodeResolver, resolvedNodes)

	// 创建 Playbook 执行器
	playbookOpts := &pbexec.PlaybookOptions{
		TimeoutConfig: &ssh.TimeoutConfig{
			ConnectTimeout: pbRunDefaultConnectTimeout,
			CommandTimeout: pbRunDefaultCommandTimeout,
		},
		RetryConfig: &command.RetryConfig{
			MaxRetries:      pbRunDefaultRetry,
			InitialInterval: pbRunDefaultRetryInterval,
			MaxInterval:     pbRunDefaultRetryMaxInterval,
		},
		CheckMode: pbRunCheck,
	}
	pbExecutor := pbexec.NewExecutorWithOptions(nodeMgr, cmdExec, nil, nodeResolver, playbookOpts)
	if bds, ok := pbExecutor.(interface{ SetPlaybookBaseDir(string) }); ok {
		bds.SetPlaybookBaseDir(filepath.Dir(playbookFile))
	}

	taskID := uuid.New().String()
	startTime := time.Now()

	meta, _ := json.Marshal(map[string]interface{}{
		"playbook": playbookFile,
		"tags":     pbRunTags,
		"check":    pbRunCheck,
	})

	var targetNodeIDs []string
	for _, n := range targetModelNodes {
		targetNodeIDs = append(targetNodeIDs, n.ID)
	}

	history.RecordOperation(&history.Operation{
		TaskID:    taskID,
		OpType:    "playbook",
		Command:   string(meta),
		Targets:   targetNodeIDs,
		Status:    "running",
		ExecutionMode: string(parsedPlaybook.ExecutionMode),
		PlaybookPath:  playbookFile,
		CreatedAt: startTime,
	})

	var runID string
	if !pbRunCheck {
		runID = taskID
		totalSteps := len(parsedPlaybook.PreTasks) + len(parsedPlaybook.Tasks) + len(parsedPlaybook.PostTasks)
		pbContent, _ := os.ReadFile(playbookFile)
		pbHash := history.ComputePlaybookHash(string(pbContent), targetNodeIDs)
		history.CreatePlaybookRun(&history.PlaybookRun{
			ID:           runID,
			PlaybookName: filepath.Base(playbookFile),
			PlaybookHash: pbHash,
			Nodes:        targetNodeIDs,
			Status:       "running",
			StartedAt:    startTime,
			TotalSteps:   totalSteps,
		})
	}

	// 执行 Playbook
	// 断点续跑
	if pbRunResume && !pbRunCheck {
		lastFailed, err := history.FindLastFailedByPlaybookPath(playbookFile)
		if err == nil && lastFailed != nil {
			fmt.Printf("%s", i18n.T("playbook.run.resume_found",
				lastFailed.TaskID, lastFailed.CurrentTaskPhase, i18n.F(lastFailed.CurrentTaskIndex)))
			if r, ok := pbExecutor.(interface{ SetResumeFrom(string, int) }); ok {
				r.SetResumeFrom(lastFailed.CurrentTaskPhase, lastFailed.CurrentTaskIndex)
			}
		} else if err != nil {
			fmt.Printf("%s", i18n.T("playbook.run.warn_resume_fail", err))
		} else {
			fmt.Println(i18n.T("playbook.run.resume_none"))
		}
	}

	// 设置 checkpoint 回调
	if c, ok := pbExecutor.(interface{ SetCheckpointFunc(func(string, int)) }); ok {
		c.SetCheckpointFunc(func(phase string, index int) {
			history.RecordCheckpoint(taskID, index, phase)
		})
	}

	fmt.Println(i18n.T("playbook.run.executing"))
	execution, err := pbExecutor.Execute(parsedPlaybook, targetModelNodes, extraVars)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.run.err_execution", err))
	}

	// 记录每个任务结果
	if !pbRunCheck {
		for taskName, results := range execution.Results {
			for _, result := range results {
				errorMsg := ""
				if result.Error != nil {
					errorMsg = result.Error.Error()
				}
				history.RecordCommandExecution(&history.CommandExecution{
				TaskID:     taskID,
				NodeID:     result.NodeID,
				Command:    taskName,
				ExitCode:   result.ExitCode,
				Stdout:     truncateStr(result.Output, 4096),
				Stderr:     errorMsg,
				DurationMs: result.EndTime.Sub(result.StartTime).Milliseconds(),
				Success:    result.ExitCode == 0,
				CreatedAt:  time.Now(),
			})
			nodeLogWriter.AppendEntry(result.NodeID, taskName, result.Action, result.ExitCode, result.Output, errorMsg, result.EndTime.Sub(result.StartTime))
			}
		}

		recordStepStates(runID, parsedPlaybook, execution)
	}

	// 更新操作最终状态
	failed := execution.FailureCount()
	success := execution.SuccessCount()
	if !pbRunCheck {
		finalStatus := "completed"
		if failed > 0 {
			if success == 0 {
				finalStatus = "failed"
			} else {
				finalStatus = "partial_failure"
			}
		}
		history.RecordOperation(&history.Operation{
			TaskID:    taskID,
			OpType:    "playbook",
			Command:   string(meta),
			Targets:   targetNodeIDs,
			Status:    finalStatus,
			CreatedAt: startTime,
		})
		history.FinishPlaybookRun(runID, finalStatus, success, failed)
	}

	// 显示执行结果
	fmt.Println()
	for taskName, results := range execution.Results {
		for _, result := range results {
			nodeName := result.NodeID
			for _, rn := range resolvedNodes {
				if rn.ID == result.NodeID {
					if rn.Name != "" {
						nodeName = rn.Name
					}
					break
				}
			}
			if result.Error != nil {
				fmt.Printf("%s", i18n.T("playbook.run.fail", nodeName, taskName, result.Error))
			} else if result.ExitCode == 0 {
				fmt.Printf("%s", i18n.T("playbook.run.ok", nodeName, taskName))
				if result.Output != "" {
					for _, line := range splitLines(truncateStr(result.Output, 1024)) {
						fmt.Printf("   %s\n", line)
					}
				}
			} else {
				fmt.Printf("%s", i18n.T("playbook.run.err_exit", nodeName, taskName, i18n.F(result.ExitCode)))
				if result.Output != "" {
					for _, line := range splitLines(truncateStr(result.Output, 1024)) {
						fmt.Printf("   %s\n", line)
					}
				}
			}
		}
	}

	fmt.Printf("%s", i18n.T("playbook.run.summary", i18n.F(success), i18n.F(failed)))
	if pbRunCheck {
		fmt.Println(i18n.T("playbook.run.status_check"))
	} else if execution.Status == pbexec.ExecutionStatusFailed {
		fmt.Printf("%s", i18n.T("playbook.run.status_failed", execution.Error))
	} else if execution.Status == pbexec.ExecutionStatusCompleted {
		fmt.Println(i18n.T("playbook.run.status_completed"))
	} else {
		fmt.Printf("%s", i18n.T("playbook.run.status", execution.Status))
	}

	if failed > 0 {
		os.Exit(1)
	}
}

func selectPlaybookRunTargetNodes(resolver *node.NodeResolver, playbookHosts []string) []*model.Node {
	var result []*model.Node
	var nodes []*node.ResolvedNode
	var err error

	if pbRunNodes != "" {
		ids := parseNodeIDsList(pbRunNodes)
		for _, id := range ids {
			rn, resolveErr := resolver.Resolve(id)
			if resolveErr == nil {
				nodes = append(nodes, rn)
			}
		}
	} else if len(pbRunGroup) > 0 {
		resolvedNodes, err := node.ListNodesByGroups(resolver, pbRunGroup)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.run.warn_group", err))
		} else {
			nodes = resolvedNodes
		}
	} else if len(pbRunLabel) > 0 {
		nodes, err = resolver.ListNodes(&node.ListOptions{Label: pbRunLabel[0]})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.run.warn_label", err))
		}
	} else if len(playbookHosts) > 0 {
		// 使用剧本中的 hosts 配置
		for _, host := range playbookHosts {
			rn, resolveErr := resolver.Resolve(host)
			if resolveErr == nil {
				nodes = append(nodes, rn)
			}
		}
	} else {
		// 如果没有任何配置，则使用所有节点
		nodes, err = resolver.ListNodes(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.run.warn_list", err))
		}
	}

	for _, rn := range nodes {
		// 如果同时指定了多个筛选条件，在 Go 层做二次过滤
		if len(pbRunGroup) > 0 {
			if !node.ContainsAny(rn.Groups, pbRunGroup) {
				continue
			}
		}
		if len(pbRunLabel) > 1 {
			match := true
			for _, label := range pbRunLabel[1:] {
				parts := splitKeyValueList(label)
				if len(parts) == 2 {
					key, value := parts[0], parts[1]
					if v, ok := rn.Labels[key]; !ok || v != value {
						match = false
						break
					}
				}
			}
			if !match {
				continue
			}
		}

		result = append(result, &model.Node{
			ID:      rn.ID,
			Name:    rn.Name,
			Address: rn.Address,
			Port:    rn.Port,
			User:    rn.User,
			Status:  model.NodeStatusOnline,
			Groups:  rn.Groups,
			Labels:  rn.Labels,
		})
	}

	return result
}

func containsNodeIDList(ids []string, id string) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

func parseNodeIDsList(s string) []string {
	result := make([]string, 0)
	for _, id := range splitStringList(s, ",") {
		if trimmed := trimStringList(id); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitStringList(s, sep string) []string {
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
		}
	}
	result = append(result, s[start:])
	return result
}

func trimStringList(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func splitKeyValueList(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func parsePlaybookRunExtraVars(vars []string) map[string]interface{} {
	result := make(map[string]interface{})
	for _, v := range vars {
		parts := splitKeyValueList(v)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

// ApplyDefaultConfig 将 YAML default 块中的默认值应用到 CLI 参数上。
// CLI 参数非空时优先使用 CLI 参数（完全替换），否则使用 YAML default。
// 返回合并后的 group, tags, skip_tags 值（逗号分隔字符串形式）。
func ApplyDefaultConfig(cliGroups []string, cliTags, cliSkipTags string,
	defaultGroups, defaultTags, defaultSkipTags []string) ([]string, string, string) {

	groups := cliGroups
	if len(groups) == 0 && len(defaultGroups) > 0 {
		groups = defaultGroups
	}

	tags := cliTags
	if tags == "" && len(defaultTags) > 0 {
		tags = strings.Join(defaultTags, ",")
	}

	skipTags := cliSkipTags
	if skipTags == "" && len(defaultSkipTags) > 0 {
		skipTags = strings.Join(defaultSkipTags, ",")
	}

	return groups, tags, skipTags
}

func runSamplePlaybook(nodes []*model.Node) {
	fmt.Println(i18n.T("playbook.run.sample_start"))

	steps := []string{
		"[Gathering Facts]",
		"[Pre Tasks]",
		"[Tasks]",
		"[Handlers]",
		"[Post Tasks]",
	}

	for _, step := range steps {
		fmt.Printf("  %s\n", step)
	}

	success := 0
	failed := 0
	for _, n := range nodes {
		if n.Status == model.NodeStatusOnline {
			success++
		} else {
			failed++
		}
	}

	fmt.Printf("%s", i18n.T("playbook.run.summary", i18n.F(success), i18n.F(failed)))
	if failed > 0 {
		os.Exit(1)
	}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				lines = append(lines, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func recordStepStates(runID string, pb *pbexec.ParsedPlaybook, exec *pbexec.PlaybookExecution) {
	if runID == "" {
		return
	}

	allTasks := make([]*pbexec.ParsedTask, 0, len(pb.PreTasks)+len(pb.Tasks)+len(pb.PostTasks))
	allTasks = append(allTasks, pb.PreTasks...)
	allTasks = append(allTasks, pb.Tasks...)
	allTasks = append(allTasks, pb.PostTasks...)

	for stepIndex, t := range allTasks {
		results, ok := exec.Results[t.Name]
		if !ok {
			continue
		}
		for _, result := range results {
			status := "completed"
			errMsg := ""
			if result.Error != nil {
				status = "failed"
				errMsg = result.Error.Error()
			} else if result.ExitCode != 0 {
				status = "failed"
				errMsg = fmt.Sprintf("exit code %d", result.ExitCode)
			}

			startedAt := result.StartTime
			finishedAt := result.EndTime
			history.UpsertStepState(&history.PlaybookStepState{
				RunID:      runID,
				NodeID:     result.NodeID,
				StepIndex:  stepIndex,
				StepName:   t.Name,
				Action:     t.Action,
				Status:     status,
				StartedAt:  &startedAt,
				FinishedAt: &finishedAt,
				DurationMs: result.EndTime.Sub(result.StartTime).Milliseconds(),
				ExitCode:   result.ExitCode,
				Stdout:     truncateStr(result.Output, 4096),
				Stderr:     errMsg,
				Error:      errMsg,
			})
		}
	}
}
