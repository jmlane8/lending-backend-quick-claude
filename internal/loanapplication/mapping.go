package loanapplication

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmlane8/lending-backend-quick-claude/internal/httpx"
	"github.com/jmlane8/lending-backend-quick-claude/internal/money"
)

// toApplication trims text fields, normalizes the license plate, and parses numbers
// and dates. It assumes nothing about business rules; Validate enforces those.
func toApplication(req SubmitRequest) (Application, error) {
	c := req.Customer
	birth, err := httpx.ParseDate(strings.TrimSpace(c.BirthDate))
	if err != nil {
		return Application{}, fmt.Errorf("customer.birthDate: %w", err)
	}
	income, err := money.ParseAmount(c.MonthlyIncome)
	if err != nil {
		return Application{}, fmt.Errorf("customer.monthlyIncome: %w", err)
	}
	year, err := parseInt(req.Collateral.ManufacturingYear)
	if err != nil {
		return Application{}, fmt.Errorf("collateral.manufacturingYear: %w", err)
	}
	amount, err := money.ParseAmount(req.Loan.Amount)
	if err != nil {
		return Application{}, fmt.Errorf("loan.amount: %w", err)
	}
	tenure, err := parseInt(req.Loan.TenureMonths)
	if err != nil {
		return Application{}, fmt.Errorf("loan.tenureMonths: %w", err)
	}

	app := Application{
		Customer: Customer{
			FullName:      strings.TrimSpace(c.FullName),
			IDNumber:      strings.TrimSpace(c.IDNumber),
			BirthDate:     birth,
			PhoneNumber:   strings.TrimSpace(c.PhoneNumber),
			Email:         strings.TrimSpace(c.Email),
			MonthlyIncome: income,
			MaritalStatus: MaritalStatus(strings.TrimSpace(c.MaritalStatus)),
			Address: Address{
				Street:  strings.TrimSpace(c.Address.Street),
				City:    strings.TrimSpace(c.Address.City),
				ZipCode: strings.TrimSpace(c.Address.ZipCode),
			},
		},
		Collateral: Collateral{
			Brand:             strings.TrimSpace(req.Collateral.Brand),
			Model:             strings.TrimSpace(req.Collateral.Model),
			ManufacturingYear: year,
			LicensePlate:      normalizePlate(req.Collateral.LicensePlate),
		},
		Loan: Loan{Amount: amount, TenureMonths: tenure},
	}

	if s := c.Spouse; s != nil {
		spouseIncome, err := money.ParseAmount(s.MonthlyIncome)
		if err != nil {
			return Application{}, fmt.Errorf("customer.spouse.monthlyIncome: %w", err)
		}
		app.Spouse = &Spouse{
			FullName:      strings.TrimSpace(s.FullName),
			IDNumber:      strings.TrimSpace(s.IDNumber),
			PhoneNumber:   strings.TrimSpace(s.PhoneNumber),
			MonthlyIncome: spouseIncome,
		}
	}
	return app, nil
}

// normalizePlate uppercases a license plate and removes spaces and hyphens,
// so "b 1234-xyz" and "B1234XYZ" are the same car.
func normalizePlate(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(s)))
}

// parseInt accepts only whole-number JSON literals such as "2021" (not "2021.0" or "2e3").
func parseInt(n json.Number) (int, error) {
	v, err := strconv.ParseInt(string(n), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%q is not a whole number", n)
	}
	return int(v), nil
}
