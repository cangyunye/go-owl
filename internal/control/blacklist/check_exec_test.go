package blacklist

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckForExec_BlockedCommands(t *testing.T) {
	c := NewChecker(&Config{Rules: DefaultRules()})

	_, err := c.CheckForExec("root", "rm -rf /var/log", false)
	require.Error(t, err)

	var blocked *BlockedError
	require.ErrorAs(t, err, &blocked)
	assert.NotEmpty(t, blocked.Result.Matches)
	assert.Equal(t, "root", blocked.Result.User)
}

func TestCheckForExec_ForceAllows(t *testing.T) {
	c := NewChecker(&Config{Rules: DefaultRules()})

	result, err := c.CheckForExec("root", "rm -rf /var/log", true)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Blocked, "force 放行时仍需返回匹配详情供审计")
}

func TestCheckForExec_SafeCommand(t *testing.T) {
	c := NewChecker(&Config{Rules: DefaultRules()})

	result, err := c.CheckForExec("root", "uptime", false)
	require.NoError(t, err)
	assert.False(t, result.Blocked)
}

func TestCheckForExec_UserScope(t *testing.T) {
	c := NewChecker(&Config{Rules: DefaultRules()})

	// sudo 规则仅对 root 用户生效
	_, err := c.CheckForExec("www-data", "echo hello", false)
	require.NoError(t, err)
}
