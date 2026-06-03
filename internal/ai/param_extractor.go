package ai

import (
	"net"
	"regexp"
	"strings"
)

var labelKeyMap = map[string]string{
	"使用人":   "owner",
	"负责人":   "owner",
	"责任人":   "owner",
	"拥有者":   "owner",
	"所有者":   "owner",
	"环境":    "env",
	"环境类型":  "env",
	"环境分组":  "env",
	"应用":    "app",
	"应用名":   "app",
	"应用名称":  "app",
	"应用类型":  "app",
	"服务":    "app",
	"服务名":   "app",
	"服务名称":  "app",
	"区域":    "region",
	"地区":    "region",
	"位置":    "region",
	"角色":    "role",
	"身份":    "role",
	"层级":    "tier",
	"层级分组":  "tier",
	"类型":    "type",
	"数据类型":  "type",
	"服务类型":  "type",
	"数据库类型": "dbtype",
	"数据库":   "dbtype",
	"db":    "dbtype",
}

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
	
	// 检查是否已经有了 user 或 owner 标签，如果有就不再尝试提取 owner
	hasUserLabel := false
	hasOwnerLabel := false
	if labels, ok := params["labels"].(map[string]interface{}); ok {
		_, hasUserLabel = labels["user"]
		_, hasOwnerLabel = labels["owner"]
	}
	
	if !hasUserLabel && !hasOwnerLabel {
		e.extractOwnerFilter(input, params)
	}
	
	e.extractEnvFilter(input, params)
	
	// 同样，检查是否已经有了 user 标签，如果有就不再尝试用 extractGenericLabelFilters
	if !hasUserLabel {
		if labels, ok := params["labels"].(map[string]interface{}); ok {
			_, hasUserLabel = labels["user"]
		}
		if !hasUserLabel {
			e.extractGenericLabelFilters(input, params)
		}
	}
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

	// 识别通用的 label=value 模式
	labelPatterns := []string{
		"user=", "env=", "owner=", 
		"role=", "group=", "os=",
		"region=", "env=", "region=", "role=",
		"USER=", "ENV=", "OWNER=",
	}
	for _, pattern := range labelPatterns {
		if strings.Contains(lowerInput, pattern) {
			patternLower := strings.ToLower(pattern)
			idx := strings.Index(lowerInput, patternLower)
			if idx >= 0 {
				// 提取 key from the pattern
				valuePart := input[idx+len(patternLower):]
				// 提取值部分，直到遇到非字母数字字符停止
				runes := []rune(valuePart)
				var value string
				endIdx := -1
				for i := 0; i < len(runes); i++ {
					r := runes[i]
					isAlnum := (r >= 'a' && r <= 'z') || 
						(r >= 'A' && r <= 'Z') || 
						(r >= '0' && r <= '9') || 
						r == '_' || r == '-'
					if isAlnum {
						endIdx = i
					} else {
						break
					}
				}
				if endIdx >= 0 {
					value = string(runes[:endIdx+1])
				} else {
					value = valuePart
				}
				
				key := strings.TrimRight(pattern, "=")
				if value != "" && key != "" {
					if labels, ok := params["labels"].(map[string]interface{}); ok {
						labels[key] = value
					} else {
						params["labels"] = map[string]interface{}{key: value}
					}
				}
			}
		}
	}
}

func (e *ParamExtractor) extractOwnerFilter(input string, params map[string]interface{}) {
	if strings.Contains(input, "使用人") ||
		strings.Contains(input, "负责人") ||
		strings.Contains(input, "拥有者") {

		var ownerValue string
		if idx := strings.Index(input, "使用人"); idx != -1 {
			ownerValue = strings.TrimSpace(input[idx+6:])
		} else if idx := strings.Index(input, "负责人"); idx != -1 {
			ownerValue = strings.TrimSpace(input[idx+6:])
		} else if idx := strings.Index(input, "拥有者"); idx != -1 {
			ownerValue = strings.TrimSpace(input[idx+6:])
		}

		if ownerValue != "" {
			ownerValue = e.extractPersonName(ownerValue)
			if ownerValue != "" {
				params["labels"] = map[string]interface{}{"owner": ownerValue}
			}
		}
		return
	}

	if strings.Contains(strings.ToLower(input), "owner=") {
		idx := strings.Index(strings.ToLower(input), "owner=")
		ownerValue := strings.TrimSpace(input[idx+6:])
		if ownerValue != "" {
			params["labels"] = map[string]interface{}{"owner": ownerValue}
		}
		return
	}

	if personName := e.extractPersonNameFromQuery(input); personName != "" {
		params["labels"] = map[string]interface{}{"owner": personName}
		return
	}

	name := e.extractPersonName(input)
	if name != "" {
		params["labels"] = map[string]interface{}{"owner": name}
	}
}

func (e *ParamExtractor) extractPersonNameFromQuery(input string) string {
	queryPatterns := []string{
		"找下", "找", "查询", "查看", "看看", "搜下",
		"找出", "看看", "看下", "找一下",
		"列出", "获取",
	}

	for _, pattern := range queryPatterns {
		idx := strings.Index(input, pattern)
		if idx >= 0 {
			remaining := input[idx+len(pattern):]
			trimmed := strings.TrimSpace(remaining)

			if strings.HasPrefix(trimmed, "的") {
				trimmed = strings.TrimPrefix(trimmed, "的")
			}

			if strings.HasPrefix(trimmed, "一") && len(trimmed) > 1 {
				trimmed = trimmed[1:]
			}

			trimmed = strings.TrimSpace(trimmed)

			if len(trimmed) >= 2 && len(trimmed) <= 4 {
					chinesePattern := regexp.MustCompile(`^[一-龥]+`)
					match := chinesePattern.FindString(trimmed)

				if match != "" && len(match) >= 2 && len(match) <= 4 {
					excludeWords := map[string]bool{
						"节点": true, "服务器": true, "主机": true,
						"环境": true, "标签": true, "分组": true,
						"信息": true, "列表": true, "在线": true, "离线": true,
					}

					if !excludeWords[match] {
						return match
					}
				}
			}
		}
	}

	return ""
}

func (e *ParamExtractor) extractEnvFilter(input string, params map[string]interface{}) {
	lowerInput := strings.ToLower(input)

	if strings.Contains(lowerInput, "环境") {
		idx := strings.Index(input, "环境")
		envPart := input[idx+6:]
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
			if labels, ok := params["labels"].(map[string]interface{}); ok {
				labels["env"] = envPart
			} else {
				params["labels"] = map[string]interface{}{"env": envPart}
			}
		}
	}
}

func (e *ParamExtractor) extractGenericLabelFilters(input string, params map[string]interface{}) {
	lowerInput := strings.ToLower(input)

	// 处理 "XXX用户" 模式 -> user=XXX
	if strings.Contains(lowerInput, "用户") {
		idx := strings.Index(lowerInput, "用户")
		if idx > 0 {
			userPart := input[0:idx]
			userPart = strings.TrimSpace(userPart)
			// 使用更智能的方式提取最后一个有效的英文单词或数字
			// 查找最后一个连续的字母数字序列
			runes := []rune(userPart)
			var userValue string
			startIdx := -1
			for i := len(runes) - 1; i >= 0; i-- {
				r := runes[i]
				isAlnum := (r >= 'a' && r <= 'z') || 
					(r >= 'A' && r <= 'Z') || 
					(r >= '0' && r <= '9') || 
					r == '_' || r == '-'
				
				if isAlnum {
					if startIdx == -1 {
						startIdx = i
					}
				} else {
					if startIdx != -1 {
						userValue = string(runes[i+1 : startIdx+1])
						break
					}
				}
			}
			// 如果没有找到有效字符，检查整个字符串
			if userValue == "" && startIdx != -1 {
				userValue = string(runes[0 : startIdx+1])
			}
			// 如果还是空，尝试直接用Fields提取最后一个词
			if userValue == "" {
				parts := strings.Fields(userPart)
				if len(parts) > 0 {
					userValue = parts[len(parts)-1]
				}
			}
			
			if userValue != "" {
				if labels, ok := params["labels"].(map[string]interface{}); ok {
					labels["user"] = userValue
				} else {
					params["labels"] = map[string]interface{}{"user": userValue}
				}
			}
		}
	}

	// 处理 "用户为XXX" 或 "用户是XXX" 模式
	if strings.Contains(lowerInput, "用户为") || 
		strings.Contains(lowerInput, "用户是") {
		var idx int
		var markerLen int
		if strings.Contains(lowerInput, "用户为") {
			idx = strings.Index(lowerInput, "用户为")
			markerLen = 6
		} else {
			idx = strings.Index(lowerInput, "用户是")
			markerLen = 6
		}
		userPart := input[idx+markerLen:]
		userPart = strings.TrimSpace(userPart)
		
		// 提取第一个连续的字母数字序列
		runes := []rune(userPart)
		var userValue string
		endIdx := -1
		for i := 0; i < len(runes); i++ {
			r := runes[i]
			isAlnum := (r >= 'a' && r <= 'z') || 
				(r >= 'A' && r <= 'Z') || 
				(r >= '0' && r <= '9') || 
				r == '_' || r == '-'
			
			if isAlnum {
				if endIdx == -1 {
					endIdx = i
				}
			} else {
				if endIdx != -1 {
					userValue = string(runes[endIdx : i])
					break
				}
			}
		}
		// 如果一直到结尾都是有效字符
		if userValue == "" && endIdx != -1 {
			userValue = string(runes[endIdx:])
		}
		// 如果还是空，尝试直接用Fields提取第一个词
		if userValue == "" {
			parts := strings.Fields(userPart)
			if len(parts) > 0 {
				userValue = parts[0]
			}
		}
		
		if userValue != "" {
			if labels, ok := params["labels"].(map[string]interface{}); ok {
				labels["user"] = userValue
			} else {
				params["labels"] = map[string]interface{}{"user": userValue}
			}
		}
	}

	// 处理 "所有XXX" 模式
	// 先判断一下是否有其他可能的模式可以扩展
}

func (e *ParamExtractor) extractPersonName(input string) string {
	excludeWords := map[string]bool{
		"节点": true, "环境": true, "标签": true, "使用人": true, "负责人": true,
		"拥有者": true, "查询": true, "查看": true, "列出": true, "在线": true,
		"离线": true, "服务器": true, "主机": true, "信息": true, "列表": true,
		"分组": true, "一下": true, "负责": true, "获取": true, "找出": true,
		"看看": true, "搜下": true, "找下": true, "看下": true,
		"点列": true, "线节": true, "节点列": true, "在线节": true,
		"点列表": true, "线节点": true, "节点列表": true, "在线节点": true,
	}
	
	excludeSingleChars := map[rune]bool{
		'人': true, '找': true, '看': true, '列': true, '查': true,
		'获': true, '搜': true, '下': true,
	}

	runes := []rune(input)
	
	for length := 2; length <= 4; length++ {
		for i := 0; i <= len(runes)-length; i++ {
			if excludeSingleChars[runes[i]] {
				continue
			}
			
			name := string(runes[i : i+length])
			
			if !isAllChinese(name) {
				continue
			}
			
			if excludeWords[name] {
				continue
			}
			
			return name
		}
	}

	return ""
}

func isAllChinese(s string) bool {
	for _, r := range s {
		if !isChineseRune(r) {
			return false
		}
	}
	return true
}

func isChineseRune(r rune) bool {
	return r >= '\u4e00' && r <= '\u9fa5'
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
