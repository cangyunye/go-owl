package handler

import (
	"database/sql"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/internal/secrets"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// OWL_ENC_KEY 测试钥:base64 的 32 字节。
var encTestKey = base64.StdEncoding.EncodeToString([]byte("owl-enc-test-key-32-bytes-xxxxxx"))

// 设了 OWL_ENC_KEY 时,经 API 创建的节点凭据在库里必须是密文。
func TestNodeCreate_CredentialEncryptedAtRest(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", encTestKey)
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "POST", "/api/v1/nodes", model.RoleEditor, h.Create)

	w := authRequest(t, router, "POST", "/api/v1/nodes", map[string]interface{}{
		"id": "enc-1", "address": "10.0.0.9", "user": "root",
		"password": "s3cret-pw", "ssh_key": "PRIVATE-KEY-DATA",
	}, "editor")
	require.Equal(t, http.StatusCreated, w.Code)

	var pw, key string
	require.NoError(t, db.QueryRow(`SELECT password, ssh_key FROM nodes WHERE id = 'enc-1'`).Scan(&pw, &key))
	assert.True(t, strings.HasPrefix(pw, "enc:v1:"), "password 落库应为密文, got %q", pw)
	assert.NotContains(t, pw, "s3cret-pw")
	assert.True(t, strings.HasPrefix(key, "enc:v1:"), "ssh_key 落库应为密文, got %q", key)

	// 响应仍不外泄凭据
	assert.NotContains(t, w.Body.String(), "s3cret-pw")
}

// 未设 OWL_ENC_KEY 时行为完全不变:凭据明文落库。
func TestNodeCreate_PlaintextWithoutKey(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", "")
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "POST", "/api/v1/nodes", model.RoleEditor, h.Create)

	w := authRequest(t, router, "POST", "/api/v1/nodes", map[string]interface{}{
		"id": "plain-1", "address": "10.0.0.9", "user": "root", "password": "s3cret-pw",
	}, "editor")
	require.Equal(t, http.StatusCreated, w.Code)

	var pw string
	require.NoError(t, db.QueryRow(`SELECT password FROM nodes WHERE id = 'plain-1'`).Scan(&pw))
	assert.Equal(t, "s3cret-pw", pw)
}

// 读路径:sshExecutor.getNodeInfo 必须把密文解回明文(执行/终端/剧本共用)。
func TestSSHExecutor_GetNodeInfoDecrypts(t *testing.T) {
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

	encPw, err := secrets.Encrypt("decrypted-pw")
	require.NoError(t, err)
	encKey, err := secrets.Encrypt("PRIVATE-KEY")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, address, port, user, password, ssh_key) VALUES ('n1', '10.0.0.1', 22, 'root', ?, ?)`, encPw, encKey)
	require.NoError(t, err)

	info, err := (&sshExecutor{db: db}).getNodeInfo("n1")
	require.NoError(t, err)
	assert.Equal(t, "decrypted-pw", info.Password, "读路径必须解密")
	assert.Equal(t, "PRIVATE-KEY", info.SSHKey)
}

// Update 路径同样加密;不传凭据时列保持原值。
func TestNodeUpdate_CredentialEncryptedAtRest(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", encTestKey)
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "POST", "/api/v1/nodes", model.RoleEditor, h.Create)
	injectRBAC(db, router, "PUT", "/api/v1/nodes/:id", model.RoleEditor, h.Update)

	w := authRequest(t, router, "POST", "/api/v1/nodes", map[string]interface{}{
		"id": "enc-u", "address": "10.0.0.9", "user": "root", "password": "old-pw",
	}, "editor")
	require.Equal(t, http.StatusCreated, w.Code)

	w = authRequest(t, router, "PUT", "/api/v1/nodes/enc-u", map[string]interface{}{
		"name": "renamed", "password": "new-pw",
	}, "editor")
	require.Equal(t, http.StatusOK, w.Code)

	var pw string
	require.NoError(t, db.QueryRow(`SELECT password FROM nodes WHERE id = 'enc-u'`).Scan(&pw))
	assert.True(t, strings.HasPrefix(pw, "enc:v1:"), "更新后应为密文, got %q", pw)
	assert.NotContains(t, pw, "new-pw")

	// 解密后必须是新密码(经读路径验证)
	info, err := (&sshExecutor{db: db}).getNodeInfo("enc-u")
	require.NoError(t, err)
	assert.Equal(t, "new-pw", info.Password)
}

// transfer 的凭据读取点同样解密。
func TestResolveNodeSSH_Decrypts(t *testing.T) {
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

	encPw, err := secrets.Encrypt("transfer-pw")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, address, port, user, password) VALUES ('n1', '10.0.0.2', 22, 'root', ?)`, encPw)
	require.NoError(t, err)

	info, err := resolveNodeSSH(db, "n1")
	require.NoError(t, err)
	assert.Equal(t, "transfer-pw", info.Password)
}

// 共享解密 helper:无 key 时必须报错(与 secrets 语义一致,逐站点接线共用)。
func TestDecryptNodeSSHInfo(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", encTestKey)
	encPw, err := secrets.Encrypt("helper-pw")
	require.NoError(t, err)

	info := &nodeSSHInfo{Password: encPw, SSHKey: "plain-key"}
	require.NoError(t, decryptNodeSSHInfo(info))
	assert.Equal(t, "helper-pw", info.Password)
	assert.Equal(t, "plain-key", info.SSHKey)

	t.Setenv("OWL_ENC_KEY", "")
	err = decryptNodeSSHInfo(&nodeSSHInfo{Password: encPw})
	assert.Error(t, err, "无 key 解密密文凭据必须报错")
}
