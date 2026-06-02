package test

import (
	"fmt"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/test/testdata"
)

func TestNodeDataSet(t *testing.T) {
	t.Logf("=== 测试数据集验证 ===")
	t.Logf("总节点数: %d", len(testdata.TestNodes))

	// 验证节点分组分布
	webNodes := testdata.GetNodesByGroup("web")
	dbNodes := testdata.GetNodesByGroup("db")
	testNodes := testdata.GetNodesByGroup("test")

	t.Logf("web组节点数: %d", len(webNodes))
	t.Logf("db组节点数: %d", len(dbNodes))
	t.Logf("test组节点数: %d", len(testNodes))

	// 验证按标签查询
	envTestNodes := testdata.GetNodesByLabel("env", "test")
	envProdNodes := testdata.GetNodesByLabel("env", "prod")
	linuxNodes := testdata.GetNodesByLabel("os", "linux")
	windowsNodes := testdata.GetNodesByLabel("os", "windows")

	t.Logf("env=test节点数: %d", len(envTestNodes))
	t.Logf("env=prod节点数: %d", len(envProdNodes))
	t.Logf("os=linux节点数: %d", len(linuxNodes))
	t.Logf("os=windows节点数: %d", len(windowsNodes))

	// 验证按状态查询
	onlineNodes := testdata.GetNodesByStatus(model.NodeStatusOnline)
	offlineNodes := testdata.GetNodesByStatus(model.NodeStatusOffline)
	unknownNodes := testdata.GetNodesByStatus(model.NodeStatusUnknown)

	t.Logf("online节点数: %d", len(onlineNodes))
	t.Logf("offline节点数: %d", len(offlineNodes))
	t.Logf("unknown节点数: %d", len(unknownNodes))

	// 打印场景摘要
	t.Logf("\n=== 测试场景摘要 ===")
	testdata.PrintScenarioSummary()
}

func TestQueryScenarios(t *testing.T) {
	t.Logf("=== 测试查询场景 ===")

	nodeScenarios := testdata.GetScenariosByAction("node")
	execScenarios := testdata.GetScenariosByAction("exec")
	playbookScenarios := testdata.GetScenariosByAction("playbook")
	fileScenarios := testdata.GetScenariosByAction("file")

	t.Logf("node类场景: %d个", len(nodeScenarios))
	t.Logf("exec类场景: %d个", len(execScenarios))
	t.Logf("playbook类场景: %d个", len(playbookScenarios))
	t.Logf("file类场景: %d个", len(fileScenarios))

	for _, scenario := range nodeScenarios {
		t.Logf("\n场景: %s", scenario.Name)
		t.Logf("  输入: %s", scenario.NaturalInput)
		t.Logf("  预期节点数: %d", len(scenario.ExpectedNodes))
	}
}

func main() {
	fmt.Println("=== 测试数据示例 ===")
	testdata.PrintScenarioSummary()

	fmt.Println("\n=== 节点数据统计 ===")
	fmt.Printf("总节点数: %d\n", len(testdata.TestNodes))
	for _, node := range testdata.TestNodes {
		fmt.Printf("  %-20s | %-15s | %-7s | groups=%v\n",
			node.Name,
			node.Address,
			node.Status,
			node.Groups)
	}
}
