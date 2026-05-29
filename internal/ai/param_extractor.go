package ai

import (
	"strings"
)

type ParamExtractor struct {
	nodeNames []string
}

func NewParamExtractor(nodeNames []string) *ParamExtractor {
	return &ParamExtractor{
		nodeNames: nodeNames,
	}
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

	params["nodes"] = e.extractNodes(input)

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
