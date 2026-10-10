# Quickstart: Loan Application Submission

**Feature**: [spec.md](./spec.md) | **Contract**: [contracts/openapi.yaml](./contracts/openapi.yaml) |
**Data model**: [data-model.md](./data-model.md)

This guide shows the feature working end to end. It is not an implementation guide.

## Prerequisites

- Go (version from `go.mod`), Docker (for PostgreSQL and Testcontainers), `migrate` CLI,
  `sqlc`, `golangci-lint`, `govulncheck`, `curl`, `jq`.
- A signed test JWT with `sub` and scope `loan-applications:submit`. A `make dev-token` helper
  will produce one, signed with the local development key.

## 1. Start dependencies and the service

```bash
docker run -d --name lending-pg -e POSTGRES_PASSWORD=dev -e POSTGRES_DB=lending -p 5432:5432 postgres:18
export DATABASE_URL="postgres://postgres:dev@localhost:5432/lending?sslmode=disable"
export APP_CURRENCY=IDR APP_TIMEZONE=Asia/Jakarta HTTP_ADDR=:8080
export AUTH_JWT_ISSUER=dev AUTH_JWT_AUDIENCE=lending-api AUTH_JWT_PUBLIC_KEY="$(cat dev/jwt.pub)"
migrate -path migrations -database "$DATABASE_URL" up
go run ./cmd/api
```

**Expect**: `curl -s localhost:8080/readyz` → `200`. Removing any required variable makes the
service exit at startup with a clear error.

## 2. Validation scenarios

Use `TOKEN=$(make -s dev-token)` and the examples in `contracts/openapi.yaml`
(`components.examples`).

| # | Scenario | Request | Expected |
|---|----------|---------|----------|
| 1 | Married customer, valid | `MarriedCustomer` example, new `Idempotency-Key` | `201`, body `{id, status: "IN_PROGRESS", currency: "IDR", submittedAt: "…+07:00"}`, `Location` header |
| 2 | Single customer, valid | `SingleCustomer` example | `201` |
| 3 | Idempotent replay | Repeat #1 with the **same** key and body | `201`, identical `id`; still one row in DB |
| 4 | Key reused, different body | Same key as #1, change `loan.amount` | `422`, `type` ends `idempotency-key-reused` |
| 5 | Customer already has open app | New key, `MarriedCustomer` with a different `licensePlate` | `409` `customer-has-open-application` |
| 6 | Car already pledged | New key, `SingleCustomer` with `idNumber` changed and `licensePlate: "b1234xyz"` | `409` `collateral-in-use` (plate normalization) |
| 7 | Multiple invalid fields | `amount: 15050`, `tenureMonths: 49`, `email: "x"`, no `city` | `400` `validation-failed`, `errors[]` has all 4 fields |
| 8 | Married without spouse | `maritalStatus: MARRIED`, no `spouse` | `400`, `/customer/spouse` `spouse_required` |
| 9 | Spouse but single | `SingleCustomer` + `spouse` object | `400`, `spouse_not_allowed` |
| 10 | Underage | `birthDate` = today minus 18 years plus 1 day | `400`, `/customer/birthDate` `under_minimum_age` |
| 11 | Unknown field | Add `"foo": 1` at root | `400` `malformed-request` |
| 12 | No token / wrong scope | Omit `Authorization` / token without scope | `401` / `403` |
| 13 | Missing key | Omit `Idempotency-Key` | `400` `invalid-idempotency-key` |

Example call:

```bash
curl -si -X POST localhost:8080/v1/loan-applications \
  -H "Authorization: Bearer $TOKEN" -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" -d @dev/examples/married.json
```

## 3. Verify storage

```sql
SELECT id, status, customer_id_number, collateral_license_plate, loan_amount, created_by,
       deleted
FROM loan_applications ORDER BY created_at DESC LIMIT 5;
SELECT * FROM loan_application_spouses;           -- one row, for scenario #1 only
SELECT * FROM loan_application_status_history;    -- one NULL → IN_PROGRESS row per application
```

**Expect**: `created_by` / `updated_by` equal the token's `sub`; `deleted = false`;
`collateral_license_plate` is `B1234XYZ`; there are no rows from rejected scenarios.

## 4. Automated checks (CI gate)

```bash
gofmt -l . && go vet ./... && golangci-lint run
go test -race ./...                         # unit tests
go test -race -tags=integration ./...       # PostgreSQL via Testcontainers, contract tests
govulncheck ./...
```

**Expect**: everything passes. The integration suite includes concurrent-submission tests:
- 20 parallel submissions for the same customer → exactly one `201`.
- 20 parallel submissions with the same idempotency key → one row, identical responses.

The suite also scans captured logs for personal data from the test fixtures (SC-006).
