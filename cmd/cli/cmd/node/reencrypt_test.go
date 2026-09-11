package node

import (
	"bytes"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/history"
)

// 命令端到端:明文凭据经 owl node reencrypt 就地加密并输出计数。
func TestReencryptCmd_EndToEnd(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("owl-enc-test-key-32-bytes-xxxxxx"))
	t.Setenv("OWL_ENC_KEY", key)

	gdb, err := history.NewDB(&history.Config{Enabled: true, DBPath: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { gdb.Close() })
	conn := gdb.Connection()

	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '',
		address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 22,
		user TEXT NOT NULL DEFAULT 'root', password TEXT NOT NULL DEFAULT '',
		ssh_key TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'offline',
		groups TEXT NOT NULL DEFAULT '[]', labels TEXT NOT NULL DEFAULT '{}',
		proxy_jump TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO nodes (id, address, password, ssh_key) VALUES ('r1', '10.0.0.1', 'plain-pw', 'plain-key')`); err != nil {
		t.Fatal(err)
	}

	cmd := NewReencryptCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "2") {
		t.Fatalf("应报告加密 2 个凭据, got %q", out.String())
	}

	var pw, keyCol string
	if err := conn.QueryRow(`SELECT password, ssh_key FROM nodes WHERE id = 'r1'`).Scan(&pw, &keyCol); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pw, "enc:v1:") || !strings.HasPrefix(keyCol, "enc:v1:") {
		t.Fatalf("落库应为密文: %q / %q", pw, keyCol)
	}
}
