package blacklist

import (
	"fmt"
	"strings"
)

// BlockedError 命令命中黑名单且未获 force 放行。
type BlockedError struct {
	Result *CheckResult
}

func (e *BlockedError) Error() string {
	var lines []string
	for _, m := range e.Result.Matches {
		lines = append(lines, fmt.Sprintf("%q 匹配规则 %q", m.Line, m.Pattern))
	}
	return fmt.Sprintf("危险命令已被黑名单拦截: %s", strings.Join(lines, "; "))
}

// CheckForExec 供 API 场景使用：
// force=false 且命中时返回 *BlockedError；force=true 时放行，
// 但仍返回 CheckResult（Blocked=true）供调用方审计记录。
func (c *Checker) CheckForExec(user, command string, force bool) (*CheckResult, error) {
	result := c.Check(user, command)
	if result.Blocked && !force {
		return result, &BlockedError{Result: result}
	}
	return result, nil
}
