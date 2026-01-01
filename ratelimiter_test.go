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
