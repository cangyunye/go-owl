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
	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
)

// playbookRunFlags
var (
	pbRunNodes                   string
	pbRunGroup                   string
	pbRunLabel                   []string
	pbRunTags                    string
	pbRunSkipTags                string
	pbRunExtraVars               []string
	pbRunCheck                   bool
	pbRunDiff                    bool
	pbRunDefaultConnectTimeout   time.Duration
	pbRunDefaultCommandTimeout   time.Duration
	pbRunDefaultRetry            int
	pbRunDefaultRetryInterval    time.Duration
	pbRunDefaultRetryMaxInterval time.Duration
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

// NewPlaybookRunCmd 创建剧本执行命令
func NewPlaybookRunCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run <playbook-file>",
		Short: "执行剧本",
		Long: `执行 Ansible 风格的 YAML 剧本。

示例：
  owl playbook run site.yml
  owl playbook run site.yml --tags nginx,mysql
  owl playbook run site.yml --extra-vars "version=1.2.3"
  owl playbook run site.yml --check`,
		Args: cobra.ExactArgs(1),
		Run:  runPlaybookRun,
	}

	runCmd.Flags().StringVar(&pbRunNodes, "nodes", "",
		"指定节点 ID (逗号分隔)")
	runCmd.Flags().StringVar(&pbRunGroup, "group", "",
		"按分组选择节点")
	runCmd.Flags().StringSliceVarP(&pbRunLabel, "label", "l", nil,
		"按标签选择节点")
	runCmd.Flags().StringVar(&pbRunTags, "tags", "",
		"执行指定标签的任务")
	runCmd.Flags().StringVar(&pbRunSkipTags, "skip-tags", "",
		"跳过指定标签的任务")
	runCmd.Flags().StringArrayVar(&pbRunExtraVars, "extra-vars", nil,
		"额外变量 (格式: key=value)")
	runCmd.Flags().BoolVar(&pbRunCheck, "check", false,
		"检查模式（不实际执行）")
	runCmd.Flags().BoolVar(&pbRunDiff, "diff", false,
		"显示变更差异")
	runCmd.Flags().DurationVar(&pbRunDefaultConnectTimeout, "default-connect-timeout", 10*time.Second,
		"全局默认 SSH 连接超时时间")
	runCmd.Flags().DurationVar(&pbRunDefaultCommandTimeout, "default-command-timeout", 5*time.Minute,
		"全局默认命令执行超时时间")
	runCmd.Flags().IntVar(&pbRunDefaultRetry, "default-retry", 0,
		"全局默认最大重试次数")
	runCmd.Flags().DurationVar(&pbRunDefaultRetryInterval, "default-retry-interval", 1*time.Second,
		"全局默认初始重试间隔")
	runCmd.Flags().DurationVar(&pbRunDefaultRetryMaxInterval, "default-retry-max-interval", 30*time.Second,
		"全局默认最大重试间隔")

	return runCmd
}

func runPlaybookRun(cmd *cobra.Command, args []string) {
	playbookFile := args[0]

	logger.Init(nil)
	defer logger.Sync()
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法初始化历史记录数据库: %v\n", err)
	}

	nodeLogWriter := logfile.NewNodeLogWriter("")

	common.CheckNodeConflictsBeforeExec()

	nodeResolver := node.NewNodeResolver()

	targetNodes := selectPlaybookRunTargetNodes(nodeResolver)
	if len(targetNodes) == 0 {
		fmt.Println("未找到目标节点")
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
	if pbRunDiff {
		fmt.Println("Mode: DIFF (showing changes)")
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

	// 检查剧本文件是否存在
	if _, err := os.Stat(playbookFile); os.IsNotExist(err) {
		runSamplePlaybook(targetNodes)
		return
	}

	// 解析剧本文件
	parser := pbexec.NewParser()
	parsedPlaybook, err := parser.ParseFromFile(playbookFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 解析剧本文件失败: %v\n", err)
		os.Exit(1)
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
		fmt.Println("未找到可用的目标节点")
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
		CreatedAt: startTime,
	})

	// 执行 Playbook
	fmt.Println("\n执行剧本...")
	execution, err := pbExecutor.Execute(parsedPlaybook, targetModelNodes, extraVars)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n错误: 剧本执行失败: %v\n", err)
	}

	// 记录每个任务结果
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

	// 更新操作最终状态
	finalStatus := "completed"
	failed := execution.FailureCount()
	success := execution.SuccessCount()
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
				fmt.Printf("❌ [%s] %s 失败: %v\n", nodeName, taskName, result.Error)
			} else if result.ExitCode == 0 {
				fmt.Printf("✅ [%s] %s 成功\n", nodeName, taskName)
				if result.Output != "" {
					for _, line := range splitLines(truncateStr(result.Output, 1024)) {
						fmt.Printf("   %s\n", line)
					}
				}
			} else {
				fmt.Printf("⚠️  [%s] %s 退出码 %d\n", nodeName, taskName, result.ExitCode)
				if result.Output != "" {
					for _, line := range splitLines(truncateStr(result.Output, 1024)) {
						fmt.Printf("   %s\n", line)
					}
				}
			}
		}
	}

	fmt.Printf("\n总结: %d 成功, %d 失败\n", success, failed)
	if execution.Status == pbexec.ExecutionStatusFailed {
		fmt.Printf("状态: 失败 (%s)\n", execution.Error)
	} else if execution.Status == pbexec.ExecutionStatusCompleted {
		fmt.Println("状态: 完成")
	} else {
		fmt.Printf("状态: %s\n", execution.Status)
	}

	if failed > 0 {
		os.Exit(1)
	}
}

func selectPlaybookRunTargetNodes(resolver *node.NodeResolver) []*model.Node {
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
	} else if pbRunGroup != "" {
		nodes, err = resolver.ListNodes(&node.ListOptions{Group: pbRunGroup})
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: 按分组查询节点失败: %v\n", err)
		}
	} else if len(pbRunLabel) > 0 {
		nodes, err = resolver.ListNodes(&node.ListOptions{Label: pbRunLabel[0]})
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: 按标签查询节点失败: %v\n", err)
		}
	} else {
		nodes, err = resolver.ListNodes(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: 获取节点列表失败: %v\n", err)
		}
	}

	for _, rn := range nodes {
		// 如果同时指定了多个筛选条件，在 Go 层做二次过滤
		if pbRunGroup != "" {
			found := false
			for _, g := range rn.Groups {
				if g == pbRunGroup {
					found = true
					break
				}
			}
			if !found {
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

func runSamplePlaybook(nodes []*model.Node) {
	fmt.Println("\n执行示例剧本...")

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

	fmt.Printf("\n总结: %d 成功, %d 失败\n", success, failed)
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
