package common

import (
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// OWL_ENC_KEY 测试钥:base64 的 32 字节。
var encTestKey = base64.StdEncoding.EncodeToString([]byte("owl-enc-test-key-32-bytes-xxxxxx"))

// 独立于 setupTestDB:需要裸 db 以核验落库字节。
func setupEncryptTestDB(t *testing.T) (*sql.DB, *NodeStoreDB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1) // :memory: 多连接 = 多个独立库,必须单连接
	t.Cleanup(func() { db.Close() })

	// 隔离:ensureConsistent 会读取并迁移用户真实的 ~/.owl/nodes.json,
	// 测试必须指向空路径,否则计数受外部数据影响。
	origJSONPath := NodeJSONPath
	NodeJSONPath = func() string { return filepath.Join(t.TempDir(), "nodes.json") }
	t.Cleanup(func() { NodeJSONPath = origJSONPath })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '',
		address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 22,
		user TEXT NOT NULL DEFAULT 'root', password TEXT NOT NULL DEFAULT '',
		ssh_key TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'offline',
		groups TEXT NOT NULL DEFAULT '[]', labels TEXT NOT NULL DEFAULT '{}',
		proxy_jump TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_check_at DATETIME)`)
	require.NoError(t, err)
	return db, NewNodeStoreDB(db)
}

// 设了 OWL_ENC_KEY 时,凭据在库里必须是密文,经 store 读出为明文。
func TestNodeStoreDB_CredentialsEncryptedAtRest(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", encTestKey)
	db, store := setupEncryptTestDB(t)

	require.NoError(t, store.Add(&NodeInfo{
		ID: "enc-1", Address: "10.0.0.1", User: "root",
		Password: "pw1", SSHKey: "key1-data",
	}))

	var pw, key string
	require.NoError(t, db.QueryRow(`SELECT password, ssh_key FROM nodes WHERE id = 'enc-1'`).Scan(&pw, &key))
	assert.True(t, strings.HasPrefix(pw, "enc:v1:"), "password 落库应为密文, got %q", pw)
	assert.NotContains(t, pw, "pw1")
	assert.True(t, strings.HasPrefix(key, "enc:v1:"), "ssh_key 落库应为密文, got %q", key)

	got, err := store.Get("enc-1")
	require.NoError(t, err)
	assert.Equal(t, "pw1", got.Password, "store 读取必须解密")
	assert.Equal(t, "key1-data", got.SSHKey)

	nodes, err := store.List()
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "pw1", nodes[0].Password)
}

// 未设 key 时行为完全不变:明文落库、原样读出。
func TestNodeStoreDB_PlaintextWithoutKey(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", "")
	db, store := setupEncryptTestDB(t)

	require.NoError(t, store.Add(&NodeInfo{ID: "p1", Address: "10.0.0.1", Password: "pw1"}))

	var pw string
	require.NoError(t, db.QueryRow(`SELECT password FROM nodes WHERE id = 'p1'`).Scan(&pw))
	assert.Equal(t, "pw1", pw)

	got, err := store.Get("p1")
	require.NoError(t, err)
	assert.Equal(t, "pw1", got.Password)
}

// Update 路径同样加密;读出为明文。
func TestNodeStoreDB_UpdateEncryptedAtRest(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", encTestKey)
	db, store := setupEncryptTestDB(t)

	require.NoError(t, store.Add(&NodeInfo{ID: "u1", Address: "10.0.0.1", Password: "old"}))
	require.NoError(t, store.Update(&NodeInfo{ID: "u1", Address: "10.0.0.1", Password: "new"}))

	var pw string
	require.NoError(t, db.QueryRow(`SELECT password FROM nodes WHERE id = 'u1'`).Scan(&pw))
	assert.True(t, strings.HasPrefix(pw, "enc:v1:"), "更新后应为密文, got %q", pw)
	assert.NotContains(t, pw, "new")

	got, err := store.Get("u1")
	require.NoError(t, err)
	assert.Equal(t, "new", got.Password)
}

// BulkUpsert(导入)同样加密。
func TestNodeStoreDB_BulkUpsertEncryptedAtRest(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", encTestKey)
	db, store := setupEncryptTestDB(t)

	require.NoError(t, store.BulkUpsert([]*NodeInfo{
		{ID: "b1", Address: "10.0.0.1", Password: "bpw", SSHKey: "bkey"},
	}))

	var pw, key string
	require.NoError(t, db.QueryRow(`SELECT password, ssh_key FROM nodes WHERE id = 'b1'`).Scan(&pw, &key))
	assert.True(t, strings.HasPrefix(pw, "enc:v1:"))
	assert.True(t, strings.HasPrefix(key, "enc:v1:"))

	got, err := store.Get("b1")
	require.NoError(t, err)
	assert.Equal(t, "bpw", got.Password)
	assert.Equal(t, "bkey", got.SSHKey)
}

// ReencryptNodeCredentials:明文凭据就地加密,已加密/空值跳过,返回加密行数;
// 未设 key 时拒绝执行。
func TestReencryptNodeCredentials(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", encTestKey)
	db, store := setupEncryptTestDB(t)
	seed := func(id, pw, key string) {
		_, err := db.Exec(`INSERT INTO nodes (id, address, password, ssh_key) VALUES (?, '10.0.0.1', ?, ?)`, id, pw, key)
		require.NoError(t, err)
	}
	seed("r1", "plain-a", "key-a")
	seed("r2", "plain-b", "")
	seed("r3", "", "key-b")
	enc, err := secrets.Encrypt("already") // 已加密行:有前缀,应被跳过
	require.NoError(t, err)
	seed("r4", enc, enc)

	// 未设 key:拒绝执行
	t.Setenv("OWL_ENC_KEY", "")
	_, err = ReencryptNodeCredentials(db)
	assert.Error(t, err, "未设 OWL_ENC_KEY 必须拒绝")

	t.Setenv("OWL_ENC_KEY", encTestKey)
	n, err := ReencryptNodeCredentials(db)
	require.NoError(t, err)
	assert.Equal(t, 4, n, "两行共 4 个明文凭据值需要加密")

	// 库内全部非空值均为密文,且 store 读出为明文
	rows, err := db.Query(`SELECT id, password, ssh_key FROM nodes`)
	require.NoError(t, err)
	defer rows.Close()
	total := 0
	for rows.Next() {
		var id, pw, key string
		require.NoError(t, rows.Scan(&id, &pw, &key))
		if pw != "" {
			assert.True(t, strings.HasPrefix(pw, "enc:v1:"), "%s password 应为密文", id)
			total++
		}
		if key != "" {
			assert.True(t, strings.HasPrefix(key, "enc:v1:"), "%s ssh_key 应为密文", id)
			total++
		}
	}
	assert.Equal(t, 6, total, "r1 两项、r2 密码、r3 ssh_key、r4 两项,共 6 个非空密文")

	got, err := store.Get("r1")
	require.NoError(t, err)
	assert.Equal(t, "plain-a", got.Password)
	assert.Equal(t, "key-a", got.SSHKey)
}
