package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 监控采集默认关闭 + 写入量估算计算器（settings.js 监控卡片）的静态断言。

func TestSettingsJS_MonitorEnabledDefaultsToUnchecked(t *testing.T) {
	src := readWebFile(t, "web/js/pages/settings.js")

	// 旧逻辑：未设置即勾选（enabled === '' || !== 'false'）。拆库后默认关闭，
	// 与服务端 readMonitorEnabled（未设置 = 关闭）对齐。
	assert.False(t, strings.Contains(src, "enabled === '' || enabled.toLowerCase() !== 'false'"),
		"monitor switch must not default to checked when the setting is unset")
	assert.True(t, strings.Contains(src, "enabled.trim().toLowerCase() === 'true'"),
		"monitor switch should be checked only when monitor.enabled is explicitly true")
}

func TestSettingsJS_MonitorEstimateCalculator(t *testing.T) {
	src := readWebFile(t, "web/js/pages/settings.js")

	// 计算器输入与联动
	for _, id := range []string{"mon-est-nodes", "mon-estimate"} {
		assert.True(t, strings.Contains(src, `getElementById('`+id+`')`),
			"settings.js must reference %s", id)
	}
	for _, id := range []string{"mon-interval", "mon-data-retention"} {
		assert.True(t, strings.Contains(src, `'`+id+`', 'mon-est-nodes', 'mon-data-retention'`) ||
			strings.Contains(src, `"`+id+`"`) ||
			strings.Contains(src, `'`+id+`'`),
			"estimate must react to %s changes", id)
	}

	// 估算公式要素：每轮指标数 × 节点数 × 每天轮数 × 每行体积 × 保留天数
	for _, frag := range []string{
		"EST_METRICS_PER_NODE",
		"EST_BYTES_PER_ROW",
		"86400 / Math.max(interval, 10)",
		"nodes * EST_METRICS_PER_NODE * roundsPerDay",
		"bytesPerDay * Math.max(retention, 1)",
	} {
		assert.True(t, strings.Contains(src, frag), "settings.js must contain %q", frag)
	}

	// 启用说明要交代：默认关闭、独立 metrics 库、写入量需评估
	assert.True(t, strings.Contains(src, "默认关闭"),
		"enable hint must state that collection is off by default")
	assert.True(t, strings.Contains(src, "独立的 metrics 库") || strings.Contains(src, "独立 metrics 库"),
		"enable hint must mention the separate metrics database")
	assert.True(t, strings.Contains(src, "写入量估算"),
		"monitor card must expose the write-volume estimator")
}
