-- name: ClaimIdempotencyKey :one
-- Returns no row when the key already exists. A concurrent uncommitted claim blocks
-- this insert until that transaction ends.
INSERT INTO idempotency_keys (
    actor, idempotency_key, request_hash, expires_at, created_by, updated_by
) VALUES (
    @actor, @idempotency_key, @request_hash, now() + interval '24 hours', @actor, @actor
)
ON CONFLICT (actor, idempotency_key) WHERE NOT deleted DO NOTHING
RETURNING id;

-- name: GetIdempotencyKey :one
SELECT request_hash, response_status, response_body
FROM idempotency_keys
WHERE actor = @actor AND idempotency_key = @idempotency_key AND NOT deleted;

-- name: CompleteIdempotencyKey :exec
UPDATE idempotency_keys
SET response_status     = @response_status,
    response_body       = @response_body,
    loan_application_id = @loan_application_id,
    updated_at          = now(),
    updated_by          = @actor
WHERE id = @id;
