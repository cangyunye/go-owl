package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 运行/步骤状态徽章必须有配色：playbooks.js 用 status-${status} 渲染
// （queued/pending/running/completed/success/failed/cancelled/partial_failure），
// app.css 此前只定义了 online/offline/unknown，运行状态全部渲染为无色文本。
func TestPlaybooksUI_RunStatusBadgePalette(t *testing.T) {
	css := readWebFile(t, "web/css/app.css")

	for _, cls := range []string{
		".status-queued", ".status-pending", ".status-running", ".status-completed",
		".status-success", ".status-failed", ".status-cancelled", ".status-partial_failure",
	} {
		assert.True(t, strings.Contains(css, cls+",") || strings.Contains(css, cls+" {"),
			"app.css must style run-status badge %s", cls)
	}
}
