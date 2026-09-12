package regius

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"testing/fstest"

	"github.com/hbarral/regius/i18n"
)

func testI18nFS() fstest.MapFS {
	return fstest.MapFS{
		"en/en.yaml": &fstest.MapFile{
			Data: []byte("en:\n  greeting: \"Hello\"\n  goodbye: \"Goodbye\"\n"),
		},
		"es/es.yaml": &fstest.MapFile{
			Data: []byte("es:\n  greeting: \"Hola\"\n"),
		},
	}
}

func TestI18nConfig_IsSupported(t *testing.T) {
	cfg := I18nConfig{
		DefaultLocale:    "en",
		SupportedLocales: []string{"en", "es"},
	}

	cases := []struct {
		locale string
		want   bool
	}{
		{"en", true},
		{"ES", true},
		{"es-ES", false},
		{"fr", false},
		{"", false},
	}

	for _, c := range cases {
		if got := cfg.IsSupported(c.locale); got != c.want {
			t.Errorf("IsSupported(%q) = %v, want %v", c.locale, got, c.want)
		}
	}
}

func TestLanguage_Disabled(t *testing.T) {
	r := &Regius{ErrorLog: log.New(io.Discard, "", 0)}
	mw := r.Language(I18nConfig{Enabled: false, DefaultLocale: "en"})

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := i18n.Locale(r.Context()); got != i18n.DefaultLocale {
			t.Errorf("expected default locale %q, got %q", i18n.DefaultLocale, got)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestLanguage_Cookie(t *testing.T) {
	if err := i18n.LoadWithDefault(testI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{ErrorLog: log.New(io.Discard, "", 0)}
	cfg := I18nConfig{
		Enabled:          true,
		DefaultLocale:    "en",
		SupportedLocales: []string{"en", "es"},
		CookieName:       "locale",
	}
	mw := r.Language(cfg)

	var locale string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale = i18n.Locale(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "locale", Value: "es"})
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if locale != "es" {
		t.Errorf("expected locale es, got %q", locale)
	}
}

func TestLanguage_AcceptLanguage(t *testing.T) {
	if err := i18n.LoadWithDefault(testI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{ErrorLog: log.New(io.Discard, "", 0)}
	cfg := I18nConfig{
		Enabled:          true,
		DefaultLocale:    "en",
		SupportedLocales: []string{"en", "es"},
		CookieName:       "locale",
	}
	mw := r.Language(cfg)

	var locale string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale = i18n.Locale(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.8")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if locale != "es" {
		t.Errorf("expected locale es from Accept-Language, got %q", locale)
	}
}

func TestLanguage_Default(t *testing.T) {
	if err := i18n.LoadWithDefault(testI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{ErrorLog: log.New(io.Discard, "", 0)}
	cfg := I18nConfig{
		Enabled:          true,
		DefaultLocale:    "en",
		SupportedLocales: []string{"en", "es"},
		CookieName:       "locale",
	}
	mw := r.Language(cfg)

	var locale string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale = i18n.Locale(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if locale != "en" {
		t.Errorf("expected default locale en, got %q", locale)
	}
}

func TestLanguage_UnsupportedFallback(t *testing.T) {
	if err := i18n.LoadWithDefault(testI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{ErrorLog: log.New(io.Discard, "", 0)}
	cfg := I18nConfig{
		Enabled:          true,
		DefaultLocale:    "en",
		SupportedLocales: []string{"en", "es"},
		CookieName:       "locale",
	}
	mw := r.Language(cfg)

	var locale string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale = i18n.Locale(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "locale", Value: "fr"})
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if locale != "en" {
		t.Errorf("expected fallback locale en, got %q", locale)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
