package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteProblem(t *testing.T) {
	SetProblemTypeBase("https://errors.test/")
	t.Cleanup(func() { SetProblemTypeBase(defaultProblemTypeBase) })

	r := httptest.NewRequest(http.MethodPost, "/v1/loan-applications", nil)
	w := httptest.NewRecorder()
	WriteProblem(w, r, Problem{
		Status: http.StatusBadRequest,
		Type:   "validation-failed",
		Title:  "Validation failed",
		Detail: "1 field is invalid.",
		Errors: []FieldError{{Field: "/loan/amount", Code: "not_multiple_of_100", Message: "must be a positive multiple of 100"}},
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["type"] != "https://errors.test/validation-failed" {
		t.Errorf("type = %v", body["type"])
	}
	if body["instance"] != "/v1/loan-applications" {
		t.Errorf("instance = %v", body["instance"])
	}
	if body["status"] != float64(400) || body["title"] != "Validation failed" || body["detail"] != "1 field is invalid." {
		t.Errorf("unexpected body %v", body)
	}
	errs, ok := body["errors"].([]any)
	if !ok || len(errs) != 1 {
		t.Fatalf("errors = %v", body["errors"])
	}
}

func TestWriteProblemOmitsEmptyErrorsAndSetsRetryAfter(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	WriteProblem(w, r, Problem{Status: http.StatusServiceUnavailable, Type: "temporarily-unavailable", Title: "Temporarily unavailable", RetryAfter: 5})

	if got := w.Header().Get("Retry-After"); got != "5" {
		t.Errorf("Retry-After = %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["errors"]; ok {
		t.Error("errors should be omitted when empty")
	}
}
