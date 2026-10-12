//go:build integration

package contract

import (
	"net/http"
	"strings"
	"testing"
)

func assertProblem(t *testing.T, r response, status int, typeSuffix string) map[string]any {
	t.Helper()
	if r.status != status {
		t.Fatalf("status = %d, want %d; body: %s", r.status, status, r.body)
	}
	if ct := r.header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q", ct)
	}
	body := r.json(t)
	if typ, _ := body["type"].(string); !strings.HasSuffix(typ, "/"+typeSuffix) {
		t.Errorf("type = %q, want suffix %q", typ, typeSuffix)
	}
	return body
}

func fieldErrors(t *testing.T, body map[string]any) map[string]string {
	t.Helper()
	out := map[string]string{}
	list, _ := body["errors"].([]any)
	for _, item := range list {
		e := item.(map[string]any)
		out[e["field"].(string)] = e["code"].(string)
	}
	return out
}

func TestSubmitValidationErrors(t *testing.T) {
	e := newEnv(t)

	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   map[string]string
	}{
		{
			name: "quickstart #7: four invalid fields reported together",
			mutate: func(b map[string]any) {
				set(b, []string{"loan", "amount"}, 15050)
				set(b, []string{"loan", "tenureMonths"}, 49)
				set(b, []string{"customer", "email"}, "x")
				set(b, []string{"customer", "address", "city"}, nil)
			},
			want: map[string]string{
				"/loan/amount":           "not_multiple_of_100",
				"/loan/tenureMonths":     "out_of_range",
				"/customer/email":        "invalid_format",
				"/customer/address/city": "required",
			},
		},
		{
			name:   "married without spouse",
			mutate: func(b map[string]any) { set(b, []string{"customer", "spouse"}, nil) },
			want:   map[string]string{"/customer/spouse": "spouse_required"},
		},
		{
			name:   "single with spouse",
			mutate: func(b map[string]any) { set(b, []string{"customer", "maritalStatus"}, "SINGLE") },
			want:   map[string]string{"/customer/spouse": "spouse_not_allowed"},
		},
		{
			name:   "license plate of only separators",
			mutate: func(b map[string]any) { set(b, []string{"collateral", "licensePlate"}, "- -") },
			want:   map[string]string{"/collateral/licensePlate": "invalid_format"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e.t = t
			body := example(t, "married")
			c.mutate(body)
			r := e.submit(call{body: body})
			problem := assertProblem(t, r, http.StatusBadRequest, "validation-failed")
			got := fieldErrors(t, problem)
			if len(got) != len(c.want) {
				t.Errorf("errors = %v, want %v", got, c.want)
			}
			for field, code := range c.want {
				if got[field] != code {
					t.Errorf("%s = %q, want %q", field, got[field], code)
				}
			}
		})
	}
	e.t = t
	if n := e.countApplications(); n != 0 {
		t.Errorf("applications stored = %d, want 0", n)
	}
}

func TestSubmitMalformedRequests(t *testing.T) {
	e := newEnv(t)
	unknown := example(t, "single")
	unknown["foo"] = 1

	cases := map[string]call{
		"unknown field": {body: unknown},
		"not JSON":      {rawBody: []byte("customer=budi")},
		"wrong type":    {rawBody: []byte(`{"customer":{"fullName":123}}`)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e.t = t
			assertProblem(t, e.submit(c), http.StatusBadRequest, "malformed-request")
		})
	}
	e.t = t
	if n := e.countApplications(); n != 0 {
		t.Errorf("applications stored = %d, want 0", n)
	}
}

func TestSubmitAuthErrors(t *testing.T) {
	e := newEnv(t)
	r := e.submit(call{body: example(t, "single"), token: "-"})
	assertProblem(t, r, http.StatusUnauthorized, "unauthenticated")

	r = e.submit(call{body: example(t, "single"), token: e.signer.Token("officer-7", "profile")})
	assertProblem(t, r, http.StatusForbidden, "forbidden")

	if n := e.countApplications(); n != 0 {
		t.Errorf("applications stored = %d, want 0", n)
	}
}

func TestSubmitOpenApplicationConflicts(t *testing.T) {
	e := newEnv(t)
	first := example(t, "married") // plate "B 1234 XYZ", id 3171234567890001
	if r := e.submit(call{body: first, validateRequest: true}); r.status != http.StatusCreated {
		t.Fatalf("first submission: %d %s", r.status, r.body)
	}

	sameCustomer := example(t, "married")
	set(sameCustomer, []string{"collateral", "licensePlate"}, "B 9999 ZZZ")
	r := e.submit(call{body: sameCustomer})
	body := assertProblem(t, r, http.StatusConflict, "customer-has-open-application")
	if detail, _ := body["detail"].(string); strings.Contains(detail, "3171234567890001") {
		t.Errorf("detail leaks data: %q", detail)
	}

	sameCar := example(t, "single")
	set(sameCar, []string{"collateral", "licensePlate"}, "b1234xyz")
	r = e.submit(call{body: sameCar})
	assertProblem(t, r, http.StatusConflict, "collateral-in-use")
	if strings.Contains(string(r.body), "B1234XYZ") {
		t.Error("conflict response leaks the plate")
	}

	if n := e.countApplications(); n != 1 {
		t.Errorf("applications stored = %d, want 1", n)
	}
}

func TestSubmitDatabaseUnavailable(t *testing.T) {
	e := newEnv(t)
	e.pool.Close()
	r := e.submit(call{body: example(t, "single")})
	assertProblem(t, r, http.StatusServiceUnavailable, "temporarily-unavailable")
	if r.header.Get("Retry-After") == "" {
		t.Error("missing Retry-After header")
	}
}
