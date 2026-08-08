package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordHashAndVerify(t *testing.T) {
	auth := NewAuthService("test-secret-key-32-bytes-long!!")

	hash, err := auth.HashPassword("my-password")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "my-password", hash)

	assert.True(t, auth.VerifyPassword(hash, "my-password"))
	assert.False(t, auth.VerifyPassword(hash, "wrong-password"))
	assert.False(t, auth.VerifyPassword("$2a$invalid", "anything"))
}

func TestGenerateToken(t *testing.T) {
	auth := NewAuthService("test-secret-key-32-bytes-long!!")

	token, err := auth.GenerateToken("admin", "admin")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestValidateToken(t *testing.T) {
	auth := NewAuthService("test-secret-key-32-bytes-long!!")

	token, _ := auth.GenerateToken("admin", "admin")

	claims, err := auth.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "admin", claims.Username)
	assert.Equal(t, "admin", claims.Role)
}

func TestValidateToken_Expired(t *testing.T) {
	auth := &AuthService{
		secret:   []byte("test-secret-key-32-bytes-long!!"),
		TokenExpiry: -1 * time.Hour,
	}

	token, _ := auth.GenerateToken("admin", "admin")
	_, err := auth.ValidateToken(token)
	assert.Error(t, err)
}

func TestValidateToken_Invalid(t *testing.T) {
	auth := NewAuthService("test-secret-key-32-bytes-long!!")

	_, err := auth.ValidateToken("invalid.token.here")
	assert.Error(t, err)
}
