package loanapplication

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// today is 2026-10-12 in the business timezone.
var today = time.Date(2026, 10, 12, 10, 0, 0, 0, time.FixedZone("WIB", 7*3600))

func TestValidateAcceptsValidRequests(t *testing.T) {
	if errs := Validate(validRequest(), today); len(errs) != 0 {
		t.Fatalf("married request: unexpected errors %+v", errs)
	}
	single := validRequest()
	single.Customer.MaritalStatus = "SINGLE"
	single.Customer.Spouse = nil
	if errs := Validate(single, today); len(errs) != 0 {
		t.Fatalf("single request: unexpected errors %+v", errs)
	}
}

func TestValidateRules(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SubmitRequest)
		field  string
		code   string
	}{
		// customer
		{"name missing", func(r *SubmitRequest) { r.Customer.FullName = "   " }, "/customer/fullName", "required"},
		{"name too long", func(r *SubmitRequest) { r.Customer.FullName = strings.Repeat("a", 101) }, "/customer/fullName", "too_long"},
		{"id missing", func(r *SubmitRequest) { r.Customer.IDNumber = "" }, "/customer/idNumber", "required"},
		{"id bad chars", func(r *SubmitRequest) { r.Customer.IDNumber = "317-123" }, "/customer/idNumber", "invalid_format"},
		{"id too long", func(r *SubmitRequest) { r.Customer.IDNumber = strings.Repeat("1", 31) }, "/customer/idNumber", "invalid_format"},
		{"street missing", func(r *SubmitRequest) { r.Customer.Address.Street = "" }, "/customer/address/street", "required"},
		{"street too long", func(r *SubmitRequest) { r.Customer.Address.Street = strings.Repeat("s", 201) }, "/customer/address/street", "too_long"},
		{"city missing", func(r *SubmitRequest) { r.Customer.Address.City = " " }, "/customer/address/city", "required"},
		{"city too long", func(r *SubmitRequest) { r.Customer.Address.City = strings.Repeat("c", 101) }, "/customer/address/city", "too_long"},
		{"zip missing", func(r *SubmitRequest) { r.Customer.Address.ZipCode = "" }, "/customer/address/zipCode", "required"},
		{"zip too short", func(r *SubmitRequest) { r.Customer.Address.ZipCode = "12" }, "/customer/address/zipCode", "invalid_format"},
		{"zip bad chars", func(r *SubmitRequest) { r.Customer.Address.ZipCode = "10110#" }, "/customer/address/zipCode", "invalid_format"},
		{"birth missing", func(r *SubmitRequest) { r.Customer.BirthDate = "" }, "/customer/birthDate", "required"},
		{"birth impossible date", func(r *SubmitRequest) { r.Customer.BirthDate = "2001-02-30" }, "/customer/birthDate", "invalid_format"},
		{"birth wrong format", func(r *SubmitRequest) { r.Customer.BirthDate = "21/03/1988" }, "/customer/birthDate", "invalid_format"},
		{"birth in future", func(r *SubmitRequest) { r.Customer.BirthDate = "2026-10-13" }, "/customer/birthDate", "in_future"},
		{"17 years 364 days", func(r *SubmitRequest) { r.Customer.BirthDate = "2008-10-13" }, "/customer/birthDate", "under_minimum_age"},
		{"phone missing", func(r *SubmitRequest) { r.Customer.PhoneNumber = "" }, "/customer/phoneNumber", "required"},
		{"phone no plus", func(r *SubmitRequest) { r.Customer.PhoneNumber = "081234567890" }, "/customer/phoneNumber", "invalid_format"},
		{"phone too short", func(r *SubmitRequest) { r.Customer.PhoneNumber = "+1234567" }, "/customer/phoneNumber", "invalid_format"},
		{"phone too long", func(r *SubmitRequest) { r.Customer.PhoneNumber = "+1234567890123456" }, "/customer/phoneNumber", "invalid_format"},
		{"email missing", func(r *SubmitRequest) { r.Customer.Email = "" }, "/customer/email", "required"},
		{"email invalid", func(r *SubmitRequest) { r.Customer.Email = "x" }, "/customer/email", "invalid_format"},
		{"email display name", func(r *SubmitRequest) { r.Customer.Email = "Budi <budi@example.com>" }, "/customer/email", "invalid_format"},
		{"email too long", func(r *SubmitRequest) { r.Customer.Email = strings.Repeat("a", 250) + "@x.io" }, "/customer/email", "too_long"},
		{"income missing", func(r *SubmitRequest) { r.Customer.MonthlyIncome = "" }, "/customer/monthlyIncome", "required"},
		{"income zero", func(r *SubmitRequest) { r.Customer.MonthlyIncome = "0" }, "/customer/monthlyIncome", "not_positive"},
		{"income negative", func(r *SubmitRequest) { r.Customer.MonthlyIncome = "-5" }, "/customer/monthlyIncome", "not_positive"},
		{"income 3 decimals", func(r *SubmitRequest) { r.Customer.MonthlyIncome = "100.005" }, "/customer/monthlyIncome", "too_many_decimals"},
		{"income exponent", func(r *SubmitRequest) { r.Customer.MonthlyIncome = "1e6" }, "/customer/monthlyIncome", "invalid_format"},
		{"marital missing", func(r *SubmitRequest) { r.Customer.MaritalStatus = "" }, "/customer/maritalStatus", "required"},
		{"marital invalid", func(r *SubmitRequest) { r.Customer.MaritalStatus = "married" }, "/customer/maritalStatus", "invalid_value"},
		// spouse
		{"married without spouse", func(r *SubmitRequest) { r.Customer.Spouse = nil }, "/customer/spouse", "spouse_required"},
		{"single with spouse", func(r *SubmitRequest) { r.Customer.MaritalStatus = "SINGLE" }, "/customer/spouse", "spouse_not_allowed"},
		{"divorced with spouse", func(r *SubmitRequest) { r.Customer.MaritalStatus = "DIVORCED" }, "/customer/spouse", "spouse_not_allowed"},
		{"spouse name missing", func(r *SubmitRequest) { r.Customer.Spouse.FullName = "" }, "/customer/spouse/fullName", "required"},
		{"spouse name too long", func(r *SubmitRequest) { r.Customer.Spouse.FullName = strings.Repeat("n", 101) }, "/customer/spouse/fullName", "too_long"},
		{"spouse id missing", func(r *SubmitRequest) { r.Customer.Spouse.IDNumber = "" }, "/customer/spouse/idNumber", "required"},
		{"spouse id bad", func(r *SubmitRequest) { r.Customer.Spouse.IDNumber = "ab cd" }, "/customer/spouse/idNumber", "invalid_format"},
		{"spouse id same as customer", func(r *SubmitRequest) { r.Customer.Spouse.IDNumber = r.Customer.IDNumber }, "/customer/spouse/idNumber", "same_as_customer"},
		{"spouse phone missing", func(r *SubmitRequest) { r.Customer.Spouse.PhoneNumber = "" }, "/customer/spouse/phoneNumber", "required"},
		{"spouse phone bad", func(r *SubmitRequest) { r.Customer.Spouse.PhoneNumber = "12345" }, "/customer/spouse/phoneNumber", "invalid_format"},
		{"spouse income missing", func(r *SubmitRequest) { r.Customer.Spouse.MonthlyIncome = "" }, "/customer/spouse/monthlyIncome", "required"},
		{"spouse income negative", func(r *SubmitRequest) { r.Customer.Spouse.MonthlyIncome = "-1" }, "/customer/spouse/monthlyIncome", "negative"},
		{"spouse income decimals", func(r *SubmitRequest) { r.Customer.Spouse.MonthlyIncome = "1.234" }, "/customer/spouse/monthlyIncome", "too_many_decimals"},
		// collateral
		{"brand missing", func(r *SubmitRequest) { r.Collateral.Brand = "" }, "/collateral/brand", "required"},
		{"brand too long", func(r *SubmitRequest) { r.Collateral.Brand = strings.Repeat("b", 51) }, "/collateral/brand", "too_long"},
		{"model missing", func(r *SubmitRequest) { r.Collateral.Model = "" }, "/collateral/model", "required"},
		{"model too long", func(r *SubmitRequest) { r.Collateral.Model = strings.Repeat("m", 51) }, "/collateral/model", "too_long"},
		{"year missing", func(r *SubmitRequest) { r.Collateral.ManufacturingYear = "" }, "/collateral/manufacturingYear", "required"},
		{"year too old", func(r *SubmitRequest) { r.Collateral.ManufacturingYear = "1899" }, "/collateral/manufacturingYear", "out_of_range"},
		{"year too new", func(r *SubmitRequest) { r.Collateral.ManufacturingYear = "2028" }, "/collateral/manufacturingYear", "out_of_range"},
		{"year fractional", func(r *SubmitRequest) { r.Collateral.ManufacturingYear = "2020.5" }, "/collateral/manufacturingYear", "invalid_format"},
		{"plate missing", func(r *SubmitRequest) { r.Collateral.LicensePlate = "" }, "/collateral/licensePlate", "required"},
		{"plate bad chars", func(r *SubmitRequest) { r.Collateral.LicensePlate = "B#1234" }, "/collateral/licensePlate", "invalid_format"},
		{"plate too long", func(r *SubmitRequest) { r.Collateral.LicensePlate = strings.Repeat("A", 16) }, "/collateral/licensePlate", "invalid_format"},
		{"plate only separators", func(r *SubmitRequest) { r.Collateral.LicensePlate = "- -" }, "/collateral/licensePlate", "invalid_format"},
		// loan
		{"amount missing", func(r *SubmitRequest) { r.Loan.Amount = "" }, "/loan/amount", "required"},
		{"amount 15050", func(r *SubmitRequest) { r.Loan.Amount = "15050" }, "/loan/amount", "not_multiple_of_100"},
		{"amount fractional", func(r *SubmitRequest) { r.Loan.Amount = "1000.50" }, "/loan/amount", "not_multiple_of_100"},
		{"amount zero", func(r *SubmitRequest) { r.Loan.Amount = "0" }, "/loan/amount", "not_positive"},
		{"amount negative", func(r *SubmitRequest) { r.Loan.Amount = "-100" }, "/loan/amount", "not_positive"},
		{"amount exponent", func(r *SubmitRequest) { r.Loan.Amount = "1e5" }, "/loan/amount", "invalid_format"},
		{"tenure missing", func(r *SubmitRequest) { r.Loan.TenureMonths = "" }, "/loan/tenureMonths", "required"},
		{"tenure 2", func(r *SubmitRequest) { r.Loan.TenureMonths = "2" }, "/loan/tenureMonths", "out_of_range"},
		{"tenure 49", func(r *SubmitRequest) { r.Loan.TenureMonths = "49" }, "/loan/tenureMonths", "out_of_range"},
		{"tenure 12.5", func(r *SubmitRequest) { r.Loan.TenureMonths = "12.5" }, "/loan/tenureMonths", "out_of_range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest()
			tt.mutate(&req)
			errs := Validate(req, today)
			if len(errs) != 1 {
				t.Fatalf("want exactly 1 error %s/%s, got %+v", tt.field, tt.code, errs)
			}
			if errs[0].Field != tt.field || errs[0].Code != tt.code {
				t.Errorf("got %s/%s, want %s/%s", errs[0].Field, errs[0].Code, tt.field, tt.code)
			}
			if errs[0].Message == "" {
				t.Error("empty message")
			}
		})
	}
}

func TestValidateBoundariesAccepted(t *testing.T) {
	tests := map[string]func(*SubmitRequest){
		"exactly 18 today":     func(r *SubmitRequest) { r.Customer.BirthDate = "2008-10-12" },
		"amount 100":           func(r *SubmitRequest) { r.Loan.Amount = "100" },
		"amount 100.00":        func(r *SubmitRequest) { r.Loan.Amount = "100.00" },
		"tenure 3":             func(r *SubmitRequest) { r.Loan.TenureMonths = "3" },
		"tenure 48":            func(r *SubmitRequest) { r.Loan.TenureMonths = "48" },
		"year 1900":            func(r *SubmitRequest) { r.Collateral.ManufacturingYear = "1900" },
		"year next year":       func(r *SubmitRequest) { r.Collateral.ManufacturingYear = "2027" },
		"name 100 chars":       func(r *SubmitRequest) { r.Customer.FullName = strings.Repeat("é", 100) },
		"plate with separator": func(r *SubmitRequest) { r.Collateral.LicensePlate = "b 1234-xyz" },
		"spouse income zero":   func(r *SubmitRequest) { r.Customer.Spouse.MonthlyIncome = "0" },
		"income 2 decimals":    func(r *SubmitRequest) { r.Customer.MonthlyIncome = "9500000.50" },
		"zip with space":       func(r *SubmitRequest) { r.Customer.Address.ZipCode = "SW1A 1AA" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			req := validRequest()
			mutate(&req)
			if errs := Validate(req, today); len(errs) != 0 {
				t.Errorf("unexpected errors %+v", errs)
			}
		})
	}
}

func TestValidateReportsAllErrorsTogether(t *testing.T) {
	req := validRequest()
	req.Loan.Amount = json.Number("15050")
	req.Loan.TenureMonths = json.Number("49")
	req.Customer.Email = "x"
	req.Customer.Address.City = ""

	errs := Validate(req, today)
	want := map[string]string{
		"/loan/amount":           "not_multiple_of_100",
		"/loan/tenureMonths":     "out_of_range",
		"/customer/email":        "invalid_format",
		"/customer/address/city": "required",
	}
	if len(errs) != len(want) {
		t.Fatalf("got %d errors, want %d: %+v", len(errs), len(want), errs)
	}
	for _, e := range errs {
		if want[e.Field] != e.Code {
			t.Errorf("unexpected error %s/%s", e.Field, e.Code)
		}
	}
}

func TestValidateEmptyRequestReportsEveryRequiredField(t *testing.T) {
	errs := Validate(SubmitRequest{}, today)
	required := 0
	for _, e := range errs {
		if e.Code == "required" {
			required++
		}
	}
	// 10 customer fields (incl. 3 address and marital status) + 4 collateral + 2 loan
	if required != 16 {
		t.Errorf("required errors = %d, want 16: %+v", required, errs)
	}
}
