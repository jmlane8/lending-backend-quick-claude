-- name: InsertLoanApplication :one
INSERT INTO loan_applications (
    status, currency, loan_amount, loan_tenure_months,
    customer_full_name, customer_id_number, customer_birth_date, customer_phone_number,
    customer_email, customer_monthly_income, customer_marital_status,
    customer_address_street, customer_address_city, customer_address_zip_code,
    collateral_brand, collateral_model, collateral_manufacturing_year, collateral_license_plate,
    created_by, updated_by
) VALUES (
    @status, @currency, @loan_amount, @loan_tenure_months,
    @customer_full_name, @customer_id_number, @customer_birth_date, @customer_phone_number,
    @customer_email, @customer_monthly_income, @customer_marital_status,
    @customer_address_street, @customer_address_city, @customer_address_zip_code,
    @collateral_brand, @collateral_model, @collateral_manufacturing_year, @collateral_license_plate,
    @actor, @actor
)
RETURNING id, submitted_at;

-- name: InsertLoanApplicationSpouse :exec
INSERT INTO loan_application_spouses (
    loan_application_id, full_name, id_number, phone_number, monthly_income, created_by, updated_by
) VALUES (
    @loan_application_id, @full_name, @id_number, @phone_number, @monthly_income, @actor, @actor
);

-- name: InsertStatusHistory :exec
INSERT INTO loan_application_status_history (
    loan_application_id, from_status, to_status, created_by, updated_by
) VALUES (
    @loan_application_id, @from_status, @to_status, @actor, @actor
);
