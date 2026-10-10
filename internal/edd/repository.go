package edd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrEDDRecordNotFound = errors.New("edd record not found")
	ErrDuplicate         = errors.New("duplicate current edd record")
	ErrInvalidInput      = errors.New("invalid edd input")
)

var allowedReviewStatuses = map[string]struct{}{
	"DRAFT":     {},
	"IN_REVIEW": {},
	"APPROVED":  {},
	"REJECTED":  {},
}

type rowScanner interface {
	Scan(dest ...any) error
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) rowScanner
}

type Repository struct {
	db queryer
}

type EDDRecord struct {
	ID                  int64
	CustomerID          int64
	AnalystUserID       int64
	SourceOfFunds       string
	RiskRationale       string
	ConfidentialComment string
	ReviewStatus        string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type CreateParams struct {
	OwnerUserID         int64
	CustomerID          int64
	AnalystUserID       int64
	SourceOfFunds       string
	RiskRationale       string
	ConfidentialComment string
	ReviewStatus        string
}

type UpdateParams struct {
	OwnerUserID         int64
	CustomerID          int64
	AnalystUserID       int64
	SourceOfFunds       string
	RiskRationale       string
	ConfidentialComment string
	ReviewStatus        string
}

func NewRepository(db *sql.DB) *Repository {
	return newRepository(sqlDBAdapter{db: db})
}

func newRepository(db queryer) *Repository {
	return &Repository{db: db}
}

type sqlDBAdapter struct {
	db *sql.DB
}

func (a sqlDBAdapter) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return a.db.QueryRowContext(ctx, query, args...)
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (EDDRecord, error) {
	if err := ctx.Err(); err != nil {
		return EDDRecord{}, err
	}
	if err := validateCreateParams(params); err != nil {
		return EDDRecord{}, err
	}
	params = normalizeCreateParams(params)

	var record EDDRecord
	err := r.db.QueryRowContext(ctx, `
INSERT INTO edd_records (customer_id, analyst_user_id, source_of_funds, risk_rationale, confidential_comment, review_status)
SELECT c.id, $3, $4, $5, $6, $7
FROM customers c
WHERE c.owner_user_id = $1 AND c.id = $2
RETURNING id, customer_id, analyst_user_id, source_of_funds, risk_rationale, confidential_comment, review_status, created_at, updated_at`,
		params.OwnerUserID,
		params.CustomerID,
		params.AnalystUserID,
		params.SourceOfFunds,
		params.RiskRationale,
		params.ConfidentialComment,
		params.ReviewStatus,
	).Scan(&record.ID, &record.CustomerID, &record.AnalystUserID, &record.SourceOfFunds, &record.RiskRationale, &record.ConfidentialComment, &record.ReviewStatus, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EDDRecord{}, ErrEDDRecordNotFound
	}
	if isUniqueViolation(err) {
		return EDDRecord{}, ErrDuplicate
	}
	if err != nil {
		return EDDRecord{}, fmt.Errorf("create edd record: %w", err)
	}
	return record, nil
}

func (r *Repository) GetByOwnerCustomer(ctx context.Context, ownerUserID, customerID int64) (EDDRecord, error) {
	if err := ctx.Err(); err != nil {
		return EDDRecord{}, err
	}
	if ownerUserID <= 0 || customerID <= 0 {
		return EDDRecord{}, ErrInvalidInput
	}

	var record EDDRecord
	err := r.db.QueryRowContext(ctx, `
SELECT er.id, er.customer_id, er.analyst_user_id, er.source_of_funds, er.risk_rationale, er.confidential_comment, er.review_status, er.created_at, er.updated_at
FROM edd_records er
JOIN customers c ON c.id = er.customer_id
WHERE c.owner_user_id = $1 AND er.customer_id = $2`,
		ownerUserID,
		customerID,
	).Scan(&record.ID, &record.CustomerID, &record.AnalystUserID, &record.SourceOfFunds, &record.RiskRationale, &record.ConfidentialComment, &record.ReviewStatus, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EDDRecord{}, ErrEDDRecordNotFound
	}
	if err != nil {
		return EDDRecord{}, fmt.Errorf("get edd record by owner customer: %w", err)
	}
	return record, nil
}

func (r *Repository) UpdateByOwnerCustomer(ctx context.Context, params UpdateParams) (EDDRecord, error) {
	if err := ctx.Err(); err != nil {
		return EDDRecord{}, err
	}
	if err := validateUpdateParams(params); err != nil {
		return EDDRecord{}, err
	}
	params = normalizeUpdateParams(params)

	var record EDDRecord
	err := r.db.QueryRowContext(ctx, `
UPDATE edd_records er
SET analyst_user_id = $3,
    source_of_funds = $4,
    risk_rationale = $5,
    confidential_comment = $6,
    review_status = $7,
    updated_at = now()
FROM customers c
WHERE c.id = er.customer_id AND c.owner_user_id = $1 AND er.customer_id = $2
RETURNING er.id, er.customer_id, er.analyst_user_id, er.source_of_funds, er.risk_rationale, er.confidential_comment, er.review_status, er.created_at, er.updated_at`,
		params.OwnerUserID,
		params.CustomerID,
		params.AnalystUserID,
		params.SourceOfFunds,
		params.RiskRationale,
		params.ConfidentialComment,
		params.ReviewStatus,
	).Scan(&record.ID, &record.CustomerID, &record.AnalystUserID, &record.SourceOfFunds, &record.RiskRationale, &record.ConfidentialComment, &record.ReviewStatus, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EDDRecord{}, ErrEDDRecordNotFound
	}
	if err != nil {
		return EDDRecord{}, fmt.Errorf("update edd record by owner customer: %w", err)
	}
	return record, nil
}

func validateCreateParams(params CreateParams) error {
	if params.OwnerUserID <= 0 || params.CustomerID <= 0 || params.AnalystUserID <= 0 {
		return ErrInvalidInput
	}
	if params.AnalystUserID != params.OwnerUserID {
		return ErrInvalidInput
	}
	if strings.TrimSpace(params.SourceOfFunds) == "" ||
		strings.TrimSpace(params.RiskRationale) == "" ||
		strings.TrimSpace(params.ConfidentialComment) == "" {
		return ErrInvalidInput
	}
	status := params.ReviewStatus
	if status == "" {
		status = "DRAFT"
	}
	if !validReviewStatus(status) {
		return ErrInvalidInput
	}
	return nil
}

func normalizeCreateParams(params CreateParams) CreateParams {
	params.SourceOfFunds = strings.TrimSpace(params.SourceOfFunds)
	params.RiskRationale = strings.TrimSpace(params.RiskRationale)
	params.ConfidentialComment = strings.TrimSpace(params.ConfidentialComment)
	if params.ReviewStatus == "" {
		params.ReviewStatus = "DRAFT"
	}
	return params
}

func validateUpdateParams(params UpdateParams) error {
	return validateCreateParams(CreateParams(params))
}

func normalizeUpdateParams(params UpdateParams) UpdateParams {
	normalized := normalizeCreateParams(CreateParams(params))
	return UpdateParams(normalized)
}

func validReviewStatus(status string) bool {
	_, ok := allowedReviewStatuses[status]
	return ok
}

type sqlStateError interface {
	SQLState() string
}

func isUniqueViolation(err error) bool {
	var stateErr sqlStateError
	if errors.As(err, &stateErr) && stateErr.SQLState() == "23505" {
		return true
	}
	return strings.Contains(strings.ToLower(fmt.Sprint(err)), "duplicate key")
}
