package loanapplication

import (
	"errors"
	"fmt"

	"github.com/jmlane8/lending-backend-quick-claude/internal/httpx"
)

// Domain errors returned by Service.Submit.
var (
	// ErrCustomerHasOpenApplication: the customer's ID number already has an IN_PROGRESS application (FR-007).
	ErrCustomerHasOpenApplication = errors.New("customer already has an open application")
	// ErrCollateralInUse: the license plate already backs an IN_PROGRESS application (FR-008).
	ErrCollateralInUse = errors.New("collateral already backs an open application")
)

// ValidationError carries every field that failed validation (FR-040).
type ValidationError struct {
	Errors []httpx.FieldError
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%d invalid fields", len(e.Errors))
}
