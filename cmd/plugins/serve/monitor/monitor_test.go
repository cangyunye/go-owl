package monitor

import (
	"database/sql"
	"encoding/base64"
	"testing"

	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/cangyunye/go-owl/internal/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

var encTestKey = base64.StdEncoding.EncodeToString([]byte("owl-enc-test-key-32-bytes-xxxxxx"))

// 采集目标源必须把密文凭据解回明文(monitor 用独立连接池读 nodes 表)。
func TestNodesTargetSource_DecryptsCredentials(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", encTestKey)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	encPw, err := secrets.Encrypt("monitor-pw")
	require.NoError(t, err)
	encKey, err := secrets.Encrypt("MONITOR-KEY")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, password, ssh_key) VALUES ('m1', 'm1', '10.0.0.3', 22, 'root', ?, ?)`, encPw, encKey)
	require.NoError(t, err)

	targets, err := NewNodesTargetSource(db).ListTargets()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, "monitor-pw", targets[0].SSHPassword, "采集目标必须解密")
	assert.Equal(t, "MONITOR-KEY", targets[0].SSHKey)
}

// 采集目标按 address:port 去重：同一台机器登记多个节点（不同用户）时
// 每轮只 SSH 采集一次，优先 user=root 的条目作为代表（问题6）。
func TestNodesTargetSource_DedupsByAddress(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", encTestKey)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	insert := func(id, addr string, port int, user string) {
		_, err := db.Exec(`INSERT INTO nodes (id, name, address, port, user) VALUES (?, ?, ?, ?, ?)`,
			id, id, addr, port, user)
		require.NoError(t, err)
	}
	insert("a1", "10.0.0.5", 22, "deploy")
	insert("a2", "10.0.0.5", 22, "root") // 同地址同端口：应胜出为代表
	insert("b1", "10.0.0.5", 2222, "deploy")
	insert("c1", "10.0.0.9", 22, "ubuntu")
	insert("a3", "10.0.0.5", 22, "admin") // 同地址第三个：仍只保留 root 代表

	targets, err := NewNodesTargetSource(db).ListTargets()
	require.NoError(t, err)
	require.Len(t, targets, 3, "同 address:port 只保留一个采集目标")

	byID := make(map[string]owlmonitor.Target)
	for _, tg := range targets {
		byID[tg.ID] = tg
	}
	rep, ok := byID["a2"]
	require.True(t, ok, "user=root 的条目应成为代表节点")
	assert.Equal(t, "root", rep.User)
	_, ok = byID["a1"]
	assert.False(t, ok, "同地址非 root 条目应被合并")
	_, ok = byID["a3"]
	assert.False(t, ok, "同地址非 root 条目应被合并")
	_, ok = byID["b1"]
	assert.True(t, ok, "不同端口视为不同目标")
	_, ok = byID["c1"]
	assert.True(t, ok, "不同地址独立采集")
}
