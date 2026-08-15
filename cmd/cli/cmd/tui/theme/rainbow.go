package theme

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"
)

var rainbow = []CompleteColor{
	{TrueColor: "#F38BA8", ANSI256: "204", ANSI: "9"},
	{TrueColor: "#FAB387", ANSI256: "215", ANSI: "11"},
	{TrueColor: "#F9E2AF", ANSI256: "223", ANSI: "3"},
	{TrueColor: "#A6E3A1", ANSI256: "150", ANSI: "10"},
	{TrueColor: "#94E2D5", ANSI256: "116", ANSI: "14"},
	{TrueColor: "#89DCEB", ANSI256: "117", ANSI: "6"},
	{TrueColor: "#CBA6F7", ANSI256: "183", ANSI: "13"},
	{TrueColor: "#B4BEFE", ANSI256: "147", ANSI: "12"},
}

func Rainbow(key string) lipgloss.TerminalColor {
	h := fnv.New32a()
	h.Write([]byte(key))
	c := rainbow[h.Sum32()%uint32(len(rainbow))]
	return hybridColor(Slot{Light: c, Dark: c})
}
