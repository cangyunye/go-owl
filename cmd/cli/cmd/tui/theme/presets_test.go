package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPresetsComplete(t *testing.T) {
	names := themeNames()
	assert.Equal(t, 5, len(names), "应内置 5 套主题")
	for _, n := range names {
		t.Run(string(n), func(t *testing.T) {
			p, ok := presets[n]
			assert.True(t, ok, "presets 含 %q", n)
			assert.NoError(t, p.Validate(), "主题 %q 数据完整", n)
		})
	}
}

func TestPresetANSIRequired(t *testing.T) {
	for _, n := range themeNames() {
		for _, k := range slotKeys() {
			assert.NotEmpty(t, presets[n].Slots[k].Light.ANSI, "%s/%s/Light.ANSI 必填", n, k)
			assert.NotEmpty(t, presets[n].Slots[k].Dark.ANSI, "%s/%s/Dark.ANSI 必填", n, k)
		}
	}
}
