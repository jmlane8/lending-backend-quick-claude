//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jmlane8/lending-backend-quick-claude/internal/loanapplication"
	"github.com/jmlane8/lending-backend-quick-claude/tests/testutil"
)

func newService(pool *pgxpool.Pool) *loanapplication.Service {
	return loanapplication.NewService(pool, loanapplication.ServiceConfig{
		Currency: "IDR",
		Location: time.FixedZone("WIB", 7*3600),
		Now:      testutil.FixedClock(),
	})
}

func singleRequest(idNumber, plate string) loanapplication.SubmitRequest {
	return loanapplication.SubmitRequest{
		Customer: loanapplication.CustomerRequest{
			FullName:      "Andi Wijaya",
			IDNumber:      idNumber,
			Address:       loanapplication.AddressRequest{Street: "Jl. Asia Afrika 8", City: "Bandung", ZipCode: "40111"},
			BirthDate:     "1999-11-02",
			PhoneNumber:   "+6285711122233",
			Email:         "andi@example.com",
			MonthlyIncome: json.Number("9500000.50"),
			MaritalStatus: "SINGLE",
		},
		Collateral: loanapplication.CollateralRequest{Brand: "Honda", Model: "Jazz", ManufacturingYear: json.Number("2019"), LicensePlate: plate},
		Loan:       loanapplication.LoanRequest{Amount: json.Number("30000000"), TenureMonths: json.Number("24")},
	}
}

func submit(svc *loanapplication.Service, req loanapplication.SubmitRequest) error {
	_, err := svc.Submit(context.Background(), loanapplication.SubmitInput{
		Request:        req,
		RawBody:        mustJSON(req),
		IdempotencyKey: uuid.NewString(),
		Actor:          actor,
	})
	return err
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// race runs n submissions concurrently and counts successes and the given conflict.
func race(t *testing.T, n int, build func(i int) loanapplication.SubmitRequest, conflict error) (ok, conflicts int) {
	t.Helper()
	svc := newService(testutil.StartPostgres(t))
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		start = make(chan struct{})
	)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := submit(svc, build(i))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, conflict):
				conflicts++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	return ok, conflicts
}

func TestConcurrentSubmissionsForSameCustomer(t *testing.T) {
	ok, conflicts := race(t, 20, func(i int) loanapplication.SubmitRequest {
		return singleRequest("CUST0001", fmt.Sprintf("PLATE%02d", i))
	}, loanapplication.ErrCustomerHasOpenApplication)
	if ok != 1 || conflicts != 19 {
		t.Errorf("ok=%d conflicts=%d, want 1 and 19", ok, conflicts)
	}
}

func TestConcurrentSubmissionsForSameCar(t *testing.T) {
	ok, conflicts := race(t, 20, func(i int) loanapplication.SubmitRequest {
		return singleRequest(fmt.Sprintf("CUST%04d", i), "b 1234-xyz")
	}, loanapplication.ErrCollateralInUse)
	if ok != 1 || conflicts != 19 {
		t.Errorf("ok=%d conflicts=%d, want 1 and 19", ok, conflicts)
	}
}

func TestSoftDeletedApplicationDoesNotBlock(t *testing.T) {
	pool := testutil.StartPostgres(t)
	svc := newService(pool)
	if err := submit(svc, singleRequest("CUST0001", "B1")); err != nil {
		t.Fatal(err)
	}
	if err := submit(svc, singleRequest("CUST0001", "B2")); !errors.Is(err, loanapplication.ErrCustomerHasOpenApplication) {
		t.Fatalf("second submission err = %v, want conflict", err)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE loan_applications SET deleted = true, deleted_at = now(), updated_at = now(), updated_by = $1`, actor); err != nil {
		t.Fatal(err)
	}
	if err := submit(svc, singleRequest("CUST0001", "B1")); err != nil {
		t.Errorf("submission after soft delete: %v", err)
	}
}

func TestServiceRejectsInvalidRequestWithoutTouchingDatabase(t *testing.T) {
	pool := testutil.StartPostgres(t)
	svc := newService(pool)
	req := singleRequest("CUST0001", "B1")
	req.Loan.Amount = "15050"

	err := submit(svc, req)
	var verr *loanapplication.ValidationError
	if !errors.As(err, &verr) || len(verr.Errors) != 1 {
		t.Fatalf("err = %v, want ValidationError with 1 error", err)
	}
	if n := count(t, pool, "loan_applications"); n != 0 {
		t.Errorf("rows = %d, want 0", n)
	}
}
