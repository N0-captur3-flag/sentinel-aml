package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrAccountNotFound = errors.New("account not found")
	ErrDuplicate       = errors.New("duplicate account iban_like")
	ErrInvalidInput    = errors.New("invalid account input")
)

var allowedStatuses = map[string]struct{}{
	"active": {},
	"closed": {},
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

type Account struct {
	ID         int64
	CustomerID int64
	IBANLike   string
	Currency   string
	Status     string
	CreatedAt  time.Time
}

type CreateParams struct {
	OwnerUserID int64
	CustomerID  int64
	IBANLike    string
	Currency    string
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

func (r *Repository) Create(ctx context.Context, params CreateParams) (Account, error) {
	if err := ctx.Err(); err != nil {
		return Account{}, err
	}
	if err := validateCreateParams(params); err != nil {
		return Account{}, err
	}
	params = normalizeCreateParams(params)

	var account Account
	err := r.db.QueryRowContext(ctx, `
INSERT INTO accounts (customer_id, iban_like, currency, status)
SELECT c.id, $3, $4, $5
FROM customers c
WHERE c.owner_user_id = $1 AND c.id = $2
RETURNING id, customer_id, iban_like, currency, status, created_at`,
		params.OwnerUserID,
		params.CustomerID,
		params.IBANLike,
		params.Currency,
		params.Status,
	).Scan(&account.ID, &account.CustomerID, &account.IBANLike, &account.Currency, &account.Status, &account.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	if isUniqueViolation(err) {
		return Account{}, ErrDuplicate
	}
	if err != nil {
		return Account{}, fmt.Errorf("create account: %w", err)
	}
	return account, nil
}

func (r *Repository) GetByOwner(ctx context.Context, ownerUserID, accountID int64) (Account, error) {
	if err := ctx.Err(); err != nil {
		return Account{}, err
	}
	if ownerUserID <= 0 || accountID <= 0 {
		return Account{}, ErrInvalidInput
	}

	var account Account
	err := r.db.QueryRowContext(ctx, `
SELECT a.id, a.customer_id, a.iban_like, a.currency, a.status, a.created_at
FROM accounts a
JOIN customers c ON c.id = a.customer_id
WHERE c.owner_user_id = $1 AND a.id = $2`,
		ownerUserID,
		accountID,
	).Scan(&account.ID, &account.CustomerID, &account.IBANLike, &account.Currency, &account.Status, &account.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("get account by owner: %w", err)
	}
	return account, nil
}

func (r *Repository) ListByOwner(ctx context.Context, ownerUserID int64) ([]Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ownerUserID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT a.id, a.customer_id, a.iban_like, a.currency, a.status, a.created_at
FROM accounts a
JOIN customers c ON c.id = a.customer_id
WHERE c.owner_user_id = $1
ORDER BY a.id`,
		ownerUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("list accounts by owner: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var account Account
		if err := rows.Scan(&account.ID, &account.CustomerID, &account.IBANLike, &account.Currency, &account.Status, &account.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return accounts, nil
}

func (r *Repository) ListByCustomerOwner(ctx context.Context, ownerUserID, customerID int64) ([]Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ownerUserID <= 0 || customerID <= 0 {
		return nil, ErrInvalidInput
	}

	var ownedCustomerID int64
	if err := r.db.QueryRowContext(ctx, `
SELECT id
FROM customers
WHERE owner_user_id = $1 AND id = $2`,
		ownerUserID,
		customerID,
	).Scan(&ownedCustomerID); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAccountNotFound
	} else if err != nil {
		return nil, fmt.Errorf("verify customer owner for accounts: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT a.id, a.customer_id, a.iban_like, a.currency, a.status, a.created_at
FROM accounts a
JOIN customers c ON c.id = a.customer_id
WHERE c.owner_user_id = $1 AND c.id = $2
ORDER BY a.id`,
		ownerUserID,
		customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list accounts by customer owner: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var account Account
		if err := rows.Scan(&account.ID, &account.CustomerID, &account.IBANLike, &account.Currency, &account.Status, &account.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return accounts, nil
}

func validateCreateParams(params CreateParams) error {
	if params.OwnerUserID <= 0 || params.CustomerID <= 0 {
		return ErrInvalidInput
	}
	if strings.TrimSpace(params.IBANLike) == "" || !validCurrency(params.Currency) {
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
	params.IBANLike = strings.TrimSpace(params.IBANLike)
	params.Currency = strings.TrimSpace(params.Currency)
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

func validCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
