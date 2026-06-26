package regius

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hbarral/regius/cache"
)

func init() {
	// Register APIKeyIdentity with gob so it round-trips correctly when stored
	// through the cache layer (which encodes values via gob). Without this,
	// structs decoded from an interface{} would come back as a
	// map[string]interface{} and the type assertion in Lookup would fail.
	gob.Register(APIKeyIdentity{})
}

// APIKeyAuthConfig holds configuration for API key authentication.
type APIKeyAuthConfig struct {
	Enabled bool

	// Keys is a static list of valid API keys. Compared in constant time to
	// mitigate timing attacks. Used when neither Validator nor Store is set.
	Keys []string

	// Validator is an optional pluggable validation function, typically used
	// for DB-backed keys. It takes precedence over Store and Keys when set.
	Validator func(key string) (APIKeyIdentity, bool)

	// Store is an optional backend (e.g. cache-backed) for key lookup and
	// revocation. Used when Validator is nil. Takes precedence over Keys.
	Store APIKeyStore

	// Header is the primary header to read the key from (default
	// "Authorization"). The Scheme prefix is stripped from this header.
	Header string

	// Scheme is the expected scheme prefix for Header (default "Bearer").
	// Ignored when reading AltHeader.
	Scheme string

	// AltHeader is a secondary header read without a scheme prefix (default
	// "X-API-Key").
	AltHeader string

	// QueryParam, when non-empty, enables reading the key from a query
	// parameter of this name. Disabled by default: keys in URLs leak via
	// access logs and Referrer headers.
	QueryParam string

	// Realm is used in the WWW-Authenticate response header (default "api").
	Realm string
}

// APIKeyIdentity represents the authenticated API consumer. It is stored in
// the request context so downstream handlers can identify the caller.
type APIKeyIdentity struct {
	Key      string
	ID       string
	Metadata map[string]string
}

// APIKeyStore is the interface for a backend that can look up and revoke API
// keys (e.g. a cache or database). Implementations are responsible for
// storing keys securely (e.g. hashed).
type APIKeyStore interface {
	Lookup(key string) (APIKeyIdentity, bool, error)
	Revoke(key string) error
}

type apiKeyContextKey struct{}

// APIKeyFromContext retrieves the authenticated API key identity from the
// request context. Returns the identity and true when present.
func APIKeyFromContext(ctx context.Context) (APIKeyIdentity, bool) {
	v, ok := ctx.Value(apiKeyContextKey{}).(APIKeyIdentity)
	return v, ok
}

// APIKeyAuthCfg returns the API key auth configuration populated from
// environment variables during New(). Use it to apply the env-driven config
// to a route group: mux.Use(r.APIKeyAuth(r.APIKeyAuthCfg())).
func (r *Regius) APIKeyAuthCfg() APIKeyAuthConfig {
	return r.config.apiKeyAuth
}

func defaultString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// APIKeyAuth returns a middleware that authenticates requests using an API
// key. When Enabled is false, it returns a no-op passthrough handler.
//
// The key is read, in order, from the configured Header (stripping the
// Scheme prefix, e.g. "Authorization: Bearer <key>"), then AltHeader (e.g.
// "X-API-Key: <key>"), then — if QueryParam is set — the query string.
//
// Validation precedence: Validator (if set) > Store (if set) > Keys
// (constant-time comparison). On failure the middleware responds with 401
// and a WWW-Authenticate header. The raw key is never logged.
//
// This middleware is intended for API route groups (e.g. /api/*); it should
// not be applied globally, as that would block cookie-authenticated web
// routes.
func (r *Regius) APIKeyAuth(cfg APIKeyAuthConfig) func(next http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	header := defaultString(cfg.Header, "Authorization")
	scheme := defaultString(cfg.Scheme, "Bearer")
	altHeader := defaultString(cfg.AltHeader, "X-API-Key")
	realm := defaultString(cfg.Realm, "api")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			key := extractAPIKey(req, header, scheme, altHeader, cfg.QueryParam)
			if key == "" {
				apiKeyUnauthorized(w, scheme, realm, "missing or malformed api key")
				return
			}

			identity, ok := r.validateAPIKey(cfg, key)
			if !ok {
				apiKeyUnauthorized(w, scheme, realm, "invalid api key")
				return
			}

			ctx := context.WithValue(req.Context(), apiKeyContextKey{}, identity)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

func extractAPIKey(req *http.Request, header, scheme, altHeader, queryParam string) string {
	// Primary header: "Authorization: Bearer <key>".
	if h := req.Header.Get(header); h != "" {
		if scheme != "" {
			parts := strings.SplitN(h, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], scheme) {
				return strings.TrimSpace(parts[1])
			}
		} else {
			return strings.TrimSpace(h)
		}
	}

	// Alt header: "X-API-Key: <key>" (no scheme prefix).
	if altHeader != "" {
		if h := req.Header.Get(altHeader); h != "" {
			return strings.TrimSpace(h)
		}
	}

	// Optional query parameter (disabled by default).
	if queryParam != "" {
		if q := req.URL.Query().Get(queryParam); q != "" {
			return strings.TrimSpace(q)
		}
	}

	return ""
}

func (r *Regius) validateAPIKey(cfg APIKeyAuthConfig, key string) (APIKeyIdentity, bool) {
	// Precedence: Validator > Store > Keys.
	if cfg.Validator != nil {
		identity, ok := cfg.Validator(key)
		if !ok {
			return APIKeyIdentity{}, false
		}
		return normalizeAPIKeyIdentity(identity, key), true
	}

	if cfg.Store != nil {
		identity, ok, err := cfg.Store.Lookup(key)
		if err != nil {
			if r.ErrorLog != nil {
				r.ErrorLog.Printf("api key store lookup failed: %v", err)
			}
			return APIKeyIdentity{}, false
		}
		if !ok {
			return APIKeyIdentity{}, false
		}
		return normalizeAPIKeyIdentity(identity, key), true
	}

	if len(cfg.Keys) > 0 {
		for _, valid := range cfg.Keys {
			if subtle.ConstantTimeCompare([]byte(key), []byte(valid)) == 1 {
				return APIKeyIdentity{Key: key}, true
			}
		}
		return APIKeyIdentity{}, false
	}

	// No backend configured.
	return APIKeyIdentity{}, false
}

func normalizeAPIKeyIdentity(identity APIKeyIdentity, key string) APIKeyIdentity {
	if identity.Key == "" {
		identity.Key = key
	}
	return identity
}

func apiKeyUnauthorized(w http.ResponseWriter, scheme, realm, message string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`%s realm="%s"`, scheme, realm))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "message": message})
}

// CacheAPIKeyStore adapts a regius cache.Cache to the APIKeyStore interface.
// Keys are stored and looked up by the SHA-256 hash of the raw key, so raw
// keys are never persisted. Use Set to register a key (optionally with an
// identity) and Revoke to invalidate one.
type CacheAPIKeyStore struct {
	cache  cache.Cache
	prefix string
}

// NewCacheAPIKeyStore returns a cache-backed APIKeyStore using the provided
// cache. Entries are namespaced under prefix (default "apikey:" when empty).
func NewCacheAPIKeyStore(c cache.Cache, prefix string) *CacheAPIKeyStore {
	if prefix == "" {
		prefix = "apikey:"
	}
	return &CacheAPIKeyStore{cache: c, prefix: prefix}
}

func (s *CacheAPIKeyStore) keyHash(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return s.prefix + hex.EncodeToString(sum[:])
}

// Set registers a valid API key in the cache with an optional identity.
// expiresInSeconds of 0 means no expiration.
func (s *CacheAPIKeyStore) Set(rawKey string, identity APIKeyIdentity, expiresInSeconds int) error {
	if identity.Key == "" {
		identity.Key = rawKey
	}
	return s.cache.Set(s.keyHash(rawKey), identity, expiresInSeconds)
}

// Lookup implements APIKeyStore.
func (s *CacheAPIKeyStore) Lookup(rawKey string) (APIKeyIdentity, bool, error) {
	val, err := s.cache.Get(s.keyHash(rawKey))
	if err != nil || val == nil {
		return APIKeyIdentity{}, false, nil
	}
	identity, ok := val.(APIKeyIdentity)
	if !ok {
		return APIKeyIdentity{}, false, nil
	}
	return identity, true, nil
}

// Revoke implements APIKeyStore by removing the key from the cache.
func (s *CacheAPIKeyStore) Revoke(rawKey string) error {
	return s.cache.Forget(s.keyHash(rawKey))
}
