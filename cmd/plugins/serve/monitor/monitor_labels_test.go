package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestServicesFromLabels 验证节点 label monitor.services（逗号分隔 unit 名）
// 的解析：空白容错、空值/非法 JSON 视为未配置。
func TestServicesFromLabels(t *testing.T) {
	require.Nil(t, servicesFromLabels(""))
	require.Nil(t, servicesFromLabels(`{}`))
	require.Nil(t, servicesFromLabels(`{"monitor.services": ""}`))
	require.Nil(t, servicesFromLabels(`not-json`))
	require.Nil(t, servicesFromLabels(`{"monitor.services": " , ,"}`))

	require.Equal(t, []string{"nginx", "redis"},
		servicesFromLabels(`{"monitor.services": "nginx, redis"}`))
	require.Equal(t, []string{"ssh.service"},
		servicesFromLabels(`{"env":"e2e","monitor.services":"ssh.service"}`))
}
