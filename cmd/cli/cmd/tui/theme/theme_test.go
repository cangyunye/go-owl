package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestParseEnv(t *testing.T) {
	cases := []struct {
		env  string
		want Name
	}{
		{"", ThemeANSI},
		{"ansi", ThemeANSI},
		{"ANSI", ThemeANSI},
		{"truecolor", ThemeTrueColor},
		{"TrueColor", ThemeTrueColor},
		{"TRUECOLOR", ThemeTrueColor},
		{"true-color", ThemeTrueColor},
		{"bogus", ThemeANSI},
		{" ", ThemeANSI},
	}
	for _, c := range cases {
		t.Run(c.env, func(t *testing.T) {
			assert.Equal(t, c.want, parseEnv(c.env))
		})
	}
}

func TestFgANSI(t *testing.T) {
	cases := map[string]string{
		CSelected:     "14",
		CDim:          "8",
		CError:        "9",
		CUser:         "10",
		CAI:           "6",
		CHighlightFg:  "0",
		CHighlightBg:  "14",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, lipgloss.Color(want), fg(ThemeANSI, name))
		})
	}
}

func TestFgTrueColor(t *testing.T) {
	for name := range palettes[ThemeTrueColor] {
		t.Run(name, func(t *testing.T) {
			got := fg(ThemeTrueColor, name)
			assert.True(t, len(got) == 7 && got[0] == '#', "truecolor 应为 #RRGGBB 格式, got %q", got)
		})
	}
	assert.NotEqual(t,
		fg(ThemeANSI, CSelected),
		fg(ThemeTrueColor, CSelected),
	)
}

func TestFgUnknownName(t *testing.T) {
	assert.Equal(t, lipgloss.Color(""), fg(ThemeANSI, "not-a-color"))
}

func TestResolveTheme(t *testing.T) {
	cases := []struct {
		name       string
		env        string
		capable    bool
		want       Name
		wantDowng  bool
	}{
		{"empty", "", true, ThemeANSI, false},
		{"ansi", "ansi", true, ThemeANSI, false},
		{"truecolor-capable", "truecolor", true, ThemeTrueColor, false},
		{"truecolor-conhost", "truecolor", false, ThemeANSI, true},
		{"unknown", "bogus", true, ThemeANSI, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, downg := resolveTheme(parseEnv(c.env), c.capable)
			assert.Equal(t, c.want, got)
			assert.Equal(t, c.wantDowng, downg)
		})
	}
}

func TestTerminalTrueColorCapable(t *testing.T) {
	cases := []struct {
		name string
		envs map[string]string
		want bool
	}{
		{"windows-terminal", map[string]string{"WT_SESSION": "abc"}, true},
		{"conemu", map[string]string{"ConEmuANSI": "ON"}, true},
		{"colorterm", map[string]string{"COLORTERM": "truecolor"}, true},
		{"colorterm-24bit", map[string]string{"COLORTERM": "24bit"}, true},
		{"conhost", map[string]string{}, false},
		{"conhost-no-colorterm", map[string]string{"COLORTERM": ""}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range map[string]string{"WT_SESSION": "", "ConEmuANSI": "", "COLORTERM": ""} {
				t.Setenv(k, v)
			}
			for k, v := range c.envs {
				t.Setenv(k, v)
			}
			assert.Equal(t, c.want, terminalTrueColorCapable())
		})
	}
}

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
