package regius

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomString_Length(t *testing.T) {
	r := &Regius{}

	for _, n := range []int{0, 1, 5, 16, 32, 100} {
		s := r.RandomString(n)
		assert.Len(t, s, n, "RandomString(%d) should return length %d", n, n)
	}
}

func TestRandomString_Charset(t *testing.T) {
	r := &Regius{}

	allowed := make(map[byte]bool)
	for i := 0; i < len(randomString); i++ {
		allowed[randomString[i]] = true
	}

	s := r.RandomString(2000)
	for i := 0; i < len(s); i++ {
		assert.True(t, allowed[s[i]], "character %q at index %d is outside the allowed charset", s[i], i)
	}
}

func TestRandomString_Uniqueness(t *testing.T) {
	r := &Regius{}

	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		s := r.RandomString(32)
		assert.False(t, seen[s], "RandomString produced a duplicate at iteration %d", i)
		seen[s] = true
	}
}

func TestCreateDirIfNotExist(t *testing.T) {
	r := &Regius{}

	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "c")

	require.NoError(t, r.CreateDirIfNotExist(nested))

	info, err := os.Stat(nested)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Calling again on an existing dir must not error (idempotent).
	assert.NoError(t, r.CreateDirIfNotExist(nested))
}

func TestCreateDirIfNotExist_InvalidPath(t *testing.T) {
	r := &Regius{}

	// A path under a non-existent file (treated as a directory parent) is invalid.
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "regularfile")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0644))

	err := r.CreateDirIfNotExist(filepath.Join(filePath, "subdir"))
	assert.Error(t, err)
}

func TestCreateFileIfNotExists(t *testing.T) {
	r := &Regius{}

	target := filepath.Join(t.TempDir(), "newfile.txt")

	require.NoError(t, r.CreateFileIfNotExists(target))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.False(t, info.IsDir())

	// Idempotent: a second call must not error and must not truncate.
	require.NoError(t, r.CreateFileIfNotExists(target))
}

func TestEncryption_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
		text string
	}{
		{"aes-128", []byte("0123456789abcdef"), "hello world"},
		{"aes-192", []byte("0123456789abcdef01234567"), "secret message"},
		{"aes-256", []byte("0123456789abcdef0123456789abcdef"), "a longer plaintext to encrypt and decrypt"},
		{"empty plaintext", []byte("0123456789abcdef"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Encryption{Key: tt.key}

			cipherText, err := e.Encrypt(tt.text)
			require.NoError(t, err)
			assert.NotEqual(t, tt.text, cipherText, "ciphertext should differ from plaintext (unless empty)")

			plain, err := e.Decrypt(cipherText)
			require.NoError(t, err)
			assert.Equal(t, tt.text, plain)
		})
	}
}

func TestEncryption_RoundTrip_DifferentCiphertexts(t *testing.T) {
	// Each Encrypt uses a random IV, so two encryptions of the same plaintext
	// must yield different ciphertexts.
	e := &Encryption{Key: []byte("0123456789abcdef")}

	c1, err := e.Encrypt("same plaintext")
	require.NoError(t, err)
	c2, err := e.Encrypt("same plaintext")
	require.NoError(t, err)

	assert.NotEqual(t, c1, c2)
}

func TestEncryption_InvalidKey(t *testing.T) {
	e := &Encryption{Key: []byte("too-short")}

	_, err := e.Encrypt("anything")
	assert.Error(t, err, "a non 16/24/32-byte key must fail")

	_, err = e.Decrypt("c29tZXRleHQ=")
	assert.Error(t, err)
}

// TODO: bug — Encryption.Decrypt returns ("", nil) for too-short ciphertext
// instead of a real error. This test pins the current (silent) behavior so the
// behavior change is intentional when the bug is fixed.
func TestDecrypt_ShortInput_ReturnsEmptyNoError(t *testing.T) {
	e := &Encryption{Key: []byte("0123456789abcdef")}

	plain, err := e.Decrypt("")
	assert.NoError(t, err, "current behavior: short input does not surface an error")
	assert.Empty(t, plain)
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	e := &Encryption{Key: []byte("0123456789abcdef")}

	// Not valid base64 — decode is ignored, ct is empty, falls into the
	// short-input branch (same silent-return path as above).
	plain, err := e.Decrypt("!!!not-base64!!!")
	assert.NoError(t, err)
	assert.Empty(t, plain)
}
