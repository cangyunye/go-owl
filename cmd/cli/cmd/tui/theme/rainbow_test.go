package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestRainbowRingComplete(t *testing.T) {
	assert.Len(t, rainbow, 8, "应有 8 色")
	for i, c := range rainbow {
		assert.NoError(t, c.Validate(), "色环[%d] 三档需合法", i)
	}
}

func TestRainbowDeterministic(t *testing.T) {
	a := Rainbow("env")
	b := Rainbow("env")
	assert.Equal(t, a, b, "同 key 应同色")
}

func TestRainbowIndexRange(t *testing.T) {
	// 0-7 索引全覆盖可通过 16 个不同 key 断言不越界
	for _, k := range []string{"env", "role", "zone", "arch", "os", "tier", "team", "app", "a", "bb", "ccc", "dddd", "eeeee", "ffffff", "g", "hh"} {
		c := Rainbow(k)
		assert.NotNil(t, c, "Rainbow(%q) 非 nil", k)
	}
}

func TestRainbowImplementsInterface(t *testing.T) {
	var _ lipgloss.TerminalColor = Rainbow("x")
}
