package store

import (
	"testing"

	commonmodel "github.com/cangyunye/go-owl/internal/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 节点行扫描收敛：nodes 表「groups/labels JSON 列解析 + 非法归一 nil」模式
// 重复于 store/node.go、handler/playbook_engine.go、handler/aiexecutor_data.go、
// handler/ai_node.go、handler/node_source.go、monitor/monitor.go 等处，
// 统一收敛到 store 包的 ParseNodeGroups / ParseNodeLabels / ScanNodeRow。

func TestParseNodeGroups(t *testing.T) {
	assert.Equal(t, []string{"web", "db"}, ParseNodeGroups(`["web","db"]`))
	assert.Equal(t, []string{"cache"}, ParseNodeGroups(`["cache"]`))
	// 非法/空/类型不匹配一律归一为 nil
	assert.Nil(t, ParseNodeGroups(""))
	assert.Nil(t, ParseNodeGroups("not-json"))
	assert.Nil(t, ParseNodeGroups(`{"a":1}`))
	// 合法空数组保持空切片（与既有 json.Unmarshal 行为一致）
	assert.Empty(t, ParseNodeGroups(`[]`))
}

func TestParseNodeLabels(t *testing.T) {
	assert.Equal(t, map[string]string{"env": "prod"}, ParseNodeLabels(`{"env":"prod"}`))
	// 非法/空/类型不匹配一律归一为 nil
	assert.Nil(t, ParseNodeLabels(""))
	assert.Nil(t, ParseNodeLabels("not-json"))
	assert.Nil(t, ParseNodeLabels(`["a"]`))
	// 合法空对象保持空 map（与既有 json.Unmarshal 行为一致）
	assert.Empty(t, ParseNodeLabels(`{}`))
}

// TestScanNodeRow 规范列清单（NodeRowColumns）+ 整行扫描：字段映射正确，
// groups/labels 非法 JSON 与 NULL 一律归一为 nil，不让脏数据炸掉整页列表。
func TestScanNodeRow(t *testing.T) {
	db := openTestDB(t)
	_, err := db.Exec(`
		CREATE TABLE nodes (
			id TEXT PRIMARY KEY,
			name TEXT, address TEXT, port INTEGER, user TEXT, status TEXT,
			groups TEXT, labels TEXT
		)`)
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO nodes (id, name, address, port, user, status, groups, labels) VALUES
		('n-ok',  'OK',      '10.0.0.1', 22,    'root', 'online',  '["web"]',        '{"env":"prod"}'),
		('n-bad', 'BadJSON', '10.0.0.2', 2222,  'ops',  'unknown', 'not-json',       'also-bad'),
		('n-null','NullCols','10.0.0.3', NULL,  NULL,   NULL,      NULL,             NULL)`)
	require.NoError(t, err)

	rows, err := db.Query(`SELECT ` + NodeRowColumns + ` FROM nodes ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()

	byID := map[string]*commonmodel.Node{}
	for rows.Next() {
		n, err := ScanNodeRow(rows)
		require.NoError(t, err)
		byID[n.ID] = n
	}
	require.NoError(t, rows.Err())

	okNode, exists := byID["n-ok"]
	require.True(t, exists)
	assert.Equal(t, "OK", okNode.Name)
	assert.Equal(t, "10.0.0.1", okNode.Address)
	assert.Equal(t, 22, okNode.Port)
	assert.Equal(t, "root", okNode.User)
	assert.Equal(t, []string{"web"}, okNode.Groups)
	assert.Equal(t, map[string]string{"env": "prod"}, okNode.Labels)

	bad := byID["n-bad"]
	require.NotNil(t, bad)
	assert.Equal(t, 2222, bad.Port)
	assert.Nil(t, bad.Groups, "非法 groups JSON 应归一为 nil")
	assert.Nil(t, bad.Labels, "非法 labels JSON 应归一为 nil")

	null := byID["n-null"]
	require.NotNil(t, null)
	assert.Equal(t, "NullCols", null.Name)
	assert.Equal(t, 22, null.Port, "NULL port 应归一为默认 22")
	// NULL 列经 COALESCE 归一为合法空集合
	assert.Empty(t, null.Groups)
	assert.Empty(t, null.Labels)
}
