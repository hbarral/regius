package regius

import (
	"context"
	"strings"

	"github.com/hbarral/regius/i18n"
)

// ValidationError is the structured form of a rule failure: the i18n key and
// interpolation params needed to translate the message, plus the English
// fallback message used when no translation is available.
type ValidationError struct {
	Field  string            // error key, e.g. "Email" or "Address.City"
	Key    string            // i18n key, e.g. "validation.email"
	Params map[string]string // interpolation params, e.g. {"min": "8"}
	Msg    string            // fallback message (English default)
}

// addError records a rule failure with its i18n key and params. It has the
// same first-wins semantics as AddError: only the first failure per field is
// kept, in both Errors and Details.
func (v *Validation) addError(field, key, msg string, params map[string]string) {
	if _, exists := v.Errors[field]; exists {
		return
	}
	v.Errors[field] = msg
	v.Details = append(v.Details, ValidationError{
		Field:  field,
		Key:    key,
		Params: params,
		Msg:    msg,
	})
}

// LocalizedErrors returns the error messages translated for the locale in
// ctx (as set by the Language middleware or i18n.WithLocale). Fields whose
// errors were added via AddError (custom messages) are returned unchanged;
// rule failures translate via their i18n key, falling back to the default
// English message when no translation is loaded.
func (v *Validation) LocalizedErrors(ctx context.Context) map[string]string {
	out := make(map[string]string, len(v.Errors))
	for field, msg := range v.Errors {
		out[field] = msg
	}
	for _, d := range v.Details {
		args := []any{i18n.Default(d.Msg)}
		if len(d.Params) > 0 {
			m := make(i18n.M, len(d.Params))
			for k, val := range d.Params {
				m[k] = val
			}
			args = append(args, m)
		}
		msg := i18n.T(ctx, d.Key, args...)
		if strings.Contains(msg, "!(MISSING") {
			msg = d.Msg
		}
		out[d.Field] = msg
	}
	return out
}
