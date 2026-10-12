# Lending Backend

Go REST API for car-backed loan applications, backed by PostgreSQL. Governed by
[`.specify/memory/constitution.md`](.specify/memory/constitution.md).

**Feature 001: loan application submission.** `POST /v1/loan-applications` validates a customer
(with spouse when married), one car as collateral, and a proposed loan, then stores the
application as `IN_PROGRESS`.
- **Spec**: [`specs/001-loan-submission/spec.md`](specs/001-loan-submission/spec.md).
- **API contract**: [`api/openapi.yaml`](api/openapi.yaml).

## Prerequisites

- Go (version from `go.mod`)
- Docker, for PostgreSQL and the Testcontainers-based tests
- Optional: `make`, `golangci-lint`, `govulncheck`, `sqlc` (or the `sqlc/sqlc` Docker image), `openssl`

## Deployment requirement: encryption at rest

The database holds sensitive personal data: ID numbers, incomes, birth dates, and phone
numbers. Every environment with real data **MUST** run PostgreSQL on encrypted storage:
- a managed PostgreSQL service with encryption at rest enabled, or
- encrypted volumes for self-hosted databases.

Backups and replicas must be encrypted too. See constitution Principle IV and
[research R13](specs/001-loan-submission/research.md).

## Configuration

All configuration comes from environment variables. The service exits at startup, listing every
problem, if anything is missing or invalid.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | yes | | PostgreSQL URL, e.g. `postgres://user:pass@host:5432/lending?sslmode=require` |
| `APP_CURRENCY` | yes | | ISO 4217 currency of all amounts, e.g. `IDR` |
| `APP_TIMEZONE` | yes | | IANA zone for API timestamps and "today", e.g. `Asia/Jakarta` |
| `AUTH_JWT_ISSUER` | yes | | Expected `iss` claim |
| `AUTH_JWT_AUDIENCE` | yes | | Expected `aud` claim |
| `AUTH_JWT_PUBLIC_KEY` | yes | | PEM public key (ECDSA or RSA) that verifies bearer tokens |
| `HTTP_ADDR` | no | `:8080` | Listen address |
| `ERROR_TYPE_BASE_URL` | no | `https://errors.lending.example/` | Prefix of problem `type` URIs |
| `SHUTDOWN_TIMEOUT` | no | `20s` | Graceful shutdown drain time |
| `DB_MAX_CONNS` | no | `10` | Connection pool size |

**Tokens**: callers need a bearer JWT with `sub` (recorded as `created_by`/`updated_by`) and the
scope `loan-applications:submit`.

## Running locally

```bash
docker run -d --name lending-pg -e POSTGRES_PASSWORD=dev -e POSTGRES_DB=lending -p 5432:5432 postgres:18
make dev-keys                     # writes dev/jwt.key and dev/jwt.pub (git-ignored)
export DATABASE_URL="postgres://postgres:dev@localhost:5432/lending?sslmode=disable"
export APP_CURRENCY=IDR APP_TIMEZONE=Asia/Jakarta AUTH_JWT_ISSUER=dev AUTH_JWT_AUDIENCE=lending-api
export AUTH_JWT_PUBLIC_KEY="$(cat dev/jwt.pub)"
make run                          # applies migrations, then listens on :8080

TOKEN=$(make -s dev-token)
curl -i -X POST localhost:8080/v1/loan-applications \
  -H "Authorization: Bearer $TOKEN" -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" -d @dev/examples/married.json
```

**Operational endpoints**:
- `GET /healthz`: liveness.
- `GET /readyz`: database reachable.
- `GET /metrics`: Prometheus format, including HTTP and connection-pool metrics.

The full validation walkthrough is in
[`specs/001-loan-submission/quickstart.md`](specs/001-loan-submission/quickstart.md).

## Make targets

| Target | What it does |
|--------|--------------|
| `make build` / `make run` | Build `bin/api` / run the service |
| `make test` | Unit tests with the race detector |
| `make test-integration` | Unit, integration, and contract tests against PostgreSQL 18 in Docker |
| `make lint` | `gofmt`, `go vet`, `golangci-lint` |
| `make vuln` | `govulncheck` |
| `make sqlc` | Regenerate `internal/db/sqlc` from `internal/db/queries` and `migrations` |
| `make dev-keys` / `make dev-token` | Local signing key / a 1-hour dev token |

### Windows notes

- **Race detector**: `-race` needs cgo and a C compiler. Without one, drop `-race` locally; CI
  always runs it on Linux.
- **Docker host**: set `DOCKER_HOST=npipe:////./pipe/docker_engine` before running integration
  tests. Without it, Testcontainers' Docker-host detection can race when test packages run in
  parallel and fail with "rootless Docker is not supported on Windows".
- **sqlc**: run `docker run --rm -v "${PWD}:/src" -w /src sqlc/sqlc generate`.

## Layout

```text
cmd/api            service entry point (config, migrations, server, graceful shutdown)
cmd/devtoken       dev-only JWT minting
api/openapi.yaml   API contract (source of truth)
migrations/        versioned SQL migrations (embedded into the binary)
internal/
  config/          environment configuration
  auth/            JWT verification, actor in context
  httpx/           problem details, strict JSON decoding, timestamp types, middleware
  money/           integer minor-unit amounts (no floating point)
  server/          routing, /healthz, /readyz, /metrics
  db/              pool, migrations runner, sqlc queries and generated code
  loanapplication/ handler → service → repository for feature 001
tests/
  testutil/        PostgreSQL container, JWT signer, fixed clock
  integration/     repository and service tests against PostgreSQL (build tag: integration)
  contract/        HTTP tests validated against api/openapi.yaml (build tag: integration)
```

The load check runs separately:
`go test -tags='integration load' -run TestSubmitLatency ./tests/contract/`.
