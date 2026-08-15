package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

func testSlot() Slot {
	return Slot{
		Light: CompleteColor{TrueColor: "#5FD4FF", ANSI256: "75", ANSI: "14"},
		Dark:  CompleteColor{TrueColor: "#864EFF", ANSI256: "99", ANSI: "13"},
	}
}

func render(profile termenv.Profile, dark bool) string {
	lipgloss.SetColorProfile(profile)
	lipgloss.SetHasDarkBackground(dark)
	hc := hybridColor(testSlot())
	return lipgloss.NewStyle().Foreground(hc).Render("x")
}

func TestHybridColorTrueColor(t *testing.T) {
	out := render(termenv.TrueColor, true)
	assert.Contains(t, out, "38;2;134;78;255", "truecolor+暗背景应取 Dark.TrueColor")
	out = render(termenv.TrueColor, false)
	assert.Contains(t, out, "38;2;95;211;255", "truecolor+亮背景应取 Light.TrueColor")
}

func TestHybridColorANSI256(t *testing.T) {
	out := render(termenv.ANSI256, true)
	assert.Contains(t, out, "38;5;99", "256+暗背景应取 Dark.ANSI256")
	out = render(termenv.ANSI256, false)
	assert.Contains(t, out, "38;5;75", "256+亮背景应取 Light.ANSI256")
}

func TestHybridColorANSI(t *testing.T) {
	out := render(termenv.ANSI, true)
	assert.Contains(t, out, "\x1b[95m", "16色+暗背景应取 Dark.ANSI(13→bright magenta)")
	out = render(termenv.ANSI, false)
	assert.Contains(t, out, "\x1b[96m", "16色+亮背景应取 Light.ANSI(14→bright cyan)")
}

func TestHybridColor256Fallback(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(true)
	hc := hybridColor(Slot{
		Light: CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"},
		Dark:  CompleteColor{TrueColor: "#864EFF", ANSI: "13"},
	})
	out := lipgloss.NewStyle().Foreground(hc).Render("x")
	assert.Contains(t, out, "38;5;", "ANSI256 缺省时由 hex 自动降级")
}

func TestHybridColorImplementsInterface(t *testing.T) {
	var _ lipgloss.TerminalColor = HybridColor{}
}
