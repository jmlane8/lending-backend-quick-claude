//go:build integration

package contract

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestIdempotencyKeyHeaderRequired(t *testing.T) {
	e := newEnv(t)
	r := e.submit(call{body: example(t, "single"), idempotencyKey: "-"})
	assertProblem(t, r, http.StatusBadRequest, "invalid-idempotency-key")

	r = e.submit(call{body: example(t, "single"), idempotencyKey: strings.Repeat("k", 256)})
	assertProblem(t, r, http.StatusBadRequest, "invalid-idempotency-key")

	if n := e.countApplications(); n != 0 {
		t.Errorf("applications = %d, want 0", n)
	}
}

func TestIdempotentReplayOverHTTP(t *testing.T) {
	e := newEnv(t)
	key := uuid.NewString()
	body := example(t, "married")

	first := e.submit(call{body: body, idempotencyKey: key, validateRequest: true})
	assertCreated(t, first)
	second := e.submit(call{body: body, idempotencyKey: key, validateRequest: true})
	assertCreated(t, second)

	if !bytes.Equal(first.body, second.body) {
		t.Errorf("replay body differs:\n%s\n%s", first.body, second.body)
	}
	if first.header.Get("Location") != second.header.Get("Location") {
		t.Errorf("Location differs: %q vs %q", first.header.Get("Location"), second.header.Get("Location"))
	}
	if n := e.countApplications(); n != 1 {
		t.Errorf("applications = %d, want 1", n)
	}

	changed := example(t, "married")
	set(changed, []string{"loan", "amount"}, 90000000)
	r := e.submit(call{body: changed, idempotencyKey: key})
	assertProblem(t, r, http.StatusUnprocessableEntity, "idempotency-key-reused")
}
