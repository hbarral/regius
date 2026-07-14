package urlsigner

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateTokenFromString_WithQueryString(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret-key")}

	token := s.GenerateTokenFromString("https://example.com/path?foo=bar")

	assert.NotEmpty(t, token)
	assert.True(t, strings.Contains(token, "https://example.com/path?foo=bar&hash="))
}

func TestGenerateTokenFromString_WithoutQueryString(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret-key")}

	token := s.GenerateTokenFromString("https://example.com/path")

	assert.NotEmpty(t, token)
	assert.True(t, strings.Contains(token, "https://example.com/path?hash="))
}

func TestGenerateTokenFromString_DifferentInputsProduceDifferentTokens(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret-key")}

	token1 := s.GenerateTokenFromString("https://example.com/one")
	token2 := s.GenerateTokenFromString("https://example.com/two")

	assert.NotEqual(t, token1, token2)
}

func TestVerifyToken_ValidToken(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret-key")}
	token := s.GenerateTokenFromString("https://example.com/path")

	assert.True(t, s.VerifyToken(token))
}

func TestVerifyToken_InvalidWithWrongSecret(t *testing.T) {
	signer1 := &Signer{Secret: []byte("secret-one")}
	token := signer1.GenerateTokenFromString("https://example.com/path")

	signer2 := &Signer{Secret: []byte("secret-two")}
	assert.False(t, signer2.VerifyToken(token))
}

func TestVerifyToken_CorruptedToken(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret-key")}
	token := s.GenerateTokenFromString("https://example.com/path")

	corrupted := token[:len(token)-1] + "X"
	assert.False(t, s.VerifyToken(corrupted))
}

func TestVerifyToken_EmptyToken(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret-key")}
	assert.False(t, s.VerifyToken(""))
}

func TestExpired_NotExpired(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret-key")}
	token := s.GenerateTokenFromString("https://example.com/path")

	assert.False(t, s.Expired(token, 60))
}

func TestExpired_ZeroMinutes(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret-key")}
	token := s.GenerateTokenFromString("https://example.com/path")

	// A token generated in the same instant has time.Since > 0, so it is
	// immediately expired when minutesUntilExpire is 0.
	assert.True(t, s.Expired(token, 0))
}

func TestExpired_NegativeMinutes(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret-key")}
	token := s.GenerateTokenFromString("https://example.com/path")

	assert.True(t, s.Expired(token, -1))
}
