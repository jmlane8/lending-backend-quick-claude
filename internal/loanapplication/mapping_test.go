package loanapplication

import (
	"encoding/json"
	"testing"

	"github.com/jmlane8/lending-backend-quick-claude/internal/money"
)

func validRequest() SubmitRequest {
	return SubmitRequest{
		Customer: CustomerRequest{
			FullName:      "  Budi Santoso ",
			IDNumber:      "3171234567890001",
			Address:       AddressRequest{Street: " Jl. Merdeka No. 10", City: "Jakarta ", ZipCode: "10110"},
			BirthDate:     "1988-03-21",
			PhoneNumber:   "+6281234567890",
			Email:         " budi@example.com ",
			MonthlyIncome: json.Number("15000000"),
			MaritalStatus: "MARRIED",
			Spouse: &SpouseRequest{
				FullName:      "Siti Rahma",
				IDNumber:      "3171234567890002",
				PhoneNumber:   "+6281298765432",
				MonthlyIncome: json.Number("0"),
			},
		},
		Collateral: CollateralRequest{Brand: "Toyota", Model: "Avanza", ManufacturingYear: json.Number("2021"), LicensePlate: "b 1234-xyz"},
		Loan:       LoanRequest{Amount: json.Number("85000000"), TenureMonths: json.Number("36")},
	}
}

func TestToApplicationMapsAndNormalizes(t *testing.T) {
	app, err := toApplication(validRequest())
	if err != nil {
		t.Fatalf("toApplication: %v", err)
	}
	c := app.Customer
	if c.FullName != "Budi Santoso" || c.Email != "budi@example.com" ||
		c.Address.Street != "Jl. Merdeka No. 10" || c.Address.City != "Jakarta" {
		t.Errorf("text fields not trimmed: %+v", c)
	}
	if c.BirthDate.String() != "1988-03-21" {
		t.Errorf("BirthDate = %s", c.BirthDate)
	}
	if c.MonthlyIncome != money.Amount(1500000000) {
		t.Errorf("MonthlyIncome = %d minor units", c.MonthlyIncome)
	}
	if c.MaritalStatus != MaritalStatusMarried {
		t.Errorf("MaritalStatus = %q", c.MaritalStatus)
	}
	if app.Spouse == nil || app.Spouse.FullName != "Siti Rahma" || app.Spouse.MonthlyIncome != 0 {
		t.Errorf("Spouse = %+v", app.Spouse)
	}
	if app.Collateral.LicensePlate != "B1234XYZ" {
		t.Errorf("LicensePlate = %q, want B1234XYZ", app.Collateral.LicensePlate)
	}
	if app.Collateral.ManufacturingYear != 2021 {
		t.Errorf("ManufacturingYear = %d", app.Collateral.ManufacturingYear)
	}
	if app.Loan.Amount != money.Amount(8500000000) || app.Loan.TenureMonths != 36 {
		t.Errorf("Loan = %+v", app.Loan)
	}
}

func TestToApplicationWithoutSpouse(t *testing.T) {
	req := validRequest()
	req.Customer.MaritalStatus = "SINGLE"
	req.Customer.Spouse = nil
	req.Customer.MonthlyIncome = json.Number("9500000.50")
	app, err := toApplication(req)
	if err != nil {
		t.Fatalf("toApplication: %v", err)
	}
	if app.Spouse != nil {
		t.Errorf("Spouse = %+v, want nil", app.Spouse)
	}
	if app.Customer.MonthlyIncome != money.Amount(950000050) {
		t.Errorf("MonthlyIncome = %d", app.Customer.MonthlyIncome)
	}
}

func TestToApplicationRejectsUnparseableValues(t *testing.T) {
	tests := map[string]func(*SubmitRequest){
		"income with 3 decimals": func(r *SubmitRequest) { r.Customer.MonthlyIncome = "1.005" },
		"bad birth date":         func(r *SubmitRequest) { r.Customer.BirthDate = "2001-02-30" },
		"fractional tenure":      func(r *SubmitRequest) { r.Loan.TenureMonths = "12.5" },
		"fractional year":        func(r *SubmitRequest) { r.Collateral.ManufacturingYear = "2020.5" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			req := validRequest()
			mutate(&req)
			if _, err := toApplication(req); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestNormalizePlate(t *testing.T) {
	for in, want := range map[string]string{
		"b 1234 xyz": "B1234XYZ",
		"D-5678-AB":  "D5678AB",
		"B1234XYZ":   "B1234XYZ",
		" - ":        "",
	} {
		if got := normalizePlate(in); got != want {
			t.Errorf("normalizePlate(%q) = %q, want %q", in, got, want)
		}
	}
}
