package regius

import (
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

type validateUser struct {
	Name    string `validate:"required,min=2,max=50"`
	Email   string `validate:"required,email"`
	Age     int    `validate:"required,min=18,max=120"`
	Website string `validate:"url"`
}

func TestValidation_ValidateStruct_Valid(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(validateUser{
		Name:  "Alice",
		Email: "alice@example.com",
		Age:   30,
	})

	assert.True(t, v.Valid())
}

func TestValidation_ValidateStruct_Invalid(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(validateUser{
		Name:  "A",
		Email: "not-an-email",
		Age:   10,
	})

	assert.False(t, v.Valid())
	assert.Contains(t, v.Errors, "Name")
	assert.Equal(t, "This field must be at least 2 characters long", v.Errors["Name"])
	assert.Contains(t, v.Errors, "Email")
	assert.Contains(t, v.Errors, "Age")
	assert.Equal(t, "This field must be at least 18", v.Errors["Age"])
}

func TestValidation_ValidateStruct_SkipsOptionalEmpty(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(validateUser{
		Name:  "Alice",
		Email: "alice@example.com",
		Age:   30,
		// Website intentionally empty
	})

	assert.NotContains(t, v.Errors, "Website", "empty optional field must be skipped")
}

func TestValidation_ValidateStruct_PointerToStruct(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(&validateUser{
		Name:  "Alice",
		Email: "alice@example.com",
		Age:   30,
	})

	assert.True(t, v.Valid())
}

func TestValidation_ValidateStruct_Nil(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	assert.NotPanics(t, func() {
		v.ValidateStruct(nil)
	})
	assert.True(t, v.Valid())
}

func TestValidation_ValidateStruct_NonStructPanics(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	assert.PanicsWithValue(t, "regius: ValidateStruct expects a struct or pointer to struct", func() {
		v.ValidateStruct(42)
	})
}

func TestValidation_ValidateStruct_TypedNilPointer(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	assert.NotPanics(t, func() {
		v.ValidateStruct((*validateUser)(nil))
	})
	assert.True(t, v.Valid())
}

type validateAddress struct {
	City string `validate:"required"`
	Zip  string `validate:"len=5"`
}

type validateOrder struct {
	ID      string          `validate:"required,uuid"`
	Address validateAddress `validate:"nested"`
}

func TestValidation_ValidateStruct_Nested(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(validateOrder{
		ID:      "not-a-uuid",
		Address: validateAddress{City: "", Zip: "123"},
	})

	assert.False(t, v.Valid())
	assert.Contains(t, v.Errors, "ID")
	assert.Contains(t, v.Errors, "Address.City", "nested field errors must use dot paths")
	assert.Equal(t, "This field cannot be blank", v.Errors["Address.City"])
	assert.Contains(t, v.Errors, "Address.Zip")
}

type validateProfile struct {
	Bio string `validate:"max=10"`
}

type validateAccount struct {
	Profile *validateProfile `validate:"nested"`
}

func TestValidation_ValidateStruct_NestedNilPointer(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(validateAccount{Profile: nil})

	assert.True(t, v.Valid(), "nil nested pointer without required must be skipped")
}

func TestValidation_ValidateStruct_NestedPointer(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(validateAccount{Profile: &validateProfile{Bio: "this bio is way too long"}})

	assert.Contains(t, v.Errors, "Profile.Bio")
}

type validateItem struct {
	Name  string `validate:"required"`
	Price int    `validate:"required,min=1"`
}

type validateCart struct {
	Items []validateItem `validate:"nested"`
}

func TestValidation_ValidateStruct_NestedSlice(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(validateCart{Items: []validateItem{
		{Name: "Widget", Price: 5},
		{Name: "", Price: 0},
	}})

	assert.Contains(t, v.Errors, "Items.1.Name", "slice element errors must use numeric indices")
	assert.Contains(t, v.Errors, "Items.1.Price")
	assert.NotContains(t, v.Errors, "Items.0.Name")
}

type validatePost struct {
	Tags []string `validate:"numeric"`
}

func TestValidation_ValidateStruct_StringSlice(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(validatePost{Tags: []string{"42", "abc", ""}})

	assert.Contains(t, v.Errors, "Tags.1")
	assert.NotContains(t, v.Errors, "Tags.0")
	assert.NotContains(t, v.Errors, "Tags.2", "empty elements must be skipped")
}

type validateStatus struct {
	Status string `validate:"required,oneof=active inactive pending"`
}

func TestValidation_ValidateStruct_OneOf(t *testing.T) {
	r := &Regius{}

	t.Run("allowed value", func(t *testing.T) {
		v := r.Validator(url.Values{})
		v.ValidateStruct(validateStatus{Status: "active"})
		assert.True(t, v.Valid())
	})

	t.Run("disallowed value", func(t *testing.T) {
		v := r.Validator(url.Values{})
		v.ValidateStruct(validateStatus{Status: "archived"})
		assert.Contains(t, v.Errors, "Status")
		assert.Equal(t, "This field must be one of: active, inactive, pending", v.Errors["Status"])
	})
}

type validateCode struct {
	Code string `validate:"required,regex=^[A-Z]{2}-[0-9]{4}$"`
}

func TestValidation_ValidateStruct_Regex(t *testing.T) {
	r := &Regius{}

	t.Run("matching value", func(t *testing.T) {
		v := r.Validator(url.Values{})
		v.ValidateStruct(validateCode{Code: "AB-1234"})
		assert.True(t, v.Valid())
	})

	t.Run("non-matching value", func(t *testing.T) {
		v := r.Validator(url.Values{})
		v.ValidateStruct(validateCode{Code: "ab-1234"})
		assert.Contains(t, v.Errors, "Code")
	})
}

type validateSignup struct {
	Email string `validate:"required,email,field=email_addr"`
}

func TestValidation_ValidateStruct_FieldNameOverride(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(validateSignup{Email: "bad"})

	assert.Contains(t, v.Errors, "email_addr", "field= must override the error key")
	assert.NotContains(t, v.Errors, "Email")
}

type validateEven struct {
	Number string `validate:"even"`
}

func TestValidation_ValidateStruct_CustomRule(t *testing.T) {
	r := &Regius{}
	r.RegisterValidation("even", func(val string) bool {
		n, err := strconv.Atoi(val)
		return err == nil && n%2 == 0
	})

	t.Run("passes custom rule", func(t *testing.T) {
		v := r.Validator(url.Values{})
		v.ValidateStruct(validateEven{Number: "42"})
		assert.True(t, v.Valid())
	})

	t.Run("fails custom rule", func(t *testing.T) {
		v := r.Validator(url.Values{})
		v.ValidateStruct(validateEven{Number: "43"})
		assert.Contains(t, v.Errors, "Number")
	})
}

type validateUnknown struct {
	Field string `validate:"nope"`
}

func TestValidation_ValidateStruct_UnknownRulePanics(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	assert.PanicsWithValue(t, `regius: unknown validation rule "nope" in tag "nope"`, func() {
		v.ValidateStruct(validateUnknown{Field: "value"})
	})
}

type validateRange struct {
	Score    string `validate:"range=1:10"`
	Priority int    `validate:"range=1:5"`
}

func TestValidation_ValidateStruct_Range(t *testing.T) {
	r := &Regius{}
	v := r.Validator(url.Values{})

	v.ValidateStruct(validateRange{Score: "15", Priority: 3})

	assert.Contains(t, v.Errors, "Score")
	assert.NotContains(t, v.Errors, "Priority")
}

type validateDates struct {
	Birth string `validate:"dateiso"`
}

func TestValidation_ValidateStruct_RegistryRules(t *testing.T) {
	r := &Regius{}

	t.Run("valid date", func(t *testing.T) {
		v := r.Validator(url.Values{})
		v.ValidateStruct(validateDates{Birth: "2024-01-31"})
		assert.True(t, v.Valid())
	})

	t.Run("invalid date", func(t *testing.T) {
		v := r.Validator(url.Values{})
		v.ValidateStruct(validateDates{Birth: "31-01-2024"})
		assert.Contains(t, v.Errors, "Birth")
	})
}

type validatePerson struct {
	Name     *string `validate:"required,min=2"`
	Nickname *string `validate:"max=10"`
}

func TestValidation_ValidateStruct_PointerToString(t *testing.T) {
	r := &Regius{}
	name := "A"

	t.Run("required pointer to short string", func(t *testing.T) {
		v := r.Validator(url.Values{})
		v.ValidateStruct(validatePerson{Name: &name})
		assert.Contains(t, v.Errors, "Name", "min must apply to the pointed-to string")
	})

	t.Run("nil optional pointer", func(t *testing.T) {
		v := r.Validator(url.Values{})
		v.ValidateStruct(validatePerson{Name: nil})
		assert.Contains(t, v.Errors, "Name")
		assert.NotContains(t, v.Errors, "Nickname", "nil optional pointer must be skipped")
	})
}
