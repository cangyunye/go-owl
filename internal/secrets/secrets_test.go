package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
)

// 测试钥匙:base64("abcdefghijklmnopqrstuvwxyz123456"),32 字节。
const testKey = "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY="

// 核心能力:设置了 OWL_ENC_KEY 时,Encrypt 产出 enc:v1: 前缀密文,Decrypt 可还原。
func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", testKey)

	stored, err := Encrypt("s3cret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(stored, "enc:v1:") {
		t.Fatalf("Encrypt 应产出 enc:v1: 前缀密文, got %q", stored)
	}
	if stored == "s3cret" {
		t.Fatal("密文不得等于明文")
	}

	got, err := Decrypt(stored)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "s3cret" {
		t.Fatalf("Decrypt = %q, want s3cret", got)
	}
}

// 开关语义:未设置 OWL_ENC_KEY 时加密关闭,原样返回明文。
func TestEncrypt_NoKeyReturnsPlaintext(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", "")

	stored, err := Encrypt("plain-password")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if stored != "plain-password" {
		t.Fatalf("未设 key 时应原样返回, got %q", stored)
	}
}

// 存量兼容:无 enc:v1: 前缀的值视为明文,原样返回(存量数据零迁移)。
func TestDecrypt_LegacyPlaintextPassthrough(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", testKey)

	got, err := Decrypt("legacy-plain-password")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "legacy-plain-password" {
		t.Fatalf("明文应原样返回, got %q", got)
	}
}

// 失败语义:读到密文本进程却没有钥匙,必须明确报错(不得把密文当明文密码使用)。
func TestDecrypt_EncryptedWithoutKeyErrors(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", testKey)
	stored, err := Encrypt("s3cret")
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("OWL_ENC_KEY", "")
	_, err = Decrypt(stored)
	if err == nil {
		t.Fatal("未设 key 解密密文必须报错")
	}
	if !strings.Contains(err.Error(), "OWL_ENC_KEY") {
		t.Fatalf("错误信息应提示 OWL_ENC_KEY, got %v", err)
	}
}

// 错误钥匙:GCM 认证失败,必须报错而非返回乱码。
func TestDecrypt_WrongKeyErrors(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", testKey)
	stored, err := Encrypt("s3cret")
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("OWL_ENC_KEY", "a2V5MzItYnl0ZXMtZm9yLXRlc3Qta2V5MzItYnl0ZXM9") // 另一把 32 字节钥
	if _, err := Decrypt(stored); err == nil {
		t.Fatal("错钥解密必须报错")
	}
}

// 空串表示"未设置凭据",不加密、原样返回。
func TestEncrypt_EmptyStaysEmpty(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", testKey)

	stored, err := Encrypt("")
	if err != nil {
		t.Fatal(err)
	}
	if stored != "" {
		t.Fatalf("空串应原样返回, got %q", stored)
	}
}

// ssh_key 是多行 PEM 文本,必须完整往返。
func TestEncryptDecrypt_MultilineSSHKey(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", testKey)
	key := "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\nline3\n-----END OPENSSH PRIVATE KEY-----\n"

	stored, err := Encrypt(key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(stored)
	if err != nil {
		t.Fatal(err)
	}
	if got != key {
		t.Fatalf("多行私钥往返不一致:\n%q\n%q", got, key)
	}
}

// 配置错误:OWL_ENC_KEY 非法(base64 坏/长度错)时必须报错并提示变量名。
func TestEncrypt_InvalidKeyErrors(t *testing.T) {
	t.Setenv("OWL_ENC_KEY", "not-base64!!!")
	if _, err := Encrypt("x"); err == nil || !strings.Contains(err.Error(), "OWL_ENC_KEY") {
		t.Fatalf("坏 base64 应报错并提示变量名, got %v", err)
	}

	short := base64.StdEncoding.EncodeToString([]byte("too-short"))
	t.Setenv("OWL_ENC_KEY", short)
	if _, err := Encrypt("x"); err == nil || !strings.Contains(err.Error(), "32") {
		t.Fatalf("长度不足应报错并提示 32 字节, got %v", err)
	}
}
