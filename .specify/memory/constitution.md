<!--
Sync Impact Report
==================
Version change: (unversioned template) → 1.0.0
Bump rationale: Initial ratification; all placeholders replaced with concrete governance.

Modified principles (template placeholder → new title):
  - [PRINCIPLE_1_NAME] → I. API Contract-First
  - [PRINCIPLE_2_NAME] → II. Test-First (NON-NEGOTIABLE)
  - [PRINCIPLE_3_NAME] → III. Data Integrity & Financial Correctness
  - [PRINCIPLE_4_NAME] → IV. Security by Default
  - [PRINCIPLE_5_NAME] → V. Observability
  - (added)            → VI. Simplicity & Idiomatic Go

Added sections:
  - Technology Stack & Constraints (was [SECTION_2_NAME])
  - Development Workflow & Quality Gates (was [SECTION_3_NAME])

Removed sections: none

Templates requiring updates (read constitution at runtime; not modified by this command):
  ✅ .specify/templates/plan-template.md — Constitution Check reads this file dynamically
  ✅ .specify/templates/spec-template.md — no change required
  ✅ .specify/templates/tasks-template.md — no change required

Follow-up TODOs: none
-->

# Lending Backend Constitution

## Core Principles

### I. API Contract-First

- Every HTTP endpoint MUST be defined in an OpenAPI 3.x specification before it is implemented.
  The spec is the source of truth; handlers MUST conform to it.
- APIs MUST follow REST conventions: resource-oriented plural nouns, correct HTTP methods and
  status codes, JSON request/response bodies, and `application/problem+json` (RFC 9457)
  for all error responses.
- Public APIs MUST be versioned via URL prefix (`/v1/...`). Breaking changes require a new
  major version; the previous version MUST remain available until a documented deprecation
  window has elapsed.
- List endpoints MUST support pagination; unbounded result sets are prohibited.
- Mutating endpoints that move money or change loan state MUST accept an `Idempotency-Key`
  header and guarantee at-most-once effect for a given key.

**Rationale**: Clients (frontends, partners, internal services) depend on a stable,
predictable contract. Idempotency prevents duplicate disbursements and payments on retry.

### II. Test-First (NON-NEGOTIABLE)

- TDD is mandatory: tests are written and observed to fail before the implementation is written
  (Red → Green → Refactor).
- Business logic MUST have unit tests using Go's standard `testing` package with table-driven
  tests where multiple cases apply.
- Repository/database code MUST be covered by integration tests running against a real
  PostgreSQL instance (e.g., via Testcontainers or a disposable Docker database). Mocking the
  database for repository tests is prohibited.
- Every endpoint MUST have contract tests verifying request/response shapes against the
  OpenAPI spec, including error cases.
- `go test -race ./...` MUST pass before merge.

**Rationale**: Lending logic (interest, schedules, balances) is error-prone and costly to get
wrong; failing-first tests prove the test actually exercises the behavior.

### III. Data Integrity & Financial Correctness

- Monetary amounts MUST NOT be represented as floating-point values. Use integer minor units
  (`int64` cents) or a fixed-point decimal type in Go, and `NUMERIC`/`BIGINT` in PostgreSQL.
  Every amount MUST carry an explicit ISO 4217 currency.
- Interest and rounding rules MUST be explicit, documented per product, and covered by tests.
- Operations that change multiple related records (e.g., applying a payment to a loan and its
  ledger) MUST execute in a single database transaction.
- Financial records (payments, disbursements, ledger entries) are append-only. Corrections MUST
  be made via compensating entries, never by UPDATE or DELETE of historical rows.
- Integrity MUST be enforced in the database as well as in code: primary keys, foreign keys,
  `NOT NULL`, `CHECK`, and unique constraints are required where applicable.
- All timestamps MUST be stored as `TIMESTAMPTZ` in UTC.
- Schema changes MUST be made only through versioned, forward-only SQL migration files that are
  committed to the repository and reviewed. Manual changes to shared databases are prohibited.

**Rationale**: Money errors and silent data corruption are the highest-impact failures for a
lending system; the database is the last line of defense.

### IV. Security by Default

- All endpoints MUST require authentication unless explicitly documented as public; authorization
  MUST be checked per resource (a caller can only access loans they are entitled to).
- All SQL MUST use parameterized queries. String-concatenated SQL is prohibited.
- All input MUST be validated at the API boundary (type, range, length, format) before reaching
  business logic.
- Secrets MUST come from the environment or a secret manager; they MUST NOT be committed to the
  repository or written to logs.
- Personally identifiable and financial data (names, government IDs, account numbers) MUST NOT
  appear in logs, and MUST be encrypted at rest where it is sensitive.
- Dependencies MUST be scanned with `govulncheck`; known exploitable vulnerabilities block merge.

**Rationale**: A lending backend holds regulated personal and financial data; breaches carry
legal and reputational consequences.

### V. Observability

- Logging MUST be structured (JSON) using `log/slog`, with a request/correlation ID propagated
  through `context.Context` on every log line for that request.
- Every service MUST expose health (`/healthz`) and readiness (`/readyz`, which checks database
  connectivity) endpoints.
- Request latency, error rates, and database pool metrics MUST be exported (e.g., Prometheus
  format).
- State changes to loans and money movements MUST produce an audit record identifying who,
  what, and when.

**Rationale**: Production issues and regulatory inquiries both require reconstructing exactly
what happened and why.

### VI. Simplicity & Idiomatic Go

- Prefer the standard library (`net/http`, `database/sql`/`pgx`, `log/slog`, `encoding/json`).
  Each new third-party dependency MUST be justified in the plan.
- Code MUST be formatted with `gofmt` and pass `go vet` and the project's `golangci-lint`
  configuration.
- Errors MUST be handled explicitly and wrapped with context (`fmt.Errorf("...: %w", err)`);
  panics are prohibited outside of program startup.
- `context.Context` MUST be the first parameter of any function performing I/O, and MUST be
  honored for cancellation and timeouts.
- Layering: HTTP handlers → services (business logic) → repositories (SQL). Handlers MUST NOT
  contain SQL; repositories MUST NOT contain business rules. Interfaces are defined by the
  consumer and introduced only when there is more than one implementation or a test need.
- YAGNI: no speculative abstractions, frameworks, or ORMs without a documented need.

**Rationale**: Small, idiomatic, explicit code is easier to review, audit, and maintain.

## Technology Stack & Constraints

- **Language**: Go (latest stable release; the `go` directive in `go.mod` is the authority).
- **Database**: PostgreSQL (supported major version, pinned in local and CI environments).
- **DB access**: `pgx` (directly or via `database/sql`); SQL is written explicitly. Query
  generation tools such as `sqlc` are permitted; ORMs require justification per Principle VI.
- **Migrations**: versioned SQL files in a `migrations/` directory, applied by a migration tool
  in CI and at deploy time.
- **API specification**: OpenAPI 3.x document(s) committed to the repository.
- **Configuration**: environment variables, following 12-factor practice; the service MUST fail
  fast at startup on missing or invalid configuration.
- **Packaging**: the service builds as a single static binary and runs in a container image.
- **Performance baseline**: p95 latency under 300 ms for standard read endpoints under expected
  load; every query on a hot path MUST be backed by an appropriate index. Deviations MUST be
  documented in the feature plan.
- **Shutdown**: the server MUST shut down gracefully on SIGTERM, draining in-flight requests and
  closing the database pool.

## Development Workflow & Quality Gates

- All changes go through a pull request with at least one approving review.
- CI MUST run and pass before merge: `gofmt` check, `go vet`, `golangci-lint`,
  `go test -race ./...` (unit and PostgreSQL integration tests), contract tests, migration
  apply-from-scratch check, and `govulncheck`.
- PRs that change money movement, interest calculation, or loan state transitions MUST include
  tests for edge cases (zero, negative, rounding boundaries, partial payments, concurrency).
- PRs that add or change migrations MUST describe their impact on existing data and locking
  behavior on large tables.
- Changes to the OpenAPI spec MUST be reviewed for backward compatibility; breaking changes
  require a version bump per Principle I.
- Feature work follows the Spec Kit flow: specify → plan (with Constitution Check) → tasks →
  implement.

## Governance

- This constitution supersedes all other development practices for this project. Where a
  guideline conflicts with it, the constitution wins.
- Amendments require a pull request that updates this file, states the rationale, includes a
  Sync Impact Report, and describes any migration needed for existing code.
- Versioning follows semantic versioning:
  - **MAJOR**: removal or backward-incompatible redefinition of a principle or governance rule.
  - **MINOR**: a new principle or section, or materially expanded guidance.
  - **PATCH**: clarifications, wording, and typo fixes with no semantic change.
- Every plan MUST pass the Constitution Check before design work and again after design. Any
  violation MUST be recorded in the plan's Complexity Tracking table with justification and the
  simpler alternative that was rejected.
- Reviewers MUST verify constitutional compliance on every PR. Unjustified complexity is grounds
  for rejection.
- Compliance with this constitution SHOULD be reviewed at least quarterly, or whenever the
  technology stack changes.

**Version**: 1.0.0 | **Ratified**: 2026-10-08 | **Last Amended**: 2026-10-08
