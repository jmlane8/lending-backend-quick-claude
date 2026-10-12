//go:build integration

package contract

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestLogsContainNoPersonalData drives accepted, invalid, conflicting, malformed, and
// replayed submissions through the full handler, then scans every log line for the
// fixtures' personal data (SC-006, FR-050).
func TestLogsContainNoPersonalData(t *testing.T) {
	e := newEnv(t)

	married := example(t, "married")
	single := example(t, "single")
	key := uuid.NewString()
	e.submit(call{body: married, idempotencyKey: key})
	e.submit(call{body: married, idempotencyKey: key}) // replay
	e.submit(call{body: single})

	invalid := example(t, "married")
	set(invalid, []string{"customer", "email"}, "budi-at-example")
	set(invalid, []string{"loan", "amount"}, 150)
	e.submit(call{body: invalid})

	e.submit(call{body: married}) // customer conflict
	unknown := example(t, "single")
	unknown["note"] = "Andi Wijaya 3273000011112222"
	e.submit(call{body: unknown}) // malformed
	reused := example(t, "married")
	set(reused, []string{"loan", "tenureMonths"}, 12)
	e.submit(call{body: reused, idempotencyKey: key}) // key reused
	e.submit(call{body: single, token: "-"})          // unauthenticated

	logs := e.logs.String()
	if strings.Count(logs, "\n") < 8 {
		t.Fatalf("expected at least 8 log lines, got:\n%s", logs)
	}
	personal := []string{
		"Budi Santoso", "3171234567890001", "+6281234567890", "budi@example.com", "budi-at-example",
		"Jl. Merdeka", "1988-03-21",
		"Siti Rahma", "3171234567890002", "+6281298765432",
		"Andi Wijaya", "3273000011112222", "+6285711122233", "andi@example.com", "Jl. Asia Afrika", "1999-11-02",
		"B 1234 XYZ", "B1234XYZ", "D-5678-AB", "D5678AB",
	}
	for _, value := range personal {
		if strings.Contains(logs, value) {
			t.Errorf("logs contain personal data %q", value)
		}
	}
}
