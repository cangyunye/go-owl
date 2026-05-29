package ai

import (
	"fmt"
	"strings"
)

type ResponseFormatter struct{}

func NewResponseFormatter() *ResponseFormatter {
	return &ResponseFormatter{}
}

func (f *ResponseFormatter) FormatConfirmation(intent IntentType, params map[string]interface{}) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("检测到您要执行操作：%s\n", GetIntentDescription(intent)))
	sb.WriteString(strings.Repeat("-", 60))
	sb.WriteString("\n")

	switch intent {
	case IntentQueryNodes:
		f.formatQueryNodesConfirmation(&sb, params)
	case IntentExecuteCmd:
		f.formatExecuteCmdConfirmation(&sb, params)
	case IntentGeneratePlaybook:
		f.formatPlaybookConfirmation(&sb, params)
	case IntentTransferFile:
		f.formatTransferConfirmation(&sb, params)
	}

	sb.WriteString("\n")
	sb.WriteString("是否继续？(Y/n): ")

	return sb.String()
}

func (f *ResponseFormatter) formatQueryNodesConfirmation(sb *strings.Builder, params map[string]interface{}) {
	sb.WriteString("  意图：查询节点信息\n")

	if group, ok := params["group"].(string); ok && group != "" {
		sb.WriteString(fmt.Sprintf("  筛选条件：分组: %s\n", group))
	}
	if labels, ok := params["labels"].(map[string]interface{}); ok && len(labels) > 0 {
		labelStr := f.formatLabels(labels)
		sb.WriteString(fmt.Sprintf("  筛选条件：标签: %s\n", labelStr))
	}
	if status, ok := params["status"].(string); ok && status != "" {
		sb.WriteString(fmt.Sprintf("  筛选条件：状态: %s\n", status))
	}
	if format, ok := params["format"].(string); ok && format != "" {
		sb.WriteString(fmt.Sprintf("  输出格式：%s\n", format))
	} else {
		sb.WriteString("  输出格式：table（默认）\n")
	}
}

func (f *ResponseFormatter) formatExecuteCmdConfirmation(sb *strings.Builder, params map[string]interface{}) {
	sb.WriteString("  意图：执行命令\n")

	if nodes, ok := params["nodes"].([]interface{}); ok {
		nodeList := f.convertToStringSlice(nodes)
		sb.WriteString(fmt.Sprintf("  目标节点：%s\n", f.formatList(nodeList)))
	}
	if command, ok := params["command"].(string); ok {
		sb.WriteString(fmt.Sprintf("  命令：%s\n", command))
	}
	if timeout, ok := params["timeout"].(float64); ok {
		sb.WriteString(fmt.Sprintf("  超时：%ds\n", int(timeout)))
	} else if timeout, ok := params["timeout"].(int); ok {
		sb.WriteString(fmt.Sprintf("  超时：%ds\n", timeout))
	} else {
		sb.WriteString("  超时：60s（默认）\n")
	}
}

func (f *ResponseFormatter) formatPlaybookConfirmation(sb *strings.Builder, params map[string]interface{}) {
	sb.WriteString("  意图：生成并执行剧本\n")

	if requirement, ok := params["requirement"].(string); ok {
		sb.WriteString(fmt.Sprintf("  需求：%s\n", requirement))
	}
	if vars, ok := params["vars"].(map[string]interface{}); ok && len(vars) > 0 {
		sb.WriteString(fmt.Sprintf("  自定义变量：%s\n", f.formatLabels(vars)))
	}
}

func (f *ResponseFormatter) formatTransferConfirmation(sb *strings.Builder, params map[string]interface{}) {
	sb.WriteString("  意图：传输文件\n")

	if sourceFile, ok := params["source_file"].(string); ok {
		sb.WriteString(fmt.Sprintf("  源文件：%s\n", sourceFile))
	}
	if nodes, ok := params["nodes"].([]interface{}); ok {
		nodeList := f.convertToStringSlice(nodes)
		sb.WriteString(fmt.Sprintf("  目标节点：%s\n", f.formatList(nodeList)))
	}
	if destDir, ok := params["dest_dir"].(string); ok {
		sb.WriteString(fmt.Sprintf("  目标目录：%s\n", destDir))
	}
	if mode, ok := params["mode"].(string); ok {
		sb.WriteString(fmt.Sprintf("  传输模式：%s\n", mode))
	} else {
		sb.WriteString("  传输模式：auto（自动）\n")
	}
	if permission, ok := params["permission"].(string); ok {
		sb.WriteString(fmt.Sprintf("  文件权限：%s\n", permission))
	} else {
		sb.WriteString("  文件权限：0644（默认）\n")
	}
}

func (f *ResponseFormatter) FormatUncertainHelp() string {
	var sb strings.Builder
	sb.WriteString("抱歉，我无法确定您要执行的具体操作。\n\n")
	sb.WriteString("我可以帮助您：\n\n")
	sb.WriteString("  1. 查询节点信息 - 查看节点状态、分组、标签\n")
	sb.WriteString("  2. 执行命令 - 在指定节点上运行 shell 命令\n")
	sb.WriteString("  3. 生成并执行剧本 - 自动化部署操作\n")
	sb.WriteString("  4. 传输文件 - 向节点分发文件\n\n")
	sb.WriteString("请告诉我您具体要做什么？\n\n")
	sb.WriteString("例如：\n")
	sb.WriteString("  - \"列出所有在线节点\"\n")
	sb.WriteString("  - \"在 web 节点上执行 uptime\"\n")
	sb.WriteString("  - \"安装 nginx\"\n")
	sb.WriteString("  - \"把 app.tar.gz 传到所有节点\"\n")
	return sb.String()
}

func (f *ResponseFormatter) FormatError(err error) string {
	return fmt.Sprintf("错误：%v", err)
}

func (f *ResponseFormatter) formatLabels(labels map[string]interface{}) string {
	if len(labels) == 0 {
		return ""
	}
	var parts []string
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, ", ")
}

func (f *ResponseFormatter) formatList(list []string) string {
	if len(list) == 0 {
		return "[]"
	}
	if len(list) <= 5 {
		return fmt.Sprintf("[%s]", strings.Join(list, ", "))
	}
	return fmt.Sprintf("[%s, ...] (%d 个节点)", strings.Join(list[:3], ", "), len(list))
}

func (f *ResponseFormatter) convertToStringSlice(slice []interface{}) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
