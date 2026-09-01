package regius

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"testing/fstest"

	"github.com/hbarral/regius/i18n"
	"github.com/stretchr/testify/assert"
)

func testValidationI18nFS() fstest.MapFS {
	return fstest.MapFS{
		"en/en.yaml": &fstest.MapFile{Data: []byte(`en:
  validation:
    required: "This field cannot be blank"
    email: "Invalid email address"
    min_length: "This field must be at least %{min} characters long"
    failed: "Validation failed for %{field}"
`)},
		"es/es.yaml": &fstest.MapFile{Data: []byte(`es:
  validation:
    required: "Este campo no puede estar vacío"
    email: "Correo electrónico inválido"
    min_length: "Este campo debe tener al menos %{min} caracteres"
`)},
	}
}

func TestValidation_Details(t *testing.T) {
	if err := i18n.LoadWithDefault(testValidationI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{}
	v := r.Validator(url.Values{})
	v.IsEmail("email", "bad")
	v.IsMinLength("name", "A", 2)

	assert.Len(t, v.Details, 2)
	assert.Equal(t, ValidationError{
		Field: "email",
		Key:   "validation.email",
		Msg:   "Invalid email address",
	}, v.Details[0])
	assert.Equal(t, "validation.min_length", v.Details[1].Key)
	assert.Equal(t, map[string]string{"min": "2"}, v.Details[1].Params)
}

func TestValidation_Details_FirstWins(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.IsEmail("email", "bad")
	v.IsInt("email", "abc")

	assert.Len(t, v.Details, 1, "only the first failure per field should be recorded")
	assert.Equal(t, "validation.email", v.Details[0].Key)
}

func TestValidation_AddError_NoDetails(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.AddError("custom", "my own message")

	assert.Len(t, v.Errors, 1)
	assert.Empty(t, v.Details, "AddError must not record a structured detail")
}

func TestValidation_LocalizedErrors(t *testing.T) {
	if err := i18n.LoadWithDefault(testValidationI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{}
	v := r.Validator(url.Values{})
	v.IsEmail("email", "bad")
	v.AddError("custom", "my own message")

	ctx, err := i18n.WithLocale(context.Background(), "es")
	if err != nil {
		t.Fatalf("failed to set locale: %v", err)
	}

	localized := v.LocalizedErrors(ctx)

	assert.Equal(t, "Correo electrónico inválido", localized["email"])
	assert.Equal(t, "my own message", localized["custom"], "custom AddError messages must pass through unchanged")
	assert.Equal(t, "Invalid email address", v.Errors["email"], "LocalizedErrors must not mutate Errors")
}

func TestValidation_LocalizedErrors_Params(t *testing.T) {
	if err := i18n.LoadWithDefault(testValidationI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{}
	v := r.Validator(url.Values{})
	v.IsMinLength("name", "A", 2)

	ctx, err := i18n.WithLocale(context.Background(), "es")
	if err != nil {
		t.Fatalf("failed to set locale: %v", err)
	}

	localized := v.LocalizedErrors(ctx)
	assert.Equal(t, "Este campo debe tener al menos 2 caracteres", localized["name"])
}

func TestValidation_LocalizedErrors_FallbackWithoutLocale(t *testing.T) {
	if err := i18n.LoadWithDefault(testValidationI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{}
	v := r.Validator(url.Values{})
	v.IsEmail("email", "bad")

	// Plain context: no locale attached; must fall back to the English message
	// instead of leaking ctxi18n's "!(MISSING LOCALE)" marker.
	localized := v.LocalizedErrors(context.Background())
	assert.Equal(t, "Invalid email address", localized["email"])
}

func TestValidation_LocalizedErrors_MissingTranslationKey(t *testing.T) {
	if err := i18n.LoadWithDefault(testValidationI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{}
	v := r.Validator(url.Values{})
	// date_iso key is absent from the test locale files; Default fallback applies
	v.IsDateISO("date", "not-a-date")

	ctx, err := i18n.WithLocale(context.Background(), "es")
	if err != nil {
		t.Fatalf("failed to set locale: %v", err)
	}

	localized := v.LocalizedErrors(ctx)
	assert.Equal(t, "This field must be a date in the form of YYYY-MM-DD", localized["date"])
}

func TestValidation_LocalizedErrors_Required(t *testing.T) {
	if err := i18n.LoadWithDefault(testValidationI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{}
	v := r.Validator(url.Values{})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Form = url.Values{"name": {""}}
	v.Required(req, "name")

	ctx, err := i18n.WithLocale(context.Background(), "es")
	if err != nil {
		t.Fatalf("failed to set locale: %v", err)
	}

	localized := v.LocalizedErrors(ctx)
	assert.Equal(t, "Este campo no puede estar vacío", localized["name"])
}

func TestValidation_LocalizedErrors_RuleDefault(t *testing.T) {
	if err := i18n.LoadWithDefault(testValidationI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{}
	v := r.Validator(url.Values{})
	v.Rule("email", "addr", "bad")

	ctx, err := i18n.WithLocale(context.Background(), "es")
	if err != nil {
		t.Fatalf("failed to set locale: %v", err)
	}

	localized := v.LocalizedErrors(ctx)
	assert.Equal(t, "Correo electrónico inválido", localized["addr"])
}

func TestValidation_LocalizedErrors_RuleCustomMessage(t *testing.T) {
	if err := i18n.LoadWithDefault(testValidationI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{}
	v := r.Validator(url.Values{})
	v.Rule("email", "addr", "bad", "custom wording")

	ctx, err := i18n.WithLocale(context.Background(), "es")
	if err != nil {
		t.Fatalf("failed to set locale: %v", err)
	}

	localized := v.LocalizedErrors(ctx)
	assert.Equal(t, "custom wording", localized["addr"], "custom Rule messages must not be translated")
}

type localizedOrder struct {
	ID      string           `validate:"required"`
	Address localizedAddress `validate:"nested"`
}

type localizedAddress struct {
	City string `validate:"required"`
}

func TestValidation_LocalizedErrors_NestedStruct(t *testing.T) {
	if err := i18n.LoadWithDefault(testValidationI18nFS(), "en"); err != nil {
		t.Fatalf("failed to load locales: %v", err)
	}

	r := &Regius{}
	v := r.Validator(url.Values{})
	v.ValidateStruct(localizedOrder{Address: localizedAddress{}})

	ctx, err := i18n.WithLocale(context.Background(), "es")
	if err != nil {
		t.Fatalf("failed to set locale: %v", err)
	}

	localized := v.LocalizedErrors(ctx)
	assert.Equal(t, "Este campo no puede estar vacío", localized["Address.City"])
	assert.Equal(t, "Este campo no puede estar vacío", localized["ID"])
}
