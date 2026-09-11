package monitor

import (
	"database/sql"
	"encoding/base64"
	"testing"

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
