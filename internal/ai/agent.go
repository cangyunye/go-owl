package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	aiPrompts "github.com/cangyunye/go-owl/internal/ai/prompts"
	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/control/playbook"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	debugLogger *zap.SugaredLogger
	logLevel    zap.AtomicLevel
)

func init() {
	logLevel = zap.NewAtomicLevelAt(zap.WarnLevel) // 默认只输出 Warning 及以上
	config := zap.Config{
		Level:            logLevel,
		Development:      false,
		Encoding:         "console",
		EncoderConfig:    zap.NewDevelopmentEncoderConfig(),
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	logger, _ := config.Build()
	debugLogger = logger.Sugar().Named("ai-debug")
}

// SetLogVerbose 设置日志为详细模式（debug 级别）
func SetLogVerbose(verbose bool) {
	if verbose {
		logLevel.SetLevel(zap.DebugLevel)
	} else {
		logLevel.SetLevel(zap.WarnLevel)
	}
}

func debugPrint(debug bool, template string, keysAndValues ...interface{}) {
	if !debug {
		return
	}
	if len(keysAndValues) == 0 {
		debugLogger.Debug(template)
	} else {
		formatted := fmt.Sprintf(template, keysAndValues...)
		debugLogger.Debug(formatted)
	}
}

type Agent struct {
	config         *Config
	nodeMgr        node.Manager
	nodeStore      NodeStoreAdapter
	registry       *ToolRegistry
	playbookParser *playbook.Parser
	chatModel      ChatModel
	systemPrompt   string
	mu             sync.RWMutex
	debug          bool

	// confirmGate 在执行工具前由 Process/ProcessWithContext 调用。
	// 返回 Confirm=true 时工具不执行，Question 作为响应返回给用户。
	confirmGate func(ToolCall) ConfirmationDecision
	// sessionMemory 由 Session 注入的会话记忆（对话+操作记录），
	// 随路由消息发给 LLM 作为参考，不作为新请求。
	sessionMemory string
	// nodeContextHook 工具执行成功且涉及节点时回调解析后的节点名列表，
	// 供 Session 保存节点上下文（跨轮复用，新一轮查询覆盖）。
	nodeContextHook func(nodes []string, source string)
}

// ConfirmationDecision 确认门判定结果。
type ConfirmationDecision struct {
	Confirm  bool   // true=拦截该工具调用
	Question string // 拦截时返回给用户的确认问题文案
	Summary  string // 操作摘要（用于展示与记录）
}

// confirmRequiredTools 需要用户确认的写操作工具集合。
var confirmRequiredTools = map[string]bool{
	"node_add":          true,
	"node_remove":       true,
	"node_update":       true,
	"node_groups":       true,
	"node_labels":       true,
	"node_import":       true,
	"node_export":       true,
	"execute_command":   true,
	"execute_script":    true,
	"run_playbook":      true,
	"transfer_file":     true,
	"file_download":     true,
	"async_cancel":      true,
	"settings_set":      true,
	"history_clean":     true,
	"playbook_generate": true,
}

// SetConfirmGate 注册确认门回调。nil 可清除。
func (a *Agent) SetConfirmGate(gate func(ToolCall) ConfirmationDecision) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.confirmGate = gate
}

// SetSessionMemory 注入会话记忆文本（对话+操作记录），随路由消息发送。
func (a *Agent) SetSessionMemory(memory string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessionMemory = memory
}

// SetNodeContextHook 注册节点上下文回调：涉及节点的工具执行成功后，
// 回调解析出的节点名列表与来源描述（程序层确定性保存，供跨轮复用）。
func (a *Agent) SetNodeContextHook(hook func(nodes []string, source string)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nodeContextHook = hook
}

// nodeTargetTools 执行后需要记录节点上下文的工具。
var nodeTargetTools = map[string]bool{
	"query_nodes":       true,
	"query_database":    true,
	"execute_command":   true,
	"execute_script":    true,
	"transfer_file":     true,
	"file_download":     true,
	"run_playbook":      true,
	"node_check":        true,
	"node_ping":         true,
	"node_status":       true,
}

// resolveToolTargets 从工具参数解析目标节点名列表（确定性，非 LLM 记忆）。
// 支持 nodes 数组 / group（逗号分隔多分组）/ label（逗号分隔多键值）/ search。
func (a *Agent) resolveToolTargets(call ToolCall) ([]string, string) {
	args := call.Arguments
	nodes := strSliceOf(args["nodes"])
	if len(nodes) == 1 && nodes[0] == "ALL_NODES" {
		nodes = nil
	}
	if len(nodes) > 0 {
		return nodes, fmt.Sprintf("nodes=%s", strings.Join(nodes, ","))
	}
	if g := strOf(args["group"]); g != "" {
		var names []string
		for _, part := range strings.Split(g, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			for _, n := range a.nodeMgr.GetByGroup(part) {
				names = append(names, n.Name)
			}
		}
		return uniqueStrings(names), fmt.Sprintf("group=%s", g)
	}
	if l := strOf(args["label"]); l != "" {
		names := a.nodeMgr.GetByLabels(parseLabelFilter(l))
		var out []string
		for _, n := range names {
			out = append(out, n.Name)
		}
		return out, fmt.Sprintf("label=%s", l)
	}
	if s := strOf(args["search"]); s != "" {
		names := a.nodeMgr.SearchByName(s)
		var out []string
		for _, n := range names {
			out = append(out, n.Name)
		}
		return out, fmt.Sprintf("search=%s", s)
	}
	// 无显式目标：视为全部节点（查询类工具默认行为）
	var all []string
	for _, n := range a.nodeMgr.List() {
		all = append(all, n.Name)
	}
	return all, "全部节点"
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ExecuteToolCall 执行工具调用（供确认门重放使用，不再经过确认门）。
func (a *Agent) ExecuteToolCall(ctx context.Context, call ToolCall) (string, error) {
	return a.executeToolCall(ctx, call)
}

// RejectWriteOpsGate 非交互模式（单次查询）使用的确认门：
// 写操作一律拒绝并提示进入交互模式。
func RejectWriteOpsGate() func(ToolCall) ConfirmationDecision {
	return func(call ToolCall) ConfirmationDecision {
		if confirmRequiredTools[call.Name] {
			return ConfirmationDecision{
				Confirm:  true,
				Summary:  SummarizeToolCall(call),
				Question: fmt.Sprintf("该操作（%s）需要交互确认，请进入交互模式执行（不带参数运行 owl ai）。", call.Name),
			}
		}
		return ConfirmationDecision{Confirm: false}
	}
}

// SummarizeToolCall 生成工具调用的人类可读摘要。
func SummarizeToolCall(call ToolCall) string {
	var sb strings.Builder
	sb.WriteString(call.Name)
	if len(call.Arguments) > 0 {
		keys := []string{"nodes", "command", "name", "group", "id", "node",
			"source_file", "dest_dir", "script", "requirement", "file", "key", "value", "type"}
		var parts []string
		for _, k := range keys {
			if v, ok := call.Arguments[k]; ok {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
		}
		if len(parts) > 0 {
			sb.WriteString("(")
			sb.WriteString(strings.Join(parts, ", "))
			sb.WriteString(")")
		}
	}
	return sb.String()
}

type ChatModel interface {
	Generate(ctx context.Context, messages []Message) (string, error)
}

type ProgressCallback func(step string, detail string)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatModelFunc func(ctx context.Context, messages []Message) (string, error)

func (f ChatModelFunc) Generate(ctx context.Context, messages []Message) (string, error) {
	return f(ctx, messages)
}

var groupPrompts = map[string]string{
	"node_list":         aiPrompts.NodeListSystemPrompt,
	"query_nodes":       aiPrompts.NodeListSystemPrompt,
	"node_add":          aiPrompts.NodeAddSystemPrompt,
	"node_update":       aiPrompts.NodeUpdateSystemPrompt,
	"node_remove":       aiPrompts.NodeRemoveSystemPrompt,
	"node_status":       aiPrompts.NodeStatusSystemPrompt,
	"node_groups":       aiPrompts.NodeGroupsSystemPrompt,
	"node_labels":       aiPrompts.NodeLabelsSystemPrompt,
	"node_import":       aiPrompts.NodeImportSystemPrompt,
	"node_ping":         aiPrompts.NodePingSystemPrompt,
	"node_check":        aiPrompts.NodeCheckSystemPrompt,
	"exec_run":          aiPrompts.ExecRunSystemPrompt,
	"exec_script":       aiPrompts.ExecScriptSystemPrompt,
	"file":              aiPrompts.FileSystemPrompt,
	"playbook_list":     aiPrompts.PlaybookListSystemPrompt,
	"playbook_run":      aiPrompts.PlaybookRunSystemPrompt,
	"playbook_validate": aiPrompts.PlaybookValidateSystemPrompt,
	// 以下类别使用通用工具目录提示词
	"node_export":          aiPrompts.GenericToolSystemPrompt,
	"file_download":        aiPrompts.GenericToolSystemPrompt,
	"playbook_generate":    aiPrompts.GenericToolSystemPrompt,
	"playbook_template_list": aiPrompts.GenericToolSystemPrompt,
	"playbook_template_info": aiPrompts.GenericToolSystemPrompt,
	"playbook_template_export": aiPrompts.GenericToolSystemPrompt,
	"playbook_scaffold":    aiPrompts.GenericToolSystemPrompt,
	"playbook_state_list":  aiPrompts.GenericToolSystemPrompt,
	"playbook_state_show":  aiPrompts.GenericToolSystemPrompt,
	"async_list":           aiPrompts.GenericToolSystemPrompt,
	"async_status":         aiPrompts.GenericToolSystemPrompt,
	"async_cancel":         aiPrompts.GenericToolSystemPrompt,
	"settings_show":        aiPrompts.GenericToolSystemPrompt,
	"settings_set":         aiPrompts.GenericToolSystemPrompt,
	"history_list":         aiPrompts.GenericToolSystemPrompt,
	"history_clean":        aiPrompts.GenericToolSystemPrompt,
}

// unsupportedRouteLabels 路由命中的豁免命令：AI 明确不支持。
var unsupportedRouteLabels = map[string]bool{
	"session": true, "serve": true, "tui": true,
	"metrics": true, "node_sample": true, "sample": true,
}

// normalizeRouteLabel 清洗路由响应: 小写、去空白、去尾部句点与 markdown 围栏。
func normalizeRouteLabel(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimRight(s, ".")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// applyRouteAliases 将别名路由标签映射到正式标签。
func applyRouteAliases(label string) string {
	switch label {
	case "exec", "execute":
		return "exec_run"
	case "playbook":
		return "playbook_list"
	case "node":
		return "node_list"
	}
	return label
}

// isValidRouteLabel 判断标签是否为已注册路由类别(精确或包含匹配)或 "uncertain"。
func isValidRouteLabel(routeLabel string) bool {
	if routeLabel == "uncertain" {
		return true
	}
	if _, ok := groupPrompts[routeLabel]; ok {
		return true
	}
	for k := range groupPrompts {
		if strings.Contains(routeLabel, k) {
			return true
		}
	}
	return false
}

var toolHints = map[string]string{
	"execute_command":   aiPrompts.ExecuteCommandPrompt,
	"execute_script":    aiPrompts.ExecuteScriptPrompt,
	"generate_playbook": aiPrompts.PlaybookPrompt,
	"transfer_file":     aiPrompts.TransferPrompt,
}

func NewAgent(executor Executor, config *Config, nodeMgr node.Manager, nodeStore NodeStoreAdapter, playbookParser *playbook.Parser, debug ...bool) (*Agent, error) {
	registry := NewToolRegistry()
	registry.Register(NewQueryNodesTool(executor, nodeMgr, nodeStore))
	registry.Register(NewExecuteCommandTool(executor, nodeMgr))
	registry.Register(NewGeneratePlaybookTool(executor, nodeMgr))
	registry.Register(NewTransferFileTool(executor, nodeMgr))
	registry.Register(NewExecuteScriptTool(executor, nodeMgr))
	registry.Register(NewQueryDatabaseTool(executor, nodeMgr))
	registry.Register(NewListPlaybooksTool(executor))
	registry.Register(NewRunPlaybookTool(executor, nodeMgr))
	registry.Register(NewFileDownloadTool(executor))
	registry.Register(NewPlaybookTemplateListTool(executor))
	registry.Register(NewPlaybookTemplateInfoTool(executor))
	registry.Register(NewPlaybookTemplateExportTool(executor))
	registry.Register(NewPlaybookScaffoldTool(executor))
	registry.Register(NewPlaybookStateListTool(executor))
	registry.Register(NewPlaybookStateShowTool(executor))
	registry.Register(NewPlaybookGenerateTool(nodeMgr))
	registry.Register(NewAsyncListTool(executor))
	registry.Register(NewAsyncStatusTool(executor))
	registry.Register(NewAsyncCancelTool(executor))
	registry.Register(NewSettingsShowTool(executor))
	registry.Register(NewSettingsSetTool(executor))
	registry.Register(NewHistoryListTool(executor))
	registry.Register(NewHistoryCleanTool(executor))
	registry.Register(NewValidatePlaybookTool(executor))
	registry.Register(NewNodeCheckTool(executor, nodeMgr))
	registry.Register(NewNodeAddTool(executor, nodeStore))
	registry.Register(NewNodeRemoveTool(executor, nodeStore))
	registry.Register(NewNodeUpdateTool(executor, nodeStore))
	registry.Register(NewNodeStatusTool(executor, nodeMgr))
	registry.Register(NewNodePingTool(executor, nodeMgr))
	registry.Register(NewNodeGroupsTool(executor))
	registry.Register(NewNodeLabelsTool(executor))
	registry.Register(NewNodeImportTool(executor))
	registry.Register(NewNodeExportTool(executor))

	isDebug := len(debug) > 0 && debug[0]

	agent := &Agent{
		config:         config,
		nodeMgr:        nodeMgr,
		nodeStore:      nodeStore,
		registry:       registry,
		playbookParser: playbookParser,
		systemPrompt:   aiPrompts.ExecRunSystemPrompt,
		debug:          isDebug,
	}

	if config.AI.APIKey != "" && config.AI.Model != "" {
		llmClient, err := CreateLLMClient(config)
		if err == nil {
			agent.chatModel = llmClient
		} else {
			agent.chatModel = ChatModelFunc(agent.defaultChatHandler)
		}
	} else {
		agent.chatModel = ChatModelFunc(agent.defaultChatHandler)
	}

	return agent, nil
}

func (a *Agent) SetChatModel(model ChatModel) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.chatModel = model
}

func (a *Agent) SetSystemPrompt(prompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.systemPrompt = prompt
}

func (a *Agent) Process(ctx context.Context, userInput string, onProgress ProgressCallback) (string, error) {
	a.mu.RLock()
	chatModel := a.chatModel
	a.mu.RUnlock()

	nodeInfo := a.getNodeInfo()

	a.mu.RLock()
	sessionMemory := a.sessionMemory
	a.mu.RUnlock()

	routerMessages := []Message{
		{Role: "system", Content: aiPrompts.RouterPrompt},
	}
	if sessionMemory != "" {
		routerMessages = append(routerMessages, Message{
			Role:    "system",
			Content: "以下是此前会话的对话与操作记录，仅作参考背景，不要把它当作新的用户请求：\n" + sessionMemory,
		})
	}
	routerMessages = append(routerMessages, Message{Role: "user", Content: userInput})

	// 调试：打印路由消息
	debugPrint(a.debug, "路由消息数量: %d", len(routerMessages))
	for i, msg := range routerMessages {
		roleStr := "system"
		if msg.Role == "user" {
			roleStr = "user"
		}
		contentLen := len(msg.Content)
		debugPrint(a.debug, "  路由消息[%d] role=%s, 内容长度=%d", i, roleStr, contentLen)
		if a.debug && contentLen < 500 {
			debugPrint(a.debug, "  路由消息[%d] 内容前200字符: %.200s", i, msg.Content)
		}
	}

	routeResp, err := generateWithRetry(ctx, chatModel, routerMessages, "路由")
	if err != nil {
		if onProgress != nil {
			onProgress("result", "失败: "+err.Error())
		}
		return "", fmt.Errorf("路由失败: %w", err)
	}

	debugPrint(a.debug, "路由原始响应长度: %d", len(routeResp))
	debugPrint(a.debug, "路由原始响应前200字符: %.200s", routeResp)

	// 模型跳过路由阶段直接输出工具调用 JSON(部分模型行为): 直接执行,无需再走工具生成阶段
	if directCalls := a.parseToolCalls(routeResp); len(directCalls) > 0 {
		debugPrint(a.debug, "路由响应直接包含工具调用,跳过路由直接执行")
		if onProgress != nil {
			onProgress("generate", directCalls[0].Name)
		}
		result, _ := a.runToolCalls(ctx, directCalls, onProgress)
		if onProgress != nil {
			onProgress("result", "完成")
		}
		return result, nil
	}

	routeLabel := applyRouteAliases(normalizeRouteLabel(routeResp))

	debugPrint(a.debug, "路由标签: %s", routeLabel)

	if routeLabel == "uncertain" || routeLabel == "" {
		return "我不确定您要做什么", nil
	}

	if unsupportedRouteLabels[routeLabel] {
		if onProgress != nil {
			onProgress("result", "不支持")
		}
		return "该功能不支持 AI 操作", nil
	}

	// 路由响应不是有效标签(如模型输出了解释/帮助文本而非标签): 追加严格指令重试一次
	if !isValidRouteLabel(routeLabel) {
		debugPrint(a.debug, "路由标签无效,追加严格指令重试")
		retryMessages := append(append([]Message{}, routerMessages...), Message{
			Role:    "system",
			Content: "你上一次返回的内容不是有效指令标签。请仅输出一个指令标签(如 node_list / exec_run / query_nodes),不要输出任何解释或帮助文本。",
		})
		retryResp, retryErr := generateWithRetry(ctx, chatModel, retryMessages, "路由")
		if retryErr == nil {
			retryLabel := applyRouteAliases(normalizeRouteLabel(retryResp))
			if retryLabel == "uncertain" || retryLabel == "" {
				return "我不确定您要做什么", nil
			}
			if isValidRouteLabel(retryLabel) {
				routeLabel = retryLabel
				debugPrint(a.debug, "重试后路由标签: %s", routeLabel)
			}
		}
		if !isValidRouteLabel(routeLabel) {
			if onProgress != nil {
				onProgress("result", "路由失败: 模型未返回有效指令标签")
			}
			return "", fmt.Errorf("模型未返回有效指令标签,无法路由请求(原始响应: %.120s)", routeResp)
		}
	}

	if onProgress != nil {
		onProgress("route", routeLabel)
	}

	groupPrompt, ok := groupPrompts[routeLabel]
	if !ok {
		for k, v := range groupPrompts {
			if strings.Contains(routeLabel, k) {
				groupPrompt = v
				break
			}
		}
		if groupPrompt == "" {
			// 未定制提示词的类别一律使用通用工具目录，不再直接拒绝
			groupPrompt = aiPrompts.GenericToolSystemPrompt
		}
	}

	debugPrint(a.debug, "使用系统提示词: %s", routeLabel)

	toolDescs := a.registry.GetToolDescriptions()
	formattedPrompt := a.formatPrompt(groupPrompt, nodeInfo, toolDescs)

	debugPrint(a.debug, "系统提示词前100字符: %.100s...", formattedPrompt)

	if onProgress != nil {
		onProgress("analyze", "正在生成 JSON...")
	}

	messages := []Message{
		{Role: "system", Content: formattedPrompt},
	}
	// 会话记忆（对话+操作+节点上下文）也注入工具生成阶段，
	// 否则 LLM 生成工具调用时看不到上一轮节点上下文
	if sessionMemory != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: "以下是此前会话的对话与操作记录，仅作参考背景，不要把它当作新的用户请求：\n" + sessionMemory,
		})
	}
	messages = append(messages, Message{Role: "user", Content: userInput})

	var fullResponse strings.Builder
	maxTurns := 10
	var lastToolName string
	var lastToolResult string // 保存最后一个工具结果

	for turn := 0; turn < maxTurns; turn++ {
		debugPrint(a.debug, "=== 第 %d 轮对话 ===", turn+1)

		debugPrint(a.debug, "messages 数量: %d", len(messages))
		for i, msg := range messages {
			hasResult := strings.Contains(msg.Content, "[TOOL_CALL_RESULT]")
			if hasResult {
				debugPrint(a.debug, "  messages[%d] 包含工具结果", i)
			}
		}

		response, err := generateWithRetry(ctx, chatModel, messages, "AI调用")
		if err != nil {
			return "", fmt.Errorf("AI 调用失败: %w", err)
		}

		debugPrint(a.debug, "AI 响应: %.200s...", response)

		toolCalls := a.parseToolCalls(response)
		debugPrint(a.debug, "解析到工具调用数量: %d", len(toolCalls))

		if len(toolCalls) == 0 {
			if turn >= 1 {
				debugPrint(a.debug, "多轮对话，检查是否有工具结果")
				if lastToolResult != "" && (len(strings.TrimSpace(response)) == 0 || response == "") {
					return lastToolResult, nil
				}
				return response, nil
			}

			if (len(response) > 100 && !strings.Contains(response, "tool_calls")) || strings.Contains(response, "我不确定您要做什么") {
				debugPrint(a.debug, "LLM 无法生成有效工具调用，尝试使用本地参数提取器")

				nodes := a.nodeMgr.List()
				nodeNames := make([]string, 0, len(nodes))
				for _, n := range nodes {
					nodeNames = append(nodeNames, n.Name)
				}

				classifier := NewIntentClassifier()
				intentResult := classifier.Classify(userInput)

				// 置信度阈值 20: 两个及以上关键词命中(如"列出节点")即视为有效意图,
				// 单关键词命中(置信度 10)仍拒绝,兼顾召回与误判。
				if intentResult.Type == IntentUncertain || intentResult.Confidence < 20 {
					debugPrint(a.debug, "本地分类器也无法确定，返回不确定")
					return "我不确定您要做什么", nil
				}

				extractor := NewParamExtractor(nodeNames)
				params := extractor.ExtractParams(intentResult.Type, userInput)

				validator := NewValidator()
				if err := validator.ValidateParams(intentResult.Type, params); err != nil {
					debugPrint(a.debug, "参数验证失败: %v", err)
					return "我不确定您要做什么", nil
				}

				debugPrint(a.debug, "使用本地参数提取成功: %v", params)

				var toolCallJSON string
				switch intentResult.Type {
				case IntentQueryNodes:
					toolCallJSON = a.buildToolCall("query_nodes", params)
				case IntentExecuteCmd:
					toolCallJSON = a.buildToolCall("execute_command", params)
				case IntentExecuteScript:
					toolCallJSON = a.buildToolCall("execute_script", params)
				case IntentGeneratePlaybook:
					toolCallJSON = a.buildToolCall("generate_playbook", params)
			case IntentTransferFile:
				toolCallJSON = a.buildToolCall("transfer_file", params)
			case IntentFileDownload:
				toolCallJSON = a.buildToolCall("file_download", params)
			default:
				return "我不确定您要做什么", nil
			}

			if toolCallJSON != "" {
				debugPrint(a.debug, "使用本地提取的工具调用")
					toolCalls := a.parseToolCalls(toolCallJSON)
					if len(toolCalls) > 0 {
						if onProgress != nil {
							onProgress("generate", toolCalls[0].Name)
						}
						lastToolName = toolCalls[0].Name
						messages = append(messages, Message{Role: "assistant", Content: toolCallJSON})

					for _, call := range toolCalls {
						if onProgress != nil {
							onProgress("execute", call.Name)
						}
						if ok, question := a.confirmToolCall(call); !ok {
							if onProgress != nil {
								onProgress("result", "等待确认")
							}
							return question, nil
						}
						result, err := a.executeToolCall(ctx, call)
						if err != nil {
							result = fmt.Sprintf("Tool execution failed: %v", err)
						}
						lastToolResult = result
						return result, nil
					}
					}
				}
			}

			debugPrint(a.debug, "无有效工具调用，返回不确定（LLM 自由文本不透出）")
			return "我不确定您要做什么", nil
		}

		if onProgress != nil && len(toolCalls) > 0 {
			onProgress("generate", toolCalls[0].Name)
		}

		lastToolName = toolCalls[0].Name
		messages = append(messages, Message{Role: "assistant", Content: response})

		var toolResultStr string
		for _, call := range toolCalls {
			if onProgress != nil {
				onProgress("execute", call.Name)
			}
			if ok, question := a.confirmToolCall(call); !ok {
				if onProgress != nil {
					onProgress("result", "等待确认")
				}
				return question, nil
			}
			result, err := a.executeToolCall(ctx, call)
			if err != nil {
				result = fmt.Sprintf("Tool execution failed: %v", err)
			}
			toolResultStr = result
			lastToolResult = result
			messages = append(messages, Message{Role: "user", Content: fmt.Sprintf("\n\n[TOOL_CALL_RESULT]\n%s\n[/TOOL_CALL_RESULT]", result)})
		}

		if turn >= 1 && lastToolName != "" {
			if hint, ok := toolHints[lastToolName]; ok {
				hintMsg := Message{
					Role:    "system",
					Content: fmt.Sprintf("\n\n%s", hint),
				}
				messages = append(messages, hintMsg)
			}
		}

		if turn == 0 && len(toolCalls) > 0 {
			debugPrint(a.debug, "首轮执行工具后直接返回结果，不再进行额外LLM调用")
			if onProgress != nil {
				onProgress("result", "完成")
			}
			return toolResultStr, nil
		}
	}

	if onProgress != nil {
		onProgress("result", "完成")
	}

	return fullResponse.String(), nil
}

func (a *Agent) ProcessWithContext(ctx context.Context, messages []Message, onProgress ProgressCallback) ([]Message, string, error) {
	a.mu.RLock()
	chatModel := a.chatModel
	a.mu.RUnlock()

	nodeInfo := a.getNodeInfo()

	if len(messages) == 0 {
		response, err := a.Process(ctx, "", onProgress)
		return nil, response, err
	}

	toolDescs := a.registry.GetToolDescriptions()

	formattedPrompt := a.formatPrompt(messages[0].Content, nodeInfo, toolDescs)
	msgs := make([]Message, len(messages))
	msgs[0] = Message{Role: messages[0].Role, Content: formattedPrompt}
	copy(msgs[1:], messages[1:])

	var fullResponse strings.Builder
	maxTurns := 10

	for turn := 0; turn < maxTurns; turn++ {
		response, err := generateWithRetry(ctx, chatModel, msgs, "AI调用")
		if err != nil {
			return msgs, "", fmt.Errorf("AI 调用失败: %w", err)
		}

		toolCalls := a.parseToolCalls(response)
		if len(toolCalls) == 0 {
			if turn >= 1 {
				// 多轮：返回 LLM 对工具结果的总结（此前版本误返回空的 fullResponse）
				if onProgress != nil {
					onProgress("result", "完成")
				}
				if strings.TrimSpace(response) == "" {
					response = "完成"
				}
				return msgs, response, nil
			}
			debugPrint(a.debug, "无有效工具调用，返回不确定（LLM 自由文本不透出）")
			return msgs, "我不确定您要做什么", nil
		}

		if onProgress != nil && len(toolCalls) > 0 {
			onProgress("generate", toolCalls[0].Name)
		}

		msgs = append(msgs, Message{Role: "assistant", Content: response})

		var toolResultStr string
		for _, call := range toolCalls {
			if onProgress != nil {
				onProgress("execute", call.Name)
			}
			if ok, question := a.confirmToolCall(call); !ok {
				if onProgress != nil {
					onProgress("result", "等待确认")
				}
				return msgs, question, nil
			}
			result, err := a.executeToolCall(ctx, call)
			if err != nil {
				result = fmt.Sprintf("Tool execution failed: %v", err)
			}
			toolResultStr = result
			msgs = append(msgs, Message{Role: "user", Content: fmt.Sprintf("\n\n[TOOL_CALL_RESULT]\n%s\n[/TOOL_CALL_RESULT]", result)})
		}

		if turn == 0 && len(toolCalls) > 0 {
			debugPrint(a.debug, "首轮执行工具后直接返回结果，不再进行额外LLM调用")
			if onProgress != nil {
				onProgress("result", "完成")
			}
			return msgs, toolResultStr, nil
		}
	}

	if onProgress != nil {
		onProgress("result", "完成")
	}

	return msgs, fullResponse.String(), nil
}

const maxRetries = 3
const retryDelay = 500 * time.Millisecond

func generateWithRetry(ctx context.Context, chatModel ChatModel, messages []Message, label string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := chatModel.Generate(ctx, messages)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(retryDelay * time.Duration(attempt)):
			}
		}
	}
	return "", fmt.Errorf("%s重试%d次后仍失败: %w", label, maxRetries, lastErr)
}

func (a *Agent) formatPrompt(systemPrompt, nodeInfo, toolDescs string) string {
	tmpl, err := template.New("system").Parse(systemPrompt)
	if err != nil {
		return systemPrompt
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		ToolDescriptions string
		NodeInfo         string
	}{
		ToolDescriptions: toolDescs,
		NodeInfo:         nodeInfo,
	})
	if err != nil {
		return systemPrompt
	}

	return buf.String()
}

func (a *Agent) getNodeInfo() string {
	nodes := a.nodeMgr.List()
	if len(nodes) == 0 {
		return "No node information available"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Total %d nodes:\n\n", len(nodes)))

	groups := make(map[string][]string)
	for _, n := range nodes {
		for _, g := range n.Groups {
			groups[g] = append(groups[g], n.Name)
		}
	}

	if len(groups) > 0 {
		sb.WriteString("Groups:\n")
		for group, nodeNames := range groups {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", group, strings.Join(nodeNames, ", ")))
		}
	}

	online := 0
	for _, n := range nodes {
		if n.Status == model.NodeStatusOnline {
			online++
		}
	}
	sb.WriteString(fmt.Sprintf("\n在线: %d/%d\n", online, len(nodes)))

	return sb.String()
}

type ToolCall struct {
	Name      string
	Arguments map[string]interface{}
}

func (a *Agent) parseToolCalls(response string) []ToolCall {
	var jsonContent string

	// Try 1: ```json ... ``` wrapper
	if idx := strings.Index(response, "```json"); idx != -1 {
		start := idx + 7
		if end := strings.Index(response[start:], "```"); end != -1 {
			jsonContent = strings.TrimSpace(response[start : start+end])
		}
	}

	// Try 2: bare ``` ... ``` wrapper (no json tag)
	if jsonContent == "" {
		if idx := strings.Index(response, "```"); idx != -1 {
			start := idx + 3
			if end := strings.Index(response[start:], "```"); end != -1 {
				candidate := strings.TrimSpace(response[start : start+end])
				if strings.Contains(candidate, `"tool_calls"`) {
					jsonContent = candidate
				}
			}
		}
	}

	// Try 3: bare JSON anywhere in response
	if jsonContent == "" {
		toolCallsIdx := strings.Index(response, `"tool_calls"`)
		if toolCallsIdx != -1 {
			braceStart := strings.LastIndex(response[:toolCallsIdx], "{")
			braceEnd := strings.LastIndex(response, "}")
			if braceStart != -1 && braceEnd != -1 && braceEnd > braceStart {
				candidate := strings.TrimSpace(response[braceStart : braceEnd+1])
				jsonContent = candidate
			}
		}
	}

	if jsonContent == "" {
		return nil
	}

	var parsed struct {
		ToolCalls []struct {
			Name string                 `json:"name"`
			Args map[string]interface{} `json:"arguments"`
		} `json:"tool_calls"`
	}

	if err := json.Unmarshal([]byte(jsonContent), &parsed); err != nil {
		return nil
	}

	var calls []ToolCall
	for _, tc := range parsed.ToolCalls {
		calls = append(calls, ToolCall{
			Name:      tc.Name,
			Arguments: tc.Args,
		})
	}
	return calls
}

// confirmToolCall 执行前过确认门。返回 (true,"") 表示放行；
// 返回 (false, question) 表示已拦截，question 为返回给用户的文案。
// 未注册确认门时，写操作默认拒绝（安全兜底），只读操作放行。
func (a *Agent) confirmToolCall(call ToolCall) (bool, string) {
	a.mu.RLock()
	gate := a.confirmGate
	a.mu.RUnlock()
	if gate == nil {
		if confirmRequiredTools[call.Name] {
			return false, fmt.Sprintf("该操作（%s）需要交互确认，当前上下文未启用确认机制，已拒绝执行。", call.Name)
		}
		return true, ""
	}
	d := gate(call)
	if d.Confirm {
		return false, d.Question
	}
	return true, ""
}

// runToolCalls 顺序执行一批工具调用。全部放行时返回(首个结果, true);
// 确认门拦截时返回(问题文案, false),与 Process 工具阶段的首轮语义一致。
func (a *Agent) runToolCalls(ctx context.Context, calls []ToolCall, onProgress ProgressCallback) (string, bool) {
	for _, call := range calls {
		if onProgress != nil {
			onProgress("execute", call.Name)
		}
		if ok, question := a.confirmToolCall(call); !ok {
			if onProgress != nil {
				onProgress("result", "等待确认")
			}
			return question, false
		}
		result, err := a.executeToolCall(ctx, call)
		if err != nil {
			result = fmt.Sprintf("Tool execution failed: %v", err)
		}
		return result, true
	}
	return "", true
}

func (a *Agent) executeToolCall(ctx context.Context, call ToolCall) (string, error) {
	debugPrint(a.debug, "执行工具: %s", call.Name)
	debugPrint(a.debug, "工具参数: %+v", call.Arguments)

	tool, ok := a.registry.Get(call.Name)
	if !ok {
		debugPrint(a.debug, "工具不存在: %s", call.Name)
		return "", fmt.Errorf("未知工具: %s", call.Name)
	}

	if err := tool.Validate(call.Arguments); err != nil {
		debugPrint(a.debug, "参数验证失败: %v", err)
		return "", fmt.Errorf("参数验证失败: %w", err)
	}

	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		debugPrint(a.debug, "工具执行失败: %v", err)
		return "", err
	}

	// 涉及节点的工具执行成功后，把解析出的节点名回调给会话（程序层保存）
	if err == nil && nodeTargetTools[call.Name] && a.nodeMgr != nil {
		nodes, source := a.resolveToolTargets(call)
		if len(nodes) > 0 {
			a.mu.RLock()
			hook := a.nodeContextHook
			a.mu.RUnlock()
			if hook != nil {
				hook(nodes, source)
			}
		}
	}

	debugPrint(a.debug, "工具执行成功，结果前100字符: %.100s...", result)
	return result, nil
}

func (a *Agent) defaultChatHandler(ctx context.Context, messages []Message) (string, error) {
	if len(messages) < 2 {
		return "", fmt.Errorf("insufficient messages")
	}

	lastMsg := messages[len(messages)-1]
	input := lastMsg.Content

	nodes := a.nodeMgr.List()
	nodeNames := make([]string, 0, len(nodes))
	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}

	classifier := NewIntentClassifier()
	intentResult := classifier.Classify(input)

	formatter := NewResponseFormatter()
	extractor := NewParamExtractor(nodeNames)
	validator := NewValidator()

	if intentResult.Type == IntentUncertain || intentResult.Confidence < 20 {
		return formatter.FormatUncertainHelp(), nil
	}

	params := extractor.ExtractParams(intentResult.Type, input)

	if err := validator.ValidateParams(intentResult.Type, params); err != nil {
		return "", fmt.Errorf("参数验证失败：%w", err)
	}

	var toolCallJSON string
	switch intentResult.Type {
	case IntentQueryNodes:
		toolCallJSON = a.buildToolCall("query_nodes", params)
	case IntentExecuteCmd:
		toolCallJSON = a.buildToolCall("execute_command", params)
	case IntentExecuteScript:
		toolCallJSON = a.buildToolCall("execute_script", params)
	case IntentGeneratePlaybook:
		toolCallJSON = a.buildToolCall("generate_playbook", params)
	case IntentTransferFile:
		toolCallJSON = a.buildToolCall("transfer_file", params)
	case IntentFileDownload:
		toolCallJSON = a.buildToolCall("file_download", params)
	}

	return toolCallJSON, nil
}

func (a *Agent) buildToolCall(toolName string, params map[string]interface{}) string {
	paramsJSON, _ := json.Marshal(params)
	return fmt.Sprintf("```json\n{\"tool_calls\": [{\"name\": \"%s\", \"arguments\": %s}]}\n```", toolName, string(paramsJSON))
}

// PendingContext 待确认操作（确认门）。由确认门在拦截时写入，
// 用户肯定后按 ToolCall 确定性重放，不依赖 LLM 从对话历史推断。
type PendingContext struct {
	State     string
	Summary   string
	Question  string
	ToolCall  ToolCall
	UserInput string
}

// OperationSummary 会话内已完成操作的记录，供"刚才那个操作"类追问。
type OperationSummary struct {
	Tool    string
	Summary string
	Result  string
	Time    time.Time
}

// NodeContext 会话级节点上下文：最近一次涉及节点操作后，
// 由程序逻辑层（非 LLM）从工具参数解析出的节点集合。
// 供后续轮次复用/筛选，直到用户发起新一轮节点查询则覆盖。
type NodeContext struct {
	Nodes  []string // 解析后的节点名
	Source string   // 来源描述，如 "group=web 的节点"
}

type Session struct {
	agent          *Agent
	messages       []Message
	history        []string
	operations     []OperationSummary
	dialogue       []Message
	nodeContext    *NodeContext
	createdAt      time.Time
	lastActive     time.Time
	OnProgress     ProgressCallback
	pendingContext *PendingContext
}

func NewSession(agent *Agent) *Session {
	s := &Session{
		agent:     agent,
		messages:  make([]Message, 0),
		history:   make([]string, 0),
		createdAt: time.Now(),
	}
	s.SetDefaultConfirmGate()
	s.registerNodeContextHook()
	return s
}

// registerNodeContextHook 注册节点上下文回调。agent 可能被多会话共享，
// 每次 Send 前需重新注册，保证最近注册的是本会话的回调。
func (s *Session) registerNodeContextHook() {
	s.agent.SetNodeContextHook(func(nodes []string, source string) {
		s.nodeContext = &NodeContext{Nodes: nodes, Source: source}
	})
}

var affirmativeReplies = map[string]bool{
	"是": true, "是的": true, "对": true, "对的": true,
	"好": true, "好的": true, "可以": true, "行": true,
	"yes": true, "ok": true, "okay": true, "y": true,
	"嗯": true, "确认": true, "确定": true,
}

var negativeReplies = map[string]bool{
	"不": true, "不是": true, "否": true, "不要": true,
	"算了": true, "取消": true, "停下": true, "停止": true,
	"no": true, "n": true, "cancel": true, "stop": true,
	"不用": true, "不需要": true, "不了": true,
}

// RenderSystemPrompt 渲染系统提示词（注入工具目录与节点信息）。
func (a *Agent) RenderSystemPrompt(prompt string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.registry == nil {
		return prompt
	}
	toolDescs := a.registry.GetToolDescriptions()
	return a.formatPrompt(prompt, a.getNodeInfo(), toolDescs)
}

func (s *Session) Send(ctx context.Context, userInput string) (string, error) {
	s.lastActive = time.Now()
	s.history = append(s.history, fmt.Sprintf("User: %s", userInput))
	// agent 可被多会话共享，每次 Send 前重新注册本会话的节点上下文回调
	s.registerNodeContextHook()

	// 有待确认操作时，先处理确认/取消，其余输入一律提示，保证单 pending 队列。
	if s.pendingContext != nil && s.pendingContext.State == "awaiting_confirmation" {
		lowerInput := strings.TrimSpace(strings.ToLower(userInput))
		if affirmativeReplies[lowerInput] {
			pending := s.pendingContext
			s.pendingContext = nil
			result, err := s.agent.ExecuteToolCall(ctx, pending.ToolCall)
			if err != nil {
				return "", err
			}
			s.recordOperation(pending.ToolCall, pending.Summary, result)
			s.messages = append(s.messages,
				Message{Role: "assistant", Content: fmt.Sprintf("```json\n%s\n```", s.toolCallJSON(pending.ToolCall))},
				Message{Role: "user", Content: fmt.Sprintf("\n\n[TOOL_CALL_RESULT]\n%s\n[/TOOL_CALL_RESULT]", result)},
			)
			msg := fmt.Sprintf("已执行：%s\n%s", pending.Summary, result)
			s.appendDialogue(userInput, msg)
			s.history = append(s.history, fmt.Sprintf("Assistant: %s", msg))
			return msg, nil
		}
		if negativeReplies[lowerInput] {
			s.pendingContext = nil
			s.appendDialogue(userInput, "已取消该操作")
			s.history = append(s.history, "Assistant: 已取消该操作")
			return "已取消该操作", nil
		}
		return fmt.Sprintf("有未确认的操作：%s。请回复「是」确认，或「否」取消。", s.pendingContext.Summary), nil
	}

	// 首次交互（无上下文），使用 Process（带路由）
	if len(s.messages) == 0 {
		response, err := s.agent.Process(ctx, userInput, s.OnProgress)
		if err != nil {
			return "", err
		}
		s.appendDialogue(userInput, response)
		s.history = append(s.history, fmt.Sprintf("Assistant: %s", response))
		return response, nil
	}

	// 多轮对话，继续使用 ProcessWithContext；注入会话记忆作为背景。
	s.messages = append(s.messages, Message{Role: "user", Content: userInput})
	msgs := s.messages
	// 首条若非带工具目录的 system 引导（如确认重放后首条是 assistant），
	// 补渲染后的通用工具引导，否则 LLM 生成阶段看不到工具目录。
	if len(msgs) == 0 || msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "输出契约") {
		base := s.agent.RenderSystemPrompt(aiPrompts.GenericToolSystemPrompt)
		msgs = append([]Message{{Role: "system", Content: base}}, msgs...)
	}
	if memory := s.buildMemory(); memory != "" {
		// 记忆并入引导消息，保证 msgs[0] 仍是完整 system 引导
		msgs[0].Content = msgs[0].Content + "\n\n" + memory
	}
	updatedMessages, response, err := s.agent.ProcessWithContext(ctx, msgs, s.OnProgress)
	if err == nil {
		if len(msgs) > len(s.messages) {
			s.messages = updatedMessages[1:]
		} else {
			s.messages = updatedMessages
		}
	}

	if err != nil {
		return "", err
	}

	s.appendDialogue(userInput, response)
	s.history = append(s.history, fmt.Sprintf("Assistant: %s", response))
	return response, nil
}

func (s *Session) toolCallJSON(call ToolCall) string {
	argsJSON, _ := json.Marshal(call.Arguments)
	return fmt.Sprintf(`{"tool_calls":[{"name":%q,"arguments":%s}]}`, call.Name, argsJSON)
}

// recordOperation 记录一次已执行的操作（确认后重放或直接执行的工具调用）。
func (s *Session) recordOperation(call ToolCall, summary, result string) {
	s.operations = append(s.operations, OperationSummary{
		Tool:    call.Name,
		Summary: summary,
		Result:  truncateStr(result, 300),
		Time:    time.Now(),
	})
	if len(s.operations) > 10 {
		s.operations = s.operations[len(s.operations)-10:]
	}
}

func (s *Session) operationMemory() string {
	if len(s.operations) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[会话操作记录]（最近操作：）\n")
	for i, op := range s.operations {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s: %s → %s\n", i+1, op.Time.Format("15:04:05"), op.Tool, op.Summary, truncateStr(op.Result, 120)))
	}
	return sb.String()
}

func (s *Session) dialogueMemory() string {
	if len(s.dialogue) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[最近对话]\n")
	for _, m := range s.dialogue {
		role := "助手"
		if m.Role == "user" {
			role = "用户"
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", role, truncateStr(m.Content, 300)))
	}
	return sb.String()
}

func (s *Session) buildMemory() string {
	mem := s.operationMemory()
	if d := s.dialogueMemory(); d != "" {
		if mem != "" {
			mem += "\n"
		}
		mem += d
	}
	if n := s.nodeContextMemory(); n != "" {
		if mem != "" {
			mem += "\n"
		}
		mem += n
	}
	return mem
}

// nodeContextMemory 注入上一轮节点上下文。提示 LLM 仅在用户未指定新目标时复用。
func (s *Session) nodeContextMemory() string {
	if s.nodeContext == nil || len(s.nodeContext.Nodes) == 0 {
		return ""
	}
	return fmt.Sprintf("[上一轮节点上下文]\n来源: %s\n节点: %s\n说明: 仅当用户指代之前操作过的节点时复用此节点列表；若用户指定了新的分组/标签/节点，忽略本上下文。",
		s.nodeContext.Source, strings.Join(s.nodeContext.Nodes, ", "))
}

func (s *Session) appendDialogue(userInput, response string) {
	s.dialogue = append(s.dialogue,
		Message{Role: "user", Content: truncateStr(userInput, 300)},
		Message{Role: "assistant", Content: truncateStr(response, 300)},
	)
	if len(s.dialogue) > 12 {
		s.dialogue = s.dialogue[len(s.dialogue)-12:]
	}
}

// SetDefaultConfirmGate 注册本会话的默认确认门（写操作拦截、保存 pending）。
// 每次调用 Send 前由 CLI 或调用方执行，保证最近注册的是本会话的门。
func (s *Session) SetDefaultConfirmGate() {
	s.agent.SetConfirmGate(func(call ToolCall) ConfirmationDecision {
		if !confirmRequiredTools[call.Name] {
			return ConfirmationDecision{Confirm: false}
		}
		summary := SummarizeToolCall(call)
		question := fmt.Sprintf("即将执行：%s\n是否继续？（是/否）", summary)
		s.pendingContext = &PendingContext{
			State:    "awaiting_confirmation",
			Summary:  summary,
			Question: question,
			ToolCall: call,
		}
		return ConfirmationDecision{Confirm: true, Summary: summary, Question: question}
	})
}

func (s *Session) GetHistory() []string {
	return s.history
}

func (s *Session) MessageCount() int {
	return len(s.messages)
}

type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

func (m *SessionManager) CreateSession(sessionID string, agent *Agent) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := NewSession(agent)
	m.sessions[sessionID] = session
	return session
}

func (m *SessionManager) GetSession(sessionID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	return session, ok
}

type NodeInfoAdapter struct {
	ID        string
	Name      string
	Address   string
	Port      int
	Status    string
	Groups    []string
	Labels    map[string]string
	CreatedAt string
	UpdatedAt string
}

type NodeStoreAdapter interface {
	List() ([]*NodeInfoAdapter, error)
	Get(id string) (*NodeInfoAdapter, error)
	Add(node *NodeInfoAdapter) error
	Remove(id string) error
	Update(node *NodeInfoAdapter) error
	Save() error
	Load() error
}

type NodeStoreBridge struct {
	nodes map[string]*NodeInfoAdapter
}

func NewNodeStoreBridge() *NodeStoreBridge {
	return &NodeStoreBridge{
		nodes: make(map[string]*NodeInfoAdapter),
	}
}

func (b *NodeStoreBridge) List() ([]*NodeInfoAdapter, error) {
	result := make([]*NodeInfoAdapter, 0, len(b.nodes))
	for _, n := range b.nodes {
		result = append(result, n)
	}
	return result, nil
}

func (b *NodeStoreBridge) SyncFromStore(store NodeStoreAdapter) error {
	nodes, err := store.List()
	if err != nil {
		return err
	}
	b.nodes = make(map[string]*NodeInfoAdapter)
	for _, n := range nodes {
		b.nodes[n.ID] = n
	}
	return nil
}

func (b *NodeStoreBridge) Get(id string) (*NodeInfoAdapter, error) {
	node, ok := b.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node not found: %s", id)
	}
	return node, nil
}

func (b *NodeStoreBridge) Add(node *NodeInfoAdapter) error {
	if _, ok := b.nodes[node.ID]; ok {
		return fmt.Errorf("node already exists: %s", node.ID)
	}
	b.nodes[node.ID] = node
	return nil
}

func (b *NodeStoreBridge) Remove(id string) error {
	if _, ok := b.nodes[id]; !ok {
		return fmt.Errorf("node not found: %s", id)
	}
	delete(b.nodes, id)
	return nil
}

func (b *NodeStoreBridge) Update(node *NodeInfoAdapter) error {
	b.nodes[node.ID] = node
	return nil
}

func (b *NodeStoreBridge) Save() error {
	return nil
}

func (b *NodeStoreBridge) Load() error {
	return nil
}

func (b *NodeStoreBridge) Refresh() {
	b.nodes = make(map[string]*NodeInfoAdapter)
}

func InitNodeManager(store NodeStoreAdapter) node.Manager {
	adapter := &nodeStoreAdapter{store: store}
	return node.NewManager(adapter)
}

type nodeStoreAdapter struct {
	store NodeStoreAdapter
}

func (a *nodeStoreAdapter) Get(id string) (*model.Node, bool) {
	info, err := a.store.Get(id)
	if err != nil {
		return nil, false
	}
	return a.toModelNode(info), true
}

func (a *nodeStoreAdapter) Set(id string, node *model.Node) {
	info := a.toNodeInfo(node)
	a.store.Update(info)
}

func (a *nodeStoreAdapter) Delete(id string) bool {
	err := a.store.Remove(id)
	return err == nil
}

func (a *nodeStoreAdapter) GetAll() []*model.Node {
	infos, err := a.store.List()
	if err != nil {
		return nil
	}
	result := make([]*model.Node, 0, len(infos))
	for _, info := range infos {
		result = append(result, a.toModelNode(info))
	}
	return result
}

func (a *nodeStoreAdapter) toModelNode(info *NodeInfoAdapter) *model.Node {
	groups := make([]string, len(info.Groups))
	copy(groups, info.Groups)
	labels := make(map[string]string)
	for k, v := range info.Labels {
		labels[k] = v
	}
	return &model.Node{
		ID:      info.ID,
		Name:    info.Name,
		Address: info.Address,
		Port:    info.Port,
		Status:  model.NodeStatus(info.Status),
		Groups:  groups,
		Labels:  labels,
	}
}

func (a *nodeStoreAdapter) toNodeInfo(node *model.Node) *NodeInfoAdapter {
	groups := make([]string, len(node.Groups))
	copy(groups, node.Groups)
	labels := make(map[string]string)
	for k, v := range node.Labels {
		labels[k] = v
	}
	return &NodeInfoAdapter{
		ID:      node.ID,
		Name:    node.Name,
		Address: node.Address,
		Port:    node.Port,
		Status:  string(node.Status),
		Groups:  groups,
		Labels:  labels,
	}
}
