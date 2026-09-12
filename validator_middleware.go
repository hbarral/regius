package regius

import (
	"context"
	"encoding/gob"
	"net/http"
	"net/url"
	"reflect"
	"strings"
)

// The "form" error format stores field errors in the session; scs encodes
// session values with gob, which requires composite types to be registered
// before they can round-trip through the session store.
func init() {
	gob.Register(map[string]string{})
}

// ValidationConfig configures the ValidateRequest middleware.
type ValidationConfig struct {
	// StructType is a sample value (e.g. SignupInput{}), a typed nil pointer
	// (e.g. (*SignupInput)(nil)), or a reflect.Type. Requests with a JSON
	// body are decoded into a new instance of this type and validated via
	// its `validate` struct tags. On success the decoded pointer is stored
	// in the request context; retrieve it with
	// ValidatedFromContext[*SignupInput](r.Context()).
	StructType interface{}

	// Rules validates form-encoded or multipart bodies field by field. Each
	// entry maps a form field name to a rule tag (e.g. "required,email").
	Rules map[string]string

	// ErrorFormat controls the failure response. "json" (the default)
	// responds 400 with the API error envelope and the localized field
	// errors as details. "form" stores the localized field errors in the
	// session under the "validation_errors" key (readable with
	// PopValidationErrors) and redirects back to the Referer, or RedirectTo
	// when set, with status 303.
	ErrorFormat string

	// RedirectTo is the fallback redirect target in "form" mode when the
	// request has no Referer header (default "/").
	RedirectTo string
}

type validatedContextKey struct{}

// ValidatedFromContext retrieves the struct decoded and validated by the
// ValidateRequest middleware. T must be the pointer type matching the
// middleware's StructType, e.g. ValidatedFromContext[*SignupInput](ctx).
func ValidatedFromContext[T any](ctx context.Context) (T, bool) {
	v, ok := ctx.Value(validatedContextKey{}).(T)
	return v, ok
}

// PopValidationErrors removes and returns the field errors stored in the
// session by the ValidateRequest middleware in "form" mode. Returns nil
// when there are none.
func (r *Regius) PopValidationErrors(ctx context.Context) map[string]string {
	if r.Session == nil {
		return nil
	}
	m, _ := r.Session.Pop(ctx, "validation_errors").(map[string]string)
	return m
}

func validationStructType(v interface{}) reflect.Type {
	if v == nil {
		return nil
	}
	t, ok := v.(reflect.Type)
	if !ok {
		t = reflect.TypeOf(v)
	}
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// ValidateRequest returns a middleware that validates the incoming request
// body and short-circuits with an error response when validation fails.
// JSON bodies (Content-Type application/json) are decoded into
// ValidationConfig.StructType and validated against its `validate` tags;
// form-encoded and multipart bodies are validated field by field against
// ValidationConfig.Rules. Apply it to individual routes or route groups,
// not globally.
//
// On success the decoded struct pointer is available downstream via
// ValidatedFromContext. On failure the middleware responds without calling
// the next handler: a 400 API error envelope with the localized field
// errors as details (ErrorFormat "json"), or a 303 redirect with the errors
// stored in the session (ErrorFormat "form", requires the session
// middleware).
func (r *Regius) ValidateRequest(cfg ValidationConfig) func(http.Handler) http.Handler {
	structType := validationStructType(cfg.StructType)
	if structType == nil && len(cfg.Rules) == 0 {
		panic("regius: ValidateRequest requires StructType or Rules")
	}
	if structType != nil && structType.Kind() != reflect.Struct {
		panic("regius: ValidateRequest StructType must be a struct")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			v := r.Validator(url.Values{})

			isJSON := strings.Contains(req.Header.Get("Content-Type"), "application/json")

			var validated interface{}

			switch {
			case isJSON && structType != nil:
				ptr := reflect.New(structType).Interface()
				if err := r.ReadJSON(w, req, ptr); err != nil {
					_ = r.WriteAPIError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body", err.Error())
					return
				}
				v.ValidateStruct(ptr)
				validated = ptr

			case len(cfg.Rules) > 0:
				if err := req.ParseForm(); err != nil {
					_ = r.WriteAPIError(w, http.StatusBadRequest, "invalid_form", "Invalid form body", err.Error())
					return
				}
				for field, tag := range cfg.Rules {
					v.applyTagRules(reflect.ValueOf(req.Form.Get(field)), tag, "", field)
				}

			default:
				// StructType is set but the request body is not JSON.
				_ = r.WriteAPIError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Expected application/json body")
				return
			}

			if !v.Valid() {
				r.writeValidationErrors(w, req, v, cfg)
				return
			}

			if validated != nil {
				ctx := context.WithValue(req.Context(), validatedContextKey{}, validated)
				req = req.WithContext(ctx)
			}
			next.ServeHTTP(w, req)
		})
	}
}

func (r *Regius) writeValidationErrors(w http.ResponseWriter, req *http.Request, v *Validation, cfg ValidationConfig) {
	fieldErrors := v.LocalizedErrors(req.Context())

	if strings.EqualFold(cfg.ErrorFormat, "form") && r.Session != nil {
		r.Session.Put(req.Context(), "validation_errors", fieldErrors)
		target := req.Header.Get("Referer")
		if target == "" {
			target = cfg.RedirectTo
		}
		if target == "" {
			target = "/"
		}
		http.Redirect(w, req, target, http.StatusSeeOther)
		return
	}

	_ = r.WriteAPIError(w, http.StatusBadRequest, "validation_error", "The given data was invalid.", fieldErrors)
}
