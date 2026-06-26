package regius

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_InMemory(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   3,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: false,
		Whitelist:  []string{},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	for i := 0; i < 3; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected status OK, got %v", i+1, rr.Code)
		}
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status TooManyRequests, got %v", rr.Code)
	}
}

func TestRateLimiter_Whitelist(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   1,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: false,
		Whitelist:  []string{"192.168.1.1"},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	for i := 0; i < 5; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected status OK, got %v", i+1, rr.Code)
		}
	}
}

func TestRateLimiter_Disabled(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    false,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   1,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: false,
		Whitelist:  []string{},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected status OK, got %v", i+1, rr.Code)
		}
	}
}

func TestRateLimiter_TokenBucket(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmTokenBucket,
		Requests:   3,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: false,
		Whitelist:  []string{},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	for i := 0; i < 3; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected status OK, got %v", i+1, rr.Code)
		}
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status TooManyRequests, got %v", rr.Code)
	}

	time.Sleep(1100 * time.Millisecond)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("After refill: expected status OK, got %v", rr.Code)
	}
}

func TestRateLimiter_DifferentPaths(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   2,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: false,
		Whitelist:  []string{},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest("GET", "/path1", nil)
	req1.RemoteAddr = "192.168.1.1:1234"

	req2 := httptest.NewRequest("GET", "/path2", nil)
	req2.RemoteAddr = "192.168.1.1:1234"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req1)
	if rr.Code != http.StatusOK {
		t.Errorf("Request to path1: expected status OK, got %v", rr.Code)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req2)
	if rr.Code != http.StatusOK {
		t.Errorf("Request to path2: expected status OK, got %v", rr.Code)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req1)
	if rr.Code != http.StatusOK {
		t.Errorf("Second request to path1: expected status OK, got %v", rr.Code)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req1)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Third request to path1: expected status TooManyRequests, got %v", rr.Code)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req2)
	if rr.Code != http.StatusOK {
		t.Errorf("Second request to path2: expected status OK, got %v", rr.Code)
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   1,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: false,
		Whitelist:  []string{},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.1:1234"

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.2:1234"

	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("IP1 first request: expected status OK, got %v", rr1.Code)
	}

	rr1 = httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusTooManyRequests {
		t.Errorf("IP1 second request: expected status TooManyRequests, got %v", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("IP2 first request: expected status OK, got %v", rr2.Code)
	}
}

func TestRateLimiter_Headers(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   1,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: false,
		Whitelist:  []string{},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-RateLimit-Limit") != "1" {
		t.Errorf("Expected X-RateLimit-Limit to be 1, got %s", rr.Header().Get("X-RateLimit-Limit"))
	}

	if rr.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("Expected X-RateLimit-Remaining to be 0, got %s", rr.Header().Get("X-RateLimit-Remaining"))
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status TooManyRequests, got %v", rr.Code)
	}

	if rr.Header().Get("Retry-After") == "" {
		t.Error("Expected Retry-After header to be set")
	}
}

func TestRateLimiter_IPv6(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   2,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: false,
		Whitelist:  []string{},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "[::1]:1234"

	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected OK, got %v", i+1, rr.Code)
		}
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected TooManyRequests for IPv6 client, got %v", rr.Code)
	}
}

func TestRateLimiter_IPv6_Whitelist(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   1,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: false,
		Whitelist:  []string{"::1"},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "[::1]:1234"

	// All requests must be exempt: the extracted IP (::1) must match the
	// whitelist entry. Previously the naive port-strip returned [::1], which
	// never matched, so IPv6 clients could not be whitelisted.
	for i := 0; i < 5; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected OK (whitelisted IPv6), got %v", i+1, rr.Code)
		}
	}
}

func TestRateLimiter_TrustProxy_XFF(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   1,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: true,
		Whitelist:  []string{},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	req.RemoteAddr = "127.0.0.1:1234"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("First request: expected OK, got %v", rr.Code)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Second request: expected TooManyRequests, got %v", rr.Code)
	}
}

func TestRateLimiter_TrustProxy_XFF_FirstIP(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   1,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: true,
		Whitelist:  []string{},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Same first IP, different proxy chain. The first IP (198.51.100.7) is the
	// identifier, so the second request must be limited. Previously the whole
	// XFF string was used, so these would be treated as distinct clients.
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
	req1.RemoteAddr = "127.0.0.1:1234"

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.2")
	req2.RemoteAddr = "127.0.0.1:1234"

	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("First request: expected OK, got %v", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("Second request (same first IP): expected TooManyRequests, got %v", rr2.Code)
	}
}

func TestRateLimiter_TrustProxy_XRealIP(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   1,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: true,
		Whitelist:  []string{},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Real-IP", "198.51.100.9")
	req.RemoteAddr = "127.0.0.1:1234"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("First request: expected OK, got %v", rr.Code)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Second request: expected TooManyRequests, got %v", rr.Code)
	}
}

func TestRateLimiter_Whitelist_CIDR(t *testing.T) {
	config := RateLimiterConfig{
		Enabled:    true,
		Algorithm:  RateLimiterAlgorithmSlidingWindow,
		Requests:   1,
		Window:     time.Second,
		Storage:    "",
		TrustProxy: false,
		Whitelist:  []string{"10.0.0.0/8"},
	}

	r := &Regius{}
	limiter := r.RateLimiter(config)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP inside the whitelisted CIDR is exempt.
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.1.2.3:1234"
	for i := 0; i < 3; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Whitelisted CIDR request %d: expected OK, got %v", i+1, rr.Code)
		}
	}

	// IP outside the CIDR is rate-limited as normal.
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req2)
	if rr.Code != http.StatusOK {
		t.Errorf("Non-whitelisted first request: expected OK, got %v", rr.Code)
	}
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req2)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Non-whitelisted second request: expected TooManyRequests, got %v", rr.Code)
	}
}
