package ai

import (
	"strings"
)

type ResponseFormatter struct{}

func NewResponseFormatter() *ResponseFormatter {
	return &ResponseFormatter{}
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
