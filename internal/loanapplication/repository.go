package loanapplication

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jmlane8/lending-backend-quick-claude/internal/db/sqlc"
	"github.com/jmlane8/lending-backend-quick-claude/internal/money"
)

// Repository stores loan applications. All methods run inside a caller-owned
// transaction; it contains SQL only, no business rules.
type Repository struct{}

// NewRepository returns a Repository.
func NewRepository() *Repository { return &Repository{} }

// Create inserts the application, its spouse (if any), and the initial status history
// row, with created_by/updated_by set to actor.
func (r *Repository) Create(ctx context.Context, tx pgx.Tx, app Application, currency, actor string) (uuid.UUID, time.Time, error) {
	q := sqlc.New(tx)
	c := app.Customer

	row, err := q.InsertLoanApplication(ctx, sqlc.InsertLoanApplicationParams{
		Status:                      StatusInProgress,
		Currency:                    currency,
		LoanAmount:                  numeric(app.Loan.Amount),
		LoanTenureMonths:            int16(app.Loan.TenureMonths), // #nosec G115 -- validated 3..48
		CustomerFullName:            c.FullName,
		CustomerIDNumber:            c.IDNumber,
		CustomerBirthDate:           pgtype.Date{Time: c.BirthDate.Time(), Valid: true},
		CustomerPhoneNumber:         c.PhoneNumber,
		CustomerEmail:               c.Email,
		CustomerMonthlyIncome:       numeric(c.MonthlyIncome),
		CustomerMaritalStatus:       string(c.MaritalStatus),
		CustomerAddressStreet:       c.Address.Street,
		CustomerAddressCity:         c.Address.City,
		CustomerAddressZipCode:      c.Address.ZipCode,
		CollateralBrand:             app.Collateral.Brand,
		CollateralModel:             app.Collateral.Model,
		CollateralManufacturingYear: int16(app.Collateral.ManufacturingYear), // #nosec G115 -- validated 1900..next year
		CollateralLicensePlate:      app.Collateral.LicensePlate,
		Actor:                       actor,
	})
	if err != nil {
		return uuid.Nil, time.Time{}, mapInsertError(err)
	}

	if s := app.Spouse; s != nil {
		if err := q.InsertLoanApplicationSpouse(ctx, sqlc.InsertLoanApplicationSpouseParams{
			LoanApplicationID: row.ID,
			FullName:          s.FullName,
			IDNumber:          s.IDNumber,
			PhoneNumber:       s.PhoneNumber,
			MonthlyIncome:     numeric(s.MonthlyIncome),
			Actor:             actor,
		}); err != nil {
			return uuid.Nil, time.Time{}, fmt.Errorf("insert spouse: %w", err)
		}
	}

	if err := q.InsertStatusHistory(ctx, sqlc.InsertStatusHistoryParams{
		LoanApplicationID: row.ID,
		FromStatus:        nil,
		ToStatus:          StatusInProgress,
		Actor:             actor,
	}); err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("insert status history: %w", err)
	}
	return row.ID, row.SubmittedAt, nil
}

// uniqueViolation is the SQLSTATE for unique_violation.
const uniqueViolation = "23505"

// mapInsertError turns violations of the open-application unique indexes into domain
// conflicts. The indexes make the rule hold under concurrent submissions (FR-009).
func mapInsertError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		switch pgErr.ConstraintName {
		case "uq_loan_applications_open_customer":
			return ErrCustomerHasOpenApplication
		case "uq_loan_applications_open_collateral":
			return ErrCollateralInUse
		}
	}
	return fmt.Errorf("create loan application: %w", err)
}

// numeric converts minor units to an exact NUMERIC parameter via its decimal text.
func numeric(a money.Amount) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(a.String()) // String always yields a valid decimal
	return n
}
