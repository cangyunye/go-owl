package serve

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestServer_ResetAdmin_NoDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "owl.db")
	cfg := &Config{
		DBPath:     dbPath,
		ListenAddr: "127.0.0.1:0",
	}

	srv := NewServer(cfg)
	_, err := srv.ResetAdmin()
	assert.Error(t, err)
}
