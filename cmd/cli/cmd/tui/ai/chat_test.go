package ai

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func runeKey(r rune) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestChat_DefaultState(t *testing.T) {
	store := common.NewInMemoryNodeStoreAt("")
	m := NewModel(store)
	if m.InsertMode() {
		t.Fatal("should start in normal mode")
	}
	if p := m.Path(); len(p) != 1 || p[0] != "ai" {
		t.Fatalf("unexpected path: %v", p)
	}
	if m.IsDirty() {
		t.Fatal("AI panel never dirty")
	}
	if got := m.View(); got == "" {
		t.Fatal("view should not be empty")
	}
}
