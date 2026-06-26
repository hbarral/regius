package regius

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeIPChecker is an in-memory IPChecker for testing.
type fakeIPChecker struct {
	decisions map[string]IPDecision
	err       error
}

func (f *fakeIPChecker) Check(ip string) (IPDecision, error) {
	if f.err != nil {
		return DecisionNone, f.err
	}
	if d, ok := f.decisions[ip]; ok {
		return d, nil
	}
	return DecisionNone, nil
}

func ipFilterHandler(r *Regius, cfg IPFilterConfig, downstream http.HandlerFunc) http.Handler {
	return r.IPFilter(cfg)(http.HandlerFunc(downstream))
}

func TestIPFilter_Disabled(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{Enabled: false}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected passthrough OK, got %v", rr.Code)
	}
}

func TestIPFilter_NoLists_AllowsAll(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{Enabled: true}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "203.0.113.9:4321"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK with no lists configured, got %v", rr.Code)
	}
}

func TestIPFilter_AllowOnly(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Allow:   []string{"192.168.1.0/24"},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name   string
		remote string
		expect int
	}{
		{"in_allow_subnet", "192.168.1.50:1000", http.StatusOK},
		{"outside_allow_subnet", "192.168.2.50:1000", http.StatusForbidden},
		{"unrelated_ip", "10.0.0.1:1000", http.StatusForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = c.remote
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != c.expect {
				t.Errorf("expected %d for %s, got %v", c.expect, c.remote, rr.Code)
			}
		})
	}
}

func TestIPFilter_DenyOnly(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Deny:    []string{"10.0.0.0/8"},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name   string
		remote string
		expect int
	}{
		{"in_deny_subnet", "10.1.2.3:1000", http.StatusForbidden},
		{"outside_deny", "192.168.1.1:1000", http.StatusOK},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = c.remote
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != c.expect {
				t.Errorf("expected %d for %s, got %v", c.expect, c.remote, rr.Code)
			}
		})
	}
}

func TestIPFilter_DenyWinsOverAllow(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Allow:   []string{"192.168.1.0/24"},
		Deny:    []string{"192.168.1.99"},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name   string
		remote string
		expect int
	}{
		{"allowed_and_not_denied", "192.168.1.50:1000", http.StatusOK},
		{"allowed_but_also_denied", "192.168.1.99:1000", http.StatusForbidden},
		{"neither_allowed", "10.0.0.1:1000", http.StatusForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = c.remote
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != c.expect {
				t.Errorf("expected %d for %s, got %v", c.expect, c.remote, rr.Code)
			}
		})
	}
}

func TestIPFilter_IPv6_CIDR(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Allow:   []string{"2001:db8::/32"},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name   string
		remote string
		expect int
	}{
		{"ipv6_in_subnet", "[2001:db8::1]:1000", http.StatusOK},
		{"ipv6_outside", "[2001:dead::1]:1000", http.StatusForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = c.remote
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != c.expect {
				t.Errorf("expected %d for %s, got %v", c.expect, c.remote, rr.Code)
			}
		})
	}
}

func TestIPFilter_BareIP(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Deny:    []string{"203.0.113.5"},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name   string
		remote string
		expect int
	}{
		{"bare_ip_denied", "203.0.113.5:1000", http.StatusForbidden},
		{"neighbor_allowed", "203.0.113.6:1000", http.StatusOK},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = c.remote
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != c.expect {
				t.Errorf("expected %d for %s, got %v", c.expect, c.remote, rr.Code)
			}
		})
	}
}

func TestIPFilter_TrustProxy_XForwardedFor(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled:    true,
		TrustProxy: true,
		Allow:      []string{"198.51.100.0/24"},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name   string
		xff    string
		expect int
	}{
		{"single_xff_allowed", "198.51.100.20", http.StatusOK},
		{"single_xff_blocked", "10.0.0.20", http.StatusForbidden},
		// The first IP in a comma-separated list is the original client.
		{"first_in_list_allowed", "198.51.100.20, 10.0.0.1", http.StatusOK},
		{"first_in_list_blocked", "10.0.0.20, 198.51.100.1", http.StatusForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Forwarded-For", c.xff)
			// RemoteAddr would otherwise be used; ensure it's blocked so we
			// prove TrustProxy is what grants access.
			req.RemoteAddr = "127.0.0.1:1234"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != c.expect {
				t.Errorf("expected %d for XFF %q, got %v", c.expect, c.xff, rr.Code)
			}
		})
	}
}

func TestIPFilter_TrustProxy_XRealIP(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled:    true,
		TrustProxy: true,
		Deny:       []string{"198.51.100.7"},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Real-IP", "198.51.100.7")
	req.RemoteAddr = "127.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for denied X-Real-IP, got %v", rr.Code)
	}
}

func TestIPFilter_TrustProxyOff_IgnoresHeaders(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled:    true,
		TrustProxy: false,
		Allow:      []string{"127.0.0.1"},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	req.RemoteAddr = "127.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// With TrustProxy off, the spoofed header is ignored and RemoteAddr (127.0.0.1) is allowed.
	if rr.Code != http.StatusOK {
		t.Errorf("expected OK (RemoteAddr in allow list), got %v", rr.Code)
	}
}

func TestIPFilter_BlockResponse(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Deny:    []string{"10.0.0.1"},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1000"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %v", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("expected application/json content type, got %q", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("expected Cache-Control no-store, got %q", got)
	}
	if !contains(rr.Body.String(), "forbidden") {
		t.Errorf("expected JSON error body, got %q", rr.Body.String())
	}
}

func TestIPFilter_CustomStatusAndMessage(t *testing.T) {
	r := &Regius{}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled:    true,
		Deny:       []string{"10.0.0.1"},
		StatusCode: http.StatusTeapot,
		Message:    "you shall not pass",
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1000"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Errorf("expected 418, got %v", rr.Code)
	}
	if !contains(rr.Body.String(), "you shall not pass") {
		t.Errorf("expected custom message in body, got %q", rr.Body.String())
	}
}

func TestIPFilter_DownstreamRunsWhenAllowed(t *testing.T) {
	r := &Regius{}
	called := false
	handler := ipFilterHandler(r, IPFilterConfig{Enabled: true}, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected downstream handler to be invoked when allowed")
	}
	if rr.Code != http.StatusTeapot {
		t.Errorf("expected downstream status 418, got %v", rr.Code)
	}
}

func TestIPFilter_InvalidEntrySkipped(t *testing.T) {
	r := &Regius{}
	// "not-an-ip" is invalid and must be skipped (not fatal); the valid CIDR
	// still applies.
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Allow:   []string{"not-an-ip", "192.168.1.0/24"},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name   string
		remote string
		expect int
	}{
		{"valid_cidr_allows", "192.168.1.5:1000", http.StatusOK},
		{"outside_allowed", "10.0.0.1:1000", http.StatusForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = c.remote
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != c.expect {
				t.Errorf("expected %d for %s, got %v", c.expect, c.remote, rr.Code)
			}
		})
	}
}

func TestIPFilter_Checker_AllowOverridesDeny(t *testing.T) {
	r := &Regius{}
	checker := &fakeIPChecker{decisions: map[string]IPDecision{"10.0.0.1": DecisionAllow}}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Deny:    []string{"10.0.0.1"},
		Checker: checker,
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1000"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("checker Allow should override static Deny, got %v", rr.Code)
	}
}

func TestIPFilter_Checker_DenyOverridesAllow(t *testing.T) {
	r := &Regius{}
	checker := &fakeIPChecker{decisions: map[string]IPDecision{"192.168.1.5": DecisionDeny}}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Allow:   []string{"192.168.1.0/24"},
		Checker: checker,
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.5:1000"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("checker Deny should override static Allow, got %v", rr.Code)
	}
}

func TestIPFilter_Checker_NoneDefersToStatic(t *testing.T) {
	r := &Regius{}
	checker := &fakeIPChecker{} // no decisions: always DecisionNone
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Allow:   []string{"192.168.1.0/24"},
		Checker: checker,
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name   string
		remote string
		expect int
	}{
		{"static_allows", "192.168.1.5:1000", http.StatusOK},
		{"static_blocks", "10.0.0.1:1000", http.StatusForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = c.remote
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != c.expect {
				t.Errorf("expected %d for %s, got %v", c.expect, c.remote, rr.Code)
			}
		})
	}
}

func TestIPFilter_CheckerError_FailsOpen(t *testing.T) {
	r := &Regius{}
	checker := &fakeIPChecker{err: errors.New("cache down")}
	handler := ipFilterHandler(r, IPFilterConfig{
		Enabled: true,
		Allow:   []string{"192.168.1.0/24"},
		Checker: checker,
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Checker errors are treated as DecisionNone: the static baseline still
	// applies. An IP outside the static allow list is still blocked.
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1000"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected static baseline to apply on checker error, got %v", rr.Code)
	}
}

func TestCacheIPChecker_RoundTrip(t *testing.T) {
	c := newFakeCache()
	checker := NewCacheIPChecker(c, "")

	if err := checker.Block("203.0.113.9", 0); err != nil {
		t.Fatalf("Block failed: %v", err)
	}

	decision, err := checker.Check("203.0.113.9")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if decision != DecisionDeny {
		t.Errorf("expected DecisionDeny, got %v", decision)
	}

	// Unblock removes the decision, deferring back to the static lists.
	if err := checker.Unblock("203.0.113.9"); err != nil {
		t.Fatalf("Unblock failed: %v", err)
	}
	decision, _ = checker.Check("203.0.113.9")
	if decision != DecisionNone {
		t.Errorf("expected DecisionNone after Unblock, got %v", decision)
	}
}

func TestCacheIPChecker_AllowDecision(t *testing.T) {
	c := newFakeCache()
	checker := NewCacheIPChecker(c, "")

	if err := checker.Allow("203.0.113.9", 60); err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	decision, _ := checker.Check("203.0.113.9")
	if decision != DecisionAllow {
		t.Errorf("expected DecisionAllow, got %v", decision)
	}
}

func TestCacheIPChecker_MissReturnsNone(t *testing.T) {
	c := newFakeCache()
	checker := NewCacheIPChecker(c, "")

	decision, err := checker.Check("unknown-ip")
	if err != nil {
		t.Fatalf("unexpected error on miss: %v", err)
	}
	if decision != DecisionNone {
		t.Errorf("expected DecisionNone on miss, got %v", decision)
	}
}

func TestCacheIPChecker_Prefix(t *testing.T) {
	c := newFakeCache()
	checker := NewCacheIPChecker(c, "")

	_ = checker.Block("1.2.3.4", 0)

	c.mu.RLock()
	defer c.mu.RUnlock()
	found := false
	for k := range c.data {
		if contains(k, "ipfilter:") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected entry namespaced under ipfilter: prefix, got keys: %v", c.data)
	}
}
