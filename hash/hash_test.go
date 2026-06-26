package hash

import (
	"strings"
	"testing"
)

func TestHasher_GenerateAndCompare(t *testing.T) {
	cases := []struct {
		name   string
		hasher Hasher
	}{
		{"bcrypt", testBcrypt},
		{"scrypt", testScrypt},
		{"argon2", testArgon2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hashed, err := tc.hasher.Generate("s3cr3t!")
			if err != nil {
				t.Fatalf("Generate returned error: %v", err)
			}
			if hashed == "" {
				t.Fatal("Generate returned empty hash")
			}
			if strings.Contains(hashed, "s3cr3t!") {
				t.Fatal("hash contains the plain text password")
			}

			ok, err := tc.hasher.Compare(hashed, "s3cr3t!")
			if err != nil {
				t.Fatalf("Compare returned error: %v", err)
			}
			if !ok {
				t.Fatal("Compare returned false for a matching password")
			}
		})
	}
}

func TestHasher_Compare_Mismatch(t *testing.T) {
	cases := []struct {
		name   string
		hasher Hasher
	}{
		{"bcrypt", testBcrypt},
		{"scrypt", testScrypt},
		{"argon2", testArgon2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hashed, err := tc.hasher.Generate("correct-password")
			if err != nil {
				t.Fatalf("Generate returned error: %v", err)
			}

			ok, err := tc.hasher.Compare(hashed, "wrong-password")
			if err != nil {
				t.Fatalf("Compare returned error on mismatch: %v", err)
			}
			if ok {
				t.Fatal("Compare returned true for a non-matching password")
			}
		})
	}
}

func TestHasher_Generate_UniqueSalts(t *testing.T) {
	cases := []struct {
		name   string
		hasher Hasher
	}{
		{"bcrypt", testBcrypt},
		{"scrypt", testScrypt},
		{"argon2", testArgon2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, err := tc.hasher.Generate("same-password")
			if err != nil {
				t.Fatalf("first Generate returned error: %v", err)
			}

			second, err := tc.hasher.Generate("same-password")
			if err != nil {
				t.Fatalf("second Generate returned error: %v", err)
			}

			if first == second {
				t.Fatal("two hashes of the same password are identical; salt not random")
			}

			ok, err := tc.hasher.Compare(first, "same-password")
			if err != nil || !ok {
				t.Fatalf("first hash did not verify: ok=%v err=%v", ok, err)
			}

			ok, err = tc.hasher.Compare(second, "same-password")
			if err != nil || !ok {
				t.Fatalf("second hash did not verify: ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestScryptHasher_Compare_MalformedHash(t *testing.T) {
	cases := []struct {
		name   string
		hasher Hasher
	}{
		{"scrypt", testScrypt},
		{"argon2", testArgon2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := tc.hasher.Compare("not-a-valid-hash", "whatever")
			if err == nil {
				t.Fatal("expected error for malformed hash, got nil")
			}
			if ok {
				t.Fatal("Compare returned true for a malformed hash")
			}
		})
	}
}

func TestBcryptHasher_Compare_MalformedHash(t *testing.T) {
	ok, err := testBcrypt.Compare("not-a-valid-bcrypt-hash", "whatever")
	if err == nil {
		t.Fatal("expected error for malformed bcrypt hash, got nil")
	}
	if ok {
		t.Fatal("Compare returned true for a malformed bcrypt hash")
	}
}

func TestNew_AppliesDefaults(t *testing.T) {
	t.Run("empty_algorithm_defaults_to_bcrypt", func(t *testing.T) {
		h := New(Config{})
		if _, ok := h.(BcryptHasher); !ok {
			t.Fatalf("expected BcryptHasher, got %T", h)
		}
	})

	t.Run("bcrypt_cost_defaulted_when_out_of_range", func(t *testing.T) {
		h := New(Config{Algorithm: AlgorithmBcrypt, Cost: 99})
		bc := h.(BcryptHasher)
		if bc.Cost != 12 {
			t.Fatalf("expected default cost 12, got %d", bc.Cost)
		}
	})

	t.Run("scrypt_defaults", func(t *testing.T) {
		h := New(Config{Algorithm: AlgorithmScrypt})
		sh := h.(ScryptHasher)
		if sh.N != 32768 || sh.R != 8 || sh.P != 1 {
			t.Fatalf("unexpected scrypt defaults: %+v", sh)
		}
	})

	t.Run("argon2_defaults", func(t *testing.T) {
		h := New(Config{Algorithm: AlgorithmArgon2})
		ah := h.(Argon2Hasher)
		if ah.Memory != 65536 || ah.Iterations != 3 || ah.Parallelism != 2 {
			t.Fatalf("unexpected argon2 defaults: %+v", ah)
		}
	})
}

func TestNew_ProducesWorkingHasher(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"bcrypt", Config{Algorithm: AlgorithmBcrypt, Cost: 10}},
		{"scrypt", Config{Algorithm: AlgorithmScrypt, ScryptN: 16384, ScryptR: 8, ScryptP: 1}},
		{"argon2", Config{Algorithm: AlgorithmArgon2, Argon2Memory: 32 * 1024, Argon2Iterations: 1, Argon2Parallelism: 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := New(tc.cfg)
			hashed, err := h.Generate("p@ssword")
			if err != nil {
				t.Fatalf("Generate returned error: %v", err)
			}

			ok, err := h.Compare(hashed, "p@ssword")
			if err != nil || !ok {
				t.Fatalf("Compare failed: ok=%v err=%v", ok, err)
			}
		})
	}
}
