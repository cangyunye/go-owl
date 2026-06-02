package testdata

import "fmt"

// TestQueryScenario 是测试用的自然语言查询场景
type TestQueryScenario struct {
	Name           string                 // 场景名称
	NaturalInput   string                 // 用户自然语言输入
	ExpectedAction string                 // 预期的 action（node/exec/playbook/file）
	ExpectedTool   string                 // 预期调用的工具
	ExpectedParams map[string]interface{} // 预期的参数
	ExpectedNodes  []string               // 预期匹配的节点名称列表
	Description    string                 // 场景描述
}

// TestQueryScenarios 是完整的测试用自然语言查询场景列表
var TestQueryScenarios = []TestQueryScenario{
	{
		Name:           "列出所有节点",
		NaturalInput:   "列出所有节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{},
		ExpectedNodes:  []string{"web-server-01", "web-server-02", "db-primary-01", "db-replica-01", "test-node-01", "cache-node-01", "monitoring-01", "windows-test-01"},
		Description:    "基础场景：查询所有节点",
	},
	{
		Name:           "按分组查询 - web组",
		NaturalInput:   "显示web组的所有节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"group": "web"},
		ExpectedNodes:  []string{"web-server-01", "web-server-02"},
		Description:    "按 group 参数查询，分组为 web",
	},
	{
		Name:           "按标签查询 - env=test",
		NaturalInput:   "查找环境为test的节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"labels": map[string]string{"env": "test"}},
		ExpectedNodes:  []string{"test-node-01", "windows-test-01"},
		Description:    "按 labels 参数查询，标签为 env:test",
	},
	{
		Name:           "按状态查询 - online",
		NaturalInput:   "显示所有在线的节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"status": "online"},
		ExpectedNodes:  []string{"web-server-01", "web-server-02", "db-primary-01", "test-node-01", "cache-node-01", "monitoring-01"},
		Description:    "按 status 参数查询，状态为 online",
	},
	{
		Name:           "按状态查询 - offline",
		NaturalInput:   "查找离线的节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"status": "offline"},
		ExpectedNodes:  []string{"db-replica-01"},
		Description:    "按 status 参数查询，状态为 offline",
	},
	{
		Name:           "名称模糊搜索 - web",
		NaturalInput:   "找名称包含web的节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"search": "web"},
		ExpectedNodes:  []string{"web-server-01", "web-server-02"},
		Description:    "使用 search 参数进行模糊搜索",
	},
	{
		Name:           "名称模糊搜索 - db",
		NaturalInput:   "找数据库相关的节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"search": "db"},
		ExpectedNodes:  []string{"db-primary-01", "db-replica-01"},
		Description:    "使用 search 参数搜索 db",
	},
	{
		Name:           "组合查询 - web组 + online",
		NaturalInput:   "显示web组在线的节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"group": "web", "status": "online"},
		ExpectedNodes:  []string{"web-server-01", "web-server-02"},
		Description:    "同时使用 group 和 status 参数组合查询",
	},
	{
		Name:           "组合查询 - env=prod + 在线",
		NaturalInput:   "prod环境在线的节点有哪些",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"labels": map[string]string{"env": "prod"}, "status": "online"},
		ExpectedNodes:  []string{"web-server-01", "web-server-02", "db-primary-01", "cache-node-01", "monitoring-01"},
		Description:    "同时使用 labels 和 status 参数组合查询",
	},
	{
		Name:           "查找特定节点 - web-server-01",
		NaturalInput:   "找到web-server-01这个节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"search": "web-server-01"},
		ExpectedNodes:  []string{"web-server-01"},
		Description:    "通过搜索精确匹配单个节点",
	},
	{
		Name:           "查找Windows节点",
		NaturalInput:   "有没有Windows节点？",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"labels": map[string]string{"os": "windows"}},
		ExpectedNodes:  []string{"windows-test-01"},
		Description:    "按 os=windows 标签查询",
	},
	{
		Name:           "查找美国东部区域的节点",
		NaturalInput:   "查找us-east区域的节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"labels": map[string]string{"region": "us-east"}},
		ExpectedNodes:  []string{"web-server-02", "db-replica-01", "windows-test-01"},
		Description:    "按 region=us-east 标签查询",
	},
	{
		Name:           "JSON格式输出",
		NaturalInput:   "以JSON格式列出所有节点",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"format": "json"},
		ExpectedNodes:  []string{"web-server-01", "web-server-02", "db-primary-01", "db-replica-01", "test-node-01", "cache-node-01", "monitoring-01", "windows-test-01"},
		Description:    "使用 format=json 参数",
	},
	{
		Name:           "查询db分组节点详情",
		NaturalInput:   "查看db分组的节点详情",
		ExpectedAction: "node",
		ExpectedTool:   "query_nodes",
		ExpectedParams: map[string]interface{}{"group": "db", "format": "summary"},
		ExpectedNodes:  []string{"db-primary-01", "db-replica-01"},
		Description:    "使用 group=db 和 format=summary",
	},
	{
		Name:           "执行命令场景",
		NaturalInput:   "在web-server-01上执行uptime命令",
		ExpectedAction: "exec",
		ExpectedTool:   "execute_command",
		ExpectedParams: map[string]interface{}{"command": "uptime", "nodes": []interface{}{"web-server-01"}},
		ExpectedNodes:  []string{"web-server-01"},
		Description:    "在特定节点执行命令的场景",
	},
	{
		Name:           "在测试环境执行命令",
		NaturalInput:   "在test环境的节点上运行df -h",
		ExpectedAction: "exec",
		ExpectedTool:   "execute_command",
		ExpectedParams: map[string]interface{}{"command": "df -h", "nodes": []interface{}{"test-node-01", "windows-test-01"}},
		ExpectedNodes:  []string{"test-node-01", "windows-test-01"},
		Description:    "需要先查询目标节点再执行命令的场景",
	},
	{
		Name:           "Playbook场景",
		NaturalInput:   "在web组的节点上安装nginx",
		ExpectedAction: "playbook",
		ExpectedTool:   "generate_playbook",
		ExpectedParams: map[string]interface{}{"requirement": "在web组的节点上安装nginx"},
		ExpectedNodes:  []string{"web-server-01", "web-server-02"},
		Description:    "需要查询目标节点后生成playbook的场景",
	},
	{
		Name:           "文件传输场景",
		NaturalInput:   "把配置文件上传到所有缓存节点",
		ExpectedAction: "file",
		ExpectedTool:   "transfer_file",
		ExpectedParams: map[string]interface{}{"nodes": []interface{}{"cache-node-01"}},
		ExpectedNodes:  []string{"cache-node-01"},
		Description:    "需要先查询目标节点再传输文件的场景",
	},
}

// PrintScenarioSummary 打印测试场景摘要
func PrintScenarioSummary() {
	fmt.Printf("=== 测试场景摘要 (%d个) ===\n", len(TestQueryScenarios))
	for i, scenario := range TestQueryScenarios {
		fmt.Printf("\n%d. %s\n", i+1, scenario.Name)
		fmt.Printf("   输入: %s\n", scenario.NaturalInput)
		fmt.Printf("   预期: action=%s, tool=%s\n", scenario.ExpectedAction, scenario.ExpectedTool)
		fmt.Printf("   描述: %s\n", scenario.Description)
	}
}

// GetScenariosByAction 按action类型获取场景
func GetScenariosByAction(action string) []TestQueryScenario {
	result := make([]TestQueryScenario, 0)
	for _, s := range TestQueryScenarios {
		if s.ExpectedAction == action {
			result = append(result, s)
		}
	}
	return result
}
