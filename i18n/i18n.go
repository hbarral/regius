package i18n

import (
	"context"
	"io/fs"

	"github.com/invopop/ctxi18n"
	ctxi18ni18n "github.com/invopop/ctxi18n/i18n"
)

// DefaultLocale is the fallback locale used when no other locale can be
// determined or when an unsupported locale is requested.
const DefaultLocale = "en"

// M is a map of interpolation values.
type M = ctxi18ni18n.M

// Default provides a fallback value when a translation key is missing.
func Default(v string) ctxi18ni18n.DefaultText {
	return ctxi18ni18n.Default(v)
}

// Load loads all locale definitions from the provided filesystem. Files may be
// YAML or JSON and are deep-merged into a global locale index.
func Load(fs fs.FS) error {
	return ctxi18n.Load(fs)
}

// LoadWithDefault loads locale definitions and merges the default locale into
// every other locale so missing translations fall back to the default language.
func LoadWithDefault(fs fs.FS, defaultLocale string) error {
	return ctxi18n.LoadWithDefault(fs, ctxi18ni18n.Code(defaultLocale))
}

// WithLocale attaches the best matching locale to the context. The locale
// string may be a simple code ("en") or an Accept-Language header value.
func WithLocale(ctx context.Context, locale string) (context.Context, error) {
	return ctxi18n.WithLocale(ctx, locale)
}

// T returns the translation for key using the locale stored in ctx.
func T(ctx context.Context, key string, args ...any) string {
	return ctxi18ni18n.T(ctx, key, args...)
}

// N returns the pluralized translation for key using the locale stored in ctx.
func N(ctx context.Context, key string, count int, args ...any) string {
	return ctxi18ni18n.N(ctx, key, count, args...)
}

// Has reports whether key exists in the current locale.
func Has(ctx context.Context, key string) bool {
	return ctxi18ni18n.Has(ctx, key)
}

// Locale returns the locale code currently stored in ctx, or DefaultLocale if
// none is set.
func Locale(ctx context.Context) string {
	l := ctxi18n.Locale(ctx)
	if l == nil {
		return DefaultLocale
	}
	return string(l.Code())
}

// Match returns the best matching locale code for the provided Accept-Language
// style string, or an empty string if no match can be made.
func Match(acceptLanguage string) string {
	l := ctxi18n.Match(acceptLanguage)
	if l == nil {
		return ""
	}
	return string(l.Code())
}
