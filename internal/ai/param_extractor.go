package ai

import (
	"net"
	"regexp"
	"strings"
)

type ParamExtractor struct {
	nodeNames []string
	nodeAddrs []string
}

func NewParamExtractor(nodeNames []string) *ParamExtractor {
	return &ParamExtractor{
		nodeNames: nodeNames,
	}
}

func (e *ParamExtractor) SetNodeAddresses(addrs []string) {
	e.nodeAddrs = addrs
}

func (e *ParamExtractor) ExtractParams(intent IntentType, input string) map[string]interface{} {
	params := make(map[string]interface{})

	switch intent {
	case IntentQueryNodes:
		e.extractQueryNodesParams(input, params)
	case IntentExecuteCmd:
		e.extractExecuteCmdParams(input, params)
	case IntentExecuteScript:
		e.extractExecuteScriptParams(input, params)
	case IntentGeneratePlaybook:
		e.extractPlaybookParams(input, params)
	case IntentTransferFile:
		e.extractTransferParams(input, params)
	}

	return params
}

func (e *ParamExtractor) extractQueryNodesParams(input string, params map[string]interface{}) {
	lowerInput := strings.ToLower(input)

	if strings.Contains(lowerInput, "online") {
		params["status"] = "online"
	}
	if strings.Contains(lowerInput, "offline") {
		params["status"] = "offline"
	}
	if strings.Contains(lowerInput, "json") {
		params["format"] = "json"
	}
	if strings.Contains(lowerInput, "summary") {
		params["format"] = "summary"
	}

	e.extractLabelFilters(input, params)
	e.extractOwnerFilter(input, params)
	e.extractEnvFilter(input, params)
}

func (e *ParamExtractor) extractLabelFilters(input string, params map[string]interface{}) {
	lowerInput := strings.ToLower(input)

	if strings.Contains(lowerInput, "标签") {
		idx := strings.Index(lowerInput, "标签")
		labelPart := input[idx+4:]
		labelPart = strings.TrimSpace(labelPart)

		if strings.Contains(labelPart, "=") {
			parts := strings.SplitN(labelPart, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" && value != "" {
				if labels, ok := params["labels"].(map[string]interface{}); ok {
					labels[key] = value
				} else {
					params["labels"] = map[string]interface{}{key: value}
				}
			}
		} else if labelPart != "" {
			params["search"] = labelPart
		}
	}
}

func (e *ParamExtractor) extractOwnerFilter(input string, params map[string]interface{}) {
	lowerInput := strings.ToLower(input)

	if strings.Contains(lowerInput, "使用人") ||
		strings.Contains(lowerInput, "负责人") ||
		strings.Contains(lowerInput, "拥有者") ||
		strings.Contains(lowerInput, "owner") {

		var ownerValue string
		if strings.Contains(lowerInput, "使用人") {
			idx := strings.Index(lowerInput, "使用人")
			ownerValue = strings.TrimSpace(input[idx+4:])
		} else if strings.Contains(lowerInput, "负责人") {
			idx := strings.Index(lowerInput, "负责人")
			ownerValue = strings.TrimSpace(input[idx+4:])
		} else if strings.Contains(lowerInput, "拥有者") {
			idx := strings.Index(lowerInput, "拥有者")
			ownerValue = strings.TrimSpace(input[idx+4:])
		} else if strings.Contains(lowerInput, "owner=") {
			idx := strings.Index(lowerInput, "owner=")
			ownerValue = strings.TrimSpace(input[idx+6:])
		}

		if ownerValue != "" {
			params["labels"] = map[string]interface{}{"owner": ownerValue}
		}
	} else {
		name := e.extractPersonName(input)
		if name != "" {
			params["labels"] = map[string]interface{}{"owner": name}
		}
	}
}

func (e *ParamExtractor) extractEnvFilter(input string, params map[string]interface{}) {
	lowerInput := strings.ToLower(input)

	if strings.Contains(lowerInput, "环境") {
		idx := strings.Index(lowerInput, "环境")
		envPart := input[idx+4:]
		envPart = strings.TrimSpace(envPart)

		if envPart == "" {
			parts := strings.Fields(input)
			for i, part := range parts {
				if strings.ToLower(part) == "环境" && i > 0 {
					envPart = parts[i-1]
					break
				}
			}
		}

		if envPart != "" {
			params["labels"] = map[string]interface{}{"env": envPart}
		}
	}
}

func (e *ParamExtractor) extractPersonName(input string) string {
	chineseNamePattern := regexp.MustCompile(`[\u4e00-\u9fa5]{2,4}`)
	matches := chineseNamePattern.FindAllString(input, -1)

	for _, match := range matches {
		excludeWords := []string{"节点", "环境", "标签", "使用人", "负责人", "拥有者", "查询", "查看", "列出", "在线", "离线"}
		isExcluded := false
		for _, exclude := range excludeWords {
			if strings.Contains(input, exclude) && strings.Contains(input, match) {
				if strings.Index(input, exclude) < strings.Index(input, match) {
					continue
				}
			}
			if match == exclude {
				isExcluded = true
				break
			}
		}
		if !isExcluded {
			return match
		}
	}
	return ""
}

func (e *ParamExtractor) extractExecuteScriptParams(input string, params map[string]interface{}) {
	lowerInput := strings.ToLower(input)

	params["nodes"] = e.extractNodes(input)

	script := e.extractScriptPath(input)
	if script != "" {
		params["script"] = script
	} else {
		params["script"] = "./script.sh"
	}

	if strings.Contains(lowerInput, "inline") || strings.Contains(lowerInput, "行内") {
		params["inline"] = true
	}
	if strings.Contains(lowerInput, "keep") || strings.Contains(lowerInput, "保留") {
		params["keep"] = true
	}
	if strings.Contains(lowerInput, "/opt") {
		params["dest"] = "/opt"
	} else {
		params["dest"] = "/tmp"
	}
	if strings.Contains(lowerInput, "--") {
		parts := strings.SplitN(input, "--", 2)
		if len(parts) > 1 {
			params["args"] = "--" + strings.TrimSpace(parts[1])
		}
	}
}

func (e *ParamExtractor) extractScriptPath(input string) string {
	words := strings.Fields(input)
	for _, word := range words {
		if strings.HasSuffix(word, ".sh") ||
			strings.HasSuffix(word, ".py") ||
			strings.HasSuffix(word, ".bash") {
			if !strings.HasPrefix(word, "./") &&
				!strings.HasPrefix(word, "/") &&
				!strings.HasPrefix(word, "http") {
				return "./" + word
			}
			return word
		}
	}
	return ""
}

func (e *ParamExtractor) extractExecuteCmdParams(input string, params map[string]interface{}) {
	lowerInput := strings.ToLower(input)

	params["nodes"] = e.extractNodes(input)

	command := e.extractCommand(input)
	if command != "" {
		params["command"] = command
	}

	if strings.Contains(lowerInput, "timeout") {
		params["timeout"] = 60
	}
}

func (e *ParamExtractor) extractPlaybookParams(input string, params map[string]interface{}) {
	params["requirement"] = input
}

func (e *ParamExtractor) extractTransferParams(input string, params map[string]interface{}) {
	sourceFile := e.extractFilePath(input)
	if sourceFile != "" {
		params["source_file"] = sourceFile
	}

	ipAddr := e.extractIPAddress(input)
	if ipAddr != "" {
		params["search"] = ipAddr
	} else {
		params["nodes"] = e.extractNodes(input)
	}

	destDir := e.extractDestDir(input)
	if destDir != "" {
		params["dest_dir"] = destDir
	}

	if strings.Contains(strings.ToLower(input), "/opt") {
		params["dest_dir"] = "/opt"
	}
	if strings.Contains(strings.ToLower(input), "/tmp") {
		params["dest_dir"] = "/tmp"
	}

	if _, ok := params["dest_dir"]; !ok {
		params["dest_dir"] = "/tmp"
	}
}

func (e *ParamExtractor) extractIPAddress(input string) string {
	words := strings.Fields(input)
	for _, word := range words {
		ip := net.ParseIP(word)
		if ip != nil {
			return word
		}
	}
	return ""
}

func (e *ParamExtractor) extractNodes(input string) []interface{} {
	var nodes []interface{}

	for _, nodeName := range e.nodeNames {
		if strings.Contains(input, nodeName) {
			nodes = append(nodes, nodeName)
		}
	}

	if len(nodes) == 0 {
		if strings.Contains(input, "所有节点") ||
			strings.Contains(input, "all nodes") ||
			strings.Contains(input, "所有") {
			for _, nodeName := range e.nodeNames {
				nodes = append(nodes, nodeName)
			}
		}
	}

	if len(nodes) == 0 && len(e.nodeNames) > 0 {
		nodes = append(nodes, e.nodeNames[0])
	}

	return nodes
}

func (e *ParamExtractor) extractCommand(input string) string {
	inputLower := strings.ToLower(input)

	commonCommands := map[string]string{
		"uptime":    "uptime",
		"df -h":     "df -h",
		"free -m":   "free -m",
		"ps aux":    "ps aux",
		"systemctl": "systemctl status",
	}

	for cmd, fullCmd := range commonCommands {
		if strings.Contains(inputLower, cmd) {
			return fullCmd
		}
	}

	if strings.Contains(inputLower, "命令") {
		idx := strings.Index(inputLower, "命令")
		if idx+6 < len(input) {
			cmdPart := strings.TrimSpace(input[idx+6:])
			cmdPart = strings.Trim(cmdPart, " \"'")
			if cmdPart != "" {
				return cmdPart
			}
		}
	}

	return "uptime"
}

func (e *ParamExtractor) extractFilePath(input string) string {
	words := strings.Fields(input)
	for _, word := range words {
		if strings.HasPrefix(word, "/") ||
			strings.HasSuffix(word, ".tar") ||
			strings.HasSuffix(word, ".gz") ||
			strings.HasSuffix(word, ".zip") ||
			strings.HasSuffix(word, ".tgz") {
			return word
		}
	}
	return ""
}

func (e *ParamExtractor) extractDestDir(input string) string {
	lowerInput := strings.ToLower(input)

	if idx := strings.Index(lowerInput, "/opt"); idx != -1 {
		return "/opt"
	}
	if idx := strings.Index(lowerInput, "/tmp"); idx != -1 {
		return "/tmp"
	}
	if idx := strings.Index(lowerInput, "/var"); idx != -1 {
		return "/var"
	}
	if idx := strings.Index(lowerInput, "/usr"); idx != -1 {
		return "/usr/local"
	}

	return ""
}

func (e *ParamExtractor) SetNodeNames(nodeNames []string) {
	e.nodeNames = nodeNames
}
