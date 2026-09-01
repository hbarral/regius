package regius

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// ValidateStruct validates s (a struct or pointer to struct) against
// `validate` struct tags and records any failures in Errors.
//
// Supported tags (comma-separated):
//
//	required        non-empty value (strings are trimmed; zero values fail)
//	nested          recurse into a nested struct, *struct, or slice of structs
//	field=name      override the error key for this field (leaf name only)
//	min=N           string fields: minimum rune length; numeric fields: minimum value
//	max=N           string fields: maximum rune length; numeric fields: maximum value
//	len=N           exact rune length (string fields only)
//	range=N:M       numeric bounds (string fields are parsed as integers)
//	oneof=a b c     value must be one of the space-separated values
//	regex=PATTERN   value must match the regular expression
//
// Any other token is looked up in the rule registry (built-in rules such as
// email, uuid, url, phone, int, float, dateiso, json, ip, boolean, alpha,
// alphanumeric, numeric, creditcard, nospaces, plus custom rules registered
// via RegisterValidation).
//
// Non-required rules are skipped for empty values; pair a format rule with
// required to enforce presence. Errors use dot-separated paths for nested
// fields (e.g. "Address.City") and numeric indices for slice elements
// (e.g. "Items.0.Name").
func (v *Validation) ValidateStruct(s interface{}) {
	if s == nil {
		return
	}
	val := reflect.ValueOf(s)
	val = derefValue(val)
	if (val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface) && val.IsNil() {
		return
	}
	if val.Kind() != reflect.Struct {
		panic("regius: ValidateStruct expects a struct or pointer to struct")
	}
	v.validateStruct(val, "")
}

func derefValue(val reflect.Value) reflect.Value {
	for val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface {
		if val.IsNil() {
			return val
		}
		val = val.Elem()
	}
	return val
}

func (v *Validation) validateStruct(val reflect.Value, path string) {
	t := val.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		tag := sf.Tag.Get("validate")
		if tag == "" || tag == "-" {
			continue
		}
		v.applyTagRules(val.Field(i), tag, path, sf.Name)
	}
}

func (v *Validation) applyTagRules(fv reflect.Value, tag, path, name string) {
	tokens := strings.Split(tag, ",")
	for _, tok := range tokens {
		if n, ok := strings.CutPrefix(strings.TrimSpace(tok), "field="); ok {
			name = n
		}
	}
	key := name
	if path != "" {
		key = path + "." + name
	}

	required := false
	nested := false
	for _, tok := range tokens {
		switch strings.TrimSpace(tok) {
		case "required":
			required = true
		case "nested":
			nested = true
		}
	}

	if required && v.isZeroValue(fv) {
		v.addError(key, "validation.required", "This field cannot be blank", nil)
		return
	}

	elem := derefValue(fv)

	if nested {
		v.validateNested(elem, key)
		return
	}

	if elem.Kind() == reflect.Slice || elem.Kind() == reflect.Array {
		if isStringSlice(elem) {
			for i := 0; i < elem.Len(); i++ {
				e := elem.Index(i)
				if v.isZeroValue(e) {
					continue
				}
				v.applyRules(tokens, tag, derefValue(e), fmt.Sprintf("%s.%d", key, i))
			}
		}
		return
	}

	if v.isZeroValue(fv) {
		return
	}

	v.applyRules(tokens, tag, elem, key)
}

func (v *Validation) validateNested(elem reflect.Value, key string) {
	switch elem.Kind() {
	case reflect.Struct:
		v.validateStruct(elem, key)
	case reflect.Slice, reflect.Array:
		for i := 0; i < elem.Len(); i++ {
			e := derefValue(elem.Index(i))
			if e.Kind() == reflect.Struct {
				v.validateStruct(e, fmt.Sprintf("%s.%d", key, i))
			}
		}
	}
}

func (v *Validation) applyRules(tokens []string, tag string, fv reflect.Value, key string) {
	str := fieldStringValue(fv)
	num, hasNum := fieldNumericValue(fv)

	for _, raw := range tokens {
		tok := strings.TrimSpace(raw)
		switch {
		case tok == "" || tok == "required" || tok == "nested" || strings.HasPrefix(tok, "field="):
			continue

		case strings.HasPrefix(tok, "min="):
			n := tagInt(tok, tag)
			if hasNum {
				if num < float64(n) {
					v.addError(key, "validation.min",
						fmt.Sprintf("This field must be at least %d", n),
						map[string]string{"min": strconv.Itoa(n)})
				}
			} else {
				v.IsMinLength(key, str, n)
			}

		case strings.HasPrefix(tok, "max="):
			n := tagInt(tok, tag)
			if hasNum {
				if num > float64(n) {
					v.addError(key, "validation.max",
						fmt.Sprintf("This field must be at most %d", n),
						map[string]string{"max": strconv.Itoa(n)})
				}
			} else {
				v.IsMaxLength(key, str, n)
			}

		case strings.HasPrefix(tok, "len="):
			v.IsLength(key, str, tagInt(tok, tag))

		case strings.HasPrefix(tok, "range="):
			lo, hi, ok := parseRangeTag(tok)
			if !ok {
				panic(fmt.Sprintf("regius: invalid tag %q (want range=N:M)", tok))
			}
			if hasNum {
				if num < float64(lo) || num > float64(hi) {
					v.addError(key, "validation.range",
						fmt.Sprintf("This field must be between %d and %d", lo, hi),
						map[string]string{"min": strconv.Itoa(lo), "max": strconv.Itoa(hi)})
				}
			} else {
				v.IsRange(key, str, lo, hi)
			}

		case strings.HasPrefix(tok, "oneof="):
			allowed := strings.Fields(tok[6:])
			found := false
			for _, a := range allowed {
				if str == a {
					found = true
					break
				}
			}
			if !found {
				v.addError(key, "validation.one_of",
					fmt.Sprintf("This field must be one of: %s", strings.Join(allowed, ", ")),
					map[string]string{"values": strings.Join(allowed, ", ")})
			}

		case strings.HasPrefix(tok, "regex="):
			re, err := regexp.Compile(tok[6:])
			if err != nil {
				panic(fmt.Sprintf("regius: invalid regex in tag %q: %v", tok, err))
			}
			v.MatchesPattern(key, str, re)

		default:
			if !v.hasRule(tok) {
				panic(fmt.Sprintf("regius: unknown validation rule %q in tag %q", tok, tag))
			}
			v.Rule(tok, key, str)
		}
	}
}

func (v *Validation) hasRule(name string) bool {
	if v.regius != nil && v.regius.HasValidation(name) {
		return true
	}
	_, ok := defaultValidationRules[name]
	return ok
}

func (v *Validation) isZeroValue(fv reflect.Value) bool {
	if fv.Kind() == reflect.String {
		return strings.TrimSpace(fv.String()) == ""
	}
	return fv.IsZero()
}

func isStringSlice(elem reflect.Value) bool {
	if elem.Kind() != reflect.Slice && elem.Kind() != reflect.Array {
		return false
	}
	t := elem.Type().Elem()
	if t.Kind() == reflect.String {
		return true
	}
	return t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.String
}

func fieldStringValue(fv reflect.Value) string {
	switch fv.Kind() {
	case reflect.String:
		return fv.String()
	case reflect.Bool:
		return strconv.FormatBool(fv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(fv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(fv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(fv.Float(), 'f', -1, 64)
	default:
		return ""
	}
}

func fieldNumericValue(fv reflect.Value) (float64, bool) {
	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(fv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(fv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return fv.Float(), true
	case reflect.String:
		f, err := strconv.ParseFloat(fv.String(), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func tagInt(tok, tag string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(tok, minMaxLenPrefix(tok)))
	if err != nil {
		panic(fmt.Sprintf("regius: invalid number in tag %q: %v", tok, err))
	}
	return n
}

func minMaxLenPrefix(tok string) string {
	for _, p := range []string{"min=", "max=", "len="} {
		if strings.HasPrefix(tok, p) {
			return p
		}
	}
	return ""
}

func parseRangeTag(tok string) (int, int, bool) {
	rest, ok := strings.CutPrefix(tok, "range=")
	if !ok {
		return 0, 0, false
	}
	lo, hi, ok := strings.Cut(rest, ":")
	if !ok {
		return 0, 0, false
	}
	loi, err1 := strconv.Atoi(lo)
	hii, err2 := strconv.Atoi(hi)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return loi, hii, true
}
