package ai

import (
	"fmt"
	"strings"

	"github.com/cangyunye/go-owl/internal/control/blacklist"
)

// SafetyPolicy 是 AI 工具执行的安全策略：黑名单硬拦截 + 低危白名单分级确认。
// 策略在内核层统一实施，CLI 与 Web 两个宿主自动同时生效。
type SafetyPolicy struct {
	// checker 命令黑名单（硬拒绝，任何确认机制均不能放行）
	checker *blacklist.Checker
	// AllowedCommands 低危命令白名单（前缀匹配），仅影响确认分级
	AllowedCommands []string
	// ConfirmLowRisk 为 true（默认，保守）时低危命令仍走确认门；
	// 显式设为 false 时低危命令跳过确认。
	ConfirmLowRisk *bool
}

// defaultLowRiskCommands 是内置低危只读命令白名单（前缀匹配）。
// 仅在 confirm_low_risk=false 时用于跳过确认，不影响黑名单拦截。
var defaultLowRiskCommands = []string{
	"df", "df -", "free", "free -", "uptime", "uname", "hostname",
	"date", "whoami", "id", "cat /proc/", "cat /etc/hostname",
	"systemctl status", "systemctl is-active", "systemctl list-units",
	"docker ps", "docker images", "docker inspect",
	"kubectl get", "kubectl top", "kubectl describe",
	"ps ", "ps aux", "top -b", "netstat", "ss -", "lsblk", "mount",
	"journalctl", "dmesg", "lsof", "ls ", "ls -", "head ", "tail ", "wc ",
	"ping -c", "dig ", "nslookup ", "curl -s -o /dev/null", "wget -q -O /dev/null",
}

// defaultSafetyPolicy 返回默认安全策略：blacklist 包默认规则 + 内置白名单 +
// 保守确认（低危也确认）。
func defaultSafetyPolicy() *SafetyPolicy {
	checker := blacklist.NewDefaultChecker()
	if cfg, err := blacklist.LoadConfig(); err == nil {
		checker = blacklist.NewChecker(cfg)
	}
	confirm := true
	return &SafetyPolicy{
		checker:         checker,
		AllowedCommands: defaultLowRiskCommands,
		ConfirmLowRisk:  &confirm,
	}
}

// safety 返回 Agent 的安全策略：显式 SetSafetyPolicy 优先，
// 其次从 config.Safety 构建，否则默认策略。
func (a *Agent) safety() *SafetyPolicy {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.safetyPolicy != nil {
		return a.safetyPolicy
	}
	return safetyPolicyFromConfig(a.config)
}

// safetyPolicyFromConfig 从 YAML 配置构建策略。
// BlockedCommands 的硬拦截由 blacklist 包（~/.owl/blacklist.yaml + 默认规则）
// 负责；此处的 AllowedCommands/ConfirmLowRisk 控制低危命令的确认分级。
func safetyPolicyFromConfig(cfg *Config) *SafetyPolicy {
	p := defaultSafetyPolicy()
	if cfg == nil {
		return p
	}
	if len(cfg.Safety.AllowedCommands) > 0 {
		p.AllowedCommands = cfg.Safety.AllowedCommands
	}
	if cfg.Safety.ConfirmLowRisk != nil {
		p.ConfirmLowRisk = cfg.Safety.ConfirmLowRisk
	}
	return p
}

// IsLowRiskCommand 判断命令是否命中低危白名单（去空格后前缀匹配）。
func (p *SafetyPolicy) IsLowRiskCommand(command string) bool {
	cmd := strings.Join(strings.Fields(command), " ")
	if cmd == "" {
		return false
	}
	for _, prefix := range p.AllowedCommands {
		prefix = strings.Join(strings.Fields(prefix), " ")
		if prefix == "" {
			continue
		}
		if cmd == strings.TrimRight(prefix, " ") || strings.HasPrefix(cmd, prefix) {
			return true
		}
	}
	return false
}

// CheckCommand 黑名单检查。命中返回拒绝文案（作为工具结果），未命中返回 ""。
func (p *SafetyPolicy) CheckCommand(user, command string) string {
	result, err := p.checker.CheckForExec(user, command, false)
	if err != nil && result != nil && result.Blocked {
		var lines []string
		for _, m := range result.Matches {
			lines = append(lines, fmt.Sprintf("%q 匹配规则 %q", m.Line, m.Pattern))
		}
		return fmt.Sprintf("⛔ 危险命令已被黑名单拦截，AI 不允许执行: %s", strings.Join(lines, "; "))
	}
	return ""
}

// commandToolArgumentKeys 从工具参数中提取待检查的命令文本。
func commandToolArgumentKeys(call ToolCall) (command string, ok bool) {
	switch call.Name {
	case "execute_command":
		ok = true
		if c, exists := call.Arguments["command"].(string); exists {
			command = c
		}
	case "execute_script":
		ok = true
		// 脚本内容整体作为检查对象（含 inline 脚本）
		if c, exists := call.Arguments["script"].(string); exists {
			command = c
		}
	}
	return command, ok
}
