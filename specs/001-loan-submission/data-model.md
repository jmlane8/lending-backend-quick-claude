# Data Model: Loan Application Submission

**Feature**: [spec.md](./spec.md) | **Research**: [research.md](./research.md) | **Date**: 2026-10-09

All tables carry the constitution's audit columns (Principle III). They are listed once here and
then marked as **[audit]** on each table:

| Column       | Type          | Constraint                                                    |
|--------------|---------------|---------------------------------------------------------------|
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT now()`                                      |
| `created_by` | `TEXT`        | `NOT NULL` (JWT `sub` of the caller, see research R6)         |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT now()`                                      |
| `updated_by` | `TEXT`        | `NOT NULL`                                                    |
| `deleted`    | `BOOLEAN`     | `NOT NULL DEFAULT false`                                      |
| `deleted_at` | `TIMESTAMPTZ` | `NULL`; `CHECK ((deleted AND deleted_at IS NOT NULL) OR (NOT deleted AND deleted_at IS NULL))` |

Hard `DELETE` is never issued. Repository queries filter `NOT deleted` by default.

## Entity relationship

```text
loan_applications 1 ──── 0..1 loan_application_spouses
loan_applications 1 ──── 1..* loan_application_status_history
idempotency_keys  * ──── 0..1 loan_applications   (via loan_application_id)
```

## `loan_applications`

One row per submitted application. It holds the loan terms, a snapshot of the customer
(including address), and the car collateral (research R3).

| Column                          | Type           | Constraint / rule                                         | Spec |
|---------------------------------|----------------|-----------------------------------------------------------|------|
| `id`                            | `UUID`         | PK, `DEFAULT uuidv7()`; returned as the application reference | FR-003 |
| `status`                        | `TEXT`         | `NOT NULL`, `CHECK (status IN ('IN_PROGRESS'))` (more statuses added by later features) | FR-003 |
| `currency`                      | `CHAR(3)`      | `NOT NULL`, ISO 4217, from `APP_CURRENCY`                 | Principle III |
| `loan_amount`                   | `NUMERIC(19,2)`| `NOT NULL`, `CHECK (loan_amount > 0 AND loan_amount % 100 = 0)` | FR-030 |
| `loan_tenure_months`            | `SMALLINT`     | `NOT NULL`, `CHECK (loan_tenure_months BETWEEN 3 AND 48)` | FR-031 |
| `customer_full_name`            | `TEXT`         | `NOT NULL`, `CHECK (char_length(...) BETWEEN 1 AND 100)`  | FR-010 |
| `customer_id_number`            | `TEXT`         | `NOT NULL`, `CHECK (customer_id_number ~ '^[A-Za-z0-9]{1,30}$')` | FR-011 |
| `customer_birth_date`           | `DATE`         | `NOT NULL`                                                | FR-013 |
| `customer_phone_number`         | `TEXT`         | `NOT NULL`, `CHECK (... ~ '^\+[0-9]{8,15}$')`             | FR-014 |
| `customer_email`                | `TEXT`         | `NOT NULL`, `CHECK (char_length(...) <= 254)`             | FR-015 |
| `customer_monthly_income`       | `NUMERIC(19,2)`| `NOT NULL`, `CHECK (customer_monthly_income > 0)`         | FR-016 |
| `customer_marital_status`       | `TEXT`         | `NOT NULL`, `CHECK (... IN ('SINGLE','MARRIED','DIVORCED','WIDOWED'))` | FR-017 |
| `customer_address_street`       | `TEXT`         | `NOT NULL`, length 1–200                                  | FR-012 |
| `customer_address_city`         | `TEXT`         | `NOT NULL`, length 1–100                                  | FR-012 |
| `customer_address_zip_code`     | `TEXT`         | `NOT NULL`, `CHECK (... ~ '^[A-Za-z0-9 -]{3,10}$')`       | FR-012 |
| `collateral_brand`              | `TEXT`         | `NOT NULL`, length 1–50                                   | FR-020 |
| `collateral_model`              | `TEXT`         | `NOT NULL`, length 1–50                                   | FR-021 |
| `collateral_manufacturing_year` | `SMALLINT`     | `NOT NULL`, `CHECK (... >= 1900)` (upper bound is time-dependent, enforced in code) | FR-022 |
| `collateral_license_plate`      | `TEXT`         | `NOT NULL`, normalized, `CHECK (... ~ '^[A-Z0-9]{1,15}$')` | FR-023 |
| `submitted_at`                  | `TIMESTAMPTZ`  | `NOT NULL DEFAULT now()`                                  | FR-003 |
| **[audit]**                     |                |                                                           | FR-051 |

**Indexes**

- `uq_loan_applications_open_customer`: `UNIQUE (customer_id_number) WHERE status =
  'IN_PROGRESS' AND NOT deleted` (FR-007, FR-009).
- `uq_loan_applications_open_collateral`: `UNIQUE (collateral_license_plate) WHERE status =
  'IN_PROGRESS' AND NOT deleted` (FR-008, FR-009).

The service maps a unique violation on either index to its 409 problem type, using the
constraint name from `pgconn.PgError.ConstraintName`.

**Notes**

- `customer_id_number` is stored exactly as entered after trimming; matching is
  case-sensitive. A country-specific normalization can be added later.
- Database `CHECK` constraints repeat the main code rules as a last line of defense
  (Principle III). Rules that depend on time (age ≥ 18, year ≤ next year) and cross-field rules
  are enforced only in code.

## `loan_application_spouses`

Present only when `customer_marital_status = 'MARRIED'` (FR-018 to FR-019c).

| Column                | Type           | Constraint / rule                                        | Spec |
|-----------------------|----------------|----------------------------------------------------------|------|
| `loan_application_id` | `UUID`         | PK, FK → `loan_applications(id)`                         |      |
| `full_name`           | `TEXT`         | `NOT NULL`, length 1–100                                 | FR-018 |
| `id_number`           | `TEXT`         | `NOT NULL`, `^[A-Za-z0-9]{1,30}$`; must differ from customer's (in code) | FR-019 |
| `phone_number`        | `TEXT`         | `NOT NULL`, `^\+[0-9]{8,15}$`                            | FR-019a |
| `monthly_income`      | `NUMERIC(19,2)`| `NOT NULL`, `CHECK (monthly_income >= 0)`                | FR-019b |
| **[audit]**           |                |                                                          |      |

The rule that a spouse row exists if and only if the customer is `MARRIED` is enforced in the
service, inside the same transaction.

## `loan_application_status_history`

Append-only audit trail of status changes (Principle V; research R10). Rows are never updated
or soft-deleted.

| Column                | Type          | Constraint                                     |
|-----------------------|---------------|------------------------------------------------|
| `id`                  | `UUID`        | PK, `DEFAULT uuidv7()`                         |
| `loan_application_id` | `UUID`        | `NOT NULL`, FK → `loan_applications(id)`, indexed |
| `from_status`         | `TEXT`        | `NULL` for the initial row                     |
| `to_status`           | `TEXT`        | `NOT NULL`                                     |
| **[audit]**           |               | `created_by` / `created_at` record who changed it and when |

## `idempotency_keys`

Request deduplication for `POST /v1/loan-applications` (FR-006; research R5).

| Column                | Type          | Constraint                                              |
|-----------------------|---------------|---------------------------------------------------------|
| `id`                  | `UUID`        | PK, `DEFAULT uuidv7()`                                  |
| `actor`               | `TEXT`        | `NOT NULL` (JWT `sub`)                                  |
| `idempotency_key`     | `TEXT`        | `NOT NULL`, length 1–255                                |
| `request_hash`        | `BYTEA`       | `NOT NULL` (SHA-256 of canonical request body)          |
| `response_status`     | `SMALLINT`    | `NULL` on insert; set before commit                     |
| `response_body`       | `JSONB`       | `NULL` on insert; set before commit (no personal data: reference, status, time, currency) |
| `loan_application_id` | `UUID`        | `NULL`, FK → `loan_applications(id)`                    |
| `expires_at`          | `TIMESTAMPTZ` | `NOT NULL` (`created_at + 24h`)                         |
| **[audit]**           |               |                                                         |

**Indexes**: `UNIQUE (actor, idempotency_key) WHERE NOT deleted`.

The key row is inserted **first** in the transaction, with response columns `NULL`. This is so a
concurrent duplicate request waits on this row instead of hitting the open-application indexes.
The response columns are filled with an `UPDATE` just before commit. A committed row therefore
always has both response columns set, enforced by
`CHECK ((response_status IS NULL) = (response_body IS NULL))`.

## State transitions

```text
(none) ──submit──▶ IN_PROGRESS
```

This feature defines only the initial state. Later features add transitions, and each one
must append a `loan_application_status_history` row in the same transaction.

## Domain types (Go, `internal/loanapplication`)

| Type                 | Purpose                                                                  |
|----------------------|--------------------------------------------------------------------------|
| `SubmitRequest`      | Decoded request DTO (JSON numbers as `json.Number`, dates as `httpx.Date`) |
| `Application`        | Validated domain value: customer, optional spouse, collateral, loan      |
| `money.Amount`       | `int64` minor units; parsed from `json.Number` text, never via float     |
| `MaritalStatus`      | String enum with `SINGLE`, `MARRIED`, `DIVORCED`, `WIDOWED`              |
| `FieldError`         | `{Field (JSON Pointer), Code, Message}`; validation returns `[]FieldError` |
| `SubmitResult`       | `{ID, Status, SubmittedAt, Currency}`; response body and idempotent replay |
