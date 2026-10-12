//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jmlane8/lending-backend-quick-claude/internal/httpx"
	"github.com/jmlane8/lending-backend-quick-claude/internal/loanapplication"
	"github.com/jmlane8/lending-backend-quick-claude/internal/money"
	"github.com/jmlane8/lending-backend-quick-claude/tests/testutil"
)

const actor = "officer-7"

func marriedApplication(idNumber, plate string) loanapplication.Application {
	birth, _ := httpx.ParseDate("1988-03-21")
	return loanapplication.Application{
		Customer: loanapplication.Customer{
			FullName:      "Budi Santoso",
			IDNumber:      idNumber,
			BirthDate:     birth,
			PhoneNumber:   "+6281234567890",
			Email:         "budi@example.com",
			MonthlyIncome: money.Amount(1500000000),
			MaritalStatus: loanapplication.MaritalStatusMarried,
			Address:       loanapplication.Address{Street: "Jl. Merdeka No. 10", City: "Jakarta", ZipCode: "10110"},
		},
		Spouse: &loanapplication.Spouse{
			FullName:      "Siti Rahma",
			IDNumber:      "3171234567890002",
			PhoneNumber:   "+6281298765432",
			MonthlyIncome: 0,
		},
		Collateral: loanapplication.Collateral{Brand: "Toyota", Model: "Avanza", ManufacturingYear: 2021, LicensePlate: plate},
		Loan:       loanapplication.Loan{Amount: money.Amount(8500000000), TenureMonths: 36},
	}
}

func create(t *testing.T, pool *pgxpool.Pool, app loanapplication.Application) (uuid.UUID, time.Time, error) {
	t.Helper()
	repo := loanapplication.NewRepository()
	var (
		id uuid.UUID
		at time.Time
	)
	err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
		var err error
		id, at, err = repo.Create(context.Background(), tx, app, "IDR", actor)
		return err
	})
	return id, at, err
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRepositoryCreatePersistsEverything(t *testing.T) {
	pool := testutil.StartPostgres(t)
	ctx := context.Background()

	id, submittedAt, err := create(t, pool, marriedApplication("3171234567890001", "B1234XYZ"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == uuid.Nil || id.Version() != 7 {
		t.Errorf("id %s is not a UUIDv7", id)
	}
	if time.Since(submittedAt) > time.Minute {
		t.Errorf("submittedAt %v is not recent", submittedAt)
	}

	var (
		status, currency, amount, income, name, idNum, phone, email, marital, street, city, zip string
		brand, model, plate, createdBy, updatedBy                                               string
		tenure, year                                                                            int
		birth                                                                                   time.Time
		deleted                                                                                 bool
		deletedAt                                                                               *time.Time
	)
	err = pool.QueryRow(ctx, `
		SELECT status, currency, loan_amount::text, loan_tenure_months, customer_full_name, customer_id_number,
		       customer_birth_date, customer_phone_number, customer_email, customer_monthly_income::text,
		       customer_marital_status, customer_address_street, customer_address_city, customer_address_zip_code,
		       collateral_brand, collateral_model, collateral_manufacturing_year, collateral_license_plate,
		       created_by, updated_by, deleted, deleted_at
		FROM loan_applications WHERE id = $1`, id).Scan(
		&status, &currency, &amount, &tenure, &name, &idNum, &birth, &phone, &email, &income,
		&marital, &street, &city, &zip, &brand, &model, &year, &plate,
		&createdBy, &updatedBy, &deleted, &deletedAt)
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string][2]any{
		"status":     {status, "IN_PROGRESS"},
		"currency":   {currency, "IDR"},
		"amount":     {amount, "85000000.00"},
		"tenure":     {tenure, 36},
		"name":       {name, "Budi Santoso"},
		"id number":  {idNum, "3171234567890001"},
		"birth date": {birth.Format("2006-01-02"), "1988-03-21"},
		"phone":      {phone, "+6281234567890"},
		"email":      {email, "budi@example.com"},
		"income":     {income, "15000000.00"},
		"marital":    {marital, "MARRIED"},
		"street":     {street, "Jl. Merdeka No. 10"},
		"city":       {city, "Jakarta"},
		"zip":        {zip, "10110"},
		"brand":      {brand, "Toyota"},
		"model":      {model, "Avanza"},
		"year":       {year, 2021},
		"plate":      {plate, "B1234XYZ"},
		"created_by": {createdBy, actor},
		"updated_by": {updatedBy, actor},
		"deleted":    {deleted, false},
	}
	for field, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %v, want %v", field, c[0], c[1])
		}
	}
	if deletedAt != nil {
		t.Errorf("deleted_at = %v, want NULL", deletedAt)
	}

	var spouseName, spouseIncome, spouseCreatedBy string
	var spouseDeleted bool
	if err := pool.QueryRow(ctx, `SELECT full_name, monthly_income::text, created_by, deleted
		FROM loan_application_spouses WHERE loan_application_id = $1`, id).
		Scan(&spouseName, &spouseIncome, &spouseCreatedBy, &spouseDeleted); err != nil {
		t.Fatalf("spouse row: %v", err)
	}
	if spouseName != "Siti Rahma" || spouseIncome != "0.00" || spouseCreatedBy != actor || spouseDeleted {
		t.Errorf("spouse = %s %s %s %v", spouseName, spouseIncome, spouseCreatedBy, spouseDeleted)
	}

	var fromStatus *string
	var toStatus, historyBy string
	if err := pool.QueryRow(ctx, `SELECT from_status, to_status, created_by
		FROM loan_application_status_history WHERE loan_application_id = $1`, id).
		Scan(&fromStatus, &toStatus, &historyBy); err != nil {
		t.Fatalf("history row: %v", err)
	}
	if fromStatus != nil || toStatus != "IN_PROGRESS" || historyBy != actor {
		t.Errorf("history = %v -> %s by %s", fromStatus, toStatus, historyBy)
	}
}

func TestRepositoryCreateWithoutSpouse(t *testing.T) {
	pool := testutil.StartPostgres(t)
	app := marriedApplication("3273000011112222", "D5678AB")
	app.Customer.MaritalStatus = loanapplication.MaritalStatusSingle
	app.Spouse = nil
	if _, _, err := create(t, pool, app); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n := count(t, pool, "loan_application_spouses"); n != 0 {
		t.Errorf("spouse rows = %d, want 0", n)
	}
}

func TestRepositoryCreateIsAtomic(t *testing.T) {
	pool := testutil.StartPostgres(t)
	app := marriedApplication("3171234567890001", "B1234XYZ")
	app.Spouse.FullName = strings.Repeat("x", 101) // violates the spouse CHECK, bypassing validation

	if _, _, err := create(t, pool, app); err == nil {
		t.Fatal("expected error from spouse CHECK constraint")
	}
	for _, table := range []string{"loan_applications", "loan_application_spouses", "loan_application_status_history"} {
		if n := count(t, pool, table); n != 0 {
			t.Errorf("%s has %d rows after failed create, want 0", table, n)
		}
	}
}

func TestDatabaseConstraintsGuardLoanTerms(t *testing.T) {
	pool := testutil.StartPostgres(t)

	notMultiple := marriedApplication("A1", "P1")
	notMultiple.Loan.Amount = money.Amount(15000) // 150.00
	if _, _, err := create(t, pool, notMultiple); err == nil {
		t.Error("loan_amount 150.00 accepted, want CHECK violation")
	}

	badTenure := marriedApplication("A2", "P2")
	badTenure.Loan.TenureMonths = 49
	if _, _, err := create(t, pool, badTenure); err == nil {
		t.Error("tenure 49 accepted, want CHECK violation")
	}
}
