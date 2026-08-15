package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseThemeName(t *testing.T) {
	cases := []struct {
		in   string
		want Name
	}{
		{"", DefaultTheme},
		{"catppuccin", "catppuccin"},
		{"CATPPUCCIN", "catppuccin"},
		{"nord", "nord"},
		{"dracula", "dracula"},
		{"solarized", "solarized"},
		{"default", "default"},
		{"bogus", DefaultTheme},
		{"ansi", DefaultTheme},
		{"truecolor", DefaultTheme},
		{" ", DefaultTheme},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, parseThemeName(c.in))
		})
	}
}

func TestThemeNamesRegistered(t *testing.T) {
	for _, n := range themeNames() {
		_, ok := presets[n]
		assert.True(t, ok, "主题 %q 已注册", n)
	}
}

func TestColorReturnsColor(t *testing.T) {
	c := Color(SlotSelected)
	assert.NotNil(t, c, "Color 应返回非 nil TerminalColor")
}

func TestStyleHasColor(t *testing.T) {
	got := Style(SlotSelected).Render("x")
	assert.NotEmpty(t, got)
}

func TestTitleBold(t *testing.T) {
	got := Title("标题")
	assert.Contains(t, got, "标题")
}

func TestInvalidEnvFallsBackToDefault(t *testing.T) {
	t.Setenv(EnvTheme, "bogus")
	assert.Equal(t, DefaultTheme, Current())
}
