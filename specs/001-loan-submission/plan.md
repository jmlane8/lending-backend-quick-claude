# Implementation Plan: Loan Application Submission

**Branch**: `001-loan-submission` | **Date**: 2026-10-09 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-loan-submission/spec.md`

## Summary

Provide `POST /v1/loan-applications`, which accepts a customer (with spouse when married), one car
as collateral, and a proposed loan (amount a positive multiple of 100, tenure 3–48 months).

- **Validation**: every field is checked, and all errors are returned together.
- **Business rules**: one `IN_PROGRESS` application per customer ID number, and one per license
  plate.
- **Storage**: the application is stored atomically with status `IN_PROGRESS`.
- **Retries**: an `Idempotency-Key` header guarantees at-most-once creation.

Technical approach:
- **Language and routing**: Go with the standard `net/http` mux.
- **Database**: PostgreSQL 18 through `pgx` and `sqlc`.
- **Open-application rules**: partial unique indexes, which stay correct under concurrent
  submissions.
- **Idempotency**: a key table written in the same transaction as the application.
- **Errors**: RFC 9457 problem responses.

This is the first feature, so it also delivers the service skeleton: configuration, auth,
logging, metrics, health checks, and migrations.

## Technical Context

**Language/Version**: Go, latest stable (1.26+; `go.mod` is the authority)

**Primary Dependencies**:
- `jackc/pgx/v5`, with `sqlc` for code generation
- `golang-migrate/migrate/v4`
- `golang-jwt/jwt/v5`
- `prometheus/client_golang`
- `google/uuid` (request IDs)
- Test only: `testcontainers-go`, `getkin/kin-openapi`

**Storage**: PostgreSQL 18 (`TIMESTAMPTZ` in UTC, `NUMERIC(19,2)` money, `uuidv7()` keys)

**Testing**: `go test -race`; table-driven unit tests; integration and contract tests against
real PostgreSQL via Testcontainers (`integration` build tag)

**Target Platform**: Linux container, single static binary

**Project Type**: web-service (REST API)

**Performance Goals**: submit p95 < 300 ms at expected load (SC-003 requires < 1 s); at least
50 submissions/s on a single instance

**Constraints**:
- Request body ≤ 64 KiB.
- No personal data in logs.
- Graceful shutdown within 20 s.
- Timestamps in `2006-01-02T15:04:05-07:00` format, rendered in `APP_TIMEZONE`.

**Scale/Scope**: one endpoint, four tables. Expected volume is on the order of thousands of
applications per day.

All unknowns are resolved in [research.md](./research.md). Nothing is marked NEEDS
CLARIFICATION.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle / Rule | How this plan complies | Pre | Post |
|------------------|------------------------|-----|------|
| I. OpenAPI before code | `contracts/openapi.yaml` written in Phase 1; copied to `api/openapi.yaml` as the source of truth | ✅ | ✅ |
| I. REST, problem+json, `/v1` | `POST /v1/loan-applications` → 201 + `Location`; all errors RFC 9457 (research R8) | ✅ | ✅ |
| I. Pagination | No list endpoint in this feature | n/a | n/a |
| I. Idempotency-Key | Required header, stored atomically (research R5) | ✅ | ✅ |
| I. Timestamp format | `httpx.Timestamp` type; non-conforming input → 400 (research R9) | ✅ | ✅ |
| II. Test-first | Tasks will order tests before implementation; unit tests, Testcontainers integration tests, kin-openapi contract tests; `-race` | ✅ | ✅ |
| III. No float money; currency | `json.Number` → `int64` minor units → `NUMERIC(19,2)`; `currency` column (research R4) | ✅ | ✅ |
| III. Single transaction | Application, spouse, status history, and idempotency key in one transaction | ✅ | ✅ |
| III. DB constraints | PK/FK, `NOT NULL`, `CHECK`, partial unique indexes (data-model) | ✅ | ✅ |
| III. Audit columns + soft delete | All 4 tables carry the 6 audit columns with the `deleted`/`deleted_at` CHECK; no hard deletes | ✅ | ✅ |
| III. Versioned migrations | `migrations/000001_*.up.sql` via golang-migrate | ✅ | ✅ |
| IV. AuthN/AuthZ | JWT + scope `loan-applications:submit` (research R6) | ✅ | ✅ |
| IV. Parameterized SQL / input validation | sqlc-generated queries; full validation at the boundary | ✅ | ✅ |
| IV. Secrets, personal data | Env/secret manager only; logs carry field paths and codes, never values (FR-050) | ✅ | ✅ |
| IV. Encryption at rest | Storage-level encryption required for every environment with real data (research R13) | ✅ | ✅ |
| IV. govulncheck | In CI gate | ✅ | ✅ |
| V. Observability | slog JSON + request ID, `/healthz`, `/readyz`, `/metrics`, status-history audit (research R10, R12) | ✅ | ✅ |
| VI. Simplicity | Standard library mux, hand-written validation, no ORM; every dependency justified in research | ✅ | ✅ |
| Stack: graceful shutdown, fail-fast configuration | Included in skeleton | ✅ | ✅ |

**Result**: PASS before and after design. No violations, so Complexity Tracking is empty.

## Project Structure

### Documentation (this feature)

```text
specs/001-loan-submission/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── openapi.yaml     # Phase 1 output
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
.github/workflows/ci.yml         # CI quality gates

cmd/
├── api/
│   └── main.go                  # wiring: config → pool → migrations check → server; graceful shutdown
└── devtoken/
    └── main.go                  # dev-only JWT minting for local testing

api/
└── openapi.yaml                 # source-of-truth contract (from contracts/openapi.yaml)

migrations/
├── embed.go                     # embeds *.sql for the migration runner
├── 000001_create_loan_applications.{up,down}.sql
└── 000002_create_idempotency_keys.{up,down}.sql

internal/
├── config/                      # env parsing, fail-fast validation (APP_CURRENCY, APP_TIMEZONE, DB, JWT)
├── auth/                        # JWT verification middleware, actor + scopes in context
├── httpx/                       # problem+json, JSON decode (strict, size limit), Timestamp/Date types,
│                                #   request-ID + logging + metrics middleware
├── money/                       # Amount (int64 minor units), parse from json.Number
├── server/                      # mux assembly, middleware chain, /healthz, /readyz, /metrics
├── db/
│   ├── pool.go                  # pgxpool setup, readiness ping
│   ├── queries/*.sql            # sqlc input
│   └── sqlc/                    # sqlc-generated code (committed)
└── loanapplication/
    ├── handler.go               # HTTP: decode → service → encode; maps errors to problems
    ├── service.go               # business rules: validate, idempotency, transaction orchestration
    ├── validate.go              # all field and cross-field rules → []FieldError
    ├── repository.go            # SQL via sqlc; maps unique violations to domain conflicts
    └── *_test.go                # unit tests (validation, mapping) next to code

tests/
├── testutil/                    # Testcontainers PostgreSQL, JWT signer, fixed clock
├── integration/                 # //go:build integration — repository + service against Testcontainers PG
└── contract/                    # //go:build integration — HTTP server validated against api/openapi.yaml
```

**Structure Decision**: Single Go module (one web service) using the standard
`cmd/` + `internal/` layout.
- **Layering** (Principle VI): `handler` (HTTP only) → `service` (rules and transactions) →
  `repository` (SQL only).
- **Shared code**: cross-cutting packages (`httpx`, `auth`, `money`, `config`, `db`) are shared
  by future features.
- **Tests**: unit tests live next to their code. Integration and contract tests live under
  `tests/` behind a build tag, so `go test ./...` stays fast without Docker.

## Complexity Tracking

No constitution violations; nothing to justify.
