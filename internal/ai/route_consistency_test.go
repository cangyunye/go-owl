package ai

import (
	"context"
	"regexp"
	"strings"
	"testing"

	aiPrompts "github.com/cangyunye/go-owl/internal/ai/prompts"
)

// TestRegistryCoveredByPrompts 保证每个注册工具都被某个 system prompt 提及，
// 防止 LLM 不知道工具而无法调用（单一事实来源漂移检测）。
func TestRegistryCoveredByPrompts(t *testing.T) {
	agent := newTestAgentForRoute(nil)
	tools := agent.registry.ListAll()

	allPromptText := aiPrompts.RouterPrompt
	// 遍历所有导出的 SystemPrompt 常量文本
	for _, name := range []string{
		"ExecRunSystemPrompt", "ExecScriptSystemPrompt",
		"NodeListSystemPrompt", "NodeAddSystemPrompt", "NodeUpdateSystemPrompt",
		"NodeRemoveSystemPrompt", "NodeStatusSystemPrompt", "NodeGroupsSystemPrompt",
		"NodeLabelsSystemPrompt", "NodeImportSystemPrompt", "NodePingSystemPrompt",
		"NodeCheckSystemPrompt", "FileSystemPrompt", "PlaybookListSystemPrompt",
		"PlaybookRunSystemPrompt", "PlaybookValidateSystemPrompt",
		"GenericToolSystemPrompt", "PlaybookPrompt", "ExecuteCommandPrompt",
		"ExecuteScriptPrompt", "TransferPrompt",
	} {
		if v, ok := promptTextByName(name); ok {
			allPromptText += v
		}
	}

	// generate_playbook 由本地意图分类器直接调用（IntentGeneratePlaybook），
	// 不依赖 LLM 引导；LLM 侧对应的是 playbook_generate。
	llmBypassedTools := map[string]bool{"generate_playbook": true}

	for _, tool := range tools {
		if llmBypassedTools[tool.Name()] {
			continue
		}
		if !strings.Contains(allPromptText, tool.Name()) {
			t.Errorf("tool %q not mentioned in any system prompt; LLM will never call it", tool.Name())
		}
	}
}

func promptTextByName(name string) (string, bool) {
	switch name {
	case "ExecRunSystemPrompt":
		return aiPrompts.ExecRunSystemPrompt, true
	case "ExecScriptSystemPrompt":
		return aiPrompts.ExecScriptSystemPrompt, true
	case "NodeListSystemPrompt":
		return aiPrompts.NodeListSystemPrompt, true
	case "NodeAddSystemPrompt":
		return aiPrompts.NodeAddSystemPrompt, true
	case "NodeUpdateSystemPrompt":
		return aiPrompts.NodeUpdateSystemPrompt, true
	case "NodeRemoveSystemPrompt":
		return aiPrompts.NodeRemoveSystemPrompt, true
	case "NodeStatusSystemPrompt":
		return aiPrompts.NodeStatusSystemPrompt, true
	case "NodeGroupsSystemPrompt":
		return aiPrompts.NodeGroupsSystemPrompt, true
	case "NodeLabelsSystemPrompt":
		return aiPrompts.NodeLabelsSystemPrompt, true
	case "NodeImportSystemPrompt":
		return aiPrompts.NodeImportSystemPrompt, true
	case "NodePingSystemPrompt":
		return aiPrompts.NodePingSystemPrompt, true
	case "NodeCheckSystemPrompt":
		return aiPrompts.NodeCheckSystemPrompt, true
	case "FileSystemPrompt":
		return aiPrompts.FileSystemPrompt, true
	case "PlaybookListSystemPrompt":
		return aiPrompts.PlaybookListSystemPrompt, true
	case "PlaybookRunSystemPrompt":
		return aiPrompts.PlaybookRunSystemPrompt, true
	case "PlaybookValidateSystemPrompt":
		return aiPrompts.PlaybookValidateSystemPrompt, true
	case "GenericToolSystemPrompt":
		return aiPrompts.GenericToolSystemPrompt, true
	case "PlaybookPrompt":
		return aiPrompts.PlaybookPrompt, true
	case "ExecuteCommandPrompt":
		return aiPrompts.ExecuteCommandPrompt, true
	case "ExecuteScriptPrompt":
		return aiPrompts.ExecuteScriptPrompt, true
	case "TransferPrompt":
		return aiPrompts.TransferPrompt, true
	}
	return "", false
}

// TestRouterLabelsHavePrompt 检查 RouterPrompt 中声明的类别都能路由到
// 具体的 group prompt 或 generic 兜底。
func TestRouterLabelsHavePrompt(t *testing.T) {
	re := regexp.MustCompile(`(?m)^([a-z_]+) - `)
	matches := re.FindAllStringSubmatch(aiPrompts.RouterPrompt, -1)
	if len(matches) < 20 {
		t.Fatalf("expected >=20 route labels in RouterPrompt, got %d", len(matches))
	}
	seen := map[string]bool{}
	for _, m := range matches {
		label := m[1]
		if seen[label] {
			continue
		}
		seen[label] = true
		if unsupportedRouteLabels[label] {
			continue
		}
		if _, ok := groupPrompts[label]; !ok {
			t.Errorf("route label %q has no group prompt", label)
		}
	}
}

// TestUnsupportedRouteLabels 验证豁免命令返回固定文案。
func TestUnsupportedRouteLabels(t *testing.T) {
	for label := range unsupportedRouteLabels {
		agent := newTestAgentForRoute([]string{label})
		resp, err := agent.Process(context.Background(), "session 相关操作", nil)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}
		if resp != "该功能不支持 AI 操作" {
			t.Errorf("expected unsupported message for %s, got %q", label, resp)
		}
	}
}

// TestNoFreeTextOnTurnZero 验证 turn 0 无工具调用时 LLM 自由文本不透出。
func TestNoFreeTextOnTurnZero(t *testing.T) {
	agent := newTestAgentForRoute([]string{"exec", "好的，我来帮您执行这个操作！"})
	resp, err := agent.Process(context.Background(), "执行命令", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp != "我不确定您要做什么" {
		t.Errorf("expected rejection for free text, got %q", resp)
	}
}

// TestNoFreeTextInProcessWithContext 验证多轮场景同样收口。
func TestNoFreeTextInProcessWithContext(t *testing.T) {
	agent := newTestAgentForRoute([]string{"好的，我来帮您！"})
	msgs := []Message{{Role: "system", Content: "system"}, {Role: "user", Content: "你好"}}
	_, resp, err := agent.ProcessWithContext(context.Background(), msgs, nil)
	if err != nil {
		t.Fatalf("ProcessWithContext failed: %v", err)
	}
	if resp != "我不确定您要做什么" {
		t.Errorf("expected rejection for free text, got %q", resp)
	}
}

// TestGenericRouteExecutesTool 验证新类别（无专属 prompt）走 generic 路由后仍能执行工具。
func TestGenericRouteExecutesTool(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"settings_show",
		"```json\n{\"tool_calls\":[{\"name\":\"async_list\",\"arguments\":{}}]}\n```",
	})
	agent.SetConfirmGate(nil)
	resp, err := agent.Process(context.Background(), "查看异步任务", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp == "我不确定您要做什么" || resp == "" {
		t.Errorf("expected tool execution result for generic route, got %q", resp)
	}
}
