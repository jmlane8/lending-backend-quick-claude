---
description: "Task list for feature 001-loan-submission"
---

# Tasks: Loan Application Submission

**Input**: Design documents from `/specs/001-loan-submission/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/openapi.yaml, quickstart.md

**Tests**: INCLUDED. Constitution Principle II (Test-First, non-negotiable) requires TDD. Every
test task MUST be written and observed failing before its implementation task is started.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested
on its own.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an unfinished task)
- **[Story]**: User story the task belongs to (US1, US2, US3)

## Conventions used by every task

- **Module path**: `github.com/jmlane8/lending-backend-quick-claude`.
- **Integration and contract tests**: start with `//go:build integration`.
- **Error codes**: problem `type` URIs are `<ERROR_TYPE_BASE_URL><suffix>`, with suffixes as in
  research.md R8. Field-error codes come from the `FieldError.code` enum in
  `api/openapi.yaml`.
- **Audit columns**: every new table includes all six audit columns from data-model.md:
  - `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
  - `created_by TEXT NOT NULL`
  - `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`
  - `updated_by TEXT NOT NULL`
  - `deleted BOOLEAN NOT NULL DEFAULT false`
  - `deleted_at TIMESTAMPTZ NULL`
  - plus `CHECK ((deleted AND deleted_at IS NOT NULL) OR (NOT deleted AND deleted_at IS NULL))`
- **Money**: never use `float32`/`float64` for money anywhere.
- **Logging**: never log personal data values.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and tooling

- [X] T001 Initialize the Go module with `go mod init github.com/jmlane8/lending-backend-quick-claude` (latest stable Go in the `go` directive) and create the directories from plan.md: `cmd/api/`, `cmd/devtoken/`, `api/`, `migrations/`, `internal/{config,auth,httpx,money,server,db/queries,db/sqlc,loanapplication}/`, `tests/{integration,contract,testutil}/`, `dev/examples/`
- [X] T002 [P] Create `.gitignore` ignoring `/bin/`, `dev/*.key`, `dev/*.pub`, `*.test`, `coverage.out`, `.env`
- [X] T003 [P] Create `.golangci.yml` enabling `errcheck`, `govet`, `staticcheck`, `gosec`, `revive`, `unused`, `gofmt`, `goimports`, `bodyclose`, `sqlclosecheck`, `contextcheck`, `errorlint`
- [X] T004 [P] Create `sqlc.yaml`:
  - `version: "2"`, `engine: postgresql`
  - `schema: migrations`, `queries: internal/db/queries`
  - Go output: package `sqlc`, path `internal/db/sqlc`, `sql_package: pgx/v5`, `emit_pointers_for_null_types: true`
- [X] T005 [P] Copy `specs/001-loan-submission/contracts/openapi.yaml` to `api/openapi.yaml`; from now on it is the source-of-truth contract (Principle I)
- [X] T006 [P] Create `Makefile` with these targets:
  - `build` → `go build -o bin/api ./cmd/api`
  - `run`
  - `test` → `go test -race ./...`
  - `test-integration` → `go test -race -tags=integration ./...`
  - `lint` → `gofmt -l . && go vet ./... && golangci-lint run`
  - `vuln` → `govulncheck ./...`
  - `sqlc` → `sqlc generate`
  - `migrate-up` → `migrate -path migrations -database "$$DATABASE_URL" up`
  - `dev-keys` → generate an ECDSA P-256 keypair into `dev/jwt.key` and `dev/jwt.pub` with openssl
  - `dev-token` → `go run ./cmd/devtoken`
- [X] T007 [P] Create `.github/workflows/ci.yml` running on push and pull_request:
  - Set up Go from `go.mod`.
  - `gofmt -l` check (fail if any output), `go vet`, `golangci-lint`.
  - `go test -race ./...`, then `go test -race -tags=integration ./...` (Docker is available on `ubuntu-latest`).
  - Migration apply-from-scratch: `migrate up` against a `postgres:18` service container.
  - `govulncheck ./...`.
- [X] T008 [P] Create `cmd/devtoken/main.go`. It reads `dev/jwt.key` (PEM ECDSA) and prints an ES256 JWT to stdout with claims:
  - `iss` = `$AUTH_JWT_ISSUER` (default `dev`)
  - `aud` = `$AUTH_JWT_AUDIENCE` (default `lending-api`)
  - `sub` = flag `-sub` (default `dev-user`)
  - `scope` = flag `-scope` (default `loan-applications:submit`)
  - `exp` = now + 1h
- [X] T009 [P] Create `dev/examples/married.json` and `dev/examples/single.json` with the `MarriedCustomer` and `SingleCustomer` example bodies from `api/openapi.yaml` (`components.examples`)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Configuration, HTTP plumbing, auth, money, DB, and test harness used by every story

**⚠️ CRITICAL**: No user story work can begin until this phase is complete. Within each pair,
write the test first and see it fail.

- [X] T010 [P] Write table-driven tests in `internal/config/config_test.go` for `config.Load(getenv func(string) string) (Config, error)`:
  - Required, missing → error naming the variable: `DATABASE_URL`, `APP_CURRENCY`, `APP_TIMEZONE`, `AUTH_JWT_ISSUER`, `AUTH_JWT_AUDIENCE`, `AUTH_JWT_PUBLIC_KEY`.
  - `APP_CURRENCY` must match `^[A-Z]{3}$`.
  - `APP_TIMEZONE` must load via `time.LoadLocation`.
  - `AUTH_JWT_PUBLIC_KEY` must be a PEM-encoded ECDSA or RSA public key.
  - Defaults: `HTTP_ADDR=:8080`, `ERROR_TYPE_BASE_URL=https://errors.lending.example/`, `SHUTDOWN_TIMEOUT=20s`, `DB_MAX_CONNS=10`.
- [X] T011 Implement `internal/config/config.go` so T010 passes. Collect all errors, not just the first, so startup fails fast with one complete message.
- [X] T012 [P] Write table-driven tests in `internal/money/amount_test.go` for `money.ParseAmount(n json.Number) (Amount, error)`, where `Amount` is `int64` minor units:
  - Accepts: `"15000"` → 1500000, `"9500000.50"` → 950000050, `"0"`, `"0.5"` → 50.
  - Rejects: more than 2 decimals (`"1.005"` → `ErrTooManyDecimals`), exponent notation (`"1e3"`), anything that overflows int64, non-numeric input.
  - Also tests `Amount.IsWhole()`, `Amount.IsMultipleOf(units int64)` (major units), and `Amount.String()` rendering `"15000.00"` for `NUMERIC` parameters.
- [X] T013 Implement `internal/money/amount.go` so T012 passes using only string and integer arithmetic (no `float64`, no `strconv.ParseFloat`).
- [X] T014 [P] Write tests in `internal/httpx/timestamp_test.go`:
  - `httpx.Timestamp` marshals using layout `2006-01-02T15:04:05-07:00` in a configured `*time.Location`: a UTC instant shows `+07:00` for `Asia/Jakarta`, and UTC itself is `+00:00`, never `Z`. Nanoseconds are truncated.
  - `Timestamp` unmarshal accepts only that layout; rejects `Z`, fractional seconds, and a missing offset.
  - `httpx.Date` marshals and unmarshals strictly `2006-01-02`, and rejects `2001-02-30`.
- [X] T015 Implement `internal/httpx/timestamp.go`:
  - `Timestamp` with `SetDisplayLocation(*time.Location)` called once at startup, defaulting to UTC.
  - `Date` type with a `Time()` accessor.
  - Make T014 pass.
- [X] T016 [P] Write tests in `internal/httpx/problem_test.go` and `internal/httpx/decode_test.go`:
  - `WriteProblem(w, r, Problem)` writes `Content-Type: application/problem+json` with fields `type`, `title`, `status`, `detail`, `instance` (= request path) and optional `errors`.
  - `DecodeJSON(r, dst, maxBytes)` uses `DisallowUnknownFields()` and `UseNumber()` and a 64 KiB limit via `http.MaxBytesReader`, and rejects trailing data after the JSON value.
  - `DecodeJSON` returns a typed `*MalformedError` describing an unknown field, a wrong type at a JSON path, a body too large, or invalid JSON.
- [X] T017 Implement `internal/httpx/problem.go` (with `ProblemTypeBase` set from config) and `internal/httpx/decode.go` so T016 passes.
- [X] T018 [P] Write tests in `internal/httpx/middleware_test.go`:
  - `RequestID` keeps a valid incoming `X-Request-ID` (≤128 chars) or generates a UUIDv7, stores it in context, and echoes it in the response header.
  - `Logging` emits one slog JSON line per request with `request_id`, `method`, `route`, `status`, `duration_ms`, and never logs bodies.
  - `Recover` turns a panic into a 500 `internal-error` problem.
  - `Metrics` records `http_request_duration_seconds` (histogram) and `http_requests_total` labeled by `route` and `code` class.
- [X] T019 Implement `internal/httpx/middleware.go` and `internal/httpx/log.go`. The latter provides a `LoggerFrom(ctx)` helper that returns the slog logger carrying `request_id`. Make T018 pass.
- [X] T020 [P] Write tests in `internal/auth/auth_test.go` for the `auth.Middleware(verifier, requiredScope)`. Tokens are signed in the test with a generated ECDSA key.
  - Missing or malformed `Authorization: Bearer` → 401 `unauthenticated`. So are a bad signature, an expired token, and a wrong `iss` or `aud`.
  - A valid token without scope `loan-applications:submit` (space-separated `scope` claim) → 403 `forbidden`.
  - A valid token → the next handler runs, and `auth.ActorFrom(ctx)` returns `sub`.
  - An empty `sub` → 401.
- [X] T021 Implement `internal/auth/jwt.go` (`golang-jwt/jwt/v5`, ES256/RS256 allowed, `iss`/`aud`/`exp` checked, 30 s leeway) and `internal/auth/context.go` (unexported context key; `WithActor`, `ActorFrom`) so T020 passes.
- [X] T022 Write table-driven tests in `internal/db/pool_test.go` for `IsUnavailable` (SQLSTATE `08006`, `53300`, `57P01` → true; `23505` → false; `context.DeadlineExceeded` and `*pgconn.ConnectError` → true; nil → false), then implement `internal/db/pool.go`:
  - `Open(ctx, cfg) (*pgxpool.Pool, error)` with `MaxConns` from config, and `Ping(ctx)` with a 1 s timeout.
  - `IsUnavailable(err) bool`: true for connection errors, `context.DeadlineExceeded`, and SQLSTATE classes `08`, `53`, `57P01`. Used to produce the 503 response.
- [X] T023 Create `migrations/embed.go` (`package migrations`, `//go:embed *.sql`, `var FS embed.FS`) and `internal/db/migrate.go` with `Up(databaseURL string) error` using `golang-migrate` with the `iofs` source over `migrations.FS`.
- [X] T024 [P] Create `tests/testutil/postgres.go`:
  - `StartPostgres(t) *pgxpool.Pool`: testcontainers-go postgres module, image `postgres:18`; runs `db.Up`; stops the container in `t.Cleanup`.
  - `TruncateAll(t, pool)`.
- [X] T025 [P] Create `tests/testutil/jwt.go` (`NewSigner(t)` generates an ECDSA key and returns the PEM public key, plus `Token(sub, scope string) string`) and `tests/testutil/clock.go` (a fixed `func() time.Time` for tests)
- [X] T026 [P] Write tests in `internal/server/server_test.go` for health and metrics:
  - `GET /healthz` → 200 `{"status":"ok"}`.
  - `GET /readyz` → 200 when the pinger succeeds and 503 problem `temporarily-unavailable` when it fails.
  - `GET /metrics` serves Prometheus text, including pgxpool stats `db_pool_acquired_conns`, `db_pool_idle_conns`, `db_pool_total_conns`, `db_pool_max_conns`, `db_pool_acquire_count_total`, `db_pool_acquire_duration_seconds_total` (constitution Principle V).
  - Unknown route → 404 problem; wrong method → 405 problem.
- [X] T027 Implement `internal/server/server.go`:
  - `New(deps Deps) http.Handler` builds the `http.ServeMux` with `/healthz`, `/readyz`, and `/metrics` (promhttp on a dedicated registry, with a custom collector reading `pool.Stat()` for the pgxpool metrics).
  - Wraps everything in `Recover → RequestID → Logging → Metrics`.
  - Exposes `deps.Routes []func(*http.ServeMux)` for feature registration.
  - Make T026 pass.
- [X] T028 Implement `cmd/api/main.go`:
  - `config.Load(os.Getenv)` and exit 1 with the error on failure.
  - Set up the slog JSON handler, call `httpx.SetDisplayLocation`, open the pool, run `db.Up`, and build `server.New`.
  - `http.Server` with `ReadHeaderTimeout: 5s`, `ReadTimeout: 10s`, `WriteTimeout: 15s`, `IdleTimeout: 60s`.
  - On `SIGTERM`/`SIGINT`, call `srv.Shutdown` within `SHUTDOWN_TIMEOUT`, then `pool.Close()`.

**Checkpoint**: `make lint test` passes. `go run ./cmd/api` starts against local PostgreSQL, and `/readyz` returns 200.

---

## Phase 3: User Story 1 - Submit a valid car-backed loan application (Priority: P1) 🎯 MVP

**Goal**: A valid application (with or without a spouse) is stored atomically with status
`IN_PROGRESS`. The response is `201` with `{id, status, currency, submittedAt}` and a `Location`
header.

**Independent Test**: POST `dev/examples/married.json` and `dev/examples/single.json` with a dev
token. Each returns 201 with matching DB rows, as in quickstart.md scenarios 1–2 and section 3.

### Tests for User Story 1 ⚠️ write first, confirm they FAIL

- [X] T029 [P] [US1] Write unit tests in `internal/loanapplication/mapping_test.go` for `toApplication(req SubmitRequest) (Application, error)`:
  - Text fields are trimmed.
  - `licensePlate` is normalized to uppercase with spaces and hyphens removed (`"b 1234-xyz"` → `"B1234XYZ"`).
  - `monthlyIncome`/`amount` are parsed via `money.ParseAmount` from `json.Number`.
  - The spouse is mapped only when present.
  - `birthDate` is kept as a date.
- [X] T030 [P] [US1] Write integration tests in `tests/integration/loanapplication_repository_test.go` using `testutil.StartPostgres`:
  - (a) `Repository.Create` in a transaction persists every `loan_applications` column with `status='IN_PROGRESS'`, `currency` from the argument, `created_by = updated_by = actor`, `deleted=false`, and `deleted_at IS NULL`.
  - (b) For a married customer, exactly one `loan_application_spouses` row exists with the same audit values.
  - (c) Exactly one `loan_application_status_history` row exists with `from_status IS NULL`, `to_status='IN_PROGRESS'`, and `created_by=actor`.
  - (d) Atomicity: a spouse insert forced to fail (e.g. `full_name` of 101 chars bypassing validation) leaves 0 rows in all three tables.
  - (e) The DB `CHECK` constraints reject `loan_amount = 150.00` and `loan_tenure_months = 49`.
- [X] T031 [P] [US1] Write contract tests in `tests/contract/submit_success_test.go`:
  - Set up `httptest.NewServer(server.New(...))` with real Postgres (`testutil`), the `testutil` JWT signer, and a fixed clock.
  - POST each of `dev/examples/married.json` and `dev/examples/single.json` with header `Idempotency-Key: <uuid>`.
  - Check the request and response with `kin-openapi` `openapi3filter.ValidateRequest`/`ValidateResponse` against `api/openapi.yaml`.
  - Assert status 201, a `Location` of `/v1/loan-applications/{id}`, `status=="IN_PROGRESS"`, `currency==APP_CURRENCY`, and `submittedAt` matching `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2}$` with the `APP_TIMEZONE` offset.
  - Boundary cases also return 201: `amount: 100`, `tenureMonths: 3`, and `tenureMonths: 48`.

### Implementation for User Story 1

- [X] T032 [US1] Create `migrations/000001_create_loan_applications.up.sql` and `.down.sql`. The down script uses `DROP TABLE` in reverse order. Every column below is followed by the six audit columns and the `deleted`/`deleted_at` CHECK from Conventions.
  - `loan_applications`:
    - `id UUID PRIMARY KEY DEFAULT uuidv7()`
    - `status TEXT NOT NULL CHECK (status IN ('IN_PROGRESS'))`
    - `currency CHAR(3) NOT NULL`
    - `loan_amount NUMERIC(19,2) NOT NULL CHECK (loan_amount > 0 AND loan_amount % 100 = 0)`
    - `loan_tenure_months SMALLINT NOT NULL CHECK (loan_tenure_months BETWEEN 3 AND 48)`
    - `customer_full_name TEXT NOT NULL CHECK (char_length(customer_full_name) BETWEEN 1 AND 100)`
    - `customer_id_number TEXT NOT NULL CHECK (customer_id_number ~ '^[A-Za-z0-9]{1,30}$')`
    - `customer_birth_date DATE NOT NULL`
    - `customer_phone_number TEXT NOT NULL CHECK (customer_phone_number ~ '^\+[0-9]{8,15}$')`
    - `customer_email TEXT NOT NULL CHECK (char_length(customer_email) <= 254)`
    - `customer_monthly_income NUMERIC(19,2) NOT NULL CHECK (customer_monthly_income > 0)`
    - `customer_marital_status TEXT NOT NULL CHECK (customer_marital_status IN ('SINGLE','MARRIED','DIVORCED','WIDOWED'))`
    - `customer_address_street TEXT NOT NULL` with length 1–200
    - `customer_address_city TEXT NOT NULL` with length 1–100
    - `customer_address_zip_code TEXT NOT NULL CHECK (customer_address_zip_code ~ '^[A-Za-z0-9 -]{3,10}$')`
    - `collateral_brand TEXT NOT NULL` with length 1–50
    - `collateral_model TEXT NOT NULL` with length 1–50
    - `collateral_manufacturing_year SMALLINT NOT NULL CHECK (collateral_manufacturing_year >= 1900)`
    - `collateral_license_plate TEXT NOT NULL CHECK (collateral_license_plate ~ '^[A-Z0-9]{1,15}$')`
    - `submitted_at TIMESTAMPTZ NOT NULL DEFAULT now()`
  - Partial unique indexes on `loan_applications`:
    - `uq_loan_applications_open_customer` on `(customer_id_number) WHERE status = 'IN_PROGRESS' AND NOT deleted`
    - `uq_loan_applications_open_collateral` on `(collateral_license_plate) WHERE status = 'IN_PROGRESS' AND NOT deleted`
  - `loan_application_spouses`:
    - `loan_application_id UUID PRIMARY KEY REFERENCES loan_applications(id)`
    - `full_name TEXT NOT NULL` with length 1–100
    - `id_number TEXT NOT NULL CHECK (id_number ~ '^[A-Za-z0-9]{1,30}$')`
    - `phone_number TEXT NOT NULL CHECK (phone_number ~ '^\+[0-9]{8,15}$')`
    - `monthly_income NUMERIC(19,2) NOT NULL CHECK (monthly_income >= 0)`
  - `loan_application_status_history`:
    - `id UUID PRIMARY KEY DEFAULT uuidv7()`
    - `loan_application_id UUID NOT NULL REFERENCES loan_applications(id)`, with an index
    - `from_status TEXT NULL`
    - `to_status TEXT NOT NULL`
- [X] T033 [US1] Write `internal/db/queries/loan_applications.sql` with sqlc queries `InsertLoanApplication :one` (RETURNING `id, submitted_at`), `InsertLoanApplicationSpouse :exec`, and `InsertStatusHistory :exec`. All set `created_by`/`updated_by` from a parameter. Run `make sqlc` and commit `internal/db/sqlc/`.
- [X] T034 [P] [US1] Create `internal/loanapplication/types.go`:
  - `SubmitRequest`, with nested `CustomerRequest`, `AddressRequest`, `SpouseRequest` (pointer, optional), `CollateralRequest`, `LoanRequest`.
    - JSON tags exactly as `api/openapi.yaml` (camelCase).
    - Money fields and `manufacturingYear`/`tenureMonths`/`amount` are `json.Number`; `birthDate` is `httpx.Date`.
  - `MaritalStatus` constants `SINGLE`, `MARRIED`, `DIVORCED`, `WIDOWED`.
  - Validated domain `Application`, with `money.Amount` fields and `*Spouse`.
  - `SubmitResult{ID uuid.UUID; Status string; Currency string; SubmittedAt httpx.Timestamp}`, with JSON tags `id`, `status`, `currency`, `submittedAt`.
  - `FieldError{Field, Code, Message string}`.
- [X] T035 [US1] Implement `internal/loanapplication/mapping.go` (`toApplication`; trimming; plate normalization `strings.ToUpper` and removal of `' '`/`'-'`) so T029 passes
- [X] T036 [US1] Implement `internal/loanapplication/repository.go`:
  - `Repository` with `Create(ctx, tx pgx.Tx, app Application, currency, actor string) (uuid.UUID, time.Time, error)`.
  - Uses sqlc `InsertLoanApplication`, then `InsertLoanApplicationSpouse` if `app.Spouse != nil`, then `InsertStatusHistory(nil → "IN_PROGRESS")`.
  - Converts `money.Amount` to `pgtype.Numeric` via `Amount.String()`.
  - Make T030 pass.
- [X] T037 [US1] Implement `internal/loanapplication/service.go`:
  - `Service{pool, repo, currency, now func() time.Time}` with `Submit(ctx, req SubmitRequest, actor string) (SubmitResult, error)`.
  - Maps the request, begins a `pgx` transaction (`pgx.BeginFunc`), calls `repo.Create`, commits, and returns the result.
  - Increments the Prometheus counter `loan_applications_submitted_total`.
- [X] T038 [US1] Implement `internal/loanapplication/handler.go`:
  - `Handler.Register(mux)` adds `POST /v1/loan-applications` wrapped by `auth.Middleware(verifier, "loan-applications:submit")`.
  - Decodes with `httpx.DecodeJSON(r, &req, 64<<10)`; a `*MalformedError` → 400 `malformed-request`.
  - Reads the actor via `auth.ActorFrom`, calls `Service.Submit`, then writes 201 with `Location: /v1/loan-applications/{id}` and the `SubmitResult` JSON.
  - Any other error → 500 `internal-error`, logged without request values.
- [X] T039 [US1] Wire the loan application `Handler` into `server.Deps.Routes` in `cmd/api/main.go`, and into the contract-test server setup helper `tests/contract/setup_test.go`. The helper builds the full handler with real Postgres, the JWT signer, and a fixed clock, and is reused by all contract tests. Run T030 and T031 until green.

**Checkpoint**: Quickstart scenarios 1–2 and section 3 pass. US1 is a demoable MVP for internal use only. It is NOT releasable to real clients until US3 (idempotency, constitution Principle I) is done. The DB CHECK constraints guard against bad data, and friendly validation errors arrive in US2.

---

## Phase 4: User Story 2 - Reject invalid applications with clear, complete feedback (Priority: P2)

**Goal**: Every rule in spec FR-007 to FR-041 is enforced before storage, and all field errors
are returned together:
- `400 validation-failed`, with `errors[]` of `{field (JSON Pointer), code, message}`.
- Open-application conflicts return `409` (`customer-has-open-application` / `collateral-in-use`).
- Database outages return `503 temporarily-unavailable` with `Retry-After`.

**Independent Test**: Quickstart scenarios 5–12 return the documented status, `type`, and
`errors[]`, and no rows are stored for any of them.

### Tests for User Story 2 ⚠️ write first, confirm they FAIL

- [X] T040 [P] [US2] Write table-driven unit tests in `internal/loanapplication/validate_test.go` for `Validate(req SubmitRequest, today time.Time /* date in APP_TIMEZONE */) []FieldError`. There is one case per rule, asserting the exact `field` and `code`, plus one multi-error case asserting all errors are returned together.

  Customer fields:

  | Field | Rule | Code(s) |
  |-------|------|---------|
  | `/customer/fullName` | required; 1–100 chars after trim | `required` / `too_long` |
  | `/customer/idNumber` | `^[A-Za-z0-9]{1,30}$` | `invalid_format` |
  | `/customer/address/street` | 1–200 | |
  | `/customer/address/city` | 1–100 | |
  | `/customer/address/zipCode` | `^[A-Za-z0-9 -]{3,10}$` | |
  | `/customer/birthDate` | not in future; age ≥ 18 on `today` (born exactly 18 years ago today → valid; 18 years minus 1 day → invalid) | `in_future` / `under_minimum_age` |
  | `/customer/phoneNumber` | `^\+[0-9]{8,15}$` | |
  | `/customer/email` | `net/mail.ParseAddress` result equals input and length ≤ 254 | `invalid_format` / `too_long` |
  | `/customer/monthlyIncome` | > 0 and ≤ 2 decimals | `not_positive` / `too_many_decimals` |
  | `/customer/maritalStatus` | one of `SINGLE`, `MARRIED`, `DIVORCED`, `WIDOWED` | `invalid_value` |

  Spouse fields:

  | Field | Rule | Code(s) |
  |-------|------|---------|
  | `/customer/spouse` | required iff `MARRIED` | `spouse_required` / `spouse_not_allowed` |
  | `/customer/spouse/fullName` | 1–100 | |
  | `/customer/spouse/idNumber` | same regex; differs from the customer's | `same_as_customer` |
  | `/customer/spouse/phoneNumber` | same regex as the customer's | |
  | `/customer/spouse/monthlyIncome` | ≥ 0 and ≤ 2 decimals | `negative` |

  Collateral and loan fields:

  | Field | Rule | Code(s) |
  |-------|------|---------|
  | `/collateral/brand` | 1–50 | |
  | `/collateral/model` | 1–50 | |
  | `/collateral/manufacturingYear` | integer 1900 through `today.Year()+1` | `out_of_range` |
  | `/collateral/licensePlate` | `^[A-Za-z0-9 -]{1,15}$` and at least one letter or digit (`"- -"` → `invalid_format`) | |
  | `/loan/amount` | whole number, > 0, multiple of 100 (`15050` and `1000.50` → `not_multiple_of_100`, `0` → `not_positive`) | |
  | `/loan/tenureMonths` | integer 3–48 (`2`, `49`, `12.5` → `out_of_range`) | |
- [X] T041 [P] [US2] Write contract tests in `tests/contract/submit_errors_test.go`. Validate each *response* (not the request, which is deliberately invalid) against `api/openapi.yaml`. Assert the `type` suffix and that `loan_applications` row count is unchanged:
  - (a) Quickstart #7 → 400 `validation-failed` with exactly 4 `errors` entries.
  - (b) `MARRIED` without spouse → `spouse_required`.
  - (c) `SINGLE` with spouse → `spouse_not_allowed`.
  - (d) Unknown root field `foo` → 400 `malformed-request`.
  - (e) Non-JSON body → 400 `malformed-request`.
  - (f) No token → 401 `unauthenticated`.
  - (g) Token without scope → 403 `forbidden`.
  - (h) Second application for the same `idNumber` → 409 `customer-has-open-application`.
  - (i) Different customer, plate `"b1234xyz"` matching an open `"B 1234 XYZ"` → 409 `collateral-in-use`.
  - (j) A database outage (the pool closed before the request) → 503 `temporarily-unavailable` with a `Retry-After` header.
- [X] T042 [P] [US2] Write integration tests in `tests/integration/open_application_test.go`:
  - (a) Launch 20 goroutines submitting via `Service.Submit` for the same `customer_id_number` with different plates: exactly 1 succeeds, 19 return `ErrCustomerHasOpenApplication`, and 1 row exists.
  - (b) The same test for one plate across 20 different customers → `ErrCollateralInUse`.
  - (c) After setting the existing row to `deleted=true, deleted_at=now()`, a new submission for that customer succeeds.

### Implementation for User Story 2

- [X] T043 [US2] Implement `internal/loanapplication/validate.go` so T040 passes:
  - Collect all errors without stopping early.
  - Precompile regexes as package-level `regexp.MustCompile`.
  - Field paths are JSON Pointers.
  - Messages come from the code table in `validate.go`.
- [X] T044 [US2] Update `internal/loanapplication/service.go`:
  - `Submit` calls `Validate(req, s.now().In(loc))` before opening the transaction.
  - On any error, return `&ValidationError{Errors []FieldError}` without touching the DB.
  - Define `ValidationError`, `ErrCustomerHasOpenApplication`, and `ErrCollateralInUse` in `internal/loanapplication/errors.go`.
- [X] T045 [US2] Update `internal/loanapplication/repository.go`:
  - Map `*pgconn.PgError` with `Code == "23505"`: `ConstraintName == "uq_loan_applications_open_customer"` → `ErrCustomerHasOpenApplication`, and `"uq_loan_applications_open_collateral"` → `ErrCollateralInUse`.
  - Wrap other errors with `fmt.Errorf("create loan application: %w", err)`.
  - Make T042 pass.
- [X] T046 [US2] Update `internal/loanapplication/handler.go` error mapping:
  - `*ValidationError` → 400 `validation-failed` with `detail: "N fields are invalid."` and `errors`.
  - `ErrCustomerHasOpenApplication` → 409 `customer-has-open-application`.
  - `ErrCollateralInUse` → 409 `collateral-in-use`.
  - The problem `detail` must never include the other application's ID or any personal data.
  - `db.IsUnavailable(err)` → 503 `temporarily-unavailable` with `Retry-After: 5`.
  - Make T041 pass.
- [X] T047 [US2] Add a WARN log in `internal/loanapplication/handler.go` for rejected submissions, with attributes `reason` (problem type suffix) and `fields` (list of `field:code` pairs only). Values are never logged (FR-050).

**Checkpoint**: Quickstart scenarios 1–2 and 5–12 pass. US1 tests are still green.

---

## Phase 5: User Story 3 - Safe retry without duplicate applications (Priority: P3)

**Goal**: The `Idempotency-Key` header is required:
- Repeating a successful request with the same key and body replays the original 201.
- The same key with a different body returns `422 idempotency-key-reused`.
- Concurrent duplicates create exactly one application.

**Independent Test**: Quickstart scenarios 3, 4, and 13. The concurrent same-key test leaves one
row and identical responses.

### Tests for User Story 3 ⚠️ write first, confirm they FAIL

- [X] T048 [P] [US3] Write unit tests in `internal/loanapplication/idempotency_test.go`:
  - `RequestHash(raw []byte) ([]byte, error)` returns SHA-256 of `json.Compact(raw)`, so whitespace-only differences give equal hashes and a changed value gives a different hash.
  - `ValidIdempotencyKey(k string) bool` accepts 1–255 chars matching `^[\x21-\x7E]+$` and rejects empty strings, 256 chars, and strings containing a space or non-ASCII.
- [X] T049 [P] [US3] Write integration tests in `tests/integration/idempotency_test.go`:
  - (a) Same actor, key, and body twice → same `SubmitResult.ID`, 1 `loan_applications` row, 1 `idempotency_keys` row with `response_status=201` and `response_body` containing only `id`, `status`, `currency`, `submittedAt`.
  - (b) Same key, different body → `ErrIdempotencyKeyReused`, still 1 row.
  - (c) The same key used by a different actor is an independent request.
  - (d) 20 goroutines with the same actor, key, and body → all 20 return the identical ID; 1 application row and 1 key row.
  - (e) A request that fails validation stores no key row, and a corrected retry with the same key succeeds.
  - (f) `expires_at = created_at + 24h`.
- [X] T050 [P] [US3] Write contract tests in `tests/contract/idempotency_test.go`:
  - Missing `Idempotency-Key` → 400 `invalid-idempotency-key`, and a key of 256 chars likewise.
  - A replay returns 201 with an identical body and `Location`.
  - A changed `loan.amount` under the same key → 422 `idempotency-key-reused`.
  - All responses validate against `api/openapi.yaml`. Requests are validated only on the success path.

### Implementation for User Story 3

- [X] T051 [US3] Create `migrations/000002_create_idempotency_keys.up.sql` and `.down.sql`, followed by the six audit columns and the `deleted`/`deleted_at` CHECK:
  - Table `idempotency_keys`:
    - `id UUID PRIMARY KEY DEFAULT uuidv7()`
    - `actor TEXT NOT NULL`
    - `idempotency_key TEXT NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 255)`
    - `request_hash BYTEA NOT NULL`
    - `response_status SMALLINT NULL`
    - `response_body JSONB NULL`
    - `CHECK ((response_status IS NULL) = (response_body IS NULL))`
    - `loan_application_id UUID NULL REFERENCES loan_applications(id)`
    - `expires_at TIMESTAMPTZ NOT NULL`
  - Unique index `uq_idempotency_keys_actor_key` on `(actor, idempotency_key) WHERE NOT deleted`.
- [X] T052 [US3] Write `internal/db/queries/idempotency_keys.sql`:
  - `InsertIdempotencyKey :one`: `INSERT ... ON CONFLICT (actor, idempotency_key) WHERE NOT deleted DO NOTHING RETURNING id`; no row means the key already exists.
  - `GetIdempotencyKey :one`: select by actor and key `WHERE NOT deleted`.
  - `CompleteIdempotencyKey :exec`: set `response_status`, `response_body`, `loan_application_id`, `updated_at = now()`, `updated_by`.
  - Then run `make sqlc`.
- [X] T053 [US3] Implement `internal/loanapplication/idempotency.go`: `RequestHash`, `ValidIdempotencyKey`, `ErrIdempotencyKeyReused` (in `errors.go`), and a repository method `ClaimKey`/`CompleteKey`/`GetKey` in `repository.go`. Make T048 pass.
- [X] T054 [US3] Update `Service.Submit` in `internal/loanapplication/service.go` to `Submit(ctx, req, rawBody []byte, key, actor string) (SubmitResult, replayed bool, error)`:
  1. Validate (US2); on failure return without touching the DB.
  2. Begin a transaction and call `ClaimKey` with `expires_at = now + 24h`.
  3. If no row is returned: roll back, `GetKey` outside the transaction, and compare `request_hash`. If equal, return the stored `response_body` as the result with `replayed=true`; otherwise return `ErrIdempotencyKeyReused`.
  4. If claimed: `repo.Create`, `CompleteKey` with status 201 and the marshaled `SubmitResult`, then commit.

  A concurrent duplicate blocks on the unique index until the first transaction commits, then gets no row, which leads to the replay. Make T049 pass.
- [X] T055 [US3] Update `internal/loanapplication/handler.go`:
  - Read the raw body once (`io.ReadAll` over `http.MaxBytesReader`, 64 KiB), then decode from those bytes.
  - Validate the `Idempotency-Key` header before decoding; invalid → 400 `invalid-idempotency-key`.
  - Pass the raw body and key to `Submit`.
  - `ErrIdempotencyKeyReused` → 422 `idempotency-key-reused`.
  - A replay writes the stored status and body with the same `Location`.
  - Update `tests/contract/submit_success_test.go` and `submit_errors_test.go` helpers to always send a fresh key. Make T050 pass.

**Checkpoint**: All quickstart scenarios 1–13 pass. All three stories are green together.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T056 [P] Write `tests/integration/logging_pii_test.go` (SC-006 / FR-050):
  - Capture slog output into a buffer through the full handler.
  - Submit valid, invalid, and conflicting applications using fixture values.
  - Assert that none of these values appear in the logs: customer and spouse full names, ID numbers, phone numbers, email, street, zip code, birth date, incomes, license plate.
- [X] T057 [P] Write a load check in `tests/integration/submit_load_test.go` (build tag `integration && load`): 500 sequential plus 50-concurrent valid submissions with unique customers and plates; assert p95 latency < 300 ms and 0 errors (plan Performance Goals; SC-003)
- [X] T058 [P] Create `README.md`: purpose, prerequisites, the storage-level encryption-at-rest deployment requirement (research R13), environment variables (from `internal/config`), `make` targets, how to run locally (link to `specs/001-loan-submission/quickstart.md`), and the API contract location `api/openapi.yaml`
- [X] T059 Run `make lint vuln` and fix all findings. Verify that `grep -rnE "float(32|64)" internal/money internal/loanapplication` returns no money-related usage (Principle III).
- [X] T060 Execute every scenario in `specs/001-loan-submission/quickstart.md` (sections 1–4) against a locally running service and record any deviations as follow-up tasks

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies.
- **Foundational (Phase 2)**: depends on Setup, and blocks all user stories.
- **US1 (Phase 3)**: depends on Foundational.
- **US2 (Phase 4)**: depends on US1. It extends `service.go`, `repository.go`, and `handler.go`, and its conflict tests need the migration and Create path from US1.
- **US3 (Phase 5)**: depends on US1. It depends on US2 only for "validate before claiming the key" (T054 step 1). If US3 is built before US2, skip that step and add it when US2 lands.
- **Polish (Phase 6)**: depends on all stories.

### Within Each Phase

- Tests come before the matching implementation and must fail first (Principle II).
- Pairs: T010→T011, T012→T013, T014→T015, T016→T017, T018→T019, T020→T021, T026→T027.
- Order within the phases:
  - Migration → sqlc queries → types → mapping → repository → service → handler → wiring.
  - US1: T032 → T033 → T034/T035 → T036 → T037 → T038 → T039.
  - US3: T051 → T052 → T053 → T054 → T055.
- `service.go` and `handler.go` are edited in US1, US2, and US3, so tasks touching the same file are never marked [P].

### Parallel Opportunities

- **Setup**: T002 to T009 all [P] once T001 has created the module.
- **Foundational**: all test tasks (T010, T012, T014, T016, T018, T020, T024, T025, T026) can be written in parallel, and the implementations after them in matching pairs. T022 and T023 are independent of the httpx and auth work.
- **US1**: T029, T030, and T031 in parallel; T034 in parallel with T032 and T033.
- **US2**: T040, T041, and T042 in parallel.
- **US3**: T048, T049, and T050 in parallel.
- **Polish**: T056, T057, and T058 in parallel.

---

## Parallel Example: User Story 1

```bash
# Write all US1 tests together (they must fail first):
Task: "Unit tests for toApplication in internal/loanapplication/mapping_test.go"
Task: "Repository integration tests in tests/integration/loanapplication_repository_test.go"
Task: "Contract tests for 201 path in tests/contract/submit_success_test.go"

# While the migration and sqlc queries are being written:
Task: "Domain/request types in internal/loanapplication/types.go"
```

## Parallel Example: User Story 2

```bash
Task: "Validation rule table tests in internal/loanapplication/validate_test.go"
Task: "Error contract tests in tests/contract/submit_errors_test.go"
Task: "Concurrent open-application tests in tests/integration/open_application_test.go"
```

## Parallel Example: User Story 3

```bash
Task: "RequestHash/ValidIdempotencyKey unit tests in internal/loanapplication/idempotency_test.go"
Task: "Idempotency integration tests in tests/integration/idempotency_test.go"
Task: "Idempotency contract tests in tests/contract/idempotency_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 Setup → Phase 2 Foundational. Checkpoint: `/readyz` returns 200.
2. Phase 3 US1. Checkpoint: quickstart scenarios 1–2 pass.
3. **Stop and validate**: run `make test test-integration`; demo submitting both examples.

### Incremental Delivery

1. **US1**: valid applications are stored, and the DB constraints block bad data.
2. **US2**: full validation feedback and the open-application conflicts.
3. **US3**: safe retries. Required by constitution Principle I; this is the first releasable increment.
4. **Polish**: personal-data log audit, load check, docs, quickstart run.

---

## Notes

- Tests that need Docker live in `tests/` behind `//go:build integration`, so `go test ./...` stays Docker-free.
- Commit after each task or each test/implementation pair. Each checkpoint is a safe stopping point.
- Spec, plan, and contract are the authority. If a task conflicts with `api/openapi.yaml`, fix the task or contract explicitly; don't let them drift.
