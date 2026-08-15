package theme

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// 环境变量 OWL_TUI_THEME: ansi(默认,兜底16色) | truecolor(HEX真彩)
const EnvTheme = "OWL_TUI_THEME"

type Name string

const (
	ThemeANSI      Name = "ansi"
	ThemeTrueColor Name = "truecolor"
)

// 语义色名
const (
	CSelected    = "selected"
	CDim         = "dim"
	CError       = "error"
	CUser        = "user"
	CAI          = "ai"
	CHighlightFg = "highlightFg"
	CHighlightBg = "highlightBg"
)

var palettes = map[Name]map[string]string{
	ThemeANSI: {
		CSelected:    "14",
		CDim:         "8",
		CError:       "9",
		CUser:        "10",
		CAI:          "6",
		CHighlightFg: "0",
		CHighlightBg: "14",
	},
	ThemeTrueColor: {
		CSelected:    "#5FD4FF",
		CDim:         "#6A6A6A",
		CError:       "#FF5C5C",
		CUser:        "#5AF78E",
		CAI:          "#8A6CF8",
		CHighlightFg: "#101014",
		CHighlightBg: "#5FD4FF",
	},
}

var (
	current    Name = initTheme()
	requested       = parseEnv(os.Getenv(EnvTheme)) == ThemeTrueColor
	downgraded      = requested && current == ThemeANSI
)

func initTheme() Name {
	t, _ := resolveTheme(parseEnv(os.Getenv(EnvTheme)), terminalTrueColorCapable())
	return t
}

// parseEnv 解析环境变量值,大小写不敏感; 未设置/未知值一律回退 ANSI
func parseEnv(v string) Name {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "truecolor", "true-color":
		return ThemeTrueColor
	default:
		return ThemeANSI
	}
}

// resolveTheme 请求 truecolor 但终端不支持时回退 ANSI,并标记降级
func resolveTheme(req Name, capable bool) (Name, bool) {
	if req == ThemeTrueColor && !capable {
		return ThemeANSI, true
	}
	return req, false
}

// terminalTrueColorCapable 探测终端是否真实支持 24 位真彩。
// lipgloss 在 Windows 上会把 conhost(Win10 build>=14931)误判为 TrueColor,
// 但 conhost 实际不渲染 38;2 真彩序列,故这里用环境信号二次校验:
// Windows Terminal(WT_SESSION)、ConEmu(ConEmuANSI)或显式 COLORTERM=truecolor/24bit。
func terminalTrueColorCapable() bool {
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	if os.Getenv("ConEmuANSI") == "ON" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM"))) {
	case "truecolor", "24bit", "yes", "true":
		return true
	}
	return false
}

// IsTrueColor 当前是否启用真彩主题
func IsTrueColor() bool { return current == ThemeTrueColor }

// RequestedTrueColor 环境变量是否请求了真彩主题
func RequestedTrueColor() bool { return requested }

// DowngradedToANSI 请求了真彩但终端不支持,已回退 ANSI
func DowngradedToANSI() bool { return downgraded }

// fg 按主题与语义名查色值
func fg(pal Name, name string) lipgloss.Color {
	if c, ok := palettes[pal][name]; ok {
		return lipgloss.Color(c)
	}
	return lipgloss.Color("")
}

// Fg 按当前主题返回语义色
func Fg(name string) lipgloss.Color { return fg(current, name) }
