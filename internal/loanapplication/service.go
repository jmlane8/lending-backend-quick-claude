package loanapplication

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jmlane8/lending-backend-quick-claude/internal/httpx"
)

// ServiceConfig configures the Service.
type ServiceConfig struct {
	Currency   string           // ISO 4217 currency of every amount (APP_CURRENCY)
	Location   *time.Location   // business timezone for "today" (APP_TIMEZONE)
	Now        func() time.Time // injectable clock
	Registerer prometheus.Registerer
}

// Service applies the submission rules and owns the transaction boundary.
type Service struct {
	pool      *pgxpool.Pool
	repo      *Repository
	cfg       ServiceConfig
	submitted prometheus.Counter
}

// NewService builds a Service and registers its metrics.
func NewService(pool *pgxpool.Pool, cfg ServiceConfig) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Location == nil {
		cfg.Location = time.UTC
	}
	submitted := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "loan_applications_submitted_total",
		Help: "Loan applications stored with status IN_PROGRESS.",
	})
	if cfg.Registerer != nil {
		cfg.Registerer.MustRegister(submitted)
	}
	return &Service{pool: pool, repo: NewRepository(), cfg: cfg, submitted: submitted}
}

// SubmitInput is one submission attempt.
type SubmitInput struct {
	Request        SubmitRequest
	RawBody        []byte // exact request body, used for the idempotency hash
	IdempotencyKey string
	Actor          string // authenticated subject; never taken from the body
}

// SubmitOutput is the outcome of a successful submission.
type SubmitOutput struct {
	Result   SubmitResult
	Replayed bool // true when an earlier response was returned for the same Idempotency-Key
}

// Submit validates the request and stores a new application with status IN_PROGRESS,
// at most once per (actor, Idempotency-Key):
//
//  1. Invalid requests return *ValidationError without touching the database.
//  2. The key is claimed first inside the transaction, so a concurrent duplicate waits
//     on it instead of racing into the open-application indexes.
//  3. If the key already exists, the stored response is replayed when the body hash
//     matches; otherwise ErrIdempotencyKeyReused is returned.
//  4. Failures roll back the claim, so the client can correct the request and retry
//     with the same key.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (SubmitOutput, error) {
	if errs := Validate(in.Request, s.cfg.Now().In(s.cfg.Location)); len(errs) > 0 {
		return SubmitOutput{}, &ValidationError{Errors: errs}
	}
	app, err := toApplication(in.Request)
	if err != nil {
		return SubmitOutput{}, fmt.Errorf("map request: %w", err)
	}
	hash, err := RequestHash(in.RawBody)
	if err != nil {
		return SubmitOutput{}, err
	}

	var out SubmitOutput
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		keyID, claimed, err := s.repo.ClaimKey(ctx, tx, in.Actor, in.IdempotencyKey, hash)
		if err != nil {
			return err
		}
		if !claimed {
			out, err = s.replay(ctx, tx, in, hash)
			return err
		}

		id, submittedAt, err := s.repo.Create(ctx, tx, app, s.cfg.Currency, in.Actor)
		if err != nil {
			return err
		}
		out.Result = SubmitResult{
			ID:          id,
			Status:      StatusInProgress,
			Currency:    s.cfg.Currency,
			SubmittedAt: httpx.Timestamp{Time: submittedAt.Truncate(time.Second)}, // API precision; equals replays
		}
		body, err := json.Marshal(out.Result)
		if err != nil {
			return fmt.Errorf("marshal response: %w", err)
		}
		return s.repo.CompleteKey(ctx, tx, keyID, http.StatusCreated, body, id, in.Actor)
	})
	if err != nil {
		return SubmitOutput{}, err
	}
	if !out.Replayed {
		s.submitted.Inc()
	}
	return out, nil
}

func (s *Service) replay(ctx context.Context, tx pgx.Tx, in SubmitInput, hash []byte) (SubmitOutput, error) {
	stored, err := s.repo.GetKey(ctx, tx, in.Actor, in.IdempotencyKey)
	if err != nil {
		return SubmitOutput{}, err
	}
	if !bytes.Equal(stored.RequestHash, hash) {
		return SubmitOutput{}, ErrIdempotencyKeyReused
	}
	if stored.ResponseStatus == nil || *stored.ResponseStatus != http.StatusCreated {
		// Committed key rows always carry a response (CHECK constraint); this is a bug.
		return SubmitOutput{}, errors.New("idempotency key has no stored response")
	}
	var res SubmitResult
	if err := json.Unmarshal(stored.ResponseBody, &res); err != nil {
		return SubmitOutput{}, fmt.Errorf("decode stored response: %w", err)
	}
	return SubmitOutput{Result: res, Replayed: true}, nil
}
