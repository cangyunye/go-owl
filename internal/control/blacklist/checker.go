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
				if strings.Contains(stripped, pattern) {
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
