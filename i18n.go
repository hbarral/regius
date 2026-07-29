package regius

import (
	"errors"
	"net/http"
	"strings"

	"github.com/invopop/ctxi18n"

	ri18n "github.com/hbarral/regius/i18n"
)

// I18nConfig holds internationalization settings.
type I18nConfig struct {
	Enabled          bool
	DefaultLocale    string
	SupportedLocales []string
	CookieName       string
}

// IsSupported reports whether locale is one of the configured supported
// locales. The comparison is case-insensitive.
func (c I18nConfig) IsSupported(locale string) bool {
	locale = strings.ToLower(locale)
	for _, l := range c.SupportedLocales {
		if strings.ToLower(l) == locale {
			return true
		}
	}
	return false
}

// Language returns middleware that detects the request locale from (in order):
//  1. the configured locale cookie
//  2. the Accept-Language header
//  3. the configured default locale
//
// The resolved locale is attached to the request context so templates and
// handlers can use the i18n package helpers.
func (r *Regius) Language(cfg I18nConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !cfg.Enabled {
				next.ServeHTTP(w, req)
				return
			}

			locale := cfg.DefaultLocale

			if c, err := req.Cookie(cfg.CookieName); err == nil && c.Value != "" {
				locale = c.Value
			} else if accept := req.Header.Get("Accept-Language"); accept != "" {
				if matched := ri18n.Match(accept); matched != "" {
					locale = matched
				}
			}

			if !cfg.IsSupported(locale) {
				locale = cfg.DefaultLocale
			}

			ctx, err := ri18n.WithLocale(req.Context(), locale)
			if err != nil {
				if !errors.Is(err, ctxi18n.ErrMissingLocale) {
					r.ErrorLog.Printf("i18n: failed to set locale %q: %v", locale, err)
				}
				ctx = req.Context()
			}

			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}
