//go:build integration

package contract

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\+07:00$`)

func assertCreated(t *testing.T, r response) map[string]any {
	t.Helper()
	if r.status != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", r.status, r.body)
	}
	body := r.json(t)
	id, _ := body["id"].(string)
	if loc := r.header.Get("Location"); loc != "/v1/loan-applications/"+id || id == "" {
		t.Errorf("Location = %q, id = %q", loc, id)
	}
	if body["status"] != "IN_PROGRESS" {
		t.Errorf("status = %v", body["status"])
	}
	if body["currency"] != currency {
		t.Errorf("currency = %v", body["currency"])
	}
	if ts, _ := body["submittedAt"].(string); !timestampPattern.MatchString(ts) {
		t.Errorf("submittedAt = %q, want YYYY-MM-DDTHH:MM:SS+07:00", ts)
	}
	return body
}

func TestSubmitExamples(t *testing.T) {
	e := newEnv(t)
	for _, name := range []string{"married", "single"} {
		t.Run(name, func(t *testing.T) {
			e.t = t
			assertCreated(t, e.submit(call{body: example(t, name), validateRequest: true}))
		})
	}
	e.t = t
	if n := e.countApplications(); n != 2 {
		t.Errorf("applications = %d, want 2", n)
	}
}

func TestSubmitBoundaryValues(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		amount, tenure int
	}{
		{100, 3},
		{100, 48},
		{15000, 24},
	}
	for i, c := range cases {
		t.Run(fmt.Sprintf("amount=%d tenure=%d", c.amount, c.tenure), func(t *testing.T) {
			e.t = t
			body := example(t, "single")
			// Each application needs its own customer and car (one open application each).
			set(body, []string{"customer", "idNumber"}, fmt.Sprintf("BOUNDARY%d", i))
			set(body, []string{"collateral", "licensePlate"}, fmt.Sprintf("BND %d", i))
			set(body, []string{"loan", "amount"}, c.amount)
			set(body, []string{"loan", "tenureMonths"}, c.tenure)
			assertCreated(t, e.submit(call{body: body, validateRequest: true}))
		})
	}
}

func TestSubmitResponseHasNoPersonalData(t *testing.T) {
	e := newEnv(t)
	r := e.submit(call{body: example(t, "married"), validateRequest: true})
	assertCreated(t, r)
	for _, pii := range []string{"Budi", "3171234567890001", "budi@example.com", "+6281234567890"} {
		if strings.Contains(string(r.body), pii) {
			t.Errorf("response body contains personal data %q", pii)
		}
	}
}
