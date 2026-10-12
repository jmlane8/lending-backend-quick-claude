// Package httpx holds HTTP plumbing shared by all features: problem details,
// strict JSON decoding, API timestamp types, and middleware.
package httpx

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync/atomic"
)

const defaultProblemTypeBase = "https://errors.lending.example/"

var problemTypeBase atomic.Value

func init() { problemTypeBase.Store(defaultProblemTypeBase) }

// SetProblemTypeBase sets the URI prefix for problem types (ERROR_TYPE_BASE_URL).
func SetProblemTypeBase(base string) { problemTypeBase.Store(base) }

// FieldError describes one invalid request field.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Problem is an RFC 9457 problem details response.
type Problem struct {
	// Type is the suffix appended to the problem type base, e.g. "validation-failed".
	Type       string
	Title      string
	Status     int
	Detail     string
	Errors     []FieldError
	RetryAfter int // seconds; sets the Retry-After header when > 0
}

type problemBody struct {
	Type     string       `json:"type"`
	Title    string       `json:"title"`
	Status   int          `json:"status"`
	Detail   string       `json:"detail,omitempty"`
	Instance string       `json:"instance"`
	Errors   []FieldError `json:"errors,omitempty"`
}

// WriteProblem writes p as application/problem+json. The instance is the request path.
func WriteProblem(w http.ResponseWriter, r *http.Request, p Problem) {
	h := w.Header()
	h.Set("Content-Type", "application/problem+json")
	if p.RetryAfter > 0 {
		h.Set("Retry-After", strconv.Itoa(p.RetryAfter))
	}
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(problemBody{
		Type:     problemTypeBase.Load().(string) + p.Type,
		Title:    p.Title,
		Status:   p.Status,
		Detail:   p.Detail,
		Instance: r.URL.Path,
		Errors:   p.Errors,
	})
}

// Common problems.

// InternalError is the generic 500 problem; details are logged, never returned.
func InternalError() Problem {
	return Problem{Status: http.StatusInternalServerError, Type: "internal-error", Title: "Internal server error"}
}

// Unavailable is the retryable 503 problem.
func Unavailable() Problem {
	return Problem{
		Status:     http.StatusServiceUnavailable,
		Type:       "temporarily-unavailable",
		Title:      "Temporarily unavailable",
		Detail:     "The service is temporarily unavailable. Retry the request with the same Idempotency-Key.",
		RetryAfter: 5,
	}
}

// Malformed is the 400 problem for requests that cannot be read.
func Malformed(detail string) Problem {
	return Problem{Status: http.StatusBadRequest, Type: "malformed-request", Title: "Malformed request", Detail: detail}
}
