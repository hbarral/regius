package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const routesAPIFixture = `package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *application) ApiRoutes() http.Handler {
	r := chi.NewRouter()

	r.Route("/", func(r chi.Router) {
		// add any API route here
	})

	return r
}
`

// withTempRoot points the CLI's global backend at a fresh temp dir and
// restores it afterwards. It seeds the dir with a routes-api.go fixture.
func withTempRoot(t *testing.T) string {
	t.Helper()

	old := b.RootPath
	dir := t.TempDir()
	b.RootPath = dir
	t.Cleanup(func() { b.RootPath = old })

	if err := os.WriteFile(filepath.Join(dir, "routes-api.go"), []byte(routesAPIFixture), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_NAME=testapp\nKEY=abc\n"), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestDoMakeWebhook_CreatesAndWires(t *testing.T) {
	dir := withTempRoot(t)

	if err := doMakeWebhook("stripe-payment", "stripe"); err != nil {
		t.Fatalf("doMakeWebhook: %v", err)
	}

	handlerPath := filepath.Join(dir, "handlers", "webhook_stripe-payment.go")
	data, err := os.ReadFile(handlerPath)
	if err != nil {
		t.Fatalf("handler file not created: %v", err)
	}
	handler := string(data)

	for _, want := range []string{
		"func (h *Handlers) StripePaymentWebhookRoutes() http.Handler",
		"func (h *Handlers) StripePaymentWebhook(w http.ResponseWriter, r *http.Request)",
		"webhook.Stripe(os.Getenv(\"WEBHOOK_STRIPE_PAYMENT_SECRET\"))",
		`r.Post("/", h.StripePaymentWebhook)`,
	} {
		if !strings.Contains(handler, want) {
			t.Errorf("generated handler missing %q", want)
		}
	}
	if strings.Contains(handler, "$") {
		t.Errorf("generated handler still contains unreplaced placeholder:\n%s", handler)
	}

	routes, err := os.ReadFile(filepath.Join(dir, "routes-api.go"))
	if err != nil {
		t.Fatal(err)
	}
	wantMount := `r.Mount("/webhooks/stripe-payment", a.Handlers.StripePaymentWebhookRoutes())`
	if !strings.Contains(string(routes), wantMount) {
		t.Errorf("routes-api.go missing mount line %q:\n%s", wantMount, routes)
	}
	if !strings.Contains(string(routes), "func(r chi.Router)") {
		t.Errorf("routes-api.go func(_ chi.Router) was not rewritten to func(r chi.Router)")
	}

	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "\nWEBHOOK_STRIPE_PAYMENT_SECRET=") {
		t.Errorf(".env missing WEBHOOK_STRIPE_PAYMENT_SECRET:\n%s", env)
	}
}

func TestDoMakeWebhook_ProviderPresets(t *testing.T) {
	tests := []struct {
		provider string
		preset   string
	}{
		{"generic", "webhook.Generic("},
		{"github", "webhook.GitHub("},
		{"stripe", "webhook.Stripe("},
	}

	for _, e := range tests {
		tt := e
		t.Run(tt.provider, func(t *testing.T) {
			dir := withTempRoot(t)

			if err := doMakeWebhook("hook", tt.provider); err != nil {
				t.Fatalf("doMakeWebhook: %v", err)
			}

			data, err := os.ReadFile(filepath.Join(dir, "handlers", "webhook_hook.go"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tt.preset) {
				t.Errorf("expected preset %q in generated handler", tt.preset)
			}
		})
	}
}

func TestDoMakeWebhook_DefaultsToGeneric(t *testing.T) {
	dir := withTempRoot(t)

	if err := doMakeWebhook("hook", ""); err != nil {
		t.Fatalf("doMakeWebhook: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "handlers", "webhook_hook.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "webhook.Generic(") {
		t.Error("expected generic preset for empty provider")
	}
}

func TestDoMakeWebhook_InvalidProvider(t *testing.T) {
	withTempRoot(t)

	if err := doMakeWebhook("hook", "twilio"); err == nil {
		t.Fatal("expected error for invalid provider")
	}
}

func TestDoMakeWebhook_DuplicateName(t *testing.T) {
	withTempRoot(t)

	if err := doMakeWebhook("hook", "generic"); err != nil {
		t.Fatalf("first doMakeWebhook: %v", err)
	}
	err := doMakeWebhook("hook", "generic")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got %v", err)
	}
}

func TestAppendEnvVar_Idempotent(t *testing.T) {
	dir := withTempRoot(t)
	envPath := filepath.Join(dir, ".env")

	if err := appendEnvVar(envPath, "WEBHOOK_HOOK_SECRET", "first"); err != nil {
		t.Fatalf("appendEnvVar: %v", err)
	}
	if err := appendEnvVar(envPath, "WEBHOOK_HOOK_SECRET", "second"); err != nil {
		t.Fatalf("appendEnvVar: %v", err)
	}

	env, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(env), "second") {
		t.Error("appendEnvVar overwrote an existing value")
	}
	if strings.Count(string(env), "WEBHOOK_HOOK_SECRET=") != 1 {
		t.Errorf("expected exactly one WEBHOOK_HOOK_SECRET entry, got:\n%s", env)
	}
}

func TestInsertRoutesBlock_MarkerAndFallback(t *testing.T) {
	noMarker := strings.Replace(routesAPIFixture, "\t\t// add any API route here\n", "", 1)

	tests := []struct {
		name    string
		fixture string
	}{
		{"marker", routesAPIFixture},
		{"fallback", noMarker},
	}

	for _, e := range tests {
		tt := e
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			routesPath := filepath.Join(dir, "routes-api.go")
			if err := os.WriteFile(routesPath, []byte(tt.fixture), 0644); err != nil {
				t.Fatal(err)
			}

			block := "r.Mount(\"/webhooks/hook\", a.Handlers.HookWebhookRoutes())"
			if err := insertRoutesBlock(routesPath, block); err != nil {
				t.Fatalf("insertRoutesBlock: %v", err)
			}
			// second call must be a no-op
			if err := insertRoutesBlock(routesPath, block); err != nil {
				t.Fatalf("insertRoutesBlock (2nd): %v", err)
			}

			routes, err := os.ReadFile(routesPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(routes), block) != 1 {
				t.Errorf("expected exactly one mount line, got:\n%s", routes)
			}
		})
	}
}
