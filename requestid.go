package regius

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/rs/xid"
)

// RequestIDConfig holds configuration for request ID tracing.
//
// When Enabled is true, the middleware stamps every request with a unique
// request ID: it reuses an incoming ID from the request header when present
// (so requests can be correlated across services and proxies) and otherwise
// generates one using the configured Format or Generator. The ID is stored
// in the request context — under the same key as chi's middleware.RequestID,
// so middleware.GetReqID and chi's request logger keep working — and is
// echoed back on the response via ResponseHeader.
type RequestIDConfig struct {
	Enabled bool

	// Header is the request header from which an incoming request ID is
	// read (default "X-Request-ID"). When present and non-empty, the
	// incoming value is reused verbatim.
	Header string

	// ResponseHeader is the response header the request ID is written to
	// (default "X-Request-ID"). It is set before invoking the downstream
	// handler, so a route may still override it via w.Header().Set(...).
	// Set to "" to disable echoing the ID on the response.
	ResponseHeader string

	// Format selects the generated ID format when no incoming ID is
	// present and Generator is nil: "uuid" (default), "xid", "short",
	// or "default" (chi-style host/random-counter).
	Format string

	// Generator, when set, overrides Format and produces the request ID.
	// Useful for custom ID schemes (e.g. ULID, tenant-prefixed IDs).
	Generator func() string
}

// Request ID format constants.
const (
	RequestIDFormatUUID    = "uuid"
	RequestIDFormatXID     = "xid"
	RequestIDFormatShort   = "short"
	RequestIDFormatDefault = "default"
)

const (
	defaultRequestIDHeader = "X-Request-ID"
	// maxRequestIDLength caps the length of an accepted incoming request
	// ID. Longer values are treated as missing (and a fresh ID generated)
	// to prevent log injection and header abuse.
	maxRequestIDLength = 128
)

// RequestIDFromContext retrieves the request ID from the request context.
// It reads from the same context key as chi's middleware.RequestID, so it
// is interoperable with middleware.GetReqID and chi's request logger.
// Returns the ID and true when present.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	id := middleware.GetReqID(ctx)
	return id, id != ""
}

// RequestIDCfg returns the request ID configuration populated from
// environment variables during New().
func (r *Regius) RequestIDCfg() RequestIDConfig {
	return r.config.requestID
}

// RequestID returns a middleware that stamps each request with a request
// ID. When Enabled is false, it returns a no-op passthrough handler.
//
// The ID is read from the configured request Header when present and
// non-empty (reused verbatim for cross-service correlation); otherwise it
// is generated via Generator (if set) or Format. The resulting ID is
// written to ResponseHeader (if set) and stored in the request context
// under chi's middleware.RequestIDKey.
func (r *Regius) RequestID(cfg RequestIDConfig) func(next http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	header := defaultString(cfg.Header, defaultRequestIDHeader)
	responseHeader := defaultString(cfg.ResponseHeader, defaultRequestIDHeader)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id := strings.TrimSpace(req.Header.Get(header))
			if len(id) > maxRequestIDLength {
				id = ""
			}
			if id == "" {
				if cfg.Generator != nil {
					id = cfg.Generator()
				} else {
					id = generateRequestID(cfg.Format)
				}
			}

			if responseHeader != "" {
				w.Header().Set(responseHeader, id)
			}

			ctx := context.WithValue(req.Context(), middleware.RequestIDKey, id)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

func generateRequestID(format string) string {
	switch format {
	case RequestIDFormatXID:
		return xid.New().String()
	case RequestIDFormatShort:
		return randomShortID(12)
	case RequestIDFormatDefault:
		return defaultRequestID()
	default:
		return uuid.NewString()
	}
}

func randomShortID(length int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return uuid.NewString()
	}
	for i := range b {
		b[i] = alphabet[b[i]%byte(len(alphabet))]
	}
	return string(b)
}

var (
	defaultReqIDPrefix  string
	defaultReqIDCounter uint64
)

func init() {
	hostname, err := os.Hostname()
	if hostname == "" || err != nil {
		hostname = "localhost"
	}
	var buf [12]byte
	var b64 string
	for len(b64) < 10 {
		_, _ = rand.Read(buf[:])
		b64 = base64.StdEncoding.EncodeToString(buf[:])
		b64 = strings.NewReplacer("+", "", "/", "").Replace(b64)
	}
	defaultReqIDPrefix = fmt.Sprintf("%s/%s", hostname, b64[0:10])
}

func defaultRequestID() string {
	n := atomic.AddUint64(&defaultReqIDCounter, 1)
	return fmt.Sprintf("%s-%06d", defaultReqIDPrefix, n)
}
