package alert

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrAlertNotFound     = errors.New("alert not found")
	ErrDuplicate         = errors.New("duplicate alert")
	ErrInvalidInput      = errors.New("invalid alert input")
	ErrInvalidTransition = errors.New("invalid alert status transition")
)

var allowedStatuses = map[string]struct{}{
	"NEW":         {},
	"TRIAGED":     {},
	"CASE_OPENED": {},
	"ESCALATED":   {},
	"CLOSED":      {},
}

var allowedTransitions = map[string]map[string]struct{}{
	"NEW": {
		"TRIAGED":   {},
		"ESCALATED": {},
		"CLOSED":    {},
	},
	"TRIAGED": {
		"CASE_OPENED": {},
		"ESCALATED":   {},
		"CLOSED":      {},
	},
	"CASE_OPENED": {
		"ESCALATED": {},
		"CLOSED":    {},
	},
	"ESCALATED": {
		"CLOSED": {},
	},
}

type rowScanner interface {
	Scan(dest ...any) error
}

type rowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) rowScanner
	QueryContext(ctx context.Context, query string, args ...any) (rowsScanner, error)
}

type Repository struct {
	db queryer
}

type Alert struct {
	ID             int64
	TransactionID  int64
	CustomerID     int64
	RuleCode       string
	RiskScore      int
	Status         string
	AssignedUserID sql.NullInt64
	TypologyHint   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateParams struct {
	OwnerUserID    int64
	TransactionID  int64
	RuleCode       string
	RiskScore      int
	Status         string
	AssignedUserID int64
	TypologyHint   string
}

type ClaimParams struct {
	OwnerUserID    int64
	AlertID        int64
	AssignedUserID int64
}

type TriageParams struct {
	OwnerUserID int64
	AlertID     int64
	Status      string
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

func (a sqlDBAdapter) QueryContext(ctx context.Context, query string, args ...any) (rowsScanner, error) {
	return a.db.QueryContext(ctx, query, args...)
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (Alert, error) {
	if err := ctx.Err(); err != nil {
		return Alert{}, err
	}
	if err := validateCreateParams(params); err != nil {
		return Alert{}, err
	}
	params = normalizeCreateParams(params)

	var alert Alert
	err := r.db.QueryRowContext(ctx, `
INSERT INTO alerts (transaction_id, customer_id, rule_code, risk_score, status, assigned_user_id, typology_hint)
SELECT t.id, c.id, $3, $4, $5, NULLIF($6, 0), $7
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN customers c ON c.id = a.customer_id
WHERE c.owner_user_id = $1 AND t.id = $2
RETURNING id, transaction_id, customer_id, rule_code, risk_score, status, assigned_user_id, typology_hint, created_at, updated_at`,
		params.OwnerUserID,
		params.TransactionID,
		params.RuleCode,
		params.RiskScore,
		params.Status,
		params.AssignedUserID,
		params.TypologyHint,
	).Scan(&alert.ID, &alert.TransactionID, &alert.CustomerID, &alert.RuleCode, &alert.RiskScore, &alert.Status, &alert.AssignedUserID, &alert.TypologyHint, &alert.CreatedAt, &alert.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Alert{}, ErrAlertNotFound
	}
	if isUniqueViolation(err) {
		return Alert{}, ErrDuplicate
	}
	if err != nil {
		return Alert{}, fmt.Errorf("create alert: %w", err)
	}
	return alert, nil
}

func (r *Repository) GetByOwner(ctx context.Context, ownerUserID, alertID int64) (Alert, error) {
	if err := ctx.Err(); err != nil {
		return Alert{}, err
	}
	if ownerUserID <= 0 || alertID <= 0 {
		return Alert{}, ErrInvalidInput
	}

	var alert Alert
	err := r.db.QueryRowContext(ctx, `
SELECT al.id, al.transaction_id, al.customer_id, al.rule_code, al.risk_score, al.status, al.assigned_user_id, al.typology_hint, al.created_at, al.updated_at
FROM alerts al
JOIN customers c ON c.id = al.customer_id
WHERE c.owner_user_id = $1 AND al.id = $2`,
		ownerUserID,
		alertID,
	).Scan(&alert.ID, &alert.TransactionID, &alert.CustomerID, &alert.RuleCode, &alert.RiskScore, &alert.Status, &alert.AssignedUserID, &alert.TypologyHint, &alert.CreatedAt, &alert.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Alert{}, ErrAlertNotFound
	}
	if err != nil {
		return Alert{}, fmt.Errorf("get alert by owner: %w", err)
	}
	return alert, nil
}

func (r *Repository) ListByOwner(ctx context.Context, ownerUserID int64) ([]Alert, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ownerUserID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT al.id, al.transaction_id, al.customer_id, al.rule_code, al.risk_score, al.status, al.assigned_user_id, al.typology_hint, al.created_at, al.updated_at
FROM alerts al
JOIN customers c ON c.id = al.customer_id
WHERE c.owner_user_id = $1
ORDER BY al.id`,
		ownerUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("list alerts by owner: %w", err)
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var alert Alert
		if err := rows.Scan(&alert.ID, &alert.TransactionID, &alert.CustomerID, &alert.RuleCode, &alert.RiskScore, &alert.Status, &alert.AssignedUserID, &alert.TypologyHint, &alert.CreatedAt, &alert.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		alerts = append(alerts, alert)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alerts: %w", err)
	}
	return alerts, nil
}

func (r *Repository) Claim(ctx context.Context, params ClaimParams) (Alert, error) {
	if err := ctx.Err(); err != nil {
		return Alert{}, err
	}
	if params.OwnerUserID <= 0 || params.AlertID <= 0 || params.AssignedUserID <= 0 || params.AssignedUserID != params.OwnerUserID {
		return Alert{}, ErrInvalidInput
	}

	var alert Alert
	err := r.db.QueryRowContext(ctx, `
UPDATE alerts al
SET assigned_user_id = $3, updated_at = now()
FROM customers c
WHERE c.id = al.customer_id AND c.owner_user_id = $1 AND al.id = $2
RETURNING al.id, al.transaction_id, al.customer_id, al.rule_code, al.risk_score, al.status, al.assigned_user_id, al.typology_hint, al.created_at, al.updated_at`,
		params.OwnerUserID,
		params.AlertID,
		params.AssignedUserID,
	).Scan(&alert.ID, &alert.TransactionID, &alert.CustomerID, &alert.RuleCode, &alert.RiskScore, &alert.Status, &alert.AssignedUserID, &alert.TypologyHint, &alert.CreatedAt, &alert.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Alert{}, ErrAlertNotFound
	}
	if err != nil {
		return Alert{}, fmt.Errorf("claim alert: %w", err)
	}
	return alert, nil
}

func (r *Repository) Triage(ctx context.Context, params TriageParams) (Alert, error) {
	if err := ctx.Err(); err != nil {
		return Alert{}, err
	}
	if params.OwnerUserID <= 0 || params.AlertID <= 0 || !validStatus(params.Status) {
		return Alert{}, ErrInvalidInput
	}

	current, err := r.GetByOwner(ctx, params.OwnerUserID, params.AlertID)
	if err != nil {
		return Alert{}, err
	}
	if !validTransition(current.Status, params.Status) {
		return Alert{}, ErrInvalidTransition
	}

	var alert Alert
	err = r.db.QueryRowContext(ctx, `
UPDATE alerts al
SET status = $3, updated_at = now()
FROM customers c
WHERE c.id = al.customer_id AND c.owner_user_id = $1 AND al.id = $2
RETURNING al.id, al.transaction_id, al.customer_id, al.rule_code, al.risk_score, al.status, al.assigned_user_id, al.typology_hint, al.created_at, al.updated_at`,
		params.OwnerUserID,
		params.AlertID,
		params.Status,
	).Scan(&alert.ID, &alert.TransactionID, &alert.CustomerID, &alert.RuleCode, &alert.RiskScore, &alert.Status, &alert.AssignedUserID, &alert.TypologyHint, &alert.CreatedAt, &alert.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Alert{}, ErrAlertNotFound
	}
	if err != nil {
		return Alert{}, fmt.Errorf("triage alert: %w", err)
	}
	return alert, nil
}

func validateCreateParams(params CreateParams) error {
	if params.OwnerUserID <= 0 || params.TransactionID <= 0 {
		return ErrInvalidInput
	}
	if strings.TrimSpace(params.RuleCode) == "" || params.RiskScore < 0 || params.RiskScore > 100 {
		return ErrInvalidInput
	}
	if params.AssignedUserID != 0 && params.AssignedUserID != params.OwnerUserID {
		return ErrInvalidInput
	}
	status := params.Status
	if status == "" {
		status = "NEW"
	}
	if !validStatus(status) || status != "NEW" {
		return ErrInvalidInput
	}
	return nil
}

func normalizeCreateParams(params CreateParams) CreateParams {
	params.RuleCode = strings.TrimSpace(params.RuleCode)
	if params.Status == "" {
		params.Status = "NEW"
	}
	params.TypologyHint = strings.TrimSpace(params.TypologyHint)
	return params
}

func validStatus(status string) bool {
	_, ok := allowedStatuses[status]
	return ok
}

func validTransition(from, to string) bool {
	next, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = next[to]
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
