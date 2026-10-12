package loanapplication

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jmlane8/lending-backend-quick-claude/internal/db/sqlc"
)

// ErrIdempotencyKeyReused is returned when an Idempotency-Key was already used with a different body.
var ErrIdempotencyKeyReused = errors.New("idempotency key reused with a different request")

// RequestHash fingerprints a request body: SHA-256 of the compacted JSON, so
// whitespace-only differences between retries do not matter.
func RequestHash(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, fmt.Errorf("compact request body: %w", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return sum[:], nil
}

// ValidIdempotencyKey reports whether k is 1–255 printable ASCII characters without spaces.
func ValidIdempotencyKey(k string) bool {
	if k == "" || len(k) > 255 {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 0x21 || k[i] > 0x7E {
			return false
		}
	}
	return true
}

// StoredKey is a previously completed idempotency record.
type StoredKey struct {
	RequestHash    []byte
	ResponseStatus *int16
	ResponseBody   []byte
}

// ClaimKey inserts the key row and reports whether this transaction owns it. When the
// key is held by a concurrent, uncommitted transaction, the insert waits for it.
func (r *Repository) ClaimKey(ctx context.Context, tx pgx.Tx, actor, key string, hash []byte) (uuid.UUID, bool, error) {
	id, err := sqlc.New(tx).ClaimIdempotencyKey(ctx, sqlc.ClaimIdempotencyKeyParams{
		Actor: actor, IdempotencyKey: key, RequestHash: hash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("claim idempotency key: %w", err)
	}
	return id, true, nil
}

// GetKey reads an existing key. Under READ COMMITTED it sees the row committed by the
// transaction that ClaimKey waited for.
func (r *Repository) GetKey(ctx context.Context, tx pgx.Tx, actor, key string) (StoredKey, error) {
	row, err := sqlc.New(tx).GetIdempotencyKey(ctx, sqlc.GetIdempotencyKeyParams{Actor: actor, IdempotencyKey: key})
	if err != nil {
		return StoredKey{}, fmt.Errorf("get idempotency key: %w", err)
	}
	return StoredKey{RequestHash: row.RequestHash, ResponseStatus: row.ResponseStatus, ResponseBody: row.ResponseBody}, nil
}

// CompleteKey stores the response so retries can replay it.
func (r *Repository) CompleteKey(ctx context.Context, tx pgx.Tx, keyID uuid.UUID, status int16, body []byte, applicationID uuid.UUID, actor string) error {
	if err := sqlc.New(tx).CompleteIdempotencyKey(ctx, sqlc.CompleteIdempotencyKeyParams{
		ResponseStatus:    &status,
		ResponseBody:      body,
		LoanApplicationID: &applicationID,
		Actor:             actor,
		ID:                keyID,
	}); err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	return nil
}
