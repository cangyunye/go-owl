package ssh

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/secrets"
)

func testKey(t *testing.T) string {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(k)
}

// 回归:CLI 执行链路(local_source→pool)把库中原始凭据值直接送进 SSH 握手,
// 加密节点会将 enc:v1: 密文当密码用,失败被伪装成"SSH 认证失败",根因被掩盖。
// ResolveConnection 作为 CLI 拨号统一入口必须解密:明文透传(幂等),密文解密,
// 缺/错钥匙返回明确错误(secrets 契约)而不是带错密码去认证。
func TestResolveConnectionDecryptsStoredCredentials(t *testing.T) {
	const pass = "s3cret-pw"
	const keyPath = "/keys/id_rsa"
	// 先设钥匙再产密文,否则 Encrypt 透传得到明文,后续断言全部空转
	key1 := testKey(t)
	t.Setenv(secrets.KeyEnvName, key1)
	encPass, err := secrets.Encrypt(pass)
	if err != nil {
		t.Fatal(err)
	}
	encKey, err := secrets.Encrypt(keyPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("密文+正确钥匙 → 解密", func(t *testing.T) {
		t.Setenv(secrets.KeyEnvName, key1)
		info, err := ResolveConnection("n1", "10.0.0.1", 22, "root", encKey, encPass, "")
		if err != nil {
			t.Fatalf("解密不应失败: %v", err)
		}
		if info.Password != pass || info.KeyFile != keyPath {
			t.Fatalf("凭据未解密: password=%q key=%q", info.Password, info.KeyFile)
		}
	})

	t.Run("密文+缺钥匙 → 明确报错", func(t *testing.T) {
		t.Setenv(secrets.KeyEnvName, "")
		_, err := ResolveConnection("n1", "10.0.0.1", 22, "root", "", encPass, "")
		if err == nil || !strings.Contains(err.Error(), secrets.KeyEnvName) {
			t.Fatalf("应报缺少 OWL_ENC_KEY 的明确错误, 实际: %v", err)
		}
	})

	t.Run("密文+错钥匙 → 明确报错", func(t *testing.T) {
		t.Setenv(secrets.KeyEnvName, testKey(t))
		_, err := ResolveConnection("n1", "10.0.0.1", 22, "root", "", encPass, "")
		if err == nil || !strings.Contains(err.Error(), "解密") {
			t.Fatalf("应报解密失败, 实际: %v", err)
		}
	})

	t.Run("明文透传(存量/legacy 兼容)", func(t *testing.T) {
		t.Setenv(secrets.KeyEnvName, "")
		info, err := ResolveConnection("n1", "10.0.0.1", 22, "root", keyPath, pass, "")
		if err != nil {
			t.Fatal(err)
		}
		if info.Password != pass || info.KeyFile != keyPath {
			t.Fatalf("明文应原样通过: password=%q key=%q", info.Password, info.KeyFile)
		}
	})
}
