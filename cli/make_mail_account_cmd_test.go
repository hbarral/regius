package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempMailRoot points the global backend at a fresh temp dir with a .env
// that already carries an (empty) MAIL_ACCOUNTS key, mirroring the scaffolded
// app template.
func withTempMailRoot(t *testing.T, env string) string {
	t.Helper()

	old := b.RootPath
	dir := t.TempDir()
	b.RootPath = dir
	t.Cleanup(func() { b.RootPath = old })

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

const mailAccountsEnvFixture = "APP_NAME=testapp\nMAIL_DOMAIN=testapp.com\nFROM_NAME=Testapp\nMAIL_ACCOUNTS=\n"

func TestAppendMailAccount_AddsToEmpty(t *testing.T) {
	dir := withTempMailRoot(t, mailAccountsEnvFixture)

	err := appendMailAccount(filepath.Join(dir, ".env"), mailAccount{
		Name:        "info",
		FromAddress: "info@testapp.com",
	})
	if err != nil {
		t.Fatalf("appendMailAccount: %v", err)
	}

	accounts, _, err := readMailAccounts(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("readMailAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].Name != "info" || accounts[0].FromAddress != "info@testapp.com" {
		t.Errorf("unexpected account: %+v", accounts[0])
	}
}

func TestAppendMailAccount_MergesAndPreservesOrder(t *testing.T) {
	dir := withTempMailRoot(t, mailAccountsEnvFixture)
	path := filepath.Join(dir, ".env")

	if err := appendMailAccount(path, mailAccount{Name: "no-reply", FromAddress: "no-reply@testapp.com"}); err != nil {
		t.Fatalf("appendMailAccount(no-reply): %v", err)
	}
	if err := appendMailAccount(path, mailAccount{Name: "info", FromAddress: "info@testapp.com"}); err != nil {
		t.Fatalf("appendMailAccount(info): %v", err)
	}

	accounts, _, err := readMailAccounts(path)
	if err != nil {
		t.Fatalf("readMailAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}
	if accounts[0].Name != "no-reply" || accounts[1].Name != "info" {
		t.Errorf("unexpected order: %+v", accounts)
	}
}

func TestAppendMailAccount_DuplicateName(t *testing.T) {
	dir := withTempMailRoot(t, mailAccountsEnvFixture)
	path := filepath.Join(dir, ".env")

	if err := appendMailAccount(path, mailAccount{Name: "info"}); err != nil {
		t.Fatalf("appendMailAccount: %v", err)
	}
	err := appendMailAccount(path, mailAccount{Name: "INFO"})
	if err == nil || !strings.Contains(err.Error(), "already configured") {
		t.Fatalf("expected already-configured error, got %v", err)
	}
}

func TestAppendMailAccount_NoExistingKey(t *testing.T) {
	dir := withTempMailRoot(t, "APP_NAME=testapp\nKEY=abc\n")

	err := appendMailAccount(filepath.Join(dir, ".env"), mailAccount{
		Name:        "billing",
		FromAddress: "billing@testapp.com",
	})
	if err != nil {
		t.Fatalf("appendMailAccount: %v", err)
	}

	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "\nMAIL_ACCOUNTS='") {
		t.Errorf(".env missing MAIL_ACCOUNTS entry:\n%s", env)
	}
}

func TestAppendMailAccount_InvalidJSON(t *testing.T) {
	dir := withTempMailRoot(t, "APP_NAME=testapp\nMAIL_ACCOUNTS=not-json\n")

	err := appendMailAccount(filepath.Join(dir, ".env"), mailAccount{Name: "info"})
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("expected invalid JSON error, got %v", err)
	}
}

func TestDoMakeMailAccount_DefaultsFromDomain(t *testing.T) {
	dir := withTempMailRoot(t, mailAccountsEnvFixture)
	t.Setenv("MAIL_DOMAIN", "testapp.com")

	// Reset the flag globals so previous tests cannot leak state.
	makeMailAccountFrom = ""
	makeMailAccountFromName = ""
	makeMailAccountDomain = ""
	makeMailAccountHost = ""
	makeMailAccountUsername = ""
	makeMailAccountPassword = ""
	makeMailAccountEncryption = ""
	makeMailAccountAPI = ""
	makeMailAccountAPIKey = ""
	makeMailAccountAPIURL = ""
	makeMailAccountPort = 0

	if err := doMakeMailAccount("no-reply"); err != nil {
		t.Fatalf("doMakeMailAccount: %v", err)
	}

	accounts, _, err := readMailAccounts(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("readMailAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].Name != "no-reply" {
		t.Errorf("unexpected name: %q", accounts[0].Name)
	}
	if accounts[0].FromAddress != "no-reply@testapp.com" {
		t.Errorf("unexpected from address: %q", accounts[0].FromAddress)
	}
}

func TestDoMakeMailAccount_FlagsAndEncoding(t *testing.T) {
	dir := withTempMailRoot(t, mailAccountsEnvFixture)

	makeMailAccountFrom = "info@example.com"
	makeMailAccountHost = "smtp.example.com"
	makeMailAccountPort = 587
	makeMailAccountUsername = "info"
	makeMailAccountPassword = `p"ss`
	makeMailAccountEncryption = "tls"
	makeMailAccountDomain = ""
	makeMailAccountFromName = ""
	makeMailAccountAPI = ""
	makeMailAccountAPIKey = ""
	makeMailAccountAPIURL = ""

	t.Cleanup(func() {
		makeMailAccountFrom = ""
		makeMailAccountHost = ""
		makeMailAccountPort = 0
		makeMailAccountUsername = ""
		makeMailAccountPassword = ""
		makeMailAccountEncryption = ""
	})

	if err := doMakeMailAccount("info"); err != nil {
		t.Fatalf("doMakeMailAccount: %v", err)
	}

	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}

	// The written MAIL_ACCOUNTS value must round-trip through JSON even with
	// special characters in the password.
	accounts, _, err := readMailAccounts(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("readMailAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].Password != `p"ss` {
		t.Errorf("password did not round-trip: %q", accounts[0].Password)
	}
	if accounts[0].Host != "smtp.example.com" || accounts[0].Port != 587 {
		t.Errorf("unexpected transport settings: %+v", accounts[0])
	}

	// Ensure the file content itself is parseable JSON after the = sign.
	line := ""
	for _, l := range strings.Split(string(env), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "MAIL_ACCOUNTS=") {
			line = strings.TrimSpace(l)
			break
		}
	}
	if line == "" {
		t.Fatal("MAIL_ACCOUNTS line not found")
	}
	raw := strings.TrimSpace(strings.TrimPrefix(line, "MAIL_ACCOUNTS="))
	raw = strings.Trim(raw, `"'`)
	var parsed []mailAccount
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("MAIL_ACCOUNTS is not valid JSON: %v\n%s", err, line)
	}
}
