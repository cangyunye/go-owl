package theme

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// 环境变量 OWL_TUI_THEME 选择主题: default | catppuccin | nord | dracula | solarized(默认 catppuccin)
const EnvTheme = "OWL_TUI_THEME"

type Name string

const DefaultTheme Name = "catppuccin"

func parseThemeName(s string) Name {
	n := strings.ToLower(strings.TrimSpace(s))
	if n == "" {
		return DefaultTheme
	}
	if _, ok := presets[Name(n)]; ok {
		return Name(n)
	}
	return DefaultTheme
}

func Current() Name {
	return parseThemeName(os.Getenv(EnvTheme))
}

func Color(key SlotKey) lipgloss.TerminalColor {
	t := presets[Current()]
	if s, ok := t.Slots[key]; ok {
		return hybridColor(s)
	}
	return lipgloss.NoColor{}
}

func Style(key SlotKey) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(Color(key))
}

func Title(text string) string {
	return Style(SlotTitle).Bold(true).Render(text)
}
