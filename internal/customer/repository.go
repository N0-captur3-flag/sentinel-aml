package customer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrCustomerNotFound = errors.New("customer not found")
	ErrDuplicate        = errors.New("duplicate customer registration number")
	ErrInvalidInput     = errors.New("invalid customer input")
)

var allowedCustomerTypes = map[string]struct{}{
	"individual":   {},
	"organization": {},
}

var allowedRiskLevels = map[string]struct{}{
	"low":    {},
	"medium": {},
	"high":   {},
}

var allowedStatuses = map[string]struct{}{
	"active":   {},
	"inactive": {},
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

type Customer struct {
	ID                 int64
	OwnerUserID        int64
	DisplayName        string
	CustomerType       string
	Country            string
	RegistrationNumber string
	RiskLevel          string
	EntityHint         string
	CreatedAt          time.Time
	Status             string
}

type CreateParams struct {
	OwnerUserID        int64
	DisplayName        string
	CustomerType       string
	Country            string
	RegistrationNumber string
	RiskLevel          string
	EntityHint         string
	Status             string
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

func (r *Repository) Create(ctx context.Context, params CreateParams) (Customer, error) {
	if err := ctx.Err(); err != nil {
		return Customer{}, err
	}
	if err := validateCreateParams(params); err != nil {
		return Customer{}, err
	}
	params = normalizeCreateParams(params)

	var customer Customer
	err := r.db.QueryRowContext(ctx, `
INSERT INTO customers (owner_user_id, display_name, customer_type, country, registration_number, risk_level, entity_hint, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, owner_user_id, display_name, customer_type, country, registration_number, risk_level, entity_hint, created_at, status`,
		params.OwnerUserID,
		params.DisplayName,
		params.CustomerType,
		params.Country,
		params.RegistrationNumber,
		params.RiskLevel,
		params.EntityHint,
		params.Status,
	).Scan(&customer.ID, &customer.OwnerUserID, &customer.DisplayName, &customer.CustomerType, &customer.Country, &customer.RegistrationNumber, &customer.RiskLevel, &customer.EntityHint, &customer.CreatedAt, &customer.Status)
	if isUniqueViolation(err) {
		return Customer{}, ErrDuplicate
	}
	if err != nil {
		return Customer{}, fmt.Errorf("create customer: %w", err)
	}
	return customer, nil
}

func (r *Repository) GetByOwner(ctx context.Context, ownerUserID, customerID int64) (Customer, error) {
	if err := ctx.Err(); err != nil {
		return Customer{}, err
	}
	if ownerUserID <= 0 || customerID <= 0 {
		return Customer{}, ErrInvalidInput
	}

	var customer Customer
	err := r.db.QueryRowContext(ctx, `
SELECT id, owner_user_id, display_name, customer_type, country, registration_number, risk_level, entity_hint, created_at, status
FROM customers
WHERE owner_user_id = $1 AND id = $2`,
		ownerUserID,
		customerID,
	).Scan(&customer.ID, &customer.OwnerUserID, &customer.DisplayName, &customer.CustomerType, &customer.Country, &customer.RegistrationNumber, &customer.RiskLevel, &customer.EntityHint, &customer.CreatedAt, &customer.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return Customer{}, ErrCustomerNotFound
	}
	if err != nil {
		return Customer{}, fmt.Errorf("get customer by owner: %w", err)
	}
	return customer, nil
}

func (r *Repository) ListByOwner(ctx context.Context, ownerUserID int64) ([]Customer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ownerUserID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT id, owner_user_id, display_name, customer_type, country, registration_number, risk_level, entity_hint, created_at, status
FROM customers
WHERE owner_user_id = $1
ORDER BY id`,
		ownerUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("list customers by owner: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		if err := rows.Scan(&customer.ID, &customer.OwnerUserID, &customer.DisplayName, &customer.CustomerType, &customer.Country, &customer.RegistrationNumber, &customer.RiskLevel, &customer.EntityHint, &customer.CreatedAt, &customer.Status); err != nil {
			return nil, fmt.Errorf("scan customer: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customers: %w", err)
	}
	return customers, nil
}

func validateCreateParams(params CreateParams) error {
	if params.OwnerUserID <= 0 {
		return ErrInvalidInput
	}
	if strings.TrimSpace(params.DisplayName) == "" ||
		strings.TrimSpace(params.CustomerType) == "" ||
		strings.TrimSpace(params.Country) == "" ||
		strings.TrimSpace(params.RegistrationNumber) == "" {
		return ErrInvalidInput
	}
	if _, ok := allowedCustomerTypes[params.CustomerType]; !ok {
		return ErrInvalidInput
	}
	riskLevel := params.RiskLevel
	if riskLevel == "" {
		riskLevel = "medium"
	}
	if _, ok := allowedRiskLevels[riskLevel]; !ok {
		return ErrInvalidInput
	}
	status := params.Status
	if status == "" {
		status = "active"
	}
	if _, ok := allowedStatuses[status]; !ok {
		return ErrInvalidInput
	}
	return nil
}

func normalizeCreateParams(params CreateParams) CreateParams {
	params.DisplayName = strings.TrimSpace(params.DisplayName)
	params.CustomerType = strings.TrimSpace(params.CustomerType)
	params.Country = strings.TrimSpace(params.Country)
	params.RegistrationNumber = strings.TrimSpace(params.RegistrationNumber)
	params.EntityHint = strings.TrimSpace(params.EntityHint)
	if params.RiskLevel == "" {
		params.RiskLevel = "medium"
	}
	if params.Status == "" {
		params.Status = "active"
	}
	return params
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
