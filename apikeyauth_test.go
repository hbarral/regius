package regius

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hbarral/regius/cache"
)

// fakeAPIKeyStore is an in-memory APIKeyStore for testing.
type fakeAPIKeyStore struct {
	mu   sync.RWMutex
	keys map[string]APIKeyIdentity
	err  error
}

func newFakeAPIKeyStore() *fakeAPIKeyStore {
	return &fakeAPIKeyStore{keys: make(map[string]APIKeyIdentity)}
}

func (s *fakeAPIKeyStore) Lookup(key string) (APIKeyIdentity, bool, error) {
	if s.err != nil {
		return APIKeyIdentity{}, false, s.err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.keys[key]
	return id, ok, nil
}

func (s *fakeAPIKeyStore) Revoke(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, key)
	return nil
}

func (s *fakeAPIKeyStore) set(key string, id APIKeyIdentity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[key] = id
}

// fakeCache is a minimal cache.Cache for testing the CacheAPIKeyStore adapter.
type fakeCache struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

func newFakeCache() *fakeCache {
	return &fakeCache{data: make(map[string]interface{})}
}

func (f *fakeCache) Has(key string) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.data[key]
	return ok, nil
}

func (f *fakeCache) Get(key string) (interface{}, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	v, ok := f.data[key]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return v, nil
}

func (f *fakeCache) Set(key string, value interface{}, _ ...int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = value
	return nil
}

func (f *fakeCache) Forget(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, key)
	return nil
}

func (f *fakeCache) EmptyByMatch(_ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data = make(map[string]interface{})
	return nil
}

func (f *fakeCache) Empty() error {
	return f.EmptyByMatch("")
}

var _ cache.Cache = (*fakeCache)(nil)

func TestAPIKeyAuth_Disabled(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{Enabled: false})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected passthrough OK, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_BearerHeader_Valid(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Keys:    []string{"secret-key"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK for valid bearer key, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_BearerHeader_WrongScheme(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Keys:    []string{"secret-key"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// "Basic" scheme must not be accepted as a Bearer key.
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic secret-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong scheme, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_BearerHeader_Missing(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Keys:    []string{"secret-key"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no key is present, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_AltHeader_Valid(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Keys:    []string{"secret-key"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "secret-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK for valid X-API-Key, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_QueryParam_Valid(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled:    true,
		Keys:       []string{"secret-key"},
		QueryParam: "api_key",
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test?api_key=secret-key", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK for valid query param key, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_QueryParam_DisabledByDefault(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Keys:    []string{"secret-key"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Query param is ignored because QueryParam is not configured.
	req := httptest.NewRequest("GET", "/test?api_key=secret-key", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when query param is disabled, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_StaticKeys_Multiple(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Keys:    []string{"key-one", "key-two", "key-three"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name   string
		key    string
		expect int
	}{
		{"first_key", "key-one", http.StatusOK},
		{"second_key", "key-two", http.StatusOK},
		{"third_key", "key-three", http.StatusOK},
		{"invalid_key", "nope", http.StatusUnauthorized},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+c.key)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != c.expect {
				t.Errorf("expected %d for %q, got %v", c.expect, c.key, rr.Code)
			}
		})
	}
}

func TestAPIKeyAuth_CustomValidator_Success(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Validator: func(key string) (APIKeyIdentity, bool) {
			if key == "db-backed-key" {
				return APIKeyIdentity{ID: "user-42", Metadata: map[string]string{"plan": "pro"}}, true
			}
			return APIKeyIdentity{}, false
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := APIKeyFromContext(r.Context())
		if !ok || id.ID != "user-42" {
			t.Errorf("expected identity user-42 in context, got %+v ok=%v", id, ok)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer db-backed-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK for validator-backed key, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_CustomValidator_Failure(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Validator: func(key string) (APIKeyIdentity, bool) {
			return APIKeyIdentity{}, false
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer whatever")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when validator rejects, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_Store_Lookup(t *testing.T) {
	store := newFakeAPIKeyStore()
	store.set("store-key", APIKeyIdentity{ID: "client-1"})

	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Store:   store,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := APIKeyFromContext(r.Context())
		if !ok || id.ID != "client-1" {
			t.Errorf("expected identity client-1, got %+v ok=%v", id, ok)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer store-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK for store-backed key, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_Store_Revoke(t *testing.T) {
	store := newFakeAPIKeyStore()
	store.set("revokable-key", APIKeyIdentity{ID: "client-1"})

	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Store:   store,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request: key is valid.
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer revokable-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected OK before revoke, got %v", rr.Code)
	}

	// Revoke, then expect 401.
	if err := store.Revoke("revokable-key"); err != nil {
		t.Fatalf("unexpected error revoking: %v", err)
	}
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after revoke, got %v", rr2.Code)
	}
}

func TestAPIKeyAuth_ValidatorPrecedenceOverStoreAndKeys(t *testing.T) {
	store := newFakeAPIKeyStore()
	store.set("shared-key", APIKeyIdentity{ID: "from-store"})

	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Keys:    []string{"shared-key"},
		Store:   store,
		Validator: func(key string) (APIKeyIdentity, bool) {
			if key == "shared-key" {
				return APIKeyIdentity{ID: "from-validator"}, true
			}
			return APIKeyIdentity{}, false
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := APIKeyFromContext(r.Context())
		if id.ID != "from-validator" {
			t.Errorf("expected validator to win precedence, got ID=%q", id.ID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer shared-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_StorePrecedenceOverKeys(t *testing.T) {
	store := newFakeAPIKeyStore()

	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Keys:    []string{"shared-key"},
		Store:   store, // empty store: lookup misses
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Store is set but has no entry, so it takes precedence over Keys and the
	// request must be rejected even though "shared-key" is in Keys.
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer shared-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when store misses (precedence over keys), got %v", rr.Code)
	}
}

func TestAPIKeyAuth_NoBackendConfigured(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer some-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no backend is configured, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_ContextPropagation(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Keys:    []string{"secret-key"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := APIKeyFromContext(r.Context())
		if !ok {
			t.Error("expected identity in context")
		}
		if id.Key != "secret-key" {
			t.Errorf("expected Key=secret-key, got %q", id.Key)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK, got %v", rr.Code)
	}
}

func TestAPIKeyAuth_UnauthorizedResponse(t *testing.T) {
	r := &Regius{}
	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Keys:    []string{"secret-key"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %v", rr.Code)
	}
	if got := rr.Header().Get("WWW-Authenticate"); got == "" || !contains(got, `realm="api"`) {
		t.Errorf("expected WWW-Authenticate with realm=\"api\", got %q", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("expected Cache-Control no-store, got %q", got)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("expected application/json content type, got %q", got)
	}
	if !contains(rr.Body.String(), "unauthorized") {
		t.Errorf("expected JSON error body, got %q", rr.Body.String())
	}
}

func TestAPIKeyAuth_KeyNotLogged(t *testing.T) {
	var buf bytes.Buffer
	r := &Regius{
		ErrorLog: log.New(&buf, "ERROR\t", log.Lshortfile),
	}

	const rawKey = "super-secret-key-123"
	store := newFakeAPIKeyStore()
	store.err = errors.New("storage exploded")

	handler := r.APIKeyAuth(APIKeyAuthConfig{
		Enabled: true,
		Store:   store,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 on store error, got %v", rr.Code)
	}
	if contains(buf.String(), rawKey) {
		t.Errorf("raw key must not appear in logs, got: %s", buf.String())
	}
}

func TestCacheAPIKeyStore_RoundTrip(t *testing.T) {
	c := newFakeCache()
	store := NewCacheAPIKeyStore(c, "")

	id := APIKeyIdentity{ID: "client-7", Metadata: map[string]string{"scope": "read"}}
	if err := store.Set("my-key", id, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, ok, err := store.Lookup("my-key")
	if err != nil || !ok {
		t.Fatalf("expected lookup to succeed, ok=%v err=%v", ok, err)
	}
	if got.ID != "client-7" {
		t.Errorf("expected ID client-7, got %q", got.ID)
	}
	if got.Key != "my-key" {
		t.Errorf("expected Key normalized to my-key, got %q", got.Key)
	}
	if got.Metadata["scope"] != "read" {
		t.Errorf("expected metadata scope=read, got %q", got.Metadata["scope"])
	}
}

func TestCacheAPIKeyStore_UnknownKey(t *testing.T) {
	c := newFakeCache()
	store := NewCacheAPIKeyStore(c, "")

	if _, ok, err := store.Lookup("nope"); ok || err != nil {
		t.Errorf("expected miss with no error, ok=%v err=%v", ok, err)
	}
}

func TestCacheAPIKeyStore_Revoke(t *testing.T) {
	c := newFakeCache()
	store := NewCacheAPIKeyStore(c, "")

	if err := store.Set("my-key", APIKeyIdentity{ID: "x"}, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if _, ok, _ := store.Lookup("my-key"); !ok {
		t.Fatal("expected key to exist before revoke")
	}

	if err := store.Revoke("my-key"); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	if _, ok, _ := store.Lookup("my-key"); ok {
		t.Error("expected key to be gone after revoke")
	}
}

func TestCacheAPIKeyStore_RawKeyNotStored(t *testing.T) {
	c := newFakeCache()
	store := NewCacheAPIKeyStore(c, "")

	if err := store.Set("plaintext-secret", APIKeyIdentity{ID: "x"}, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	for k := range c.data {
		if contains(k, "plaintext-secret") {
			t.Errorf("raw key must not be used as cache key, found %q", k)
		}
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
