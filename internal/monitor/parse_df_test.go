package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseDF_Usage 验证 df -P 输出解析为每挂载点使用率。
func TestParseDF_Usage(t *testing.T) {
	raw := `Filesystem     1024-blocks    Used Available Capacity Mounted on
/dev/nvme0n1p2   205113712 85641132 108722116      45% /
/dev/nvme0n1p1      523248    6188    517060       2% /boot/efi
tmpfs              8175832       0   8175832       0% /dev/shm
`

	samples, err := ParseDF(raw, "node-a", 1750000000, "disk.usage")
	require.NoError(t, err)
	require.Equal(t, []Sample{
		{NodeID: "node-a", Metric: "disk.usage./", TS: 1750000000, Value: 45},
		{NodeID: "node-a", Metric: "disk.usage./boot/efi", TS: 1750000000, Value: 2},
		{NodeID: "node-a", Metric: "disk.usage./dev/shm", TS: 1750000000, Value: 0},
	}, samples)
}

// TestParseDF_Inodes 验证 df -Pi 输出解析为每挂载点 inode 使用率。
func TestParseDF_Inodes(t *testing.T) {
	raw := `Filesystem     Inodes IUsed IFree IUse% Mounted on
/dev/nvme0n1p2 12845056 296247 12548809    3% /
`

	samples, err := ParseDF(raw, "node-a", 1750000000, "disk.inodes")
	require.NoError(t, err)
	require.Equal(t, []Sample{
		{NodeID: "node-a", Metric: "disk.inodes./", TS: 1750000000, Value: 3},
	}, samples)
}

// TestParseDF_SkipsUnparseableRows 验证百分比不可解析的行（如虚拟文件系统
// 的 IUse% 为 "-"）被跳过而非整体失败——否则一条坏行会丢弃全部挂载点指标。
func TestParseDF_SkipsUnparseableRows(t *testing.T) {
	raw := `Filesystem       Inodes   IUsed    IFree IUse% Mounted on
none            2036745       5  2036740    1% /usr/lib/modules
drivers              999 -999001  1000000     - /usr/lib/wsl/drivers
/dev/nvme0n1p2 12845056  296247 12548809    3% /
`

	samples, err := ParseDF(raw, "node-a", 1750000000, "disk.inodes")
	require.NoError(t, err)
	require.Equal(t, []Sample{
		{NodeID: "node-a", Metric: "disk.inodes./usr/lib/modules", TS: 1750000000, Value: 1},
		{NodeID: "node-a", Metric: "disk.inodes./", TS: 1750000000, Value: 3},
	}, samples)
}
