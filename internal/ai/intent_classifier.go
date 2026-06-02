package ai

import (
	"strings"
)

type IntentType string

const (
	IntentQueryNodes       IntentType = "query_nodes"
	IntentExecuteCmd       IntentType = "execute_command"
	IntentExecuteScript    IntentType = "execute_script"
	IntentGeneratePlaybook IntentType = "generate_playbook"
	IntentTransferFile     IntentType = "transfer_file"
	IntentUncertain        IntentType = "uncertain"
)

type IntentResult struct {
	Type       IntentType
	Confidence int // 0-100
	RawInput   string
}

type IntentClassifier struct {
	Keywords map[IntentType][]string
}

func NewIntentClassifier() *IntentClassifier {
	return &IntentClassifier{
		Keywords: map[IntentType][]string{
			IntentQueryNodes: {
				"查询", "查看", "列出", "list", "show", "query",
				"节点", "node", "nodes", "主机", "server", "servers",
				"有多少", "多少个", "状态", "status",
				"有什么", "有哪些", "哪些", "什么",
			},
			IntentExecuteCmd: {
				"执行", "运行", "命令", "execute", "run", "command", "shell",
				"在...上", "运行...命令",
				"uptime", "df", "free", "ps", "systemctl",
			},
			IntentExecuteScript: {
				"脚本", "script", ".sh", ".py", ".bash",
				"脚本文件", "执行脚本", "运行脚本",
				"inline", "行内执行",
			},
			IntentGeneratePlaybook: {
				"生成", "创建", "剧本", "playbook",
				"安装", "部署", "install", "deploy",
				"nginx", "apache", "mysql", "redis", "docker",
				"重启", "restart", "启动", "start", "停止", "stop",
			},
			IntentTransferFile: {
				"传输", "上传", "下载", "文件",
				"transfer", "upload", "download", "file",
				"传到", "复制到", "拷贝", "copy",
				".tar", ".gz", ".zip", ".tgz",
			},
		},
	}
}

func (c *IntentClassifier) Classify(input string) *IntentResult {
	lowerInput := strings.ToLower(input)
	trimmed := strings.TrimSpace(lowerInput)

	if len(trimmed) == 0 {
		return &IntentResult{
			Type:       IntentUncertain,
			Confidence: 0,
			RawInput:   input,
		}
	}

	scores := make(map[IntentType]int)

	for intent, keywords := range c.Keywords {
		for _, keyword := range keywords {
			if strings.Contains(lowerInput, keyword) {
				scores[intent]++
			}
		}
	}

	if c.isPathOrFileTransfer(input) {
		scores[IntentTransferFile] += 5
	}

	if c.isInstallOrDeploy(input) {
		scores[IntentGeneratePlaybook] += 5
	}

	if c.isDirectCommand(input) {
		scores[IntentExecuteCmd] += 5
	}

	if c.isScriptFile(input) {
		scores[IntentExecuteScript] += 5
	}

	maxScore := 0
	result := &IntentResult{
		Type:       IntentUncertain,
		Confidence: 0,
		RawInput:   input,
	}

	for intent, score := range scores {
		if score > maxScore {
			maxScore = score
			result.Type = intent
			result.Confidence = score * 10 // scale to 0-100
		}
	}

	if result.Confidence == 0 {
		result.Confidence = 20
	}

	return result
}

func (c *IntentClassifier) isPathOrFileTransfer(input string) bool {
	lowerInput := strings.ToLower(input)
	if strings.HasPrefix(input, "/") ||
		strings.Contains(input, "./") ||
		strings.Contains(lowerInput, ".tar") ||
		strings.Contains(lowerInput, ".gz") ||
		strings.Contains(lowerInput, ".zip") ||
		strings.Contains(lowerInput, ".tgz") {
		return true
	}
	return false
}

func (c *IntentClassifier) isInstallOrDeploy(input string) bool {
	lowerInput := strings.ToLower(input)
	installKeywords := []string{"install", "安装", "deploy", "部署", "setup"}
	for _, kw := range installKeywords {
		if strings.Contains(lowerInput, kw) {
			return true
		}
	}
	return false
}

func (c *IntentClassifier) isDirectCommand(input string) bool {
	commonCommands := []string{
		"uptime", "df -h", "free -m", "ps aux", "systemctl", "service",
		"ls", "cat", "grep", "netstat", "ss", "curl", "wget",
	}
	for _, cmd := range commonCommands {
		if strings.Contains(input, cmd) {
			return true
		}
	}
	return false
}

func (c *IntentClassifier) isScriptFile(input string) bool {
	lowerInput := strings.ToLower(input)
	return strings.Contains(lowerInput, ".sh") ||
		strings.Contains(lowerInput, ".py") ||
		strings.Contains(lowerInput, ".bash") ||
		strings.Contains(lowerInput, "脚本") ||
		strings.Contains(lowerInput, "script") ||
		strings.Contains(lowerInput, "inline")
}

func GetIntentDescription(intent IntentType) string {
	switch intent {
	case IntentQueryNodes:
		return "查询节点信息"
	case IntentExecuteCmd:
		return "执行命令"
	case IntentExecuteScript:
		return "执行脚本"
	case IntentGeneratePlaybook:
		return "生成并执行剧本"
	case IntentTransferFile:
		return "传输文件"
	default:
		return "无法确定"
	}
}
