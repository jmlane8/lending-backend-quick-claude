# Research: Loan Application Submission

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-10-09

Each entry records a decision, why it was chosen, and what else was considered. Constitution
references use the form "Principle III".

## R1. Go version and HTTP routing

- **Decision**: Latest stable Go (the `go` directive in `go.mod` is the authority, Go 1.26 or
  later). Routing uses the standard `net/http.ServeMux` with method-and-path patterns
  (`POST /v1/loan-applications`).
- **Rationale**: Since Go 1.22 the standard mux supports method matching and path parameters,
  so no router dependency is needed (Principle VI).
- **Alternatives considered**: chi, gin, echo. They add little for a single endpoint, and
  Principle VI requires every dependency to be justified.

## R2. Database access, query generation, and migrations

- **Decision**: PostgreSQL 18. Access through `github.com/jackc/pgx/v5` (`pgxpool`), with
  queries written in SQL and turned into type-safe Go by `sqlc`. Migrations are versioned SQL
  files in `migrations/`, applied by `github.com/golang-migrate/migrate/v4` in CI, at deploy
  time, and in integration tests.
- **Rationale**: Explicit SQL, parameterized by construction (Principle IV), no ORM
  (Principle VI). PostgreSQL 18 provides `uuidv7()` built in for time-ordered primary keys.
- **Alternatives considered**: GORM or ent (ORMs, rejected under Principle VI); hand-written
  `pgx` scanning (workable, but sqlc removes boilerplate and catches schema drift at generate
  time); goose (equivalent to golang-migrate; either is fine).

## R3. Table shape: one wide application table, not separate customer/collateral tables

- **Decision**: The customer snapshot, address, and car collateral are stored as prefixed
  columns of `loan_applications` (`customer_*`, `collateral_*`). Spouse data goes in a separate
  optional table, `loan_application_spouses` (zero or one row per application).
- **Rationale**:
  - The spec treats customer data as a per-application snapshot. There is no customer
    master record yet (Assumptions), so a separate `customers` table would add a
    relationship nothing uses.
  - The two "one open application" rules (FR-007, FR-008) become plain partial unique indexes
    on a single table (`WHERE status = 'IN_PROGRESS' AND NOT deleted`). These are enforced by
    PostgreSQL even under concurrent submissions (FR-009), with no locking code.
  - Spouse data is optional and has its own identity fields, so it gets its own table rather
    than many nullable columns.
- **Alternatives considered**:
  - Normalized `customers` / `collaterals` tables. Cross-table "one open application"
    uniqueness would need triggers, advisory locks, or duplicated status columns.
  - `SELECT ... FOR UPDATE` plus an application-level check. This is racy without
    serializable isolation, and harder to prove correct than a unique index.

## R4. Money representation

- **Decision**:
  - **API**: amounts are JSON numbers. The decoder uses `json.Decoder.UseNumber()`, so a value
    is never turned into a `float64`.
  - **Go**: each amount is parsed from its exact decimal text into `int64` minor units
    (cents), via a small `money` package.
  - **Database**: amounts are stored as `NUMERIC(19,2)`.
  - **Precision**: at most 2 decimal places; more is a validation error.
  - **Loan amount**: must additionally be a whole number and a multiple of 100.
  - **Currency**: every application stores a `currency` (ISO 4217) taken from the
    `APP_CURRENCY` configuration (single-currency assumption), and responses include it.
- **Rationale**: Principle III bans floating-point money and requires an explicit currency.
  JSON numbers keep the API natural for clients while `UseNumber` keeps exact precision.
- **Alternatives considered**:
  - Decimal strings in JSON (`"15000.00"`): exact, but unusual for clients.
  - `shopspring/decimal`: an unneeded dependency, since amounts only need validation and
    storage, not arithmetic.

## R5. Idempotency (FR-006)

- **Decision**:
  - **Header**: `Idempotency-Key` is required on `POST /v1/loan-applications` and must be 1–255
    printable ASCII characters; missing or invalid → 400.
  - **Table**: `idempotency_keys` holds `(actor, key)`, a SHA-256 hash of the canonicalized
    request body, the response status, and the response body.
  - **Same transaction**: the key row is inserted in the same transaction as the application.
  - **First request with a key**: the key row is inserted with `INSERT ... ON CONFLICT DO
    NOTHING` before the application row, with the response columns empty. The stored response
    is filled in with an `UPDATE` just before commit.
  - **Hash**: the request hash is SHA-256 of the request body after `json.Compact`.
  - **Repeated key, same body hash**: replay the stored status and body; no new application.
  - **Repeated key, different body hash**: 422 with problem type `idempotency-key-reused`.
  - **Two identical requests at once**: the second one's insert waits on the first one's
    uncommitted row (row lock on the unique index). After the first commits, the second sees
    the conflict and replays the first's response.
  - **Validation failures and other 4xx outcomes** are not stored, so the client can fix the
    request and retry with the same key.
  - **Retention**: keys are kept 24 hours. Expired keys are soft-deleted by a later cleanup job
    (out of scope here; the column exists).
- **Rationale**: Principle I requires an `Idempotency-Key` on mutating money/loan endpoints. A
  single transaction makes "application created" and "key recorded" atomic, so a crash can
  never leave one without the other.
- **Alternatives considered**: a Redis-based idempotency store (another dependency, and not
  atomic with the insert); making the key optional (weakens SC-004).

## R6. Authentication and actor identity

- **Decision**: Bearer JWT. The signature is checked with `github.com/golang-jwt/jwt/v5`, using
  keys from configuration (`AUTH_JWT_ISSUER`, `AUTH_JWT_AUDIENCE`, and `AUTH_JWT_JWKS_URL` or
  `AUTH_JWT_PUBLIC_KEY`).
  - **Actor**: the token's `sub` claim is the actor. It is stored in a typed context key and
    used for `created_by` / `updated_by` (Principle III), never taken from the request body.
  - **Authorization**: the token must carry the scope `loan-applications:submit`.
  - **Errors**: missing or invalid token → 401; missing scope → 403.
- **Rationale**: Principle IV requires authentication and per-resource authorization. A JWT
  lets a customer-facing app and a loan-officer tool both call the API without session state.
  The spec leaves the mechanism to project standards, and this is the minimal standard choice.
- **Alternatives considered**: API keys (no per-user actor identity for audit columns); mTLS
  only (identifies services, not users).
- **Follow-up**: confirm the identity provider and claim names before deployment. Only
  configuration changes, not code. This feature implements `AUTH_JWT_PUBLIC_KEY` only;
  `AUTH_JWT_JWKS_URL` support is deferred until the identity provider is chosen.

## R13. Encryption of personal data at rest

- **Decision**: Storage-level encryption, decided 2026-10-12. Every environment holding real
  data MUST run PostgreSQL on encrypted storage: a managed PostgreSQL service with encryption at
  rest enabled, or encrypted volumes for self-hosted. Backups and replicas MUST be encrypted too.
  This is a deployment requirement recorded in `README.md`; there is no application code.
- **Rationale**: Satisfies constitution Principle IV. It keeps the "one open application"
  unique index on the plain ID number, and avoids application-level key management.
- **Alternatives considered**: column-level AES-GCM encryption of ID numbers with an HMAC column
  for uniqueness. This protects against database-level read access as well, but adds key
  management and rotation. Revisit it if a compliance review requires field-level protection.
- **Currency of spouse income**: the spouse's `monthly_income` is in the `currency` of its parent
  `loan_applications` row (one-to-one through the FK). This satisfies Principle III's explicit
  currency rule without a duplicate column.

## R7. Validation approach

- **Decision**: Hand-written validation in the `loanapplication` package. Every rule is
  checked and all failures are collected into a list of `{field, code, message}`, then
  returned together (FR-040).
  - **Strict decoding**: JSON decoding uses `DisallowUnknownFields()` (unknown fields are
    rejected, per Edge Cases) and a 64 KiB body size limit.
  - **Field paths**: use JSON Pointer style, e.g. `/customer/address/city`.
  - **Dates**: "today" (for the age check) and "next calendar year" (for the year check) are
    computed in the configured display timezone (`APP_TIMEZONE`). The clock is injected so
    tests can control it.
  - **Phone**: `^\+[0-9]{8,15}$`.
  - **Email**: parsed with `net/mail.ParseAddress`, the address must equal the input (no
    display name), and length must be ≤ 254.
- **Rationale**: About 20 rules; a validation library adds struct-tag magic and still needs
  custom rules for cross-field logic (spouse only if `MARRIED`, spouse ID ≠ customer ID). Plain
  code is explicit and easy to table-test (Principles II, VI).
- **Alternatives considered**: `go-playground/validator` (struct tags, awkward cross-field and
  aggregate error shaping); JSON Schema validation at runtime (duplicates the OpenAPI document
  and gives poor messages for business rules).

## R8. Error responses

- **Decision**: All errors are `application/problem+json` (RFC 9457). The `type` is a URI
  under `https://errors.lending.example/` (placeholder base set by configuration). The code
  (sent as the problem `type`) and status for each case:

  | Situation | Status | `type` suffix |
  |-----------|--------|---------------|
  | Body unreadable / not JSON / wrong JSON types / unknown field / too large | 400 | `malformed-request` |
  | Missing or invalid `Idempotency-Key` header | 400 | `invalid-idempotency-key` |
  | One or more field rules fail | 400 | `validation-failed` (with `errors[]`) |
  | Missing or invalid token | 401 | `unauthenticated` |
  | Missing scope | 403 | `forbidden` |
  | Customer ID already has an `IN_PROGRESS` application | 409 | `customer-has-open-application` |
  | License plate already backs an `IN_PROGRESS` application | 409 | `collateral-in-use` |
  | Same idempotency key, different body | 422 | `idempotency-key-reused` |
  | Database unavailable / timeout | 503 | `temporarily-unavailable` (with `Retry-After`) |
  | Unexpected error | 500 | `internal-error` |

- **Rationale**: Principle I requires problem details. Following the constitution's
  timestamp rule, all field validation errors use 400. The 409 conflict types tell the client
  which rule fired (FR-041) without exposing the other application's identity or data.
- **Alternatives considered**: 422 for field validation errors (also common, but the
  constitution already fixes 400 for format errors, and one code is simpler for clients).

## R9. Timestamps

- **Decision**: A shared `httpx.Timestamp` type wraps `time.Time`. Its `MarshalJSON` formats
  the value in the `APP_TIMEZONE` location with layout `2006-01-02T15:04:05-07:00`, and
  `UnmarshalJSON` accepts only that layout. Birth date is a separate `httpx.Date` type using
  `2006-01-02`. The database stores `TIMESTAMPTZ`, which is UTC.
- **Rationale**: Principle I fixes the timestamp format and Principle III requires UTC storage.
  Go's default `time.Time` JSON includes nanoseconds and `Z`, which would break the format.
- **Alternatives considered**: formatting at each call site (easy to forget; a single type
  enforces the format everywhere).

## R10. Audit trail of status changes

- **Decision**: Besides the audit columns, insert a `loan_application_status_history` row
  (`from_status` NULL → `to_status` `IN_PROGRESS`, `changed_by`, `changed_at`) in the same
  transaction as the application.
- **Rationale**: Principle V requires an audit record of who, what, and when for loan state
  changes. Audit columns only keep the latest update, so later transitions need a history
  table, and starting it now avoids a backfill.
- **Alternatives considered**: relying only on `created_at`/`created_by` (enough for creation,
  but loses history once later features change status); an event-sourcing log (beyond this
  feature's needs).

## R11. Testing stack

- **Decision**:
  - **Unit tests**: standard `testing` with table-driven tests for validation, the money
    parser, and timestamp/date types.
  - **Integration tests**: `github.com/testcontainers/testcontainers-go` with the
    `postgres:18` module, run under the `integration` build tag. Migrations are applied, and
    the real repository and service are exercised, including concurrent-submission tests for
    FR-009 and the idempotency race.
  - **Contract tests**: start the real HTTP handler (`httptest.Server`) against a Testcontainers
    database, and check every request and response against `api/openapi.yaml` using
    `github.com/getkin/kin-openapi/openapi3filter`.
  - **Race detector**: all tests run with `-race`.
- **Rationale**: Principle II forbids mocking the database for repository tests and requires
  contract tests against the OpenAPI spec.
- **Alternatives considered**: `pgxmock` (forbidden for repository tests); dockertest (similar
  to Testcontainers; Testcontainers has a maintained Postgres module).

## R12. Observability and operations

- **Decision**:
  - **Logging**: `log/slog` JSON handler with a request-ID middleware. The ID comes from the
    `X-Request-ID` header or is generated as a UUIDv7. Validation error logs carry field paths
    and codes only, never values (FR-050).
  - **Metrics**: Prometheus via `github.com/prometheus/client_golang`, exposed on
    `GET /metrics`:
    - HTTP request duration histogram and request counter, labeled by route and status class.
    - pgxpool stats.
    - Counter `loan_applications_submitted_total`.
  - **Health**: `GET /healthz` (process up) and `GET /readyz` (`pool.Ping` within 1 s).
  - **Shutdown**: graceful shutdown on SIGTERM with a 20 s drain.
- **Rationale**: Principle V. This is the first feature, so it also delivers the service
  skeleton that later features reuse.
- **Alternatives considered**: OpenTelemetry tracing (valuable later; not required by the
  constitution now, and it adds several dependencies).
