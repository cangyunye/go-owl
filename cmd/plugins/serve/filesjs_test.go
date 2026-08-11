package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readWebFile(t *testing.T, name string) string {
	t.Helper()
	b, err := webFS.ReadFile(name)
	require.NoError(t, err, "read %s", name)
	return string(b)
}

func TestFilesJS_UploadButton_SingleClickTriggersTransfer(t *testing.T) {
	src := readWebFile(t, "web/js/pages/files.js")

	assert.True(t, strings.Contains(src, "upload-btn"), "files.js must reference upload-btn")
	assert.True(t, strings.Contains(src, "handleTransfer('push')"), "upload must trigger handleTransfer('push')")

	for _, btn := range []string{"upload-btn", "download-btn"} {
		assert.False(t, strings.Contains(src, "dblclick"),
			"transfer must not be gated behind double-click for %s", btn)
	}
}
