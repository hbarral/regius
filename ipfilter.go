package regius

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/hbarral/regius/cache"
)

// IPFilterConfig holds configuration for IP whitelist/blacklist filtering.
type IPFilterConfig struct {
	Enabled bool

	// Allow is a list of IPs or CIDR ranges that are permitted. When non-empty,
	// only requests from these networks are allowed (subject to Deny, which
	// always takes precedence). Bare IPs are treated as /32 (IPv4) or /128
	// (IPv6).
	Allow []string

	// Deny is a list of IPs or CIDR ranges that are always blocked, even when
	// also matched by Allow.
	Deny []string

	// TrustProxy, when true, reads the client IP from the X-Forwarded-For (or
	// X-Real-IP) header instead of RemoteAddr. Only enable when running behind
	// a trusted reverse proxy; otherwise an attacker can spoof the header to
	// bypass the filter. Defaults to false.
	TrustProxy bool

	// StatusCode returned for blocked requests (default 403).
	StatusCode int

	// Message included in the JSON body for blocked requests (default
	// "ip address not allowed").
	Message string

	// Checker is an optional pluggable backend (e.g. cache-backed) for
	// dynamic decisions. When set, its explicit Allow/Deny overrides the
	// static Allow/Deny lists; DecisionNone defers to the static lists
	// (deny-wins). Useful for fail2ban-style runtime blocking.
	Checker IPChecker
}

// IPDecision is the verdict returned by an IPChecker.
type IPDecision int

const (
	// DecisionNone means the checker has no opinion; the request is evaluated
	// against the static Allow/Deny lists.
	DecisionNone IPDecision = iota
	// DecisionAllow overrides the static lists and permits the request.
	DecisionAllow
	// DecisionDeny overrides the static lists and blocks the request.
	DecisionDeny
)

// IPChecker decides whether a given IP is allowed. Implementations are
// responsible for any storage (e.g. a cache or database) and TTL handling.
type IPChecker interface {
	Check(ip string) (IPDecision, error)
}

// IPFilterCfg returns the IP filter configuration populated from environment
// variables during New().
func (r *Regius) IPFilterCfg() IPFilterConfig {
	return r.config.ipFilter
}

// IPFilter returns a middleware that allows or denies requests based on the
// client IP. When Enabled is false, it returns a no-op passthrough handler.
//
// Evaluation order:
//  1. Checker (if set): an explicit Allow or Deny decision wins; DecisionNone
//     falls through to the static lists. Checker errors are logged and treated
//     as DecisionNone (the dynamic layer fails open; the static baseline still
//     applies).
//  2. Static lists (deny-wins): a matching Deny entry blocks; when Allow is
//     non-empty, any IP not present in Allow is blocked.
//
// Blocked requests receive StatusCode (default 403) with a JSON body. The
// middleware is intended to run early in the chain (after RealIP) so denied
// requests short-circuit before heavier middleware runs.
func (r *Regius) IPFilter(cfg IPFilterConfig) func(next http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	allowNets := parseIPNetworks(cfg.Allow, r.ErrorLog)
	denyNets := parseIPNetworks(cfg.Deny, r.ErrorLog)
	statusCode := cfg.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusForbidden
	}
	message := defaultString(cfg.Message, "ip address not allowed")
	checker := cfg.Checker

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip := clientIPAddress(req, cfg.TrustProxy)
			if !r.ipAllowed(ip, allowNets, denyNets, checker) {
				ipFilterBlock(w, statusCode, message)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func (r *Regius) ipAllowed(ip string, allowNets, denyNets []*net.IPNet, checker IPChecker) bool {
	if checker != nil {
		decision, err := checker.Check(ip)
		if err != nil {
			if r.ErrorLog != nil {
				r.ErrorLog.Printf("ipfilter: checker error for %s: %v", ip, err)
			}
		} else {
			switch decision {
			case DecisionAllow:
				return true
			case DecisionDeny:
				return false
			}
		}
	}

	if ipInNetworks(ip, denyNets) {
		return false
	}
	if len(allowNets) > 0 && !ipInNetworks(ip, allowNets) {
		return false
	}
	return true
}

func ipInNetworks(ip string, nets []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

func parseIPNetworks(entries []string, log *log.Logger) []*net.IPNet {
	var nets []*net.IPNet
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		n, err := parseIPNetwork(e)
		if err != nil {
			if log != nil {
				log.Printf("ipfilter: skipping invalid entry %q: %v", e, err)
			}
			continue
		}
		nets = append(nets, n)
	}
	return nets
}

func parseIPNetwork(s string) (*net.IPNet, error) {
	if _, n, err := net.ParseCIDR(s); err == nil {
		return n, nil
	}
	if ip := net.ParseIP(s); ip != nil {
		if ip.To4() != nil {
			return &net.IPNet{IP: ip.To4(), Mask: net.CIDRMask(32, 32)}, nil
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
	}
	return nil, fmt.Errorf("not a valid IP address or CIDR range")
}

func ipFilterBlock(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden", "message": message})
}

// CacheIPChecker adapts a regius cache.Cache to the IPChecker interface. It
// stores per-IP decisions (typically dynamic blocks) with optional TTLs, so
// runtime code can block or unblock IPs without restarting the application
// (e.g. fail2ban-style). Entries are namespaced under prefix (default
// "ipfilter:" when empty).
type CacheIPChecker struct {
	cache  cache.Cache
	prefix string
}

// NewCacheIPChecker returns a cache-backed IPChecker. Entries are namespaced
// under prefix (default "ipfilter:" when empty).
func NewCacheIPChecker(c cache.Cache, prefix string) *CacheIPChecker {
	if prefix == "" {
		prefix = "ipfilter:"
	}
	return &CacheIPChecker{cache: c, prefix: prefix}
}

func (c *CacheIPChecker) entryKey(ip string) string {
	return c.prefix + ip
}

// Set stores a decision for ip. expiresInSeconds of 0 means no expiration.
func (c *CacheIPChecker) Set(ip string, decision IPDecision, expiresInSeconds int) error {
	return c.cache.Set(c.entryKey(ip), int(decision), expiresInSeconds)
}

// Block is a convenience for Set(ip, DecisionDeny, ...).
func (c *CacheIPChecker) Block(ip string, expiresInSeconds int) error {
	return c.Set(ip, DecisionDeny, expiresInSeconds)
}

// Allow is a convenience for Set(ip, DecisionAllow, ...).
func (c *CacheIPChecker) Allow(ip string, expiresInSeconds int) error {
	return c.Set(ip, DecisionAllow, expiresInSeconds)
}

// Unblock removes any stored decision for ip, deferring back to the static
// lists.
func (c *CacheIPChecker) Unblock(ip string) error {
	return c.cache.Forget(c.entryKey(ip))
}

// Check implements IPChecker.
func (c *CacheIPChecker) Check(ip string) (IPDecision, error) {
	val, err := c.cache.Get(c.entryKey(ip))
	if err != nil || val == nil {
		return DecisionNone, nil
	}
	switch v := val.(type) {
	case int:
		return IPDecision(v), nil
	case int64:
		return IPDecision(v), nil
	default:
		return DecisionNone, nil
	}
}
