package loanapplication

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jmlane8/lending-backend-quick-claude/internal/auth"
	"github.com/jmlane8/lending-backend-quick-claude/internal/db"
	"github.com/jmlane8/lending-backend-quick-claude/internal/httpx"
)

// SubmitScope is the token scope required to submit applications.
const SubmitScope = "loan-applications:submit"

const maxBodyBytes = 64 << 10

var errInvalidIdempotencyKey = errors.New("missing or invalid Idempotency-Key header")

// Handler exposes the loan application endpoints.
type Handler struct {
	svc      *Service
	verifier *auth.Verifier
}

// NewHandler returns a Handler.
func NewHandler(svc *Service, verifier *auth.Verifier) *Handler {
	return &Handler{svc: svc, verifier: verifier}
}

// Register adds the routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /v1/loan-applications", auth.Middleware(h.verifier, SubmitScope)(http.HandlerFunc(h.submit)))
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if !ValidIdempotencyKey(key) {
		h.reject(w, r, errInvalidIdempotencyKey)
		return
	}
	raw, err := httpx.ReadBody(w, r, maxBodyBytes)
	if err != nil {
		h.reject(w, r, err)
		return
	}
	var req SubmitRequest
	if err := httpx.DecodeJSONBytes(raw, &req); err != nil {
		h.reject(w, r, err)
		return
	}
	actor, _ := auth.ActorFrom(r.Context())

	out, err := h.svc.Submit(r.Context(), SubmitInput{
		Request:        req,
		RawBody:        raw,
		IdempotencyKey: key,
		Actor:          actor,
	})
	if err != nil {
		h.reject(w, r, err)
		return
	}
	writeCreated(w, out.Result)
}

func writeCreated(w http.ResponseWriter, res SubmitResult) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/v1/loan-applications/"+res.ID.String())
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
}

// reject maps errors to problem responses. Logs carry problem types and field
// paths/codes only — never request values (FR-050). Conflict details never reveal
// the other application.
func (h *Handler) reject(w http.ResponseWriter, r *http.Request, err error) {
	logger := httpx.LoggerFrom(r.Context())
	var (
		malformed *httpx.MalformedError
		invalid   *ValidationError
		problem   httpx.Problem
	)
	switch {
	case errors.As(err, &malformed):
		problem = httpx.Malformed(malformed.Reason)
	case errors.Is(err, errInvalidIdempotencyKey):
		problem = httpx.Problem{
			Status: http.StatusBadRequest,
			Type:   "invalid-idempotency-key",
			Title:  "Invalid Idempotency-Key",
			Detail: "The Idempotency-Key header is required: 1 to 255 printable ASCII characters without spaces.",
		}
	case errors.Is(err, ErrIdempotencyKeyReused):
		problem = httpx.Problem{
			Status: http.StatusUnprocessableEntity,
			Type:   "idempotency-key-reused",
			Title:  "Idempotency-Key reused",
			Detail: "This Idempotency-Key was already used with a different request body. Use a new key for a new request.",
		}
	case errors.As(err, &invalid):
		problem = httpx.Problem{
			Status: http.StatusBadRequest,
			Type:   "validation-failed",
			Title:  "Validation failed",
			Detail: fieldCount(len(invalid.Errors)),
			Errors: invalid.Errors,
		}
	case errors.Is(err, ErrCustomerHasOpenApplication):
		problem = httpx.Problem{
			Status: http.StatusConflict,
			Type:   "customer-has-open-application",
			Title:  "Customer already has an open application",
			Detail: "The customer already has a loan application in progress.",
		}
	case errors.Is(err, ErrCollateralInUse):
		problem = httpx.Problem{
			Status: http.StatusConflict,
			Type:   "collateral-in-use",
			Title:  "Collateral already in use",
			Detail: "The car is already pledged on a loan application in progress.",
		}
	case db.IsUnavailable(err):
		logger.Error("database unavailable", slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.Unavailable())
		return
	default:
		logger.Error("loan application submission failed", slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.InternalError())
		return
	}

	attrs := []any{slog.String("reason", problem.Type)}
	if len(problem.Errors) > 0 {
		fields := make([]string, len(problem.Errors))
		for i, fe := range problem.Errors {
			fields[i] = fe.Field + ":" + fe.Code
		}
		attrs = append(attrs, slog.Any("fields", fields))
	}
	logger.Warn("loan application rejected", attrs...)
	httpx.WriteProblem(w, r, problem)
}

func fieldCount(n int) string {
	if n == 1 {
		return "1 field is invalid."
	}
	return fmt.Sprintf("%d fields are invalid.", n)
}
