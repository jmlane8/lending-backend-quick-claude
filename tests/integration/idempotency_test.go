//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmlane8/lending-backend-quick-claude/internal/loanapplication"
	"github.com/jmlane8/lending-backend-quick-claude/tests/testutil"
)

func submitWithKey(svc *loanapplication.Service, req loanapplication.SubmitRequest, key, who string) (loanapplication.SubmitOutput, error) {
	return svc.Submit(context.Background(), loanapplication.SubmitInput{
		Request:        req,
		RawBody:        mustJSON(req),
		IdempotencyKey: key,
		Actor:          who,
	})
}

func TestIdempotentReplay(t *testing.T) {
	pool := testutil.StartPostgres(t)
	svc := newService(pool)
	req := singleRequest("CUST0001", "B1")

	first, err := submitWithKey(svc, req, "key-1", actor)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed {
		t.Error("first submission marked as replay")
	}
	second, err := submitWithKey(svc, req, "key-1", actor)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !second.Replayed || second.Result.ID != first.Result.ID || !second.Result.SubmittedAt.Equal(first.Result.SubmittedAt.Time) {
		t.Errorf("replay = %+v, want same result as %+v", second, first)
	}
	if n := count(t, pool, "loan_applications"); n != 1 {
		t.Errorf("applications = %d, want 1", n)
	}
	if n := count(t, pool, "idempotency_keys"); n != 1 {
		t.Errorf("idempotency keys = %d, want 1", n)
	}

	var (
		status  int
		body    []byte
		created time.Time
		expires time.Time
		by      string
	)
	if err := pool.QueryRow(context.Background(),
		`SELECT response_status, response_body, created_at, expires_at, created_by FROM idempotency_keys`).
		Scan(&status, &body, &created, &expires, &by); err != nil {
		t.Fatal(err)
	}
	if status != 201 || by != actor {
		t.Errorf("status=%d created_by=%s", status, by)
	}
	if got := expires.Sub(created); got != 24*time.Hour {
		t.Errorf("expires_at - created_at = %v, want 24h", got)
	}
	var stored map[string]any
	if err := json.Unmarshal(body, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 4 || stored["id"] == nil || stored["status"] == nil || stored["currency"] == nil || stored["submittedAt"] == nil {
		t.Errorf("stored response_body = %v, want only id/status/currency/submittedAt", stored)
	}
}

func TestIdempotencyKeyReusedWithDifferentBody(t *testing.T) {
	pool := testutil.StartPostgres(t)
	svc := newService(pool)
	if _, err := submitWithKey(svc, singleRequest("CUST0001", "B1"), "key-1", actor); err != nil {
		t.Fatal(err)
	}
	changed := singleRequest("CUST0001", "B1")
	changed.Loan.Amount = "40000000"
	if _, err := submitWithKey(svc, changed, "key-1", actor); !errors.Is(err, loanapplication.ErrIdempotencyKeyReused) {
		t.Errorf("err = %v, want ErrIdempotencyKeyReused", err)
	}
	if n := count(t, pool, "loan_applications"); n != 1 {
		t.Errorf("applications = %d, want 1", n)
	}
}

func TestIdempotencyKeysAreScopedPerActor(t *testing.T) {
	pool := testutil.StartPostgres(t)
	svc := newService(pool)
	if _, err := submitWithKey(svc, singleRequest("CUST0001", "B1"), "shared-key", "officer-1"); err != nil {
		t.Fatal(err)
	}
	out, err := submitWithKey(svc, singleRequest("CUST0002", "B2"), "shared-key", "officer-2")
	if err != nil || out.Replayed {
		t.Fatalf("other actor's submission: out=%+v err=%v", out, err)
	}
	if n := count(t, pool, "loan_applications"); n != 2 {
		t.Errorf("applications = %d, want 2", n)
	}
}

func TestConcurrentRetriesWithSameKey(t *testing.T) {
	pool := testutil.StartPostgres(t)
	svc := newService(pool)
	req := singleRequest("CUST0001", "B1")

	const n = 20
	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		outs  = make([]loanapplication.SubmitOutput, n)
		errs  = make([]error, n)
	)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			outs[i], errs[i] = submitWithKey(svc, req, "same-key", actor)
		}()
	}
	close(start)
	wg.Wait()

	replays := 0
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if outs[i].Result.ID != outs[0].Result.ID {
			t.Errorf("goroutine %d got id %s, want %s", i, outs[i].Result.ID, outs[0].Result.ID)
		}
		if outs[i].Replayed {
			replays++
		}
	}
	if replays != n-1 {
		t.Errorf("replays = %d, want %d", replays, n-1)
	}
	if a, k := count(t, pool, "loan_applications"), count(t, pool, "idempotency_keys"); a != 1 || k != 1 {
		t.Errorf("applications=%d keys=%d, want 1 and 1", a, k)
	}
}

func TestFailedSubmissionDoesNotConsumeKey(t *testing.T) {
	pool := testutil.StartPostgres(t)
	svc := newService(pool)

	invalid := singleRequest("CUST0001", "B1")
	invalid.Loan.TenureMonths = "49"
	var verr *loanapplication.ValidationError
	if _, err := submitWithKey(svc, invalid, "key-1", actor); !errors.As(err, &verr) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	if n := count(t, pool, "idempotency_keys"); n != 0 {
		t.Errorf("keys after validation failure = %d, want 0", n)
	}

	// A conflict (409) is not stored either.
	if _, err := submitWithKey(svc, singleRequest("CUST0001", "B9"), "other-key", actor); err != nil {
		t.Fatal(err)
	}
	if _, err := submitWithKey(svc, singleRequest("CUST0001", "B1"), "key-1", actor); !errors.Is(err, loanapplication.ErrCustomerHasOpenApplication) {
		t.Fatalf("err = %v, want conflict", err)
	}
	if n := count(t, pool, "idempotency_keys"); n != 1 {
		t.Errorf("keys = %d, want 1 (only the successful submission)", n)
	}

	// The corrected retry with the original key succeeds.
	if _, err := submitWithKey(svc, singleRequest("CUST0002", "B1"), "key-1", actor); err != nil {
		t.Errorf("corrected retry: %v", err)
	}
}
