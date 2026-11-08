package mailer

import (
	"testing"
)

func TestAccount_ToMail(t *testing.T) {
	acc := Account{
		Name:        "info",
		Domain:      "example.com",
		Host:        "smtp.example.com",
		Port:        587,
		Username:    "info@example.com",
		Password:    "secret",
		Encryption:  "tls",
		FromAddress: "info@example.com",
		FromName:    "Info Team",
	}

	m := acc.ToMail("/app/mail")

	if m.Domain != "example.com" {
		t.Errorf("expected domain example.com, got %q", m.Domain)
	}
	if m.Templates != "/app/mail" {
		t.Errorf("expected templates to fall back to /app/mail, got %q", m.Templates)
	}
	if m.Host != "smtp.example.com" || m.Port != 587 {
		t.Errorf("unexpected transport settings: %s:%d", m.Host, m.Port)
	}
	if m.Username != "info@example.com" || m.Password != "secret" {
		t.Errorf("unexpected credentials")
	}
	if m.FromAddress != "info@example.com" || m.FromName != "Info Team" {
		t.Errorf("unexpected from settings")
	}
	if m.Jobs == nil || m.Results == nil {
		t.Errorf("expected buffered Jobs/Results channels to be created")
	}
}

func TestAccount_ToMail_CustomTemplates(t *testing.T) {
	acc := Account{Templates: "/custom/mail"}
	m := acc.ToMail("/app/mail")
	if m.Templates != "/custom/mail" {
		t.Errorf("expected custom templates dir, got %q", m.Templates)
	}
}

func TestMail_InheritFrom(t *testing.T) {
	base := Mail{
		Domain:     "example.com",
		Host:       "smtp.example.com",
		Port:       587,
		Encryption: "tls",
		FromName:   "Example",
		API:        "sendgrid",
		APIKey:     "key",
		APIUrl:     "https://api.sendgrid.com",
	}

	account := &Mail{
		Username:    "info@example.com",
		Password:    "secret",
		FromAddress: "info@example.com",
	}
	account.InheritFrom(base)

	if account.Host != "smtp.example.com" || account.Port != 587 || account.Encryption != "tls" {
		t.Errorf("transport settings were not inherited: %+v", account)
	}
	if account.Domain != "example.com" || account.FromName != "Example" {
		t.Errorf("domain/from-name were not inherited: %+v", account)
	}
	if account.API != "sendgrid" || account.APIKey != "key" || account.APIUrl != "https://api.sendgrid.com" {
		t.Errorf("API settings were not inherited: %+v", account)
	}
	if account.Username != "info@example.com" || account.Password != "secret" || account.FromAddress != "info@example.com" {
		t.Errorf("identity fields must not be overwritten by inheritance: %+v", account)
	}
}

func TestMail_InheritFrom_DoesNotClobberExplicitValues(t *testing.T) {
	base := Mail{Host: "smtp.default.com", Port: 587, Encryption: "tls"}
	account := &Mail{Host: "smtp.override.com", Port: 2525, Encryption: "none"}
	account.InheritFrom(base)

	if account.Host != "smtp.override.com" || account.Port != 2525 || account.Encryption != "none" {
		t.Errorf("explicit values were clobbered: %+v", account)
	}
}

func TestManager_RegisterAndAccount(t *testing.T) {
	mgr := NewManager()

	defaultMail := &Mail{FromAddress: "default@example.com"}
	if err := mgr.Register("default", defaultMail); err != nil {
		t.Fatalf("register default: %v", err)
	}
	if err := mgr.Register("Info", &Mail{FromAddress: "info@example.com"}); err != nil {
		t.Fatalf("register info: %v", err)
	}

	// Empty name resolves to the default.
	def, err := mgr.Account("")
	if err != nil {
		t.Fatalf("account default: %v", err)
	}
	if def != defaultMail {
		t.Errorf("expected default account to be returned")
	}

	// Names are case-insensitive.
	info, err := mgr.Account("INFO")
	if err != nil {
		t.Fatalf("account INFO: %v", err)
	}
	if info.FromAddress != "info@example.com" {
		t.Errorf("unexpected info account: %q", info.FromAddress)
	}

	if mgr.Default() != defaultMail {
		t.Errorf("Default() should return the first registered account")
	}
}

func TestManager_RegisterErrors(t *testing.T) {
	mgr := NewManager()

	if err := mgr.Register("", &Mail{}); err == nil {
		t.Errorf("expected error for empty name")
	}
	if err := mgr.Register("x", nil); err == nil {
		t.Errorf("expected error for nil mail")
	}
	if err := mgr.Register("x", &Mail{}); err != nil {
		t.Fatalf("register x: %v", err)
	}
	if err := mgr.Register("X", &Mail{}); err == nil {
		t.Errorf("expected error for duplicate name (case-insensitive)")
	}
}

func TestManager_AccountMissing(t *testing.T) {
	mgr := NewManager()
	if _, err := mgr.Account("missing"); err == nil {
		t.Errorf("expected error for missing account")
	}
	if _, err := mgr.Account(""); err == nil {
		t.Errorf("expected error when no default is configured")
	}
}

func TestManager_AccountsCopy(t *testing.T) {
	mgr := NewManager()
	_ = mgr.Register("a", &Mail{})
	_ = mgr.Register("b", &Mail{})

	got := mgr.Accounts()
	if len(got) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(got))
	}
	got["c"] = &Mail{}
	if len(mgr.Accounts()) != 2 {
		t.Errorf("Accounts() should return a copy, not the internal map")
	}
}
