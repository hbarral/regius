package regius

import (
	"net/http"
	"strconv"
)

// SecurityHeadersConfig holds configuration for HTTP security response
// headers, providing an Express "helmet" equivalent for Regius.
//
// When Enabled is true, the middleware applies Helmet-style safe defaults for
// every field left at its zero value. Populate a field with a non-empty value
// to override the default for that header.
type SecurityHeadersConfig struct {
	Enabled bool

	// ContentSecurityPolicy overrides the default "default-src 'self'".
	ContentSecurityPolicy string

	// HSTS settings. Strict-Transport-Security is only emitted when the
	// application is running with Secure=true (see Server.Secure), since
	// emitting HSTS over plain HTTP is ignored by browsers and can lock
	// users out of a local development server.
	//
	// HSTSMaxAge defaults to 31536000 (one year) when zero.
	// HSTSIncludeSubDomains and HSTSPreload add the corresponding
	// directives when true.
	HSTSMaxAge            int
	HSTSIncludeSubDomains bool
	HSTSPreload           bool

	// ReferrerPolicy overrides the default "strict-origin-when-cross-origin".
	ReferrerPolicy string

	// XFrameOptions overrides the default "SAMEORIGIN".
	XFrameOptions string

	// XPermittedCrossDomainPolicies overrides the default "none".
	XPermittedCrossDomainPolicies string

	// CrossOriginOpenerPolicy overrides the default "same-origin".
	CrossOriginOpenerPolicy string

	// CrossOriginResourcePolicy overrides the default "same-origin".
	CrossOriginResourcePolicy string

	// XDNSPrefetchControl overrides the default "off".
	XDNSPrefetchControl string
}

const (
	defaultCSP                           = "default-src 'self'"
	defaultReferrerPolicy                = "strict-origin-when-cross-origin"
	defaultXFrameOptions                 = "SAMEORIGIN"
	defaultXPermittedCrossDomainPolicies = "none"
	defaultCrossOriginOpenerPolicy       = "same-origin"
	defaultCrossOriginResourcePolicy     = "same-origin"
	defaultXDNSPrefetchControl           = "off"
	defaultHSTSMaxAge                    = 31536000
)

// SecurityHeaders returns a middleware handler that sets a bundle of HTTP
// security response headers. When Enabled is false, it returns a no-op
// passthrough handler.
//
// The headers are set before invoking the downstream handler so that a route
// may still override any of them by calling w.Header().Set(...) before writing
// the response. Strict-Transport-Security is only emitted when the server is
// running in secure (HTTPS) mode.
func (r *Regius) SecurityHeaders(cfg SecurityHeadersConfig) func(next http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	csp := cfg.ContentSecurityPolicy
	if csp == "" {
		csp = defaultCSP
	}
	referrerPolicy := cfg.ReferrerPolicy
	if referrerPolicy == "" {
		referrerPolicy = defaultReferrerPolicy
	}
	xFrameOptions := cfg.XFrameOptions
	if xFrameOptions == "" {
		xFrameOptions = defaultXFrameOptions
	}
	xPermittedCrossDomainPolicies := cfg.XPermittedCrossDomainPolicies
	if xPermittedCrossDomainPolicies == "" {
		xPermittedCrossDomainPolicies = defaultXPermittedCrossDomainPolicies
	}
	coop := cfg.CrossOriginOpenerPolicy
	if coop == "" {
		coop = defaultCrossOriginOpenerPolicy
	}
	corp := cfg.CrossOriginResourcePolicy
	if corp == "" {
		corp = defaultCrossOriginResourcePolicy
	}
	xdnsPrefetchControl := cfg.XDNSPrefetchControl
	if xdnsPrefetchControl == "" {
		xdnsPrefetchControl = defaultXDNSPrefetchControl
	}

	hstsMaxAge := cfg.HSTSMaxAge
	if hstsMaxAge <= 0 {
		hstsMaxAge = defaultHSTSMaxAge
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			h := w.Header()

			h.Set("Content-Security-Policy", csp)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", referrerPolicy)
			h.Set("X-Frame-Options", xFrameOptions)
			h.Set("X-Permitted-Cross-Domain-Policies", xPermittedCrossDomainPolicies)
			h.Set("Cross-Origin-Opener-Policy", coop)
			h.Set("Cross-Origin-Resource-Policy", corp)
			h.Set("X-DNS-Prefetch-Control", xdnsPrefetchControl)

			// HSTS is only meaningful over HTTPS; reading Server.Secure at
			// request time (rather than middleware construction time) because
			// the routes are wired before Server is populated during New().
			if r.Server.Secure {
				hstsValue := "max-age=" + strconv.Itoa(hstsMaxAge)
				if cfg.HSTSIncludeSubDomains {
					hstsValue += "; includeSubDomains"
				}
				if cfg.HSTSPreload {
					hstsValue += "; preload"
				}
				h.Set("Strict-Transport-Security", hstsValue)
			}

			next.ServeHTTP(w, req)
		})
	}
}
