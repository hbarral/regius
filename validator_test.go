package regius

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
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

func TestValidation_IsURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid http", "http://example.com", false},
		{"valid https", "https://example.com/path?q=1", false},
		{"valid with port", "http://example.com:8080", false},
		{"missing scheme", "example.com", true},
		{"missing host", "http://", true},
		{"plain text", "not a url", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsURL("url", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "url")
			} else {
				assert.NotContains(t, v.Errors, "url")
			}
		})
	}
}

func TestValidation_IsUUID(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid uuid v4", "550e8400-e29b-41d4-a716-446655440000", false},
		{"valid uuid v1", "123e4567-e89b-12d3-a456-426614174000", false},
		{"valid uppercase", "550E8400-E29B-41D4-A716-446655440000", false},
		{"missing dashes", "550e8400e29b41d4a716446655440000", true},
		{"too short", "550e8400", true},
		{"empty", "", true},
		{"text", "not-a-uuid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsUUID("id", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "id")
			} else {
				assert.NotContains(t, v.Errors, "id")
			}
		})
	}
}

func TestValidation_IsPhone(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid with plus", "+1234567890", false},
		{"valid without plus", "1234567890", false},
		{"valid short", "+1234567", false},
		{"valid max length", "+123456789012345", false},
		{"too short", "+123456", true},
		{"too long", "+1234567890123456", true},
		{"with spaces", "+1 234 567 890", true},
		{"with letters", "+1abcd567890", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsPhone("phone", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "phone")
			} else {
				assert.NotContains(t, v.Errors, "phone")
			}
		})
	}
}

func TestValidation_IsCreditCard(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid visa", "4111111111111111", false},
		{"valid mastercard", "5555555555554444", false},
		{"valid amex", "378282246310005", false},
		{"invalid number", "1234567890123456", true},
		{"too short", "123", true},
		{"empty", "", true},
		{"text", "not-a-card", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsCreditCard("card", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "card")
			} else {
				assert.NotContains(t, v.Errors, "card")
			}
		})
	}
}

func TestValidation_IsAlpha(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"letters only", "hello", false},
		{"mixed case", "HelloWorld", false},
		{"with numbers", "hello123", true},
		{"with spaces", "hello world", true},
		{"with symbols", "hello!", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsAlpha("name", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "name")
			} else {
				assert.NotContains(t, v.Errors, "name")
			}
		})
	}
}

func TestValidation_IsAlphanumeric(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"letters and numbers", "hello123", false},
		{"letters only", "hello", false},
		{"numbers only", "12345", false},
		{"with spaces", "hello 123", true},
		{"with symbols", "hello!", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsAlphanumeric("code", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "code")
			} else {
				assert.NotContains(t, v.Errors, "code")
			}
		})
	}
}

func TestValidation_IsNumeric(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"digits only", "12345", false},
		{"single digit", "5", false},
		{"with letters", "12a34", true},
		{"with spaces", "12 34", true},
		{"negative", "-123", true},
		{"float", "3.14", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsNumeric("count", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "count")
			} else {
				assert.NotContains(t, v.Errors, "count")
			}
		})
	}
}

func TestValidation_IsMinLength(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		min     int
		wantErr bool
	}{
		{"exact min", "abcde", 5, false},
		{"above min", "abcdef", 5, false},
		{"below min", "abc", 5, true},
		{"empty", "", 1, true},
		{"unicode runes", "héllo", 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsMinLength("field", tt.value, tt.min)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "field")
			} else {
				assert.NotContains(t, v.Errors, "field")
			}
		})
	}
}

func TestValidation_IsMaxLength(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		max     int
		wantErr bool
	}{
		{"below max", "abc", 5, false},
		{"exact max", "abcde", 5, false},
		{"above max", "abcdef", 5, true},
		{"empty ok", "", 5, false},
		{"unicode runes", "hélloworld", 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsMaxLength("field", tt.value, tt.max)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "field")
			} else {
				assert.NotContains(t, v.Errors, "field")
			}
		})
	}
}

func TestValidation_IsLength(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		n       int
		wantErr bool
	}{
		{"exact length", "abcde", 5, false},
		{"too short", "abc", 5, true},
		{"too long", "abcdef", 5, true},
		{"unicode runes", "héllo", 5, false},
		{"empty wrong", "", 1, true},
		{"empty right", "", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsLength("field", tt.value, tt.n)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "field")
			} else {
				assert.NotContains(t, v.Errors, "field")
			}
		})
	}
}

func TestValidation_IsRange(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		min     int
		max     int
		wantErr bool
	}{
		{"in range", "5", 1, 10, false},
		{"at min", "1", 1, 10, false},
		{"at max", "10", 1, 10, false},
		{"below min", "0", 1, 10, true},
		{"above max", "11", 1, 10, true},
		{"not integer", "abc", 1, 10, true},
		{"empty", "", 1, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsRange("field", tt.value, tt.min, tt.max)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "field")
			} else {
				assert.NotContains(t, v.Errors, "field")
			}
		})
	}
}

func TestValidation_IsJSON(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid object", `{"key":"value"}`, false},
		{"valid array", `[1,2,3]`, false},
		{"valid string", `"hello"`, false},
		{"valid number", "42", false},
		{"valid boolean", "true", false},
		{"valid null", "null", false},
		{"invalid", "{key:value}", true},
		{"empty", "", true},
		{"plain text", "hello", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsJSON("data", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "data")
			} else {
				assert.NotContains(t, v.Errors, "data")
			}
		})
	}
}

func TestValidation_IsIP(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid ipv4", "192.168.1.1", false},
		{"valid ipv6", "::1", false},
		{"valid ipv6 full", "2001:db8::1", false},
		{"invalid", "999.999.999.999", true},
		{"text", "not-an-ip", true},
		{"empty", "", true},
		{"with port", "192.168.1.1:8080", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsIP("ip", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "ip")
			} else {
				assert.NotContains(t, v.Errors, "ip")
			}
		})
	}
}

func TestValidation_IsBoolean(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"true", "true", false},
		{"false", "false", false},
		{"one", "1", false},
		{"zero", "0", false},
		{"yes", "yes", false},
		{"no", "no", false},
		{"TRUE uppercase", "TRUE", false},
		{"Yes mixed", "Yes", false},
		{"maybe", "maybe", true},
		{"empty", "", true},
		{"text", "not-a-bool", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.IsBoolean("flag", tt.value)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "flag")
			} else {
				assert.NotContains(t, v.Errors, "flag")
			}
		})
	}
}

func TestValidation_MatchesPattern(t *testing.T) {
	pattern := regexp.MustCompile(`^[A-Z]{2}-[0-9]{4}$`)

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid", "AB-1234", false},
		{"valid different", "XY-9999", false},
		{"lowercase letters", "ab-1234", true},
		{"too few digits", "AB-123", true},
		{"no dash", "AB1234", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Regius{}
			v := r.Validator(url.Values{})

			v.MatchesPattern("code", tt.value, pattern)

			if tt.wantErr {
				assert.Contains(t, v.Errors, "code")
			} else {
				assert.NotContains(t, v.Errors, "code")
			}
		})
	}
}
