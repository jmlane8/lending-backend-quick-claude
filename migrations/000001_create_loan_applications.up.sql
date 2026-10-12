-- Loan application submission (feature 001-loan-submission).
-- Every table carries the constitution's audit columns; rows are soft-deleted only.

CREATE TABLE loan_applications (
    id                            UUID          PRIMARY KEY DEFAULT uuidv7(),
    status                        TEXT          NOT NULL CHECK (status IN ('IN_PROGRESS')),
    currency                      CHAR(3)       NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    loan_amount                   NUMERIC(19,2) NOT NULL CHECK (loan_amount > 0 AND loan_amount % 100 = 0),
    loan_tenure_months            SMALLINT      NOT NULL CHECK (loan_tenure_months BETWEEN 3 AND 48),

    customer_full_name            TEXT          NOT NULL CHECK (char_length(customer_full_name) BETWEEN 1 AND 100),
    customer_id_number            TEXT          NOT NULL CHECK (customer_id_number ~ '^[A-Za-z0-9]{1,30}$'),
    customer_birth_date           DATE          NOT NULL,
    customer_phone_number         TEXT          NOT NULL CHECK (customer_phone_number ~ '^\+[0-9]{8,15}$'),
    customer_email                TEXT          NOT NULL CHECK (char_length(customer_email) BETWEEN 3 AND 254),
    customer_monthly_income       NUMERIC(19,2) NOT NULL CHECK (customer_monthly_income > 0),
    customer_marital_status       TEXT          NOT NULL CHECK (customer_marital_status IN ('SINGLE', 'MARRIED', 'DIVORCED', 'WIDOWED')),
    customer_address_street       TEXT          NOT NULL CHECK (char_length(customer_address_street) BETWEEN 1 AND 200),
    customer_address_city         TEXT          NOT NULL CHECK (char_length(customer_address_city) BETWEEN 1 AND 100),
    customer_address_zip_code     TEXT          NOT NULL CHECK (customer_address_zip_code ~ '^[A-Za-z0-9 -]{3,10}$'),

    collateral_brand              TEXT          NOT NULL CHECK (char_length(collateral_brand) BETWEEN 1 AND 50),
    collateral_model              TEXT          NOT NULL CHECK (char_length(collateral_model) BETWEEN 1 AND 50),
    collateral_manufacturing_year SMALLINT      NOT NULL CHECK (collateral_manufacturing_year >= 1900),
    collateral_license_plate      TEXT          NOT NULL CHECK (collateral_license_plate ~ '^[A-Z0-9]{1,15}$'),

    submitted_at                  TIMESTAMPTZ   NOT NULL DEFAULT now(),

    created_at                    TIMESTAMPTZ   NOT NULL DEFAULT now(),
    created_by                    TEXT          NOT NULL,
    updated_at                    TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_by                    TEXT          NOT NULL,
    deleted                       BOOLEAN       NOT NULL DEFAULT false,
    deleted_at                    TIMESTAMPTZ   NULL,
    CONSTRAINT loan_applications_deleted_consistent
        CHECK ((deleted AND deleted_at IS NOT NULL) OR (NOT deleted AND deleted_at IS NULL))
);

-- At most one open application per customer and per car (FR-007, FR-008, FR-009).
CREATE UNIQUE INDEX uq_loan_applications_open_customer
    ON loan_applications (customer_id_number)
    WHERE status = 'IN_PROGRESS' AND NOT deleted;

CREATE UNIQUE INDEX uq_loan_applications_open_collateral
    ON loan_applications (collateral_license_plate)
    WHERE status = 'IN_PROGRESS' AND NOT deleted;

CREATE TABLE loan_application_spouses (
    loan_application_id UUID          PRIMARY KEY REFERENCES loan_applications (id),
    full_name           TEXT          NOT NULL CHECK (char_length(full_name) BETWEEN 1 AND 100),
    id_number           TEXT          NOT NULL CHECK (id_number ~ '^[A-Za-z0-9]{1,30}$'),
    phone_number        TEXT          NOT NULL CHECK (phone_number ~ '^\+[0-9]{8,15}$'),
    monthly_income      NUMERIC(19,2) NOT NULL CHECK (monthly_income >= 0),

    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    created_by          TEXT          NOT NULL,
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_by          TEXT          NOT NULL,
    deleted             BOOLEAN       NOT NULL DEFAULT false,
    deleted_at          TIMESTAMPTZ   NULL,
    CONSTRAINT loan_application_spouses_deleted_consistent
        CHECK ((deleted AND deleted_at IS NOT NULL) OR (NOT deleted AND deleted_at IS NULL))
);

-- Append-only audit trail of status changes (constitution Principle V).
CREATE TABLE loan_application_status_history (
    id                  UUID        PRIMARY KEY DEFAULT uuidv7(),
    loan_application_id UUID        NOT NULL REFERENCES loan_applications (id),
    from_status         TEXT        NULL,
    to_status           TEXT        NOT NULL,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by          TEXT        NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by          TEXT        NOT NULL,
    deleted             BOOLEAN     NOT NULL DEFAULT false,
    deleted_at          TIMESTAMPTZ NULL,
    CONSTRAINT loan_application_status_history_deleted_consistent
        CHECK ((deleted AND deleted_at IS NOT NULL) OR (NOT deleted AND deleted_at IS NULL))
);

CREATE INDEX ix_loan_application_status_history_application
    ON loan_application_status_history (loan_application_id);
