package serve

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func TestServer_ResetAdmin(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "owl.db")
	cfg := &Config{
		DBPath:     dbPath,
		ListenAddr: "127.0.0.1:0",
	}

	srv := NewServer(cfg)
	creds1, err := srv.Init()
	require.NoError(t, err)
	require.NotNil(t, creds1)

	creds2, err := srv.ResetAdmin()
	require.NoError(t, err)
	require.NotNil(t, creds2)
	assert.Equal(t, "admin", creds2.Username)
	assert.Len(t, creds2.Password, 12)
	assert.NotEqual(t, creds1.Password, creds2.Password)

	token1, err := srv.Auth.GenerateToken("admin", "admin")
	require.NoError(t, err)
	_, err = srv.Auth.ValidateToken(token1)
	assert.NoError(t, err)
}

// TestServer_ResetAdmin_NoDB 说明：--reset-admin 的入口（cmd/owl-serve main）
// 会先用 os.Stat 拦截不存在的库，因此正常不会走到 ResetAdmin。
// 若直接调用，ResetAdmin 现在自愈式补齐 schema（web_users + settings），
// 不再因缺 settings 表而报错。
func TestServer_ResetAdmin_NoDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "owl.db")
	cfg := &Config{
		DBPath:     dbPath,
		ListenAddr: "127.0.0.1:0",
	}

	srv := NewServer(cfg)
	creds, err := srv.ResetAdmin()
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "admin", creds.Username)
	assert.Len(t, creds.Password, 12)
}

// TestServer_ResetAdmin_LegacyDB_NoSettingsTable 覆盖旧版本建库场景：
// 库已存在但缺 settings 表（--reset-admin 是给旧库恢复口令的入口）。
// ResetAdmin 必须自行确保 settings 表存在，否则 getOrCreateJWTSecret
// 会报 "no such table: settings"。
func TestServer_ResetAdmin_LegacyDB_NoSettingsTable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "owl.db")
	cfg := &Config{
		DBPath:     dbPath,
		ListenAddr: "127.0.0.1:0",
	}

	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE settings`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	creds, err := srv.ResetAdmin()
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "admin", creds.Username)
	assert.Len(t, creds.Password, 12)
}
