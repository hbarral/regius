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

type Validation struct {
	Data   url.Values
	Errors map[string]string
}

func (r *Regius) Validator(data url.Values) *Validation {
	return &Validation{
		Errors: make(map[string]string),
		Data:   data,
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
			v.AddError(field, "This field cannot be blank")
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
		v.AddError(field, "Invalid email address")
	}
}

func (v *Validation) IsInt(field, value string) {
	_, err := strconv.Atoi(value)
	if err != nil {
		v.AddError(field, "This field must be an integer")
	}
}

func (v *Validation) IsFloat(field, value string) {
	_, err := strconv.ParseFloat(value, 64)
	if err != nil {
		v.AddError(field, "This field must be a floating point number")
	}
}

func (v *Validation) IsDateISO(field, value string) {
	_, err := time.Parse("2006-01-02", value)
	if err != nil {
		v.AddError(field, "This field must be a date in the form of YYYY-MM-DD")
	}
}

func (v *Validation) NoSpaces(field, value string) {
	if govalidator.HasWhitespace(value) {
		v.AddError(field, "No spaces allowed")
	}
}

var (
	uuidRegex  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	phoneRegex = regexp.MustCompile(`^\+?[0-9]{7,15}$`)
)

func (v *Validation) IsURL(field, value string) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		v.AddError(field, "This field must be a valid URL")
	}
}

func (v *Validation) IsUUID(field, value string) {
	if !uuidRegex.MatchString(value) {
		v.AddError(field, "This field must be a valid UUID")
	}
}

func (v *Validation) IsPhone(field, value string) {
	if !phoneRegex.MatchString(value) {
		v.AddError(field, "This field must be a valid phone number")
	}
}

func (v *Validation) IsCreditCard(field, value string) {
	if !govalidator.IsCreditCard(value) {
		v.AddError(field, "This field must be a valid credit card number")
	}
}

func (v *Validation) IsAlpha(field, value string) {
	if !govalidator.IsAlpha(value) {
		v.AddError(field, "This field must contain only letters")
	}
}

func (v *Validation) IsAlphanumeric(field, value string) {
	if !govalidator.IsAlphanumeric(value) {
		v.AddError(field, "This field must contain only letters and numbers")
	}
}

func (v *Validation) IsNumeric(field, value string) {
	if !govalidator.IsNumeric(value) {
		v.AddError(field, "This field must contain only digits")
	}
}

func (v *Validation) IsMinLength(field, value string, min int) {
	if utf8.RuneCountInString(value) < min {
		v.AddError(field, fmt.Sprintf("This field must be at least %d characters long", min))
	}
}

func (v *Validation) IsMaxLength(field, value string, max int) {
	if utf8.RuneCountInString(value) > max {
		v.AddError(field, fmt.Sprintf("This field must be at most %d characters long", max))
	}
}

func (v *Validation) IsLength(field, value string, n int) {
	if utf8.RuneCountInString(value) != n {
		v.AddError(field, fmt.Sprintf("This field must be exactly %d characters long", n))
	}
}

func (v *Validation) IsRange(field, value string, min, max int) {
	n, err := strconv.Atoi(value)
	if err != nil {
		v.AddError(field, "This field must be an integer")
		return
	}
	if n < min || n > max {
		v.AddError(field, fmt.Sprintf("This field must be between %d and %d", min, max))
	}
}

func (v *Validation) IsJSON(field, value string) {
	if !json.Valid([]byte(value)) {
		v.AddError(field, "This field must be valid JSON")
	}
}

func (v *Validation) IsIP(field, value string) {
	if net.ParseIP(value) == nil {
		v.AddError(field, "This field must be a valid IP address")
	}
}

func (v *Validation) IsBoolean(field, value string) {
	switch strings.ToLower(value) {
	case "true", "false", "1", "0", "yes", "no":
		return
	default:
		v.AddError(field, "This field must be a boolean value")
	}
}

func (v *Validation) MatchesPattern(field, value string, re *regexp.Regexp) {
	if !re.MatchString(value) {
		v.AddError(field, "This field has an invalid format")
	}
}
