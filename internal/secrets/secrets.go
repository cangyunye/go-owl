// Package secrets 提供 owl.db 内静态凭据(如 nodes.password/ssh_key)的透明加密。
//
// 开关与同源:进程环境变量 OWL_ENC_KEY(base64 编码的 32 字节 AES-256 密钥)。
// 未设置时 Encrypt/Decrypt 均为透传,写库保持明文(默认关闭,行为不变);
// 对同一份 owl.db 的所有进程(CLI/owl-serve/monitor)使用同一个值即可互读。
//
// 存储格式:enc:v1:<base64(nonce|ciphertext|tag)>。前缀兼作格式版本号:
// 读路径对无前缀的值原样返回,因此存量明文无需迁移即可继续使用。
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Prefix 是加密值的存储前缀,兼作格式版本号。
const Prefix = "enc:v1:"

// KeyEnvName 是加密钥匙的环境变量名。
const KeyEnvName = "OWL_ENC_KEY"

// keyEnvName 是加密钥匙的环境变量名。
const keyEnvName = KeyEnvName

// LoadKey 从环境变量加载 32 字节 AES 密钥。
// 返回 (key, enabled, err):未设置时 enabled=false;格式非法时返回错误。
func LoadKey() ([]byte, bool, error) {
	raw := strings.TrimSpace(os.Getenv(keyEnvName))
	if raw == "" {
		return nil, false, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, true, fmt.Errorf("%s 不是有效的 base64: %w", keyEnvName, err)
	}
	if len(key) != 32 {
		return nil, true, fmt.Errorf("%s 解码后为 %d 字节,需要 32 字节(生成方式: openssl rand -base64 32)", keyEnvName, len(key))
	}
	return key, true, nil
}

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Encrypt 加密明文凭据;未设置 OWL_ENC_KEY 时原样返回明文(加密关闭)。
// 已带前缀的输入原样返回(幂等,避免重复加密)。
func Encrypt(plain string) (string, error) {
	key, enabled, err := LoadKey()
	if err != nil {
		return "", err
	}
	if !enabled || plain == "" {
		return plain, nil
	}
	if strings.HasPrefix(plain, Prefix) {
		return plain, nil
	}
	a, err := gcm(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := a.Seal(nonce, nonce, []byte(plain), nil)
	return Prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 还原 Encrypt 的产物;无前缀的值视为存量明文原样返回。
// 值已加密但本进程未设置 OWL_ENC_KEY、或钥匙不匹配时,返回明确错误。
func Decrypt(stored string) (string, error) {
	if !strings.HasPrefix(stored, Prefix) {
		return stored, nil
	}
	key, enabled, err := LoadKey()
	if err != nil {
		return "", err
	}
	if !enabled {
		return "", fmt.Errorf("凭据已加密(%s...)但未设置 %s,无法解密", stored[:len(Prefix)+8], keyEnvName)
	}
	a, err := gcm(key)
	if err != nil {
		return "", err
	}
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, Prefix))
	if err != nil {
		return "", fmt.Errorf("凭据格式非法: %w", err)
	}
	if len(sealed) < a.NonceSize() {
		return "", errors.New("凭据格式非法: 长度不足")
	}
	plain, err := a.Open(nil, sealed[:a.NonceSize()], sealed[a.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("凭据解密失败,请检查 %s 是否与加密时一致: %w", keyEnvName, err)
	}
	return string(plain), nil
}
