package hash

import (
	"os"
	"testing"
)

var (
	testBcrypt BcryptHasher
	testScrypt ScryptHasher
	testArgon2 Argon2Hasher
)

func TestMain(m *testing.M) {
	testBcrypt = BcryptHasher{Cost: 10}
	testScrypt = ScryptHasher{N: 16384, R: 8, P: 1}
	testArgon2 = Argon2Hasher{Memory: 32 * 1024, Iterations: 1, Parallelism: 1}

	os.Exit(m.Run())
}
