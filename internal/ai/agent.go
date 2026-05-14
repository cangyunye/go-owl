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
	aitools "github.com/cangyunye/go-owl/internal/ai/tools"
	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/control/playbook"
)

type Agent struct {
	config         *Config
	nodeMgr        node.Manager
	registry       *aitools.ToolRegistry
	playbookParser *playbook.Parser
	chatModel      ChatModel
	systemPrompt   string
	mu             sync.RWMutex
}

type ChatModel interface {
	Generate(ctx context.Context, messages []Message) (string, error)
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatModelFunc func(ctx context.Context, messages []Message) (string, error)

func (f ChatModelFunc) Generate(ctx context.Context, messages []Message) (string, error) {
	return f(ctx, messages)
}

func NewAgent(cfg *Config, nodeMgr node.Manager) (*Agent, error) {
	registry := aitools.NewToolRegistry()
	registry.Register(aitools.NewQueryNodesTool(nodeMgr))
	registry.Register(aitools.NewExecuteCommandTool(nodeMgr))
	registry.Register(aitools.NewGeneratePlaybookTool(nodeMgr))
	registry.Register(aitools.NewTransferFileTool(nodeMgr))

	agent := &Agent{
		config:         cfg,
		nodeMgr:        nodeMgr,
		registry:       registry,
		playbookParser: playbook.NewParser(),
		systemPrompt:   aiPrompts.SystemPrompt,
	}

	if cfg.AI.APIKey != "" && cfg.AI.Model != "" {
		llmClient, err := CreateLLMClient(cfg)
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

func (a *Agent) Process(ctx context.Context, userInput string) (string, error) {
	a.mu.RLock()
	chatModel := a.chatModel
	systemPrompt := a.systemPrompt
	a.mu.RUnlock()

	nodeInfo := a.getNodeInfo()
	toolDescs := a.registry.GetToolDescriptions()

	formattedPrompt := a.formatPrompt(systemPrompt, nodeInfo, toolDescs)

	messages := []Message{
		{Role: "system", Content: formattedPrompt},
		{Role: "user", Content: userInput},
	}

	var fullResponse strings.Builder
	maxTurns := 10

	for turn := 0; turn < maxTurns; turn++ {
		response, err := chatModel.Generate(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("AI 调用失败: %w", err)
		}

		fullResponse.WriteString(response)

		toolCalls := a.parseToolCalls(response)
		if len(toolCalls) == 0 {
			break
		}

		messages = append(messages, Message{Role: "assistant", Content: response})

		for _, call := range toolCalls {
			result, err := a.executeToolCall(ctx, call)
			if err != nil {
				result = fmt.Sprintf("Tool execution failed: %v", err)
			}
			toolResult := fmt.Sprintf("\n\n[TOOL_CALL_RESULT]\n%s\n[/TOOL_CALL_RESULT]", result)
			messages = append(messages, Message{Role: "user", Content: toolResult})
		}
	}

	return fullResponse.String(), nil
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
	var calls []ToolCall

	if len(response) < 7 {
		return calls
	}

	jsonStart := strings.Index(response, "```json")
	if jsonStart == -1 {
		return calls
	}

	jsonEnd := strings.Index(response[jsonStart+7:], "```")
	if jsonEnd == -1 {
		return calls
	}

	jsonContent := strings.TrimSpace(response[jsonStart+7 : jsonStart+7+jsonEnd])

	var parsed struct {
		ToolCalls []struct {
			Name string                 `json:"name"`
			Args map[string]interface{} `json:"arguments"`
		} `json:"tool_calls"`
	}

	if err := json.Unmarshal([]byte(jsonContent), &parsed); err != nil {
		return calls
	}

	for _, tc := range parsed.ToolCalls {
		calls = append(calls, ToolCall{
			Name:      tc.Name,
			Arguments: tc.Args,
		})
	}

	return calls
}

func (a *Agent) executeToolCall(ctx context.Context, call ToolCall) (string, error) {
	tool, ok := a.registry.Get(call.Name)
	if !ok {
		return "", fmt.Errorf("未知工具: %s", call.Name)
	}

	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return "", err
	}

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
	var sb strings.Builder

	if strings.Contains(strings.ToLower(content), "nginx") || strings.Contains(strings.ToLower(content), "安装") {
		sb.WriteString("```json\n")
		sb.WriteString(`{"tool_calls": [{"name": "generate_playbook", "arguments": {"requirement": "`)
		sb.WriteString(content)
		sb.WriteString(`"}}]}`)
		sb.WriteString("\n```")
	}

	return sb.String(), nil
}

func (a *Agent) handleExecuteCommand(content string) (string, error) {
	command := a.extractCommand(content)
	if command == "" {
		command = "uptime"
	}

	nodes := a.nodeMgr.List()
	var targets []string
	for _, n := range nodes {
		targets = append(targets, n.Name)
	}

	var sb strings.Builder
	sb.WriteString("```json\n")
	sb.WriteString(fmt.Sprintf(`{"tool_calls": [{"name": "execute_command", "arguments": {"targets": %s, "command": "%s", "timeout": 60}}]}`, a.stringsToJSON(targets), command))
	sb.WriteString("\n```")

	return sb.String(), nil
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

	nodes := a.nodeMgr.List()
	var targets []string
	for _, n := range nodes {
		targets = append(targets, n.Name)
	}

	mode := "direct"
	if len(nodes) >= 5 {
		mode = "diffusion"
	}

	var sb strings.Builder
	sb.WriteString("```json\n")
	sb.WriteString(fmt.Sprintf(`{"tool_calls": [{"name": "transfer_file", "arguments": {"source_file": "%s", "targets": %s, "dest_dir": "%s", "mode": "%s"}}]}`, sourceFile, a.stringsToJSON(targets), destDir, mode))
	sb.WriteString("\n```")

	return sb.String(), nil
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

type Session struct {
	agent      *Agent
	messages   []Message
	history    []string
	createdAt  time.Time
	lastActive time.Time
}

func NewSession(agent *Agent) *Session {
	return &Session{
		agent:     agent,
		messages:  make([]Message, 0),
		history:   make([]string, 0),
		createdAt: time.Now(),
	}
}

func (s *Session) Send(ctx context.Context, userInput string) (string, error) {
	s.lastActive = time.Now()
	s.history = append(s.history, fmt.Sprintf("User: %s", userInput))

	response, err := s.agent.Process(ctx, userInput)
	if err != nil {
		return "", err
	}

	s.history = append(s.history, fmt.Sprintf("Assistant: %s", response))
	return response, nil
}

func (s *Session) GetHistory() []string {
	return s.history
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
