package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func sign(t *testing.T, secret string, message []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}

func signedRequest(t *testing.T, body string, header, value string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/api/webhooks/test", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if header != "" {
		req.Header.Set(header, value)
	}
	return req
}

func TestVerify_GenericHex(t *testing.T) {
	body := `{"event":"ping"}`
	tests := []struct {
		name     string
		header   string
		value    string
		wantErr  error
		wantBody string
	}{
		{"valid", "X-Signature", sign(t, "secret", []byte(body)), nil, body},
		{"tampered body signature", "X-Signature", sign(t, "secret", []byte(`{"event":"evil"}`)), ErrBadSignature, ""},
		{"wrong secret", "X-Signature", sign(t, "other", []byte(body)), ErrBadSignature, ""},
		{"missing header", "", "", ErrMissingHeader, ""},
		{"malformed hex", "X-Signature", "zz-not-hex", ErrBadSignature, ""},
		{"empty header", "X-Signature", "   ", ErrMissingHeader, ""},
	}

	for _, e := range tests {
		tt := e
		t.Run(tt.name, func(t *testing.T) {
			req := signedRequest(t, body, tt.header, tt.value)
			payload, err := Verify(req, Generic("secret"))

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr == nil && string(payload) != tt.wantBody {
				t.Errorf("expected payload %q, got %q", tt.wantBody, payload)
			}
		})
	}
}

func TestVerify_NoSecret(t *testing.T) {
	req := signedRequest(t, "{}", "X-Signature", "abc")
	_, err := Verify(req, Options{Header: "X-Signature"})
	if !errors.Is(err, ErrNoSecret) {
		t.Fatalf("expected ErrNoSecret, got %v", err)
	}
}

func TestVerify_GitHub(t *testing.T) {
	body := `{"action":"opened"}`
	sig := "sha256=" + sign(t, "ghsecret", []byte(body))

	t.Run("valid", func(t *testing.T) {
		req := signedRequest(t, body, "X-Hub-Signature-256", sig)
		if _, err := Verify(req, GitHub("ghsecret")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("wrong scheme prefix rejected", func(t *testing.T) {
		value := "sha1=" + sign(t, "ghsecret", []byte(body))
		req := signedRequest(t, body, "X-Hub-Signature-256", value)
		if _, err := Verify(req, GitHub("ghsecret")); !errors.Is(err, ErrBadSignature) {
			t.Fatalf("expected ErrBadSignature (sha1= prefix), got %v", err)
		}
	})

	t.Run("legacy sha1 hash", func(t *testing.T) {
		mac := hmac.New(sha1.New, []byte("ghsecret"))
		mac.Write([]byte(body))
		value := "sha1=" + hex.EncodeToString(mac.Sum(nil))

		opts := GitHub("ghsecret")
		opts.Header = "X-Hub-Signature"
		opts.Prefix = "sha1="
		opts.Hash = sha1.New

		req := signedRequest(t, body, "X-Hub-Signature", value)
		if _, err := Verify(req, opts); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestVerify_Stripe(t *testing.T) {
	body := `{"type":"payment_intent.succeeded"}`
	stripeSign := func(t *testing.T, secret, ts, body string) string {
		t.Helper()
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(ts + "." + body))
		return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
	}
	now := strconv.FormatInt(time.Now().Unix(), 10)

	t.Run("valid", func(t *testing.T) {
		value := stripeSign(t, "sk_test", now, body)
		req := signedRequest(t, body, "Stripe-Signature", value)
		if _, err := Verify(req, Stripe("sk_test")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("stale timestamp", func(t *testing.T) {
		stale := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
		value := stripeSign(t, "sk_test", stale, body)
		req := signedRequest(t, body, "Stripe-Signature", value)
		_, err := Verify(req, Stripe("sk_test"))
		if !errors.Is(err, ErrBadTimestamp) {
			t.Fatalf("expected ErrBadTimestamp, got %v", err)
		}
	})

	t.Run("tolerance disabled", func(t *testing.T) {
		stale := strconv.FormatInt(time.Now().Add(-72*time.Hour).Unix(), 10)
		opts := Stripe("sk_test")
		opts.TimestampTolerance = -1
		req := signedRequest(t, body, "Stripe-Signature", stripeSign(t, "sk_test", stale, body))
		if _, err := Verify(req, opts); err != nil {
			t.Fatalf("unexpected error with tolerance disabled: %v", err)
		}
	})

	t.Run("tampered body", func(t *testing.T) {
		value := stripeSign(t, "sk_test", now, `{"type":"evil"}`)
		req := signedRequest(t, body, "Stripe-Signature", value)
		_, err := Verify(req, Stripe("sk_test"))
		if !errors.Is(err, ErrBadSignature) {
			t.Fatalf("expected ErrBadSignature, got %v", err)
		}
	})

	t.Run("missing t", func(t *testing.T) {
		value := "v1=" + sign(t, "sk_test", []byte(now+"."+body))
		req := signedRequest(t, body, "Stripe-Signature", value)
		_, err := Verify(req, Stripe("sk_test"))
		if !errors.Is(err, ErrBadSignature) {
			t.Fatalf("expected ErrBadSignature, got %v", err)
		}
	})

	t.Run("missing v1", func(t *testing.T) {
		req := signedRequest(t, body, "Stripe-Signature", "t="+now)
		_, err := Verify(req, Stripe("sk_test"))
		if !errors.Is(err, ErrBadSignature) {
			t.Fatalf("expected ErrBadSignature, got %v", err)
		}
	})

	t.Run("rotation: second v1 matches new secret", func(t *testing.T) {
		oldSig := strings.SplitN(stripeSign(t, "sk_old", now, body), "v1=", 2)[1]
		newSig := strings.SplitN(stripeSign(t, "sk_new", now, body), "v1=", 2)[1]
		value := "t=" + now + ",v1=" + oldSig + ",v1=" + newSig

		req := signedRequest(t, body, "Stripe-Signature", value)
		if _, err := Verify(req, Stripe("sk_old", "sk_new")); err != nil {
			t.Fatalf("unexpected error during rotation: %v", err)
		}
	})

	t.Run("rotation: old secret still accepted", func(t *testing.T) {
		value := stripeSign(t, "sk_old", now, body)
		req := signedRequest(t, body, "Stripe-Signature", value)
		if _, err := Verify(req, Stripe("sk_new", "sk_old")); err != nil {
			t.Fatalf("unexpected error for old secret: %v", err)
		}
	})
}

func TestVerify_Base64Encoding(t *testing.T) {
	body := `{"event":"ping"}`
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(body))
	value := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	opts := Generic("secret")
	opts.Encoding = "base64"

	req := signedRequest(t, body, "X-Signature", value)
	if _, err := Verify(req, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerify_UnknownEncoding(t *testing.T) {
	req := signedRequest(t, "{}", "X-Signature", "abc")
	opts := Generic("secret")
	opts.Encoding = "rot13"
	_, err := Verify(req, opts)
	if !errors.Is(err, ErrBadSignature) {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestVerify_BodyRestored(t *testing.T) {
	body := `{"event":"ping","nested":{"a":1}}`
	req := signedRequest(t, body, "X-Signature", sign(t, "secret", []byte(body)))

	payload, err := Verify(req, Generic("secret"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reread, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to re-read body: %v", err)
	}
	if !bytes.Equal(reread, payload) {
		t.Errorf("re-read body %q does not match returned payload %q", reread, payload)
	}
	if req.ContentLength != int64(len(body)) {
		t.Errorf("expected ContentLength %d, got %d", len(body), req.ContentLength)
	}
}

func TestVerify_MaxBytesReaderStillEnforced(t *testing.T) {
	body := strings.Repeat("x", 1024)
	req := signedRequest(t, body, "X-Signature", "ignored")
	req.Body = http.MaxBytesReader(nil, req.Body, 64)

	_, err := Verify(req, Generic("secret"))
	if err == nil || errors.Is(err, ErrMissingHeader) || errors.Is(err, ErrBadSignature) {
		t.Fatalf("expected MaxBytesReader error, got %v", err)
	}
}
