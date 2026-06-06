package blacklist

import (
	"strings"
)

type MatchItem struct {
	Pattern string
	Line    string
}

type CheckResult struct {
	Blocked  bool
	User     string
	Matches  []MatchItem
}

type Checker struct {
	config *Config
}

func NewChecker(cfg *Config) *Checker {
	return &Checker{config: cfg}
}

func NewDefaultChecker() *Checker {
	return &Checker{config: &Config{Rules: DefaultRules()}}
}

func (c *Checker) Check(user, command string) *CheckResult {
	result := &CheckResult{
		User: user,
	}

	lines := splitCommand(command)

	for _, rule := range c.config.Rules {
		if rule.User != user && rule.User != "*" {
			continue
		}

		for _, pattern := range rule.Patterns {
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				stripped := stripQuoted(trimmed)
				if matchesCommand(stripped, pattern) {
					result.Matches = append(result.Matches, MatchItem{
						Pattern: pattern,
						Line:    trimmed,
					})
				}
			}
		}
	}

	result.Blocked = len(result.Matches) > 0
	return result
}

// matchesCommand 检查命令是否匹配模式，只匹配命令开始部分
// pattern 应该以命令分隔符结尾（如空格、|、&等）以确保匹配完整命令
func matchesCommand(cmd, pattern string) bool {
	// 直接包含匹配（用于复杂模式如 "rm -rf"）
	if strings.Contains(cmd, pattern) {
		// 如果模式以空格结尾，检查是否在单词边界
		if strings.HasSuffix(pattern, " ") || strings.HasSuffix(pattern, "=") {
			idx := strings.Index(cmd, pattern)
			// 确保匹配在命令开始或前面是分隔符
			if idx == 0 {
				return true
			}
			// 检查前面是否是分隔符
			before := cmd[:idx]
			if len(before) > 0 {
				lastChar := before[len(before)-1]
				if lastChar == ' ' || lastChar == ';' || lastChar == '&' || lastChar == '|' || lastChar == '\n' || lastChar == '\t' {
					return true
				}
			}
		} else {
			// 对于没有空格后缀的模式，检查是否在开始或前面有分隔符
			idx := strings.Index(cmd, pattern)
			if idx == 0 {
				return true
			}
			// 检查前面是否是分隔符
			before := cmd[:idx]
			if len(before) > 0 {
				lastChar := before[len(before)-1]
				if lastChar == ' ' || lastChar == ';' || lastChar == '&' || lastChar == '|' || lastChar == '\n' || lastChar == '\t' {
					return true
				}
			}
		}
	}
	return false
}

func stripQuoted(s string) string {
	var b strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			escaped = false
			b.WriteByte(c)
			continue
		}

		if c == '\\' {
			escaped = true
			continue
		}

		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}

		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if !inSingle && !inDouble {
			b.WriteByte(c)
		}
	}

	return b.String()
}

func splitCommand(command string) []string {
	var lines []string
	current := ""
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for i := 0; i < len(command); i++ {
		c := command[i]

		if escaped {
			current += string(c)
			escaped = false
			continue
		}

		if c == '\\' {
			current += string(c)
			escaped = true
			continue
		}

		switch c {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
			current += string(c)
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
			current += string(c)
		case '\n':
			if !inSingleQuote && !inDoubleQuote {
				if strings.TrimSpace(current) != "" {
					lines = append(lines, current)
				}
				current = ""
			} else {
				current += string(c)
			}
		case ';':
			if !inSingleQuote && !inDoubleQuote {
				if strings.TrimSpace(current) != "" {
					lines = append(lines, current)
				}
				current = ""
			} else {
				current += string(c)
			}
		case '&':
			if !inSingleQuote && !inDoubleQuote && i+1 < len(command) && command[i+1] == '&' {
				if strings.TrimSpace(current) != "" {
					lines = append(lines, current)
				}
				current = ""
				i++
			} else {
				current += string(c)
			}
		case '|':
			if !inSingleQuote && !inDoubleQuote {
				if i+1 < len(command) && command[i+1] == '|' {
					if strings.TrimSpace(current) != "" {
						lines = append(lines, current)
					}
					current = ""
					i++
				} else {
					current += string(c)
				}
			} else {
				current += string(c)
			}
		default:
			current += string(c)
		}
	}

	if strings.TrimSpace(current) != "" {
		lines = append(lines, current)
	}

	return lines
}
