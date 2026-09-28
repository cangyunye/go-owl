package blacklist

import (
	"regexp"
	"strings"
)

type MatchItem struct {
	Pattern string
	Line    string
}

type CheckResult struct {
	Blocked bool
	User    string
	Matches []MatchItem
}

// compiledPattern 规则模式的预编译形态：
// 能编译成正则的按正则匹配（如 chown .*:[0-9]+ ），否则按字面子串匹配
// （如含非法正则语法的 :(){ :|:& };: 管道炸弹）。
type compiledPattern struct {
	orig  string
	re    *regexp.Regexp
	lower string
}

type compiledRule struct {
	user     string
	patterns []compiledPattern
}

type Checker struct {
	config *Config
	rules  []compiledRule
}

func NewChecker(cfg *Config) *Checker {
	c := &Checker{config: cfg}
	for _, r := range cfg.Rules {
		cr := compiledRule{user: r.User}
		for _, p := range r.Patterns {
			cp := compiledPattern{orig: p, lower: strings.ToLower(p)}
			if re, err := regexp.Compile(`(?i)` + p); err == nil {
				cp.re = re
			}
			cr.patterns = append(cr.patterns, cp)
		}
		c.rules = append(c.rules, cr)
	}
	return c
}

// NewDefaultChecker 使用默认规则创建检查器（Web 端剧本/命令执行校验使用）。
func NewDefaultChecker() *Checker {
	return NewChecker(&Config{Rules: DefaultRules()})
}

func (c *Checker) Check(user, command string) *CheckResult {
	result := &CheckResult{
		User: user,
	}

	lines := splitCommand(command)

	for _, rule := range c.rules {
		if rule.user != user && rule.user != "*" {
			continue
		}

		for _, pattern := range rule.patterns {
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				stripped := strings.ToLower(stripQuoted(trimmed))
				if idx, ok := matchPattern(pattern, stripped); ok && atBoundary(stripped, idx) {
					result.Matches = append(result.Matches, MatchItem{
						Pattern: pattern.orig,
						Line:    trimmed,
					})
				}
			}
		}
	}

	result.Blocked = len(result.Matches) > 0
	if result.Blocked {
		return result
	}
	result.Matches = append(result.Matches, checkBareShell(lines)...)
	result.Blocked = len(result.Matches) > 0
	return result
}

func matchPattern(p compiledPattern, line string) (int, bool) {
	if p.re != nil {
		loc := p.re.FindStringIndex(line)
		if loc == nil {
			return 0, false
		}
		return loc[0], true
	}
	idx := strings.Index(line, p.lower)
	return idx, idx >= 0
}

// atBoundary 检查命中点是否位于命令开始，或前一个字符是分隔符
// （空格/分号/管道/换行/制表符，以及 shell 展开点 ( ` $ ），
// 避免把 ftrim -rf 之类单词内部误判成 rm -rf。
func atBoundary(line string, idx int) bool {
	if idx <= 0 {
		return true
	}
	switch line[idx-1] {
	case ' ', ';', '&', '|', '\n', '\t', '(', '`', '$':
		return true
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

// bareShellWords 裸 shell 解释器：作为管道末端或独立段出现时，
// 载荷来自上游输出（base64 解码、curl 下载等），文本匹配天然不可见，
// 必须对整条管道要求确认。
var bareShellWords = map[string]bool{
	"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true, "ash": true,
}

func checkBareShell(lines []string) []MatchItem {
	var items []MatchItem
	for _, line := range lines {
		for _, seg := range splitPipes(line) {
			fields := strings.Fields(seg)
			for len(fields) > 0 {
				head := strings.ToLower(fields[0])
				if head != "sudo" && head != "su" {
					break
				}
				fields = fields[1:]
				if head == "sudo" && len(fields) > 1 && strings.EqualFold(fields[0], "-u") {
					fields = fields[2:]
				}
			}
			if len(fields) == 0 {
				continue
			}
			if bareShellWords[strings.ToLower(fields[0])] {
				items = append(items, MatchItem{
					Pattern: fields[0] + " (裸 shell 解释器)",
					Line:    strings.TrimSpace(line),
				})
			}
		}
	}
	return items
}

// splitPipes 在引号外的单个 | 处切段（splitCommand 只切 ; && || 和换行，
// 不切管道符）。|| 已在 splitCommand 处理，这里遇到只会剩单管道。
func splitPipes(line string) []string {
	var segs []string
	var current strings.Builder
	inSingle, inDouble := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == '|' && !inSingle && !inDouble:
			segs = append(segs, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	segs = append(segs, current.String())
	return segs
}
