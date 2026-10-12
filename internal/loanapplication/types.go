// Package loanapplication implements loan application submission (feature 001):
// HTTP handler → service (rules, transactions) → repository (SQL).
package loanapplication

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/jmlane8/lending-backend-quick-claude/internal/httpx"
	"github.com/jmlane8/lending-backend-quick-claude/internal/money"
)

// Request DTOs mirror api/openapi.yaml. Numbers stay as json.Number (exact text)
// and the birth date stays a string so every problem is reported as a field error.

// SubmitRequest is the body of POST /v1/loan-applications.
type SubmitRequest struct {
	Customer   CustomerRequest   `json:"customer"`
	Collateral CollateralRequest `json:"collateral"`
	Loan       LoanRequest       `json:"loan"`
}

// CustomerRequest is the applicant.
type CustomerRequest struct {
	FullName      string         `json:"fullName"`
	IDNumber      string         `json:"idNumber"`
	Address       AddressRequest `json:"address"`
	BirthDate     string         `json:"birthDate"`
	PhoneNumber   string         `json:"phoneNumber"`
	Email         string         `json:"email"`
	MonthlyIncome json.Number    `json:"monthlyIncome"`
	MaritalStatus string         `json:"maritalStatus"`
	Spouse        *SpouseRequest `json:"spouse"`
}

// AddressRequest is the applicant's address.
type AddressRequest struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	ZipCode string `json:"zipCode"`
}

// SpouseRequest is required if and only if the applicant is married.
type SpouseRequest struct {
	FullName      string      `json:"fullName"`
	IDNumber      string      `json:"idNumber"`
	PhoneNumber   string      `json:"phoneNumber"`
	MonthlyIncome json.Number `json:"monthlyIncome"`
}

// CollateralRequest is the car pledged against the loan.
type CollateralRequest struct {
	Brand             string      `json:"brand"`
	Model             string      `json:"model"`
	ManufacturingYear json.Number `json:"manufacturingYear"`
	LicensePlate      string      `json:"licensePlate"`
}

// LoanRequest is the proposed loan.
type LoanRequest struct {
	Amount       json.Number `json:"amount"`
	TenureMonths json.Number `json:"tenureMonths"`
}

// MaritalStatus of the applicant.
type MaritalStatus string

// Marital status values.
const (
	MaritalStatusSingle   MaritalStatus = "SINGLE"
	MaritalStatusMarried  MaritalStatus = "MARRIED"
	MaritalStatusDivorced MaritalStatus = "DIVORCED"
	MaritalStatusWidowed  MaritalStatus = "WIDOWED"
)

// StatusInProgress is the status of every newly submitted application.
const StatusInProgress = "IN_PROGRESS"

// Application is a normalized loan application ready to store.
type Application struct {
	Customer   Customer
	Spouse     *Spouse
	Collateral Collateral
	Loan       Loan
}

// Customer is the normalized applicant.
type Customer struct {
	FullName      string
	IDNumber      string
	BirthDate     httpx.Date
	PhoneNumber   string
	Email         string
	MonthlyIncome money.Amount
	MaritalStatus MaritalStatus
	Address       Address
}

// Address is the normalized applicant address.
type Address struct {
	Street  string
	City    string
	ZipCode string
}

// Spouse is the normalized spouse.
type Spouse struct {
	FullName      string
	IDNumber      string
	PhoneNumber   string
	MonthlyIncome money.Amount
}

// Collateral is the normalized car; LicensePlate is uppercase without spaces or hyphens.
type Collateral struct {
	Brand             string
	Model             string
	ManufacturingYear int
	LicensePlate      string
}

// Loan holds the proposed loan terms.
type Loan struct {
	Amount       money.Amount
	TenureMonths int
}

// SubmitResult is the 201 response body. It deliberately contains no personal data,
// so it can be stored for idempotent replay.
type SubmitResult struct {
	ID          uuid.UUID       `json:"id"`
	Status      string          `json:"status"`
	Currency    string          `json:"currency"`
	SubmittedAt httpx.Timestamp `json:"submittedAt"`
}
