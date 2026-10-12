package loanapplication

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jmlane8/lending-backend-quick-claude/internal/httpx"
	"github.com/jmlane8/lending-backend-quick-claude/internal/money"
)

// Field error codes (the FieldError.code enum in api/openapi.yaml).
const (
	codeRequired         = "required"
	codeTooLong          = "too_long"
	codeInvalidFormat    = "invalid_format"
	codeInvalidValue     = "invalid_value"
	codeNotPositive      = "not_positive"
	codeNegative         = "negative"
	codeTooManyDecimals  = "too_many_decimals"
	codeNotMultipleOf100 = "not_multiple_of_100"
	codeOutOfRange       = "out_of_range"
	codeInFuture         = "in_future"
	codeUnderMinimumAge  = "under_minimum_age"
	codeSpouseRequired   = "spouse_required"
	codeSpouseNotAllowed = "spouse_not_allowed"
	codeSameAsCustomer   = "same_as_customer"
)

const (
	minimumAge     = 18
	minYear        = 1900
	minTenure      = 3
	maxTenure      = 48
	amountMultiple = 100
)

var (
	idNumberPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,30}$`)
	zipCodePattern  = regexp.MustCompile(`^[A-Za-z0-9 -]{3,10}$`)
	phonePattern    = regexp.MustCompile(`^\+[0-9]{8,15}$`)
	platePattern    = regexp.MustCompile(`^[A-Za-z0-9 -]{1,15}$`)
	plateHasAlnum   = regexp.MustCompile(`[A-Za-z0-9]`)
)

// validator collects every field error; it never stops at the first one (FR-040).
type validator struct{ errs []httpx.FieldError }

func (v *validator) add(field, code, message string) {
	v.errs = append(v.errs, httpx.FieldError{Field: field, Code: code, Message: message})
}

// text checks a required string of 1..maxLen characters after trimming.
// It reports whether the value is present and within length.
func (v *validator) text(field, value string, maxLen int) bool {
	s := strings.TrimSpace(value)
	switch {
	case s == "":
		v.add(field, codeRequired, "is required")
		return false
	case utf8.RuneCountInString(s) > maxLen:
		v.add(field, codeTooLong, fmt.Sprintf("must be at most %d characters", maxLen))
		return false
	}
	return true
}

// pattern checks a required string against re.
func (v *validator) pattern(field, value string, re *regexp.Regexp, message string) {
	s := strings.TrimSpace(value)
	switch {
	case s == "":
		v.add(field, codeRequired, "is required")
	case !re.MatchString(s):
		v.add(field, codeInvalidFormat, message)
	}
}

// amount parses a required money value; ok is false if an error was recorded.
func (v *validator) amount(field string, n json.Number) (money.Amount, bool) {
	if n == "" {
		v.add(field, codeRequired, "is required")
		return 0, false
	}
	a, err := money.ParseAmount(n)
	switch {
	case errors.Is(err, money.ErrTooManyDecimals):
		v.add(field, codeTooManyDecimals, "must have at most 2 decimal places")
		return 0, false
	case err != nil:
		v.add(field, codeInvalidFormat, "must be a plain decimal number")
		return 0, false
	}
	return a, true
}

// Validate checks every rule of spec FR-010..FR-031 against the request and returns
// all failures. today is the current time in the business timezone.
func Validate(req SubmitRequest, today time.Time) []httpx.FieldError {
	v := &validator{}
	validateCustomer(v, req.Customer, today)
	validateCollateral(v, req.Collateral, today)
	validateLoan(v, req.Loan)
	return v.errs
}

func validateCustomer(v *validator, c CustomerRequest, today time.Time) {
	v.text("/customer/fullName", c.FullName, 100)
	v.pattern("/customer/idNumber", c.IDNumber, idNumberPattern, "must be 1 to 30 letters or digits")
	v.text("/customer/address/street", c.Address.Street, 200)
	v.text("/customer/address/city", c.Address.City, 100)
	v.pattern("/customer/address/zipCode", c.Address.ZipCode, zipCodePattern,
		"must be 3 to 10 letters, digits, spaces, or hyphens")
	validateBirthDate(v, c.BirthDate, today)
	v.pattern("/customer/phoneNumber", c.PhoneNumber, phonePattern, "must be + followed by 8 to 15 digits")
	validateEmail(v, c.Email)
	if income, ok := v.amount("/customer/monthlyIncome", c.MonthlyIncome); ok && income <= 0 {
		v.add("/customer/monthlyIncome", codeNotPositive, "must be greater than 0")
	}

	status := MaritalStatus(strings.TrimSpace(c.MaritalStatus))
	switch status {
	case "":
		v.add("/customer/maritalStatus", codeRequired, "is required")
		return
	case MaritalStatusSingle, MaritalStatusMarried, MaritalStatusDivorced, MaritalStatusWidowed:
	default:
		v.add("/customer/maritalStatus", codeInvalidValue, "must be one of SINGLE, MARRIED, DIVORCED, WIDOWED")
		return
	}

	switch {
	case status == MaritalStatusMarried && c.Spouse == nil:
		v.add("/customer/spouse", codeSpouseRequired, "is required when maritalStatus is MARRIED")
	case status != MaritalStatusMarried && c.Spouse != nil:
		v.add("/customer/spouse", codeSpouseNotAllowed, "is only allowed when maritalStatus is MARRIED")
	case c.Spouse != nil:
		validateSpouse(v, *c.Spouse, c.IDNumber)
	}
}

func validateBirthDate(v *validator, value string, today time.Time) {
	const field = "/customer/birthDate"
	s := strings.TrimSpace(value)
	if s == "" {
		v.add(field, codeRequired, "is required")
		return
	}
	d, err := httpx.ParseDate(s)
	if err != nil {
		v.add(field, codeInvalidFormat, "must be a real calendar date in format YYYY-MM-DD")
		return
	}
	birth := d.Time()
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	switch {
	case birth.After(todayDate):
		v.add(field, codeInFuture, "must not be in the future")
	case birth.AddDate(minimumAge, 0, 0).After(todayDate):
		v.add(field, codeUnderMinimumAge, fmt.Sprintf("customer must be at least %d years old", minimumAge))
	}
}

func validateEmail(v *validator, value string) {
	const field = "/customer/email"
	s := strings.TrimSpace(value)
	switch {
	case s == "":
		v.add(field, codeRequired, "is required")
		return
	case len(s) > 254:
		v.add(field, codeTooLong, "must be at most 254 characters")
		return
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || addr.Name != "" {
		v.add(field, codeInvalidFormat, "must be a valid email address")
	}
}

func validateSpouse(v *validator, s SpouseRequest, customerID string) {
	v.text("/customer/spouse/fullName", s.FullName, 100)
	spouseID := strings.TrimSpace(s.IDNumber)
	v.pattern("/customer/spouse/idNumber", spouseID, idNumberPattern, "must be 1 to 30 letters or digits")
	if spouseID != "" && strings.EqualFold(spouseID, strings.TrimSpace(customerID)) {
		v.add("/customer/spouse/idNumber", codeSameAsCustomer, "must differ from the customer's ID number")
	}
	v.pattern("/customer/spouse/phoneNumber", s.PhoneNumber, phonePattern, "must be + followed by 8 to 15 digits")
	if income, ok := v.amount("/customer/spouse/monthlyIncome", s.MonthlyIncome); ok && income < 0 {
		v.add("/customer/spouse/monthlyIncome", codeNegative, "must be 0 or greater")
	}
}

func validateCollateral(v *validator, c CollateralRequest, today time.Time) {
	v.text("/collateral/brand", c.Brand, 50)
	v.text("/collateral/model", c.Model, 50)

	const yearField = "/collateral/manufacturingYear"
	maxYear := today.Year() + 1
	if c.ManufacturingYear == "" {
		v.add(yearField, codeRequired, "is required")
	} else if year, err := parseInt(c.ManufacturingYear); err != nil {
		v.add(yearField, codeInvalidFormat, "must be a whole number")
	} else if year < minYear || year > maxYear {
		v.add(yearField, codeOutOfRange, fmt.Sprintf("must be between %d and %d", minYear, maxYear))
	}

	const plateField = "/collateral/licensePlate"
	plate := strings.TrimSpace(c.LicensePlate)
	switch {
	case plate == "":
		v.add(plateField, codeRequired, "is required")
	case !platePattern.MatchString(plate) || !plateHasAlnum.MatchString(plate):
		v.add(plateField, codeInvalidFormat,
			"must be 1 to 15 letters, digits, spaces, or hyphens, with at least one letter or digit")
	}
}

func validateLoan(v *validator, l LoanRequest) {
	const amountField = "/loan/amount"
	if l.Amount == "" {
		v.add(amountField, codeRequired, "is required")
	} else if a, err := money.ParseAmount(l.Amount); errors.Is(err, money.ErrTooManyDecimals) {
		v.add(amountField, codeNotMultipleOf100, "must be a positive multiple of 100")
	} else if err != nil {
		v.add(amountField, codeInvalidFormat, "must be a plain whole number")
	} else if a <= 0 {
		v.add(amountField, codeNotPositive, "must be a positive multiple of 100")
	} else if !a.IsMultipleOf(amountMultiple) {
		v.add(amountField, codeNotMultipleOf100, "must be a positive multiple of 100")
	}

	const tenureField = "/loan/tenureMonths"
	if l.TenureMonths == "" {
		v.add(tenureField, codeRequired, "is required")
	} else if t, err := parseInt(l.TenureMonths); err != nil || t < minTenure || t > maxTenure {
		v.add(tenureField, codeOutOfRange, fmt.Sprintf("must be a whole number of months from %d to %d", minTenure, maxTenure))
	}
}
