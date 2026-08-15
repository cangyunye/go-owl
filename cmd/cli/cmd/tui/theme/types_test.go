package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSlotKeySet(t *testing.T) {
	keys := slotKeys()
	assert.Len(t, keys, 13, "应有 13 个语义槽")
	for _, k := range keys {
		assert.NotEmpty(t, string(k), "SlotKey 不应为空")
	}
}

func TestCompleteColorValidate(t *testing.T) {
	ok := CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"}
	assert.NoError(t, ok.Validate())
	noAnsi := CompleteColor{TrueColor: "#5FD4FF"}
	assert.Error(t, noAnsi.Validate(), "ANSI 必填")
	badHex := CompleteColor{TrueColor: "5FD4FF", ANSI: "14"}
	assert.Error(t, badHex.Validate(), "hex 需 #RRGGBB")
	badAnsi := CompleteColor{TrueColor: "#5FD4FF", ANSI: "1000"}
	assert.Error(t, badAnsi.Validate(), "ANSI 超出 0-255")
}

func TestSlotValidate(t *testing.T) {
	ok := Slot{
		Light: CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"},
		Dark:  CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"},
	}
	assert.NoError(t, ok.Validate())
	assert.Error(t, Slot{Light: ok.Light}.Validate(), "Dark 必填")
}
