package integration

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestIntegrationEnabled(t *testing.T) {
	if os.Getenv("OWL_TEST_ENABLED") != "true" {
		t.Skip("跳过集成测试（需要设置 OWL_TEST_ENABLED=true）")
	}
}

func getTestNodes() []string {
	nodes := os.Getenv("OWL_TEST_NODES")
	if nodes == "" {
		return []string{"self-test-1"}
	}
	return strings.Split(nodes, ",")
}

func TestExecRunBasicCommand(t *testing.T) {
	TestIntegrationEnabled(t)

	nodes := getTestNodes()
	node := nodes[0]

	cmd := exec.Command("owl", "exec", "run", "echo hello", "--nodes", node)
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("命令执行失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "hello") {
		t.Errorf("输出不包含预期内容\nExpected: hello\nGot: %s", outputStr)
	}
}

func TestExecRunUptime(t *testing.T) {
	TestIntegrationEnabled(t)

	nodes := getTestNodes()
	node := nodes[0]

	cmd := exec.Command("owl", "exec", "run", "uptime", "--nodes", node)
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("命令执行失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "load") {
		t.Errorf("uptime 输出不包含 load 信息\nGot: %s", outputStr)
	}
}

func TestExecRunWithSerialMode(t *testing.T) {
	TestIntegrationEnabled(t)

	nodes := getTestNodes()
	if len(nodes) < 2 {
		t.Skip("需要至少 2 个测试节点")
	}

	node := nodes[0]
	cmd := exec.Command("owl", "exec", "run", "echo serial", "--nodes", node, "--serial")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("串行模式执行失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "serial") {
		t.Errorf("输出不包含预期内容\nExpected: serial\nGot: %s", outputStr)
	}
}

func TestExecRunMultipleNodes(t *testing.T) {
	TestIntegrationEnabled(t)

	nodes := getTestNodes()
	if len(nodes) < 2 {
		t.Skip("需要至少 2 个测试节点")
	}

	nodesStr := strings.Join(nodes, ",")
	cmd := exec.Command("owl", "exec", "run", "echo $HOSTNAME", "--nodes", nodesStr)
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("多节点执行失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	successCount := strings.Count(outputStr, "✅")
	if successCount < 2 {
		t.Errorf("多节点执行未全部成功\nGot: %s", outputStr)
	}
}

func TestExecRunTimeout(t *testing.T) {
	TestIntegrationEnabled(t)

	nodes := getTestNodes()
	node := nodes[0]

	cmd := exec.Command("owl", "exec", "run", "sleep 2", "--nodes", node, "--command-timeout", "5s")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("超时命令执行失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "成功") {
		t.Errorf("超时命令未成功执行\nGot: %s", outputStr)
	}
}

func TestNodeList(t *testing.T) {
	TestIntegrationEnabled(t)

	cmd := exec.Command("owl", "node", "list")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("获取节点列表失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	nodes := getTestNodes()
	for _, node := range nodes {
		if !strings.Contains(outputStr, node) {
			t.Errorf("节点列表中未找到节点 %s\nGot: %s", node, outputStr)
		}
	}
}

func TestHistoryCommand(t *testing.T) {
	TestIntegrationEnabled(t)

	cmd := exec.Command("owl", "history")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("获取历史记录失败: %v\nOutput: %s", err, output)
		return
	}

	t.Logf("历史记录查询成功: %s", output)
}

func TestSettingsShow(t *testing.T) {
	TestIntegrationEnabled(t)

	cmd := exec.Command("owl", "settings", "show")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("获取设置失败: %v\nOutput: %s", err, output)
		return
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Output:") {
		t.Errorf("设置输出格式不正确\nGot: %s", outputStr)
	}
}

func TestHelpCommand(t *testing.T) {
	TestIntegrationEnabled(t)

	tests := []struct {
		name string
		args []string
	}{
		{"exec help", []string{"exec", "--help"}},
		{"node help", []string{"node", "--help"}},
		{"history help", []string{"history", "--help"}},
		{"settings help", []string{"settings", "--help"}},
		{"playbook help", []string{"playbook", "--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("owl", tt.args...)
			output, err := cmd.CombinedOutput()

			if err != nil {
				t.Errorf("帮助命令失败: %v\nOutput: %s", err, output)
				return
			}

			outputStr := string(output)
			if len(outputStr) < 10 {
				t.Errorf("帮助输出过短\nGot: %s", outputStr)
			}
		})
	}
}
