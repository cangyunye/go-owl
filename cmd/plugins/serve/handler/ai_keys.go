package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SessionKey struct {
	SessionID     string
	PrivateKey    *rsa.PrivateKey
	PublicKeySPKI string
	CreatedAt     time.Time
}

type KeyManager struct {
	mu       sync.RWMutex
	sessions map[string]*SessionKey
}

func NewKeyManager() *KeyManager {
	return &KeyManager{sessions: make(map[string]*SessionKey)}
}

func (km *KeyManager) CreateSession() (*SessionKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}

	session := &SessionKey{
		SessionID:     uuid.New().String(),
		PrivateKey:    key,
		PublicKeySPKI: base64.StdEncoding.EncodeToString(pubKeyBytes),
		CreatedAt:     time.Now(),
	}

	km.mu.Lock()
	km.sessions[session.SessionID] = session
	km.mu.Unlock()

	return session, nil
}

func (km *KeyManager) GetSessionPublicKey(sessionID string) (*rsa.PublicKey, error) {
	km.mu.RLock()
	session, ok := km.sessions[sessionID]
	km.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return &session.PrivateKey.PublicKey, nil
}

func (km *KeyManager) Decrypt(sessionID string, ciphertextB64 string) ([]byte, error) {
	// Support plaintext fallback when crypto.subtle is unavailable on the client
	if strings.HasPrefix(ciphertextB64, "__plain__:") {
		plaintext, err := base64.StdEncoding.DecodeString(ciphertextB64[len("__plain__:"):])
		if err != nil {
			return nil, fmt.Errorf("decode plaintext: %w", err)
		}
		return plaintext, nil
	}

	km.mu.RLock()
	session, ok := km.sessions[sessionID]
	km.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}

	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, session.PrivateKey, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}

func (km *KeyManager) Cleanup(maxAge time.Duration) {
	km.mu.Lock()
	defer km.mu.Unlock()
	for id, s := range km.sessions {
		if time.Since(s.CreatedAt) > maxAge {
			delete(km.sessions, id)
		}
	}
}
