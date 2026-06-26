package hash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
)

const (
	AlgorithmBcrypt = "bcrypt"
	AlgorithmScrypt = "scrypt"
	AlgorithmArgon2 = "argon2"

	saltLen = 16
	keyLen  = 32
	encSep  = "$"
)

// Hasher is the contract for password hashing implementations.
type Hasher interface {
	Generate(plain string) (string, error)
	Compare(hashed, plain string) (bool, error)
}

// Config holds the parameters for every supported algorithm. Only the
// fields relevant to the selected Algorithm are used; zero values are
// replaced with safe defaults by New.
type Config struct {
	Algorithm string

	// bcrypt
	Cost int

	// scrypt
	ScryptN int
	ScryptR int
	ScryptP int

	// argon2id
	Argon2Memory      uint32
	Argon2Iterations  uint32
	Argon2Parallelism uint8
}

// New returns the Hasher matching cfg.Algorithm, applying defaults for
// any zero-value parameter. An unrecognized algorithm falls back to
// bcrypt so the application always has a working hasher.
func New(cfg Config) Hasher {
	switch strings.ToLower(cfg.Algorithm) {
	case AlgorithmScrypt:
		return newScryptHasher(cfg)
	case AlgorithmArgon2:
		return newArgon2Hasher(cfg)
	default:
		return newBcryptHasher(cfg)
	}
}

func newBcryptHasher(cfg Config) BcryptHasher {
	cost := cfg.Cost
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = 12
	}
	return BcryptHasher{Cost: cost}
}

func newScryptHasher(cfg Config) ScryptHasher {
	n, r, p := cfg.ScryptN, cfg.ScryptR, cfg.ScryptP
	if n <= 0 {
		n = 32768
	}
	if r <= 0 {
		r = 8
	}
	if p <= 0 {
		p = 1
	}
	return ScryptHasher{N: n, R: r, P: p}
}

func newArgon2Hasher(cfg Config) Argon2Hasher {
	mem, it, par := cfg.Argon2Memory, cfg.Argon2Iterations, cfg.Argon2Parallelism
	if mem == 0 {
		mem = 65536
	}
	if it == 0 {
		it = 3
	}
	if par == 0 {
		par = 2
	}
	return Argon2Hasher{Memory: mem, Iterations: it, Parallelism: par}
}

// BcryptHasher hashes passwords using bcrypt.
type BcryptHasher struct {
	Cost int
}

// Generate returns a bcrypt hash of plain.
func (h BcryptHasher) Generate(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), h.Cost)
	if err != nil {
		return "", fmt.Errorf("hash: bcrypt generate: %w", err)
	}
	return string(b), nil
}

// Compare reports whether hashed verifies against plain.
func (h BcryptHasher) Compare(hashed, plain string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return false, fmt.Errorf("hash: bcrypt compare: %w", err)
	}
	return true, nil
}

// ScryptHasher hashes passwords using scrypt.
type ScryptHasher struct {
	N int
	R int
	P int
}

// Generate returns a scrypt hash encoded as base64(salt)$base64(key).
func (h ScryptHasher) Generate(plain string) (string, error) {
	salt, err := randomSalt()
	if err != nil {
		return "", err
	}

	key, err := scrypt.Key([]byte(plain), salt, h.N, h.R, h.P, keyLen)
	if err != nil {
		return "", fmt.Errorf("hash: scrypt generate: %w", err)
	}

	return encode(salt, key), nil
}

// Compare reports whether hashed verifies against plain.
func (h ScryptHasher) Compare(hashed, plain string) (bool, error) {
	salt, key, err := decode(hashed)
	if err != nil {
		return false, err
	}

	derived, err := scrypt.Key([]byte(plain), salt, h.N, h.R, h.P, keyLen)
	if err != nil {
		return false, fmt.Errorf("hash: scrypt compare: %w", err)
	}

	return subtle.ConstantTimeCompare(key, derived) == 1, nil
}

// Argon2Hasher hashes passwords using argon2id.
type Argon2Hasher struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
}

// Generate returns an argon2id hash encoded as base64(salt)$base64(key).
func (h Argon2Hasher) Generate(plain string) (string, error) {
	salt, err := randomSalt()
	if err != nil {
		return "", err
	}

	key := argon2.IDKey([]byte(plain), salt, h.Iterations, h.Memory, h.Parallelism, keyLen)
	return encode(salt, key), nil
}

// Compare reports whether hashed verifies against plain.
func (h Argon2Hasher) Compare(hashed, plain string) (bool, error) {
	salt, key, err := decode(hashed)
	if err != nil {
		return false, err
	}

	derived := argon2.IDKey([]byte(plain), salt, h.Iterations, h.Memory, h.Parallelism, keyLen)
	return subtle.ConstantTimeCompare(key, derived) == 1, nil
}

func randomSalt() ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("hash: generate salt: %w", err)
	}
	return salt, nil
}

func encode(salt, key []byte) string {
	return base64.StdEncoding.EncodeToString(salt) + encSep + base64.StdEncoding.EncodeToString(key)
}

func decode(hashed string) (salt, key []byte, err error) {
	parts := strings.SplitN(hashed, encSep, 2)
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("hash: invalid encoded hash: %s", strconv.Quote(hashed))
	}

	salt, err = base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("hash: decode salt: %w", err)
	}

	key, err = base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("hash: decode key: %w", err)
	}

	return salt, key, nil
}
