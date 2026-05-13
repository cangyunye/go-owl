package session

import (
	"strings"
)

// CommandParser 命令解析器
type CommandParser struct {
	history    *CommandHistory
	builtins   []string
	systemCmds []string
}

// NewCommandParser 创建命令解析器
func NewCommandParser(history *CommandHistory) *CommandParser {
	return &CommandParser{
		history:    history,
		builtins:   []string{"exit", "quit", "help", "history", "nodes", "clear"},
		systemCmds: loadSystemCommands(),
	}
}

// Complete 补全命令
func (p *CommandParser) Complete(input string) []string {
	if input == "" {
		return nil
	}

	var candidates []string

	// 1. 补全内置命令
	for _, cmd := range p.builtins {
		if strings.HasPrefix(cmd, input) {
			candidates = append(candidates, cmd)
		}
	}

	// 2. 补全历史命令
	historyCmds := p.history.GetMatching(input)
	for _, cmd := range historyCmds {
		if !contains(candidates, cmd) {
			candidates = append(candidates, cmd)
		}
	}

	// 3. 补全系统命令
	for _, cmd := range p.systemCmds {
		if strings.HasPrefix(cmd, input) {
			if !contains(candidates, cmd) {
				candidates = append(candidates, cmd)
			}
		}
	}

	return candidates
}

// ParseCommand 解析命令
func (p *CommandParser) ParseCommand(input string) (string, []string) {
	input = strings.TrimSpace(input)

	// 检查内置命令
	for _, builtin := range p.builtins {
		if input == builtin {
			return builtin, nil
		}
	}

	// 检查历史命令引用
	if strings.HasPrefix(input, "!") {
		historyCmd := p.history.GetByIndex(input[1:])
		if historyCmd != "" {
			return historyCmd, nil
		}
	}

	// 普通命令
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", nil
	}

	return parts[0], parts[1:]
}

// IsBuiltin 检查是否为内置命令
func (p *CommandParser) IsBuiltin(command string) bool {
	for _, builtin := range p.builtins {
		if command == builtin {
			return true
		}
	}
	return false
}

// GetHelp 获取帮助信息
func (p *CommandParser) GetHelp(command string) string {
	switch command {
	case "exit", "quit":
		return "exit, quit - 优雅退出会话"
	case "help":
		return "help - 显示帮助信息"
	case "history":
		return "history - 显示命令历史"
	case "nodes":
		return "nodes - 显示当前连接的节点"
	case "clear":
		return "clear - 清屏"
	default:
		return ""
	}
}

// GetBuiltinCommands 获取所有内置命令
func (p *CommandParser) GetBuiltinCommands() []string {
	return p.builtins
}

// contains 检查切片是否包含元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// loadSystemCommands 加载系统命令
func loadSystemCommands() []string {
	var commands []string
	seen := make(map[string]bool)

	// 常用系统命令
	commonCommands := []string{
		"ls", "cd", "pwd", "cat", "grep", "sed", "awk",
		"find", "xargs", "sort", "uniq", "wc", "head", "tail",
		"cp", "mv", "rm", "mkdir", "chmod", "chown",
		"ps", "top", "htop", "kill", "killall",
		"df", "du", "free", "mount",
		" systemctl", "service", "journalctl",
		"docker", "docker-compose", "kubectl",
		"git", "svn",
		"curl", "wget", "ssh", "scp", "rsync",
		"tar", "gzip", "gunzip", "zip", "unzip",
		"vim", "nano", "less", "more",
		"ping", "netstat", "ss", "ip", "ifconfig",
		"cron", "at",
		"yum", "apt", "apt-get", "dnf",
		"python", "python3", "pip", "pip3",
		"node", "npm", "yarn",
		"java", "javac", "mvn", "gradle",
		"nginx", "apache2", "httpd",
		"mysql", "psql", "mongod",
		"redis-cli",
		"memcached",
		"iptables", "firewall-cmd",
	}

	for _, cmd := range commonCommands {
		if !seen[cmd] {
			commands = append(commands, cmd)
			seen[cmd] = true
		}
	}

	return commands
}
