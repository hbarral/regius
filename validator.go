package regius

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/asaskevich/govalidator"
)

type ValidationFunc func(value string) bool

type Validation struct {
	Data    url.Values
	Errors  map[string]string
	Details []ValidationError
	regius  *Regius
}

func (r *Regius) Validator(data url.Values) *Validation {
	return &Validation{
		Errors: make(map[string]string),
		Data:   data,
		regius: r,
	}
}

func (v *Validation) Valid() bool {
	return len(v.Errors) == 0
}

func (v *Validation) AddError(key, message string) {
	if _, exists := v.Errors[key]; !exists {
		v.Errors[key] = message
	}
}

func (v *Validation) Has(field string, r *http.Request) bool {
	x := r.Form.Get(field)

	return x != ""
}

func (v *Validation) Required(r *http.Request, fields ...string) {
	for _, field := range fields {
		value := r.Form.Get(field)
		if strings.TrimSpace(value) == "" {
			v.addError(field, "validation.required", "This field cannot be blank", nil)
		}
	}
}

func (v *Validation) Check(ok bool, key, message string) {
	if !ok {
		v.AddError(key, message)
	}
}

func (v *Validation) IsEmail(field, value string) {
	if !govalidator.IsEmail(value) {
		v.addError(field, "validation.email", "Invalid email address", nil)
	}
}

func (v *Validation) IsInt(field, value string) {
	_, err := strconv.Atoi(value)
	if err != nil {
		v.addError(field, "validation.integer", "This field must be an integer", nil)
	}
}

func (v *Validation) IsFloat(field, value string) {
	_, err := strconv.ParseFloat(value, 64)
	if err != nil {
		v.addError(field, "validation.float", "This field must be a floating point number", nil)
	}
}

func (v *Validation) IsDateISO(field, value string) {
	_, err := time.Parse("2006-01-02", value)
	if err != nil {
		v.addError(field, "validation.date_iso", "This field must be a date in the form of YYYY-MM-DD", nil)
	}
}

func (v *Validation) NoSpaces(field, value string) {
	if govalidator.HasWhitespace(value) {
		v.addError(field, "validation.no_spaces", "No spaces allowed", nil)
	}
}

var (
	uuidRegex  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	phoneRegex = regexp.MustCompile(`^\+?[0-9]{7,15}$`)
)

func (v *Validation) IsURL(field, value string) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		v.addError(field, "validation.url", "This field must be a valid URL", nil)
	}
}

func (v *Validation) IsUUID(field, value string) {
	if !uuidRegex.MatchString(value) {
		v.addError(field, "validation.uuid", "This field must be a valid UUID", nil)
	}
}

func (v *Validation) IsPhone(field, value string) {
	if !phoneRegex.MatchString(value) {
		v.addError(field, "validation.phone", "This field must be a valid phone number", nil)
	}
}

func (v *Validation) IsCreditCard(field, value string) {
	if !govalidator.IsCreditCard(value) {
		v.addError(field, "validation.credit_card", "This field must be a valid credit card number", nil)
	}
}

func (v *Validation) IsAlpha(field, value string) {
	if !govalidator.IsAlpha(value) {
		v.addError(field, "validation.alpha", "This field must contain only letters", nil)
	}
}

func (v *Validation) IsAlphanumeric(field, value string) {
	if !govalidator.IsAlphanumeric(value) {
		v.addError(field, "validation.alphanumeric", "This field must contain only letters and numbers", nil)
	}
}

func (v *Validation) IsNumeric(field, value string) {
	if !govalidator.IsNumeric(value) {
		v.addError(field, "validation.numeric", "This field must contain only digits", nil)
	}
}

func (v *Validation) IsMinLength(field, value string, min int) {
	if utf8.RuneCountInString(value) < min {
		v.addError(field, "validation.min_length",
			fmt.Sprintf("This field must be at least %d characters long", min),
			map[string]string{"min": strconv.Itoa(min)})
	}
}

func (v *Validation) IsMaxLength(field, value string, max int) {
	if utf8.RuneCountInString(value) > max {
		v.addError(field, "validation.max_length",
			fmt.Sprintf("This field must be at most %d characters long", max),
			map[string]string{"max": strconv.Itoa(max)})
	}
}

func (v *Validation) IsLength(field, value string, n int) {
	if utf8.RuneCountInString(value) != n {
		v.addError(field, "validation.length",
			fmt.Sprintf("This field must be exactly %d characters long", n),
			map[string]string{"count": strconv.Itoa(n)})
	}
}

func (v *Validation) IsRange(field, value string, min, max int) {
	n, err := strconv.Atoi(value)
	if err != nil {
		v.addError(field, "validation.integer", "This field must be an integer", nil)
		return
	}
	if n < min || n > max {
		v.addError(field, "validation.range",
			fmt.Sprintf("This field must be between %d and %d", min, max),
			map[string]string{"min": strconv.Itoa(min), "max": strconv.Itoa(max)})
	}
}

func (v *Validation) IsJSON(field, value string) {
	if !json.Valid([]byte(value)) {
		v.addError(field, "validation.json", "This field must be valid JSON", nil)
	}
}

func (v *Validation) IsIP(field, value string) {
	if net.ParseIP(value) == nil {
		v.addError(field, "validation.ip", "This field must be a valid IP address", nil)
	}
}

func (v *Validation) IsBoolean(field, value string) {
	switch strings.ToLower(value) {
	case "true", "false", "1", "0", "yes", "no":
		return
	default:
		v.addError(field, "validation.boolean", "This field must be a boolean value", nil)
	}
}

func (v *Validation) MatchesPattern(field, value string, re *regexp.Regexp) {
	if !re.MatchString(value) {
		v.addError(field, "validation.pattern", "This field has an invalid format", nil)
	}
}

var defaultValidationRules = map[string]ValidationFunc{
	"email":        govalidator.IsEmail,
	"url":          func(val string) bool { u, err := url.Parse(val); return err == nil && u.Scheme != "" && u.Host != "" },
	"uuid":         uuidRegex.MatchString,
	"phone":        phoneRegex.MatchString,
	"creditcard":   govalidator.IsCreditCard,
	"alpha":        govalidator.IsAlpha,
	"alphanumeric": govalidator.IsAlphanumeric,
	"numeric":      govalidator.IsNumeric,
	"int":          func(val string) bool { _, err := strconv.Atoi(val); return err == nil },
	"float":        func(val string) bool { _, err := strconv.ParseFloat(val, 64); return err == nil },
	"dateiso":      func(val string) bool { _, err := time.Parse("2006-01-02", val); return err == nil },
	"json":         func(val string) bool { return json.Valid([]byte(val)) },
	"ip":           func(val string) bool { return net.ParseIP(val) != nil },
	"boolean": func(val string) bool {
		switch strings.ToLower(val) {
		case "true", "false", "1", "0", "yes", "no":
			return true
		}
		return false
	},
	"nospaces": func(val string) bool { return !govalidator.HasWhitespace(val) },
}

type validationRuleDefault struct {
	key string
	msg string
}

var validationRuleDefaults = map[string]validationRuleDefault{
	"email":        {"validation.email", "Invalid email address"},
	"url":          {"validation.url", "This field must be a valid URL"},
	"uuid":         {"validation.uuid", "This field must be a valid UUID"},
	"phone":        {"validation.phone", "This field must be a valid phone number"},
	"creditcard":   {"validation.credit_card", "This field must be a valid credit card number"},
	"alpha":        {"validation.alpha", "This field must contain only letters"},
	"alphanumeric": {"validation.alphanumeric", "This field must contain only letters and numbers"},
	"numeric":      {"validation.numeric", "This field must contain only digits"},
	"int":          {"validation.integer", "This field must be an integer"},
	"float":        {"validation.float", "This field must be a floating point number"},
	"dateiso":      {"validation.date_iso", "This field must be a date in the form of YYYY-MM-DD"},
	"json":         {"validation.json", "This field must be valid JSON"},
	"ip":           {"validation.ip", "This field must be a valid IP address"},
	"boolean":      {"validation.boolean", "This field must be a boolean value"},
	"nospaces":     {"validation.no_spaces", "No spaces allowed"},
}

func (r *Regius) RegisterValidation(name string, fn ValidationFunc) {
	if r.validationRules == nil {
		r.validationRules = make(map[string]ValidationFunc, len(defaultValidationRules))
		for k, v := range defaultValidationRules {
			r.validationRules[k] = v
		}
	}
	r.validationRules[name] = fn
}

func (r *Regius) HasValidation(name string) bool {
	if r.validationRules != nil {
		if _, ok := r.validationRules[name]; ok {
			return true
		}
	}
	_, ok := defaultValidationRules[name]
	return ok
}

func (v *Validation) Rule(name, field, value string, message ...string) {
	var fn ValidationFunc
	if v.regius != nil && v.regius.validationRules != nil {
		fn = v.regius.validationRules[name]
	}
	if fn == nil {
		fn = defaultValidationRules[name]
	}
	if fn == nil {
		panic(fmt.Sprintf("regius: unknown validation rule %q", name))
	}
	if !fn(value) {
		if len(message) > 0 {
			v.AddError(field, message[0])
			return
		}
		if d, ok := validationRuleDefaults[name]; ok {
			v.addError(field, d.key, d.msg, nil)
			return
		}
		v.addError(field, "validation.failed",
			fmt.Sprintf("Validation failed for %s", field),
			map[string]string{"field": field})
	}
}
