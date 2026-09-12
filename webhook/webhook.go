// Package webhook verifies inbound webhook payloads via HMAC signatures.
//
// Webhook providers (Stripe, GitHub, generic HMAC senders) authenticate their
// callbacks by signing the raw request body with a shared secret. Verification
// therefore requires the byte-exact body: it must run before any middleware or
// handler parses, sanitizes, or otherwise rewrites r.Body.
//
// Usage:
//
//	opts := webhook.Stripe(os.Getenv("WEBHOOK_STRIPE_SECRET"))
//	payload, err := webhook.Verify(r, opts)
//	if err != nil {
//	    h.App.WriteAPIError(w, http.StatusUnauthorized, "bad_signature", "invalid webhook signature")
//	    return
//	}
//
// On success, r.Body is restored so downstream code can re-read the payload.
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Errors returned by Verify. They are typed so handlers can map them to
// distinct status codes or log entries. ErrNoSecret signals a configuration
// problem (no secret provided); the others describe the incoming request.
var (
	// ErrNoSecret is returned when Options carries no secrets.
	ErrNoSecret = errors.New("webhook: no secret configured")

	// ErrMissingHeader is returned when the signature header is absent.
	ErrMissingHeader = errors.New("webhook: signature header missing")

	// ErrBadSignature is returned when no provided signature matches the
	// payload, or the header is malformed.
	ErrBadSignature = errors.New("webhook: signature mismatch")

	// ErrBadTimestamp is returned when a signed payload's timestamp falls
	// outside the configured tolerance window (replay protection).
	ErrBadTimestamp = errors.New("webhook: timestamp outside tolerance")
)

// Options configures verification for a provider family. Zero values are
// replaced with defaults by Verify: Header "X-Signature", Encoding "hex",
// sha256 hashing, and (when SignedPayload is set) a 5 minute tolerance.
type Options struct {
	// Secrets holds the shared signing secrets. More than one secret is
	// supported for secret rotation: a payload is accepted when any
	// provided signature matches under any secret.
	Secrets []string

	// Header is the signature header name (default "X-Signature").
	Header string

	// Encoding is how the signature digest is encoded in the header:
	// "hex" (default) or "base64".
	Encoding string

	// Prefix is stripped from the header value before decoding, e.g.
	// "sha256=" for GitHub's X-Hub-Signature-256.
	Prefix string

	// SignedPayload enables Stripe-style headers ("t=<unix>,v1=<sig>[,v1=...]"),
	// where the MAC is computed over "<timestamp>.<body>" instead of the body
	// alone, and the timestamp is checked against TimestampTolerance.
	SignedPayload bool

	// TimestampTolerance rejects signed payloads whose timestamp differs from
	// the current time by more than this duration. Zero applies the 5 minute
	// default; a negative value disables the check. Only applies when
	// SignedPayload is true.
	TimestampTolerance time.Duration

	// Hash builds the hash used by the HMAC. Defaults to sha256; sha1 is
	// allowed for GitHub's legacy X-Hub-Signature header (still sent
	// alongside X-Hub-Signature-256).
	Hash func() hash.Hash
}

// Generic returns options for providers that send a plain HMAC digest of the
// body in an "X-Signature" header, hex-encoded (the most common convention).
func Generic(secret string, moreSecrets ...string) Options {
	return Options{
		Secrets: append([]string{secret}, moreSecrets...),
		Header:  "X-Signature",
	}
}

// GitHub returns options for GitHub webhooks, which sign the body with HMAC
// sha256 and send it as "X-Hub-Signature-256: sha256=<hex>".
func GitHub(secret string, moreSecrets ...string) Options {
	return Options{
		Secrets: append([]string{secret}, moreSecrets...),
		Header:  "X-Hub-Signature-256",
		Prefix:  "sha256=",
	}
}

// Stripe returns options for Stripe webhooks. Stripe sends
// "Stripe-Signature: t=<unix>,v1=<sig>" (one v1 per secret during rotation),
// the MAC is computed over "<t>.<body>", and timestamps older than 5 minutes
// are rejected to blunt replay attacks.
func Stripe(secret string, moreSecrets ...string) Options {
	return Options{
		Secrets:       append([]string{secret}, moreSecrets...),
		Header:        "Stripe-Signature",
		SignedPayload: true,
	}
}

const defaultTolerance = 5 * time.Minute

// Verify reads the raw request body, checks its signature against the
// configured secrets, and returns the payload bytes on success. The request
// body is restored afterwards, so downstream handlers can re-read it (e.g.
// with ReadJSON).
//
// Verify must be called before anything else consumes r.Body. If r.Body is
// wrapped in an http.MaxBytesReader, the limit is still enforced while
// reading.
//
// All comparisons are constant time (crypto/subtle); secrets and the raw
// payload are never part of the returned errors.
func Verify(r *http.Request, opts Options) ([]byte, error) {
	if len(opts.Secrets) == 0 {
		return nil, ErrNoSecret
	}

	header := defaultString(opts.Header, "X-Signature")
	encoding := defaultString(opts.Encoding, "hex")
	hashFn := opts.Hash
	if hashFn == nil {
		hashFn = sha256.New
	}
	tolerance := opts.TimestampTolerance
	if tolerance == 0 {
		tolerance = defaultTolerance
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	closeBody(r, payload)

	value := strings.TrimSpace(r.Header.Get(header))
	if value == "" {
		return nil, ErrMissingHeader
	}

	if opts.Prefix != "" {
		value = strings.TrimSpace(strings.TrimPrefix(value, opts.Prefix))
	}

	if opts.SignedPayload {
		return payload, verifySignedPayload(payload, value, opts.Secrets, hashFn, tolerance)
	}

	expected, err := decodeDigest(value, encoding)
	if err != nil {
		return nil, ErrBadSignature
	}

	for _, secret := range opts.Secrets {
		if matchMAC(hashFn, secret, payload, expected) {
			return payload, nil
		}
	}

	return nil, ErrBadSignature
}

// verifySignedPayload handles Stripe-style "t=<unix>,v1=<sig>[,v1=...]"
// headers. The MAC is computed over "<t>.<body>". Any v1 may match under any
// secret (rotation); the timestamp must be within tolerance when enabled.
func verifySignedPayload(payload []byte, headerValue string, secrets []string, hashFn func() hash.Hash, tolerance time.Duration) error {
	var timestamp string
	var signatures []string

	for _, part := range strings.Split(headerValue, ",") {
		key, val, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "t":
			if timestamp == "" {
				timestamp = strings.TrimSpace(val)
			}
		case "v1":
			signatures = append(signatures, strings.TrimSpace(val))
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return ErrBadSignature
	}

	if tolerance > 0 {
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return ErrBadSignature
		}
		delta := time.Since(time.Unix(ts, 0))
		if delta < 0 {
			delta = -delta
		}
		if delta > tolerance {
			return ErrBadTimestamp
		}
	}

	message := append([]byte(timestamp), append([]byte("."), payload...)...)

	for _, sig := range signatures {
		expected, err := decodeDigest(sig, "hex")
		if err != nil {
			continue
		}
		for _, secret := range secrets {
			if matchMAC(hashFn, secret, message, expected) {
				return nil
			}
		}
	}

	return ErrBadSignature
}

func matchMAC(hashFn func() hash.Hash, secret string, message, expected []byte) bool {
	mac := hmac.New(hashFn, []byte(secret))
	mac.Write(message)
	return hmac.Equal(mac.Sum(nil), expected)
}

func decodeDigest(value, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "", "hex":
		return hex.DecodeString(value)
	case "base64":
		return base64.StdEncoding.DecodeString(value)
	default:
		return nil, ErrBadSignature
	}
}

// closeBody restores r.Body so the payload can be re-read downstream, and
// fixes ContentLength to match the buffered bytes.
func closeBody(r *http.Request, payload []byte) {
	r.Body = io.NopCloser(bytes.NewReader(payload))
	r.ContentLength = int64(len(payload))
}

func defaultString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
