package regius

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
)

// SanitizePolicyStrict strips all HTML elements and attributes, returning only
// the safe text content (with HTML entities escaped). It is the safest choice
// for arbitrary input that should never contain HTML (most form fields and
// query parameters).
const SanitizePolicyStrict = "strict"

// SanitizePolicyUGC allows a safe subset of HTML (e.g. <b>, <i>, <p>, <a>) for
// user-generated content such as comments, while removing dangerous elements
// like <script> and event-handler attributes like onerror. Use it only for
// fields that are intended to accept limited HTML.
const SanitizePolicyUGC = "ugc"

// RequestSanitizerConfig holds configuration for request sanitization (XSS
// prevention). When Enabled is true, the middleware sanitizes query params,
// form-encoded values, and the configured request headers using bluemonday
// before downstream handlers observe them.
//
// Defaults (applied inside RequestSanitizer): Policy "strict", Query true,
// Form true, Exempt "/api/.*". Headers defaults to none when empty; configure
// a safe allowlist (e.g. Referer, User-Agent). Never include structural headers
// used by the framework or its middleware (Authorization, Cookie, X-CSRF-Token,
// Content-*, X-Forwarded-*, X-Request-ID) — sanitizing those can break routing,
// auth, and tracing.
type RequestSanitizerConfig struct {
	Enabled bool

	// Policy selects the bluemonday policy: "strict" (default) or "ugc".
	// Ignored when Custom is set.
	Policy string

	// Query enables sanitization of URL query parameters. nil defaults to
	// true; set to a pointer to false to disable.
	Query *bool

	// Form enables sanitization of form-encoded values
	// (application/x-www-form-urlencoded and multipart/form-data). JSON and
	// other request bodies are never parsed or modified. nil defaults to
	// true; set to a pointer to false to disable.
	Form *bool

	// Headers is the allowlist of request header names to sanitize. Header
	// names are canonicalized. Defaults to none; populate with headers that
	// are commonly reflected in HTML (e.g. Referer, User-Agent). Never add
	// structural headers used by the framework or its middleware.
	Headers []string

	// Exempt is a regular expression matched against the request path;
	// matching requests bypass sanitization entirely (default "/api/.*"). Set
	// to "" to sanitize every path.
	Exempt string

	// Custom, when set, overrides Policy with an arbitrary bluemonday policy.
	Custom *bluemonday.Policy
}

const defaultSanitizeExempt = "/api/.*"

// RequestSanitizerCfg returns the request sanitizer configuration populated
// from environment variables during New(). Use it to apply the env-driven
// config: mux.Use(r.RequestSanitizer(r.RequestSanitizerCfg())).
func (r *Regius) RequestSanitizerCfg() RequestSanitizerConfig {
	return r.config.requestSanitizer
}

// RequestSanitizer returns a middleware that sanitizes incoming request input
// (query params, form values, and selected headers) for XSS prevention. When
// Enabled is false, it returns a no-op passthrough handler.
//
// Sanitization uses bluemonday. By default the strict policy strips all HTML;
// set Policy to "ugc" (or provide Custom) to allow a safe HTML subset.
//
// The middleware is safe for JSON APIs: only form-encoded bodies are parsed
// (so JSON bodies are never consumed), and paths matching Exempt (default
// "/api/.*") bypass sanitization entirely.
func (r *Regius) RequestSanitizer(cfg RequestSanitizerConfig) func(next http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	policy := cfg.Custom
	if policy == nil {
		policy = newSanitizePolicy(cfg.Policy)
	}

	var exempt *regexp.Regexp
	if v := defaultString(cfg.Exempt, defaultSanitizeExempt); v != "" {
		exempt, _ = regexp.Compile(v)
	}

	query := true
	if cfg.Query != nil {
		query = *cfg.Query
	}
	form := true
	if cfg.Form != nil {
		form = *cfg.Form
	}
	headers := canonicalHeaders(cfg.Headers)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if exempt != nil && exempt.MatchString(req.URL.Path) {
				next.ServeHTTP(w, req)
				return
			}

			if query {
				sanitizeQuery(req, policy)
			}
			if form {
				sanitizeForm(req, policy)
			}
			if len(headers) > 0 {
				sanitizeHeaders(req, policy, headers)
			}

			next.ServeHTTP(w, req)
		})
	}
}

// Sanitize sanitizes a string using the application's configured policy. It is
// intended for targeted use in handlers (e.g. before storing user input, or
// for fields exempted from the global middleware).
func (r *Regius) Sanitize(s string) string {
	return r.Sanitizer().Sanitize(s)
}

// Sanitizer returns the bluemonday policy derived from the application's
// configured request sanitizer settings. Use it for advanced sanitization
// needs beyond a single string.
func (r *Regius) Sanitizer() *bluemonday.Policy {
	if r.config.requestSanitizer.Custom != nil {
		return r.config.requestSanitizer.Custom
	}
	return newSanitizePolicy(r.config.requestSanitizer.Policy)
}

// Sanitize sanitizes a string using the strict policy. It is a package-level
// convenience for use without a Regius instance (e.g. in helpers that only
// import the regius package).
func Sanitize(s string) string {
	return strictSanitizePolicy().Sanitize(s)
}

func newSanitizePolicy(policy string) *bluemonday.Policy {
	if strings.EqualFold(policy, SanitizePolicyUGC) {
		return ugcSanitizePolicy()
	}
	return strictSanitizePolicy()
}

// BoolPtr returns a pointer to b. Use it to populate the *bool config fields
// (Query, Form): a nil field defaults to true, while BoolPtr(false) explicitly
// disables that scope.
func BoolPtr(b bool) *bool {
	return &b
}

func canonicalHeaders(headers []string) []string {
	if len(headers) == 0 {
		return nil
	}
	out := make([]string, 0, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for _, h := range headers {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		c := http.CanonicalHeaderKey(h)
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

// sanitizeQuery rewrites req.URL.RawQuery with sanitized values when any value
// changed. It must run before sanitizeForm so that values merged from the query
// into req.Form (during ParseForm) are already clean.
func sanitizeQuery(req *http.Request, p *bluemonday.Policy) {
	q := req.URL.Query()
	changed := false
	for k, vs := range q {
		for i, v := range vs {
			if s := p.Sanitize(v); s != v {
				vs[i] = s
				changed = true
			}
		}
		q[k] = vs
	}
	if changed {
		req.URL.RawQuery = q.Encode()
	}
}

// sanitizeForm parses (if needed) and sanitizes form-encoded values. For
// non-form content types it is a no-op: JSON and other bodies are never read,
// so handlers can still access req.Body. Multipart text fields are sanitized in
// req.Form, req.PostForm, and req.MultipartForm.Value; file parts are left
// untouched.
func sanitizeForm(req *http.Request, p *bluemonday.Policy) {
	ct := req.Header.Get("Content-Type")
	parsed := false
	switch {
	case isFormURLEncoded(ct):
		if err := req.ParseForm(); err == nil {
			parsed = true
		}
	case isMultipart(ct):
		if err := req.ParseMultipartForm(sanitizeMaxFormMemory); err == nil {
			parsed = true
		}
	}
	if !parsed {
		return
	}
	sanitizeValues(req.Form, p)
	if req.PostForm != nil {
		sanitizeValues(req.PostForm, p)
	}
	if req.MultipartForm != nil {
		sanitizeValues(req.MultipartForm.Value, p)
	}
}

func sanitizeHeaders(req *http.Request, p *bluemonday.Policy, headers []string) {
	for _, h := range headers {
		vals := req.Header.Values(h)
		if len(vals) == 0 {
			continue
		}
		changed := false
		for i, v := range vals {
			if s := p.Sanitize(v); s != v {
				vals[i] = s
				changed = true
			}
		}
		if changed {
			req.Header[h] = vals
		}
	}
}

func sanitizeValues(v url.Values, p *bluemonday.Policy) {
	for key, vs := range v {
		for i, val := range vs {
			if s := p.Sanitize(val); s != val {
				vs[i] = s
			}
		}
		v[key] = vs
	}
}

func isFormURLEncoded(ct string) bool {
	ct = strings.TrimSpace(strings.ToLower(ct))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct == "application/x-www-form-urlencoded"
}

func isMultipart(ct string) bool {
	ct = strings.TrimSpace(strings.ToLower(ct))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct == "multipart/form-data"
}

// sanitizeMaxFormMemory is the memory threshold (bytes) passed to
// ParseMultipartForm; data above this is buffered to disk. It mirrors
// http.defaultMaxMemory.
const sanitizeMaxFormMemory = 32 << 20

var (
	strictSanitizeOnce sync.Once
	strictSanitizePol  *bluemonday.Policy

	ugcSanitizeOnce sync.Once
	ugcSanitizePol  *bluemonday.Policy
)

func strictSanitizePolicy() *bluemonday.Policy {
	strictSanitizeOnce.Do(func() {
		strictSanitizePol = bluemonday.StrictPolicy()
	})
	return strictSanitizePol
}

func ugcSanitizePolicy() *bluemonday.Policy {
	ugcSanitizeOnce.Do(func() {
		ugcSanitizePol = bluemonday.UGCPolicy()
	})
	return ugcSanitizePol
}
