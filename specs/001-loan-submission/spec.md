# Feature Specification: Loan Application Submission

**Feature Branch**: `001-loan-submission` (no git repository yet; branch not created)

**Created**: 2026-10-09

**Status**: Draft

**Input**: User description: "Create a REST API for submitting a loan. The API will need the
following data. Customer data: full name, ID number, address (street, city, zip code), birth date,
phone number, email, and current monthly income. Collateral data is always a car: brand, model,
manufacturing year, and license plate. Proposed loan data: amount (must be a multiple of 100),
tenure (between 3-48 months). If all inputs are valid, store the data in the database as
"IN_PROGRESS" status."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Submit a valid car-backed loan application (Priority: P1)

An authorized client (for example, the customer-facing app or a loan officer's tool) submits a
loan application containing the customer's details, the car offered as collateral, and the
proposed loan amount and tenure. The system accepts the application, records it with status
`IN_PROGRESS`, and returns an application reference so the application can be tracked.

**Why this priority**: Accepting applications is the entry point of the lending process; nothing
downstream (review, approval, disbursement) can happen without it.

**Independent Test**: Submit one complete, valid application and confirm the response contains an
application reference and status `IN_PROGRESS`, and that the stored application contains exactly
the submitted customer, collateral, and loan data.

**Acceptance Scenarios**:

1. **Given** an authorized client, **When** it submits an application where every field is valid
   (e.g., amount 15,000 and tenure 24 months), **Then** the system stores the application with
   status `IN_PROGRESS` and responds with a success result containing the application reference,
   the status, and the submission time.
2. **Given** a successfully submitted application, **When** the stored record is inspected,
   **Then** it contains all submitted customer, address, collateral, and loan values unchanged,
   plus who submitted it and when.
3. **Given** a valid application with the boundary values amount = 100, tenure = 3 months, and
   tenure = 48 months, **When** each is submitted, **Then** each is accepted.

---

### User Story 2 - Reject invalid applications with clear, complete feedback (Priority: P2)

When an application contains missing or invalid data, the system rejects it without storing
anything and tells the client every field that is wrong and why, so the client can correct all
problems in a single retry.

**Why this priority**: Prevents bad data from entering the lending pipeline and avoids
back-and-forth correction cycles for applicants.

**Independent Test**: Submit applications that each violate one or more rules and confirm each is
rejected, nothing is stored, and the response lists every invalid field with a reason.

**Acceptance Scenarios**:

1. **Given** an application with amount 15,050 (not a multiple of 100), **When** it is submitted,
   **Then** it is rejected, nothing is stored, and the response identifies the amount field and
   the multiple-of-100 rule.
2. **Given** an application with tenure 2 or 49 months, **When** it is submitted, **Then** it is
   rejected with an error identifying the tenure field and the allowed range 3–48.
3. **Given** an application with an invalid email, a missing city, and a manufacturing year in
   the future, **When** it is submitted, **Then** it is rejected and the response lists all three
   problems together.
4. **Given** a request that is not well-formed (unreadable body, wrong data types), **When** it is
   submitted, **Then** it is rejected with an error explaining the request could not be read.

---

### User Story 3 - Safe retry without duplicate applications (Priority: P3)

If a client does not receive a response (e.g., network timeout) and resends the same submission
with the same request key, the system returns the original result instead of creating a second
application.

**Why this priority**: Duplicate applications create confusion for operations and risk duplicate
loans for the same customer; however, the feature delivers value even before this is in place.

**Independent Test**: Submit the same valid application twice with the same request key and
confirm only one application exists and both responses carry the same application reference.

**Acceptance Scenarios**:

1. **Given** a successful submission made with request key K, **When** the identical submission
   is sent again with key K, **Then** the system returns the original application reference and
   status and does not create a new application.
2. **Given** a successful submission made with request key K, **When** a different submission
   body is sent with the same key K, **Then** the system rejects it as a key conflict and creates
   nothing.

---

### Edge Cases

- Amount of 0, negative, or non-whole amount (e.g., 1,000.50) → rejected (must be a positive
  multiple of 100).
- Tenure given as a non-whole number (e.g., 12.5) → rejected.
- Birth date in the future, not a real calendar date (e.g., 2001-02-30), or making the customer
  younger than the minimum age → rejected.
- Monthly income of 0 or negative → rejected.
- Manufacturing year later than next calendar year or earlier than 1900 → rejected.
- Leading/trailing whitespace in text fields is trimmed before validation; a field that is empty
  after trimming is treated as missing.
- License plate is compared case-insensitively and ignoring spaces (e.g., "b 1234 xyz" and
  "B1234XYZ" are the same plate).
- Unknown extra fields in the request are rejected rather than silently ignored.
- Unauthenticated or unauthorized request → rejected, nothing stored.
- A storage failure mid-submission → no partial application is stored (customer, collateral, and
  loan data are saved all together or not at all), and the client receives an error it can retry.
- Same customer (same ID number) submits again while an earlier application is still
  `IN_PROGRESS` → [NEEDS CLARIFICATION: Should a customer be allowed to have more than one
  `IN_PROGRESS` application at a time, or should a second submission be rejected?]

## Requirements *(mandatory)*

### Functional Requirements

**Submission**

- **FR-001**: System MUST provide a way for an authenticated, authorized client to submit a loan
  application containing customer data, car collateral data, and proposed loan data in a single
  request.
- **FR-002**: System MUST validate the entire application before storing anything; an application
  is stored only if every field passes validation.
- **FR-003**: On successful validation, System MUST store the application with status
  `IN_PROGRESS` and return the application reference, status, and submission time.
- **FR-004**: System MUST store the customer, collateral, and loan data of one application
  together atomically — either all are saved or none are.
- **FR-005**: System MUST record who submitted the application and when.
- **FR-006**: A retried submission with the same request key and identical content MUST return the
  original result without creating a new application; the same key with different content MUST be
  rejected as a conflict.

**Customer data validation** (all fields required)

- **FR-010**: Full name — 1 to 100 characters after trimming.
- **FR-011**: ID number — 1 to 30 characters, letters and digits only.
- **FR-012**: Address — street (1–200 characters), city (1–100 characters), and zip code
  (3–10 characters, letters, digits, spaces, or hyphens) are all required.
- **FR-013**: Birth date — a valid calendar date in `YYYY-MM-DD` form, not in the future, and the
  customer MUST be at least [NEEDS CLARIFICATION: minimum borrower age — 18, 21, or another age?
  Is there also a maximum age?] years old on the submission date.
- **FR-014**: Phone number — international format: a leading "+" followed by 8 to 15 digits.
- **FR-015**: Email — a syntactically valid email address of at most 254 characters.
- **FR-016**: Current monthly income — a positive amount.

**Collateral (car) validation** (all fields required)

- **FR-020**: Brand — 1 to 50 characters.
- **FR-021**: Model — 1 to 50 characters.
- **FR-022**: Manufacturing year — a whole number from 1900 up to and including the next calendar
  year.
- **FR-023**: License plate — 1 to 15 characters, letters, digits, spaces, or hyphens; stored in
  normalized form (uppercase, without spaces or hyphens).

**Proposed loan validation** (all fields required)

- **FR-030**: Amount — a positive whole number that is a multiple of 100, and within
  [NEEDS CLARIFICATION: minimum and maximum loan amount — is any multiple of 100 from 100 upward
  acceptable, or are there product limits such as 1,000 to 500,000?].
- **FR-031**: Tenure — a whole number of months from 3 to 48 inclusive.

**Errors**

- **FR-040**: When validation fails, System MUST reject the application, store nothing, and return
  every failing field with a human-readable reason in a single response.
- **FR-041**: Errors MUST distinguish between: invalid input, unauthenticated/unauthorized caller,
  request-key conflict, and temporary system failure (retryable).

**Data protection and record keeping**

- **FR-050**: Personal data (name, ID number, address, birth date, phone, email, income) MUST NOT
  appear in system logs.
- **FR-051**: Applications MUST NOT be physically deleted; every stored record keeps creation,
  update, and deletion audit information as required by the project constitution.
- **FR-052**: All timestamps returned by the system (e.g., submission time) MUST use the project's
  standard format, e.g., `2026-05-05T14:30:00+07:00`.

### Key Entities *(include if feature involves data)*

- **Customer**: The person applying. Attributes: full name, ID number, birth date, phone number,
  email, current monthly income. Has one address at the time of application.
- **Address**: The customer's address: street, city, zip code.
- **Collateral (Car)**: The vehicle pledged against the loan. Attributes: brand, model,
  manufacturing year, license plate. Belongs to exactly one loan application.
- **Loan Application**: The request for a loan. Attributes: application reference, amount, tenure
  in months, status (initially `IN_PROGRESS`), submission time, submitted by. Links one customer
  and one car collateral.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of applications that satisfy every validation rule are stored with status
  `IN_PROGRESS` and receive an application reference.
- **SC-002**: 100% of applications that violate any rule are rejected with nothing stored, and the
  response lists every violated field (verified against a test set covering each rule).
- **SC-003**: A client can submit an application and receive a confirmation in under 1 second for
  95% of submissions under normal load.
- **SC-004**: Retrying a submission with the same request key never produces more than one
  application (0 duplicates across repeated retry tests).
- **SC-005**: An applicant with several invalid fields can correct all problems in one retry,
  because all errors are reported at once.
- **SC-006**: 0 occurrences of customer personal data in system logs during testing.

## Assumptions

- Callers are authenticated clients of this system (e.g., a customer-facing app or a loan
  officer's tool); the authentication mechanism follows the project's standard and is not defined
  by this feature.
- All monetary values (loan amount, monthly income) are in a single currency for the whole
  system; multi-currency support is out of scope.
- "Multiple of 100" refers to the whole loan amount in the system's currency (e.g., 100, 200,
  15,000), with no fractional amounts.
- No country-specific format rules are applied to the ID number, zip code, or license plate;
  only the general rules above are enforced.
- Each application captures the customer's details as submitted; matching applications to an
  existing customer by ID number is permitted, but changing a customer's previously submitted
  details is out of scope.
- Only creating the application is in scope. Viewing, updating, cancelling, credit assessment,
  approval/rejection, and later status changes are out of scope for this feature.
- Birth date is a calendar date (not a timestamp) and therefore uses the `YYYY-MM-DD` form.
- Exactly one car is pledged per application; other collateral types and multiple cars are out
  of scope.
