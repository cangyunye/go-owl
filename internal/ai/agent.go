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
	"playbook_info":     aiPrompts.PlaybookInfoSystemPrompt,
	"playbook_validate": aiPrompts.PlaybookValidateSystemPrompt,
}

var toolHints = map[string]string{
	"execute_command":   aiPrompts.ExecuteCommandPrompt,
	"execute_script":    aiPrompts.ExecuteScriptPrompt,
	"generate_playbook": aiPrompts.PlaybookPrompt,
	"transfer_file":     aiPrompts.TransferPrompt,
}

func NewAgent(config *Config, nodeMgr node.Manager, nodeStore NodeStoreAdapter, playbookParser *playbook.Parser, debug ...bool) (*Agent, error) {
	registry := NewToolRegistry()
	registry.Register(NewQueryNodesTool(nodeMgr, nodeStore))
	registry.Register(NewExecuteCommandTool(nodeMgr))
	registry.Register(NewGeneratePlaybookTool(nodeMgr))
	registry.Register(NewTransferFileTool(nodeMgr))
	registry.Register(NewExecuteScriptTool(nodeMgr))
	registry.Register(NewQueryDatabaseTool(nodeMgr))
	registry.Register(NewListPlaybooksTool())
	registry.Register(NewRunPlaybookTool(nodeMgr))
	registry.Register(NewPlaybookInfoTool())
	registry.Register(NewValidatePlaybookTool())

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

	routerMessages := []Message{
		{Role: "system", Content: aiPrompts.RouterPrompt},
		{Role: "user", Content: userInput},
	}

	routeResp, err := generateWithRetry(ctx, chatModel, routerMessages, "路由")
	if err != nil {
		if onProgress != nil {
			onProgress("result", "失败: "+err.Error())
		}
		return "", fmt.Errorf("路由失败: %w", err)
	}

	debugPrint(a.debug, "路由原始响应: %s", routeResp)

	routeLabel := strings.TrimSpace(strings.ToLower(routeResp))
	routeLabel = strings.TrimRight(routeLabel, ".")
	routeLabel = strings.TrimPrefix(routeLabel, "```")
	routeLabel = strings.TrimSuffix(routeLabel, "```")
	routeLabel = strings.TrimSpace(routeLabel)

	debugPrint(a.debug, "路由标签: %s", routeLabel)

	if routeLabel == "uncertain" || routeLabel == "" {
		return "我不确定您要做什么", nil
	}

	if routeLabel == "exec" || routeLabel == "execute" {
		routeLabel = "exec_run"
	}

	if routeLabel == "playbook" {
		routeLabel = "playbook_list"
	}

	if routeLabel == "node" {
		routeLabel = "node_list"
	}

	if routeLabel == "node_groups" {
		routeLabel = "node_list"
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
			return "我不确定您要做什么", nil
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
		{Role: "user", Content: userInput},
	}

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

				if intentResult.Type == IntentUncertain || intentResult.Confidence < 30 {
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

			fullResponse.WriteString(response)
			break
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
				if onProgress != nil {
					onProgress("result", "完成")
				}
				return msgs, fullResponse.String(), nil
			}
			if len(response) > 100 && !strings.Contains(response, "tool_calls") {
				return msgs, "我不确定您要做什么", nil
			}
			fullResponse.WriteString(response)
			msgs = append(msgs, Message{Role: "assistant", Content: response})
			break
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

	if intentResult.Type == IntentUncertain || intentResult.Confidence < 30 {
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
	}

	return toolCallJSON, nil
}

func (a *Agent) buildToolCall(toolName string, params map[string]interface{}) string {
	paramsJSON, _ := json.Marshal(params)
	return fmt.Sprintf("```json\n{\"tool_calls\": [{\"name\": \"%s\", \"arguments\": %s}]}\n```", toolName, string(paramsJSON))
}

func (a *Agent) handleQueryNodes(content string) (string, error) {
	var filter string
	if strings.Contains(content, "web") {
		filter = "group:web"
	} else if strings.Contains(content, "db") {
		filter = "group:db"
	} else if strings.Contains(content, "online") {
		filter = "status:online"
	}

	nodes := a.nodeMgr.List()

	var filtered []*model.Node
	for _, n := range nodes {
		if filter == "" {
			filtered = append(filtered, n)
			continue
		}
		if strings.HasPrefix(filter, "group:") {
			group := strings.TrimPrefix(filter, "group:")
			if n.HasGroup(group) {
				filtered = append(filtered, n)
			}
		} else if strings.HasPrefix(filter, "status:") {
			status := strings.TrimPrefix(filter, "status:")
			if string(n.Status) == status {
				filtered = append(filtered, n)
			}
		}
	}

	if len(filtered) == 0 {
		return "No matching nodes found", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("```json\n%s\n```", a.nodesToJSON(filtered)))
	return sb.String(), nil
}

func (a *Agent) handleGeneratePlaybook(content string) (string, error) {
	if !strings.Contains(strings.ToLower(content), "nginx") && !strings.Contains(strings.ToLower(content), "安装") {
		return "", nil
	}
	params := map[string]interface{}{
		"requirement": content,
	}
	return a.buildToolCall("generate_playbook", params), nil
}

func (a *Agent) handleExecuteCommand(content string) (string, error) {
	command := a.extractCommand(content)
	if command == "" {
		command = "uptime"
	}
	nodes := a.getAllNodeNames()
	params := map[string]interface{}{
		"nodes":   nodes,
		"command": command,
		"timeout": 60,
	}
	return a.buildToolCall("execute_command", params), nil
}

func (a *Agent) handleTransferFile(content string) (string, error) {
	sourceFile := a.extractFilePath(content)
	if sourceFile == "" {
		return "Please specify the file path to transfer", nil
	}
	destDir := "/tmp"
	if strings.Contains(content, "/opt") {
		destDir = "/opt"
	}
	nodes := a.getAllNodeNames()
	mode := "direct"
	if len(nodes) >= 5 {
		mode = "diffusion"
	}
	params := map[string]interface{}{
		"source_file": sourceFile,
		"nodes":       nodes,
		"dest_dir":    destDir,
		"mode":        mode,
	}
	return a.buildToolCall("transfer_file", params), nil
}

func (a *Agent) getAllNodeNames() []string {
	nodes := a.nodeMgr.List()
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		names = append(names, n.Name)
	}
	return names
}

func (a *Agent) handleGeneralQuestion(content string) (string, error) {
	return fmt.Sprintf("I understand your requirement: %s\n\nI can help you with:\n- Query node information\n- Execute batch commands\n- Generate and execute Ansible playbooks\n- Transfer files\n\nPlease provide more specific operation instructions.", content), nil
}

func (a *Agent) extractCommand(content string) string {
	keywords := []string{"uptime", "df -h", "free -m", "ps aux", "netstat", "systemctl", "service", "ls", "cat", "grep"}
	contentLower := strings.ToLower(content)

	for _, kw := range keywords {
		if strings.Contains(contentLower, kw) {
			return kw
		}
	}

	if strings.Contains(contentLower, "status") {
		return "uptime && df -h"
	}
	if strings.Contains(contentLower, "memory") {
		return "free -m"
	}
	if strings.Contains(contentLower, "disk") {
		return "df -h"
	}
	if strings.Contains(contentLower, "process") {
		return "ps aux"
	}

	return ""
}

func (a *Agent) extractFilePath(content string) string {
	words := strings.Fields(content)
	for _, word := range words {
		if strings.HasPrefix(word, "/") || strings.HasSuffix(word, ".tar") || strings.HasSuffix(word, ".gz") || strings.HasSuffix(word, ".zip") {
			return word
		}
	}
	return ""
}

func (a *Agent) nodesToJSON(nodes []*model.Node) string {
	type NodeInfo struct {
		Name    string   `json:"name"`
		Address string   `json:"address"`
		Port    int      `json:"port"`
		Status  string   `json:"status"`
		Groups  []string `json:"groups"`
	}

	info := make([]NodeInfo, len(nodes))
	for i, n := range nodes {
		info[i] = NodeInfo{
			Name:    n.Name,
			Address: n.Address,
			Port:    n.Port,
			Status:  string(n.Status),
			Groups:  n.Groups,
		}
	}

	data, _ := json.MarshalIndent(info, "", "  ")
	return string(data)
}

func (a *Agent) stringsToJSON(strs []string) string {
	data, _ := json.Marshal(strs)
	return string(data)
}

type PendingContext struct {
	State        string
	Action       string
	LastToolName string
	LastParams   map[string]interface{}
	Question     string
}

type Session struct {
	agent          *Agent
	messages       []Message
	history        []string
	createdAt      time.Time
	lastActive     time.Time
	OnProgress     ProgressCallback
	pendingContext *PendingContext
}

func NewSession(agent *Agent) *Session {
	return &Session{
		agent:     agent,
		messages:  make([]Message, 0),
		history:   make([]string, 0),
		createdAt: time.Now(),
	}
}

var affirmativeReplies = map[string]bool{
	"是": true, "是的": true, "对": true, "对的": true,
	"好": true, "好的": true, "可以": true, "行": true,
	"yes": true, "ok": true, "okay": true, "y": true,
	"嗯": true, "确认": true,
}

var questionKeywords = []string{"是否", "要不要", "需要我", "要我", "要不要我"}

func (s *Session) Send(ctx context.Context, userInput string) (string, error) {
	s.lastActive = time.Now()
	s.history = append(s.history, fmt.Sprintf("User: %s", userInput))

	if s.pendingContext != nil && s.pendingContext.State == "awaiting_confirmation" {
		lowerInput := strings.TrimSpace(strings.ToLower(userInput))
		if affirmativeReplies[lowerInput] {
			pendingMsg := fmt.Sprintf(
				"[系统提示] 用户刚才回复了「是」，确认了你的问题：「%s」。请继续执行之前的操作。",
				s.pendingContext.Question,
			)

			s.messages = append(s.messages, Message{Role: "system", Content: pendingMsg})
			s.messages = append(s.messages, Message{Role: "user", Content: fmt.Sprintf("好的，请继续：%s", s.pendingContext.Action)})
		}
		s.pendingContext = nil
	} else {
		s.messages = append(s.messages, Message{Role: "user", Content: userInput})
	}

	var response string
	var err error
	if len(s.messages) > 0 {
		var updatedMessages []Message
		updatedMessages, response, err = s.agent.ProcessWithContext(ctx, s.messages, s.OnProgress)
		if err == nil {
			s.messages = updatedMessages
		}
	} else {
		response, err = s.agent.Process(ctx, userInput, s.OnProgress)
	}
	if err != nil {
		return "", err
	}

	s.maybeSetPendingContext(response)

	s.history = append(s.history, fmt.Sprintf("Assistant: %s", response))
	return response, nil
}

func (s *Session) maybeSetPendingContext(response string) {
	trimmed := strings.TrimSpace(response)
	if strings.HasSuffix(trimmed, "？") || strings.HasSuffix(trimmed, "?") {
		for _, kw := range questionKeywords {
			if strings.Contains(trimmed, kw) {
				s.pendingContext = &PendingContext{
					State:    "awaiting_confirmation",
					Action:   "继续之前的查询",
					Question: trimmed,
				}
				return
			}
		}
	}
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

func (m *SessionManager) RemoveSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}

func (m *SessionManager) ListSessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
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
