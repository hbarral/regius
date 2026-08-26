package regius

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func BenchmarkGenerateRequestID_UUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateRequestID(RequestIDFormatUUID)
	}
}

func BenchmarkGenerateRequestID_XID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateRequestID(RequestIDFormatXID)
	}
}

func BenchmarkGenerateRequestID_Short(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateRequestID(RequestIDFormatShort)
	}
}

func BenchmarkGenerateRequestID_Default(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateRequestID(RequestIDFormatDefault)
	}
}

func BenchmarkRandomString_32(b *testing.B) {
	r := &Regius{}
	for i := 0; i < b.N; i++ {
		_ = r.RandomString(32)
	}
}

func BenchmarkEncryption_Encrypt(b *testing.B) {
	e := &Encryption{Key: []byte("0123456789abcdef")}
	for i := 0; i < b.N; i++ {
		_, _ = e.Encrypt("a moderately sized secret payload")
	}
}

func BenchmarkEncryption_Decrypt(b *testing.B) {
	e := &Encryption{Key: []byte("0123456789abcdef")}
	cipher, _ := e.Encrypt("a moderately sized secret payload")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = e.Decrypt(cipher)
	}
}

func BenchmarkValidation_IsEmail(b *testing.B) {
	r := &Regius{}
	v := r.Validator(url.Values{})
	for i := 0; i < b.N; i++ {
		v.IsEmail("email", "user@example.com")
	}
}

func BenchmarkValidation_IsInt(b *testing.B) {
	r := &Regius{}
	v := r.Validator(url.Values{})
	for i := 0; i < b.N; i++ {
		v.IsInt("age", "42")
	}
}

func BenchmarkValidation_IsDateISO(b *testing.B) {
	r := &Regius{}
	v := r.Validator(url.Values{})
	for i := 0; i < b.N; i++ {
		v.IsDateISO("date", "2024-01-31")
	}
}

func BenchmarkValidation_IsUUID(b *testing.B) {
	r := &Regius{}
	v := r.Validator(url.Values{})
	for i := 0; i < b.N; i++ {
		v.IsUUID("id", "550e8400-e29b-41d4-a716-446655440000")
	}
}

func BenchmarkValidation_IsURL(b *testing.B) {
	r := &Regius{}
	v := r.Validator(url.Values{})
	for i := 0; i < b.N; i++ {
		v.IsURL("url", "https://example.com/path?q=1")
	}
}

func BenchmarkSanitize_Strict(b *testing.B) {
	input := "<p>Hello <script>alert('xss')</script>world</p>"
	for i := 0; i < b.N; i++ {
		_ = Sanitize(input)
	}
}

func BenchmarkSanitize_UGC(b *testing.B) {
	input := "<p>Hello <script>alert('xss')</script><b>world</b></p>"
	policy := ugcSanitizePolicy()
	for i := 0; i < b.N; i++ {
		_ = policy.Sanitize(input)
	}
}

func BenchmarkClientIPAddress(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = clientIPAddress(req, true)
	}
}

func BenchmarkWriteJSON(b *testing.B) {
	r := &Regius{}
	data := map[string]interface{}{
		"status": "ok",
		"items":  []int{1, 2, 3, 4, 5},
	}
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		_ = r.WriteJSON(w, http.StatusOK, data)
	}
}
