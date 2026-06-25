package regius

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegius_Validator(t *testing.T) {
	r := &Regius{}
	data := url.Values{"name": {"alice"}}

	v := r.Validator(data)

	if assert.NotNil(t, v) {
		assert.Equal(t, data, v.Data)
		assert.NotNil(t, v.Errors)
		assert.Empty(t, v.Errors)
	}
}

func TestValidation_Valid(t *testing.T) {
	r := &Regius{}

	// No errors -> valid
	v := r.Validator(url.Values{})
	assert.True(t, v.Valid())

	// With an error -> invalid
	v.AddError("name", "required")
	assert.False(t, v.Valid())
}

func TestValidation_AddError_FirstWins(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.AddError("email", "first message")
	v.AddError("email", "second message")

	assert.Len(t, v.Errors, 1)
	assert.Equal(t, "first message", v.Errors["email"], "AddError must keep the first message for a key")
}

func TestValidation_Check(t *testing.T) {
	r := &Regius{}

	// ok=true -> no error added
	v := r.Validator(url.Values{})
	v.Check(true, "field", "should not be added")
	assert.Empty(t, v.Errors)

	// ok=false -> error added
	v.Check(false, "field", "must be added")
	assert.Contains(t, v.Errors, "field")
	assert.Equal(t, "must be added", v.Errors["field"])
}

func TestValidation_IsEmail(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid email", "user@example.com", false},
		{"valid email with subdomain", "user@mail.example.com", false},
		{"missing @", "not-an-email", true},
		{"empty", "", true},
		{"missing domain", "user@", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsEmail("email", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "email")
			} else {
				assert.NotContains(t, v.Errors, "email")
			}
		})
	}
}

func TestValidation_IsInt(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"positive", "42", false},
		{"negative", "-7", false},
		{"zero", "0", false},
		{"float", "3.14", true},
		{"text", "abc", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsInt("age", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "age")
			} else {
				assert.NotContains(t, v.Errors, "age")
			}
		})
	}
}

func TestValidation_IsFloat(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"integer", "42", false},
		{"float", "3.14", false},
		{"negative float", "-0.5", false},
		{"text", "abc", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsFloat("price", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "price")
			} else {
				assert.NotContains(t, v.Errors, "price")
			}
		})
	}
}

func TestValidation_IsDateISO(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid iso", "2024-01-31", false},
		{"valid leap day", "2020-02-29", false},
		{"us format", "01/31/2024", true},
		{"european format", "31-01-2024", true},
		{"missing leading zero", "2024-1-1", true},
		{"text", "not-a-date", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsDateISO("date", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "date")
			} else {
				assert.NotContains(t, v.Errors, "date")
			}
		})
	}
}

func TestValidation_NoSpaces(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"no spaces", "nospaces", false},
		{"with space", "has spaces", true},
		{"leading space", " leading", true},
		{"trailing tab", "trailing\t", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.NoSpaces("username", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "username")
			} else {
				assert.NotContains(t, v.Errors, "username")
			}
		})
	}
}

func TestValidation_Required(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	req := httptest.NewRequest("POST", "/", nil)
	req.Form = url.Values{
		"name":  {"alice"},
		"email": {""},
	}

	v.Required(req, "name", "email", "missing")

	assert.Contains(t, v.Errors, "email", "blank field should error")
	assert.Contains(t, v.Errors, "missing", "absent field should error")
	assert.NotContains(t, v.Errors, "name", "present field should not error")
}

func TestValidation_Required_TrimmedWhitespace(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	req := httptest.NewRequest("POST", "/", nil)
	req.Form = url.Values{"name": {"   "}}

	v.Required(req, "name")

	assert.Contains(t, v.Errors, "name", "whitespace-only value should be treated as blank")
}

func TestValidation_Has(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	req := httptest.NewRequest("POST", "/", nil)
	req.Form = url.Values{
		"present": {"value"},
		"empty":   {""},
	}

	assert.True(t, v.Has("present", req))
	assert.False(t, v.Has("empty", req))
	assert.False(t, v.Has("absent", req))
}

func TestValidation_Has_PreservesRequestMethodSemantics(t *testing.T) {
	// Has relies only on r.Form.Get, which works regardless of method.
	r := &Regius{}
	v := r.Validator(url.Values{})

	req := httptest.NewRequest(http.MethodGet, "/?q=hello", nil)
	_ = req.ParseForm()

	assert.True(t, v.Has("q", req))
}
