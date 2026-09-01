package regius

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/hbarral/regius/api"
	"github.com/stretchr/testify/assert"
)

type signupInput struct {
	Name  string `validate:"required,min=2"`
	Email string `validate:"required,email"`
	Age   int    `validate:"min=18"`
}

func jsonRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func formRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func decodeAPIError(t *testing.T, rec *httptest.ResponseRecorder) api.Response {
	t.Helper()
	var resp api.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error envelope: %v", err)
	}
	return resp
}

func TestValidateRequest_JSON_Valid(t *testing.T) {
	r := &Regius{}
	mw := r.ValidateRequest(ValidationConfig{StructType: signupInput{}})

	var got *signupInput
	nextCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		nextCalled = true
		got, _ = ValidatedFromContext[*signupInput](req.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, jsonRequest(http.MethodPost, "/signup",
		`{"name":"Alice","email":"alice@example.com","age":30}`))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, nextCalled)
	if assert.NotNil(t, got) {
		assert.Equal(t, "Alice", got.Name)
		assert.Equal(t, "alice@example.com", got.Email)
		assert.Equal(t, 30, got.Age)
	}
}

func TestValidateRequest_JSON_InvalidFields(t *testing.T) {
	r := &Regius{}
	mw := r.ValidateRequest(ValidationConfig{StructType: signupInput{}})

	nextCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		nextCalled = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, jsonRequest(http.MethodPost, "/signup",
		`{"name":"A","email":"bad","age":10}`))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, nextCalled, "next handler must not run on validation failure")

	resp := decodeAPIError(t, rec)
	if assert.NotNil(t, resp.Error) {
		assert.Equal(t, "validation_error", resp.Error.Code)
		details, ok := resp.Error.Details.(map[string]interface{})
		if assert.True(t, ok, "details must be a field->message map") {
			assert.Equal(t, "This field must be at least 2 characters long", details["Name"])
			assert.Equal(t, "Invalid email address", details["Email"])
			assert.Equal(t, "This field must be at least 18", details["Age"])
		}
	}
}

func TestValidateRequest_JSON_MalformedBody(t *testing.T) {
	r := &Regius{}
	mw := r.ValidateRequest(ValidationConfig{StructType: signupInput{}})

	nextCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		nextCalled = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, jsonRequest(http.MethodPost, "/signup", `{not json`))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, nextCalled)
	resp := decodeAPIError(t, rec)
	assert.Equal(t, "invalid_json", resp.Error.Code)
}

func TestValidateRequest_JSON_NonJSONContentType(t *testing.T) {
	r := &Regius{}
	mw := r.ValidateRequest(ValidationConfig{StructType: signupInput{}})

	nextCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		nextCalled = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, formRequest(http.MethodPost, "/signup", "name=Alice"))

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	assert.False(t, nextCalled)
	resp := decodeAPIError(t, rec)
	assert.Equal(t, "unsupported_media_type", resp.Error.Code)
}

func TestValidateRequest_Form_Valid(t *testing.T) {
	r := &Regius{}
	mw := r.ValidateRequest(ValidationConfig{
		Rules: map[string]string{
			"email":    "required,email",
			"password": "required,min=8",
		},
	})

	nextCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		nextCalled = true
		_, ok := ValidatedFromContext[*signupInput](req.Context())
		assert.False(t, ok, "no struct is stored in Rules mode")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, formRequest(http.MethodPost, "/login",
		"email=alice@example.com&password=secretpass"))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, nextCalled)
}

func TestValidateRequest_Form_Invalid(t *testing.T) {
	r := &Regius{}
	mw := r.ValidateRequest(ValidationConfig{
		Rules: map[string]string{
			"email":    "required,email",
			"password": "required,min=8",
		},
	})

	nextCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		nextCalled = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, formRequest(http.MethodPost, "/login",
		"email=bad&password=short"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, nextCalled)
	resp := decodeAPIError(t, rec)
	details, ok := resp.Error.Details.(map[string]interface{})
	if assert.True(t, ok) {
		assert.Equal(t, "Invalid email address", details["email"])
		assert.Equal(t, "This field must be at least 8 characters long", details["password"])
	}
}

func TestValidateRequest_Form_Redirect(t *testing.T) {
	sm := scs.New()
	r := &Regius{Session: sm}
	mw := r.ValidateRequest(ValidationConfig{
		Rules: map[string]string{
			"email": "required,email",
		},
		ErrorFormat: "form",
	})

	nextCalled := false
	handler := sm.LoadAndSave(mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		nextCalled = true
	})))

	req := formRequest(http.MethodPost, "/login", "email=bad")
	req.Header.Set("Referer", "/login")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
	assert.False(t, nextCalled)

	// Follow the session cookie and confirm the errors were stored.
	req2 := httptest.NewRequest(http.MethodGet, "/login", nil)
	for _, c := range rec.Result().Cookies() {
		req2.AddCookie(c)
	}

	var popped map[string]string
	handler2 := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		popped = r.PopValidationErrors(req.Context())
	}))
	handler2.ServeHTTP(httptest.NewRecorder(), req2)

	if assert.NotNil(t, popped) {
		assert.Equal(t, "Invalid email address", popped["email"])
	}
}

func TestValidateRequest_Form_RedirectFallbackTarget(t *testing.T) {
	sm := scs.New()
	r := &Regius{Session: sm}
	mw := r.ValidateRequest(ValidationConfig{
		Rules:       map[string]string{"email": "required,email"},
		ErrorFormat: "form",
		RedirectTo:  "/login?retry=1",
	})
	handler := sm.LoadAndSave(mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {})))

	// No Referer header: falls back to RedirectTo.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, formRequest(http.MethodPost, "/login", "email=bad"))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login?retry=1", rec.Header().Get("Location"))

	// No Referer and no RedirectTo: falls back to "/".
	mw2 := r.ValidateRequest(ValidationConfig{
		Rules:       map[string]string{"email": "required,email"},
		ErrorFormat: "form",
	})
	handler2 := sm.LoadAndSave(mw2(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {})))
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, formRequest(http.MethodPost, "/login", "email=bad"))
	assert.Equal(t, "/", rec2.Header().Get("Location"))
}

func TestValidateRequest_RequiresStructTypeOrRules(t *testing.T) {
	r := &Regius{}

	assert.PanicsWithValue(t, "regius: ValidateRequest requires StructType or Rules", func() {
		r.ValidateRequest(ValidationConfig{})
	})
}

func TestValidateRequest_StructTypeMustBeStruct(t *testing.T) {
	r := &Regius{}

	assert.PanicsWithValue(t, "regius: ValidateRequest StructType must be a struct", func() {
		r.ValidateRequest(ValidationConfig{StructType: 42})
	})
}

func TestValidateRequest_StructTypeVariants(t *testing.T) {
	r := &Regius{}

	variants := []interface{}{
		signupInput{},       // sample value
		(*signupInput)(nil), // typed nil pointer
		&signupInput{},      // non-nil pointer
	}

	for _, sample := range variants {
		assert.NotPanics(t, func() {
			mw := r.ValidateRequest(ValidationConfig{StructType: sample})
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {}))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, jsonRequest(http.MethodPost, "/x",
				`{"name":"Alice","email":"alice@example.com","age":30}`))
			assert.Equal(t, http.StatusOK, rec.Code)
		}, "all StructType forms must behave identically")
	}
}

func TestValidateRequest_ValidatedFromContextMissing(t *testing.T) {
	_, ok := ValidatedFromContext[*signupInput](context.Background())
	assert.False(t, ok)
}
