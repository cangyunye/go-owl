package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func aiIntegrationEnabled(t *testing.T) {
	TestIntegrationEnabled(t)
	home, _ := os.UserHomeDir()
	configPath := home + "/.owl/config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("跳过 AI 集成测试（需要配置 ~/.owl/config.yaml 的 AI 部分）")
	}
}

type toolCallWrapper struct {
	ToolCalls []struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	} `json:"tool_calls"`
}

func extractToolCalls(output string) (*toolCallWrapper, error) {
	jsonStart := strings.Index(output, "```json")
	if jsonStart == -1 {
		return nil, nil
	}
	jsonEnd := strings.Index(output[jsonStart+7:], "```")
	if jsonEnd == -1 {
		return nil, nil
	}
	jsonContent := strings.TrimSpace(output[jsonStart+7 : jsonStart+7+jsonEnd])
	var parsed toolCallWrapper
	if err := json.Unmarshal([]byte(jsonContent), &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func TestAIRouteExec(t *testing.T) {
	aiIntegrationEnabled(t)

	nodes := getTestNodes()
	node := nodes[0]

	cmd := exec.Command("owl", "ai", "在 "+node+" 执行 echo hello")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("AI 命令失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	calls, parseErr := extractToolCalls(outputStr)
	if parseErr != nil {
		t.Errorf("解析 tool_calls JSON 失败: %v\nOutput: %s", parseErr, outputStr)
		return
	}
	if calls == nil || len(calls.ToolCalls) == 0 {
		t.Errorf("输出未包含 tool_calls JSON\nOutput: %s", outputStr)
		return
	}
	if calls.ToolCalls[0].Name != "execute_command" {
		t.Errorf("期望 execute_command，实际: %s", calls.ToolCalls[0].Name)
	}
}

func TestAIRouteNode(t *testing.T) {
	aiIntegrationEnabled(t)

	cmd := exec.Command("owl", "ai", "列出所有节点")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("AI 命令失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	calls, parseErr := extractToolCalls(outputStr)
	if parseErr != nil {
		t.Errorf("解析 tool_calls JSON 失败: %v\nOutput: %s", parseErr, outputStr)
		return
	}
	if calls == nil || len(calls.ToolCalls) == 0 {
		t.Errorf("输出未包含 tool_calls JSON\nOutput: %s", outputStr)
		return
	}
	if calls.ToolCalls[0].Name != "query_nodes" {
		t.Errorf("期望 query_nodes，实际: %s", calls.ToolCalls[0].Name)
	}
}

func TestAIRouteUncertain(t *testing.T) {
	aiIntegrationEnabled(t)

	cmd := exec.Command("owl", "ai", "随便来点什么不知所云的东西abcdefg12345")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("AI 命令失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	if strings.Contains(outputStr, `"tool_calls"`) {
		t.Logf("提示: 模糊输入仍然路由到了某个命令组\nOutput: %s", outputStr)
	}
}

func TestAIExecRunDetailFormat(t *testing.T) {
	aiIntegrationEnabled(t)

	nodes := getTestNodes()
	node := nodes[0]

	cmd := exec.Command("owl", "ai", "在 "+node+" 上以 detail 格式执行 df -h")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("AI 命令失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	calls, parseErr := extractToolCalls(outputStr)
	if parseErr != nil {
		t.Errorf("解析 tool_calls JSON 失败: %v\nOutput: %s", parseErr, outputStr)
		return
	}
	if calls == nil || len(calls.ToolCalls) == 0 {
		t.Errorf("输出未包含 tool_calls JSON\nOutput: %s", outputStr)
		return
	}

	tc := calls.ToolCalls[0]
	if tc.Name != "execute_command" {
		t.Errorf("期望 execute_command，实际: %s", tc.Name)
	}
	if format, ok := tc.Arguments["format"]; !ok || format != "detail" {
		t.Logf("提示: format 参数可能未设为 'detail'，实际: %v", tc.Arguments)
	}
}

func TestAIExecScript(t *testing.T) {
	aiIntegrationEnabled(t)

	nodes := getTestNodes()
	node := nodes[0]

	cmd := exec.Command("owl", "ai", "在 "+node+" 执行脚本 tests/testdata/scripts/test-script.sh")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("AI 命令失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	calls, parseErr := extractToolCalls(outputStr)
	if parseErr != nil {
		t.Errorf("解析 tool_calls JSON 失败: %v\nOutput: %s", parseErr, outputStr)
		return
	}
	if calls == nil || len(calls.ToolCalls) == 0 {
		t.Errorf("输出未包含 tool_calls JSON\nOutput: %s", outputStr)
		return
	}

	tc := calls.ToolCalls[0]
	if tc.Name != "execute_script" {
		t.Errorf("期望 execute_script，实际: %s", tc.Name)
	}
	if script, ok := tc.Arguments["script"]; !ok || !strings.Contains(script.(string), "test-script") {
		t.Logf("提示: script 参数可能不包含 'test-script'，实际: %v", tc.Arguments)
	}
}
