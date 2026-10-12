-- Request deduplication for POST /v1/loan-applications (FR-006, constitution Principle I).
-- The key row is inserted first in the submission transaction with NULL response columns,
-- so concurrent duplicates wait on it; the response is filled in just before commit.

CREATE TABLE idempotency_keys (
    id                  UUID        PRIMARY KEY DEFAULT uuidv7(),
    actor               TEXT        NOT NULL,
    idempotency_key     TEXT        NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 255),
    request_hash        BYTEA       NOT NULL,
    response_status     SMALLINT    NULL,
    response_body       JSONB       NULL,
    loan_application_id UUID        NULL REFERENCES loan_applications (id),
    expires_at          TIMESTAMPTZ NOT NULL,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by          TEXT        NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by          TEXT        NOT NULL,
    deleted             BOOLEAN     NOT NULL DEFAULT false,
    deleted_at          TIMESTAMPTZ NULL,
    CONSTRAINT idempotency_keys_response_complete
        CHECK ((response_status IS NULL) = (response_body IS NULL)),
    CONSTRAINT idempotency_keys_deleted_consistent
        CHECK ((deleted AND deleted_at IS NOT NULL) OR (NOT deleted AND deleted_at IS NULL))
);

CREATE UNIQUE INDEX uq_idempotency_keys_actor_key
    ON idempotency_keys (actor, idempotency_key)
    WHERE NOT deleted;
