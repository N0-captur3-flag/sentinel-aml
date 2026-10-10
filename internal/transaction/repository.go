package transaction

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrDuplicate           = errors.New("duplicate transaction trace id")
	ErrInvalidInput        = errors.New("invalid transaction input")
)

var allowedDirections = map[string]struct{}{
	"inbound":  {},
	"outbound": {},
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

type Transaction struct {
	ID                      int64
	AccountID               int64
	CounterpartyName        string
	CounterpartyCountry     string
	CounterpartyFingerprint string
	Amount                  string
	Currency                string
	Direction               string
	Purpose                 string
	TraceID                 string
	CreatedAt               time.Time
	RiskScore               int
}

type CreateParams struct {
	OwnerUserID             int64
	AccountID               int64
	CounterpartyName        string
	CounterpartyCountry     string
	CounterpartyFingerprint string
	Amount                  string
	Currency                string
	Direction               string
	Purpose                 string
	TraceID                 string
}

type RiskScoreParams struct {
	OwnerUserID   int64
	TransactionID int64
	RiskScore     int
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

func (r *Repository) Create(ctx context.Context, params CreateParams) (Transaction, error) {
	if err := ctx.Err(); err != nil {
		return Transaction{}, err
	}
	if err := validateCreateParams(params); err != nil {
		return Transaction{}, err
	}
	params = normalizeCreateParams(params)

	var transaction Transaction
	err := r.db.QueryRowContext(ctx, `
INSERT INTO transactions (account_id, counterparty_name, counterparty_country, counterparty_fingerprint, amount, currency, direction, purpose, trace_id)
SELECT a.id, $3, $4, $5, $6, $7, $8, $9, $10
FROM accounts a
JOIN customers c ON c.id = a.customer_id
WHERE c.owner_user_id = $1 AND a.id = $2
RETURNING id, account_id, counterparty_name, counterparty_country, counterparty_fingerprint, amount::text, currency, direction, purpose, trace_id, created_at, risk_score`,
		params.OwnerUserID,
		params.AccountID,
		params.CounterpartyName,
		params.CounterpartyCountry,
		params.CounterpartyFingerprint,
		params.Amount,
		params.Currency,
		params.Direction,
		params.Purpose,
		params.TraceID,
	).Scan(&transaction.ID, &transaction.AccountID, &transaction.CounterpartyName, &transaction.CounterpartyCountry, &transaction.CounterpartyFingerprint, &transaction.Amount, &transaction.Currency, &transaction.Direction, &transaction.Purpose, &transaction.TraceID, &transaction.CreatedAt, &transaction.RiskScore)
	if errors.Is(err, sql.ErrNoRows) {
		return Transaction{}, ErrTransactionNotFound
	}
	if isUniqueViolation(err) {
		return Transaction{}, ErrDuplicate
	}
	if err != nil {
		return Transaction{}, fmt.Errorf("create transaction: %w", err)
	}
	return transaction, nil
}

func (r *Repository) GetByOwner(ctx context.Context, ownerUserID, transactionID int64) (Transaction, error) {
	if err := ctx.Err(); err != nil {
		return Transaction{}, err
	}
	if ownerUserID <= 0 || transactionID <= 0 {
		return Transaction{}, ErrInvalidInput
	}

	var transaction Transaction
	err := r.db.QueryRowContext(ctx, `
SELECT t.id, t.account_id, t.counterparty_name, t.counterparty_country, t.counterparty_fingerprint, t.amount::text, t.currency, t.direction, t.purpose, t.trace_id, t.created_at, t.risk_score
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN customers c ON c.id = a.customer_id
WHERE c.owner_user_id = $1 AND t.id = $2`,
		ownerUserID,
		transactionID,
	).Scan(&transaction.ID, &transaction.AccountID, &transaction.CounterpartyName, &transaction.CounterpartyCountry, &transaction.CounterpartyFingerprint, &transaction.Amount, &transaction.Currency, &transaction.Direction, &transaction.Purpose, &transaction.TraceID, &transaction.CreatedAt, &transaction.RiskScore)
	if errors.Is(err, sql.ErrNoRows) {
		return Transaction{}, ErrTransactionNotFound
	}
	if err != nil {
		return Transaction{}, fmt.Errorf("get transaction by owner: %w", err)
	}
	return transaction, nil
}

func (r *Repository) UpdateRiskScoreByOwner(ctx context.Context, params RiskScoreParams) (Transaction, error) {
	if err := ctx.Err(); err != nil {
		return Transaction{}, err
	}
	if params.OwnerUserID <= 0 || params.TransactionID <= 0 || params.RiskScore < 0 || params.RiskScore > 100 {
		return Transaction{}, ErrInvalidInput
	}

	var transaction Transaction
	err := r.db.QueryRowContext(ctx, `
UPDATE transactions t
SET risk_score = $3
FROM accounts a
JOIN customers c ON c.id = a.customer_id
WHERE t.account_id = a.id AND c.owner_user_id = $1 AND t.id = $2
RETURNING t.id, t.account_id, t.counterparty_name, t.counterparty_country, t.counterparty_fingerprint, t.amount::text, t.currency, t.direction, t.purpose, t.trace_id, t.created_at, t.risk_score`,
		params.OwnerUserID,
		params.TransactionID,
		params.RiskScore,
	).Scan(&transaction.ID, &transaction.AccountID, &transaction.CounterpartyName, &transaction.CounterpartyCountry, &transaction.CounterpartyFingerprint, &transaction.Amount, &transaction.Currency, &transaction.Direction, &transaction.Purpose, &transaction.TraceID, &transaction.CreatedAt, &transaction.RiskScore)
	if errors.Is(err, sql.ErrNoRows) {
		return Transaction{}, ErrTransactionNotFound
	}
	if err != nil {
		return Transaction{}, fmt.Errorf("update transaction risk score by owner: %w", err)
	}
	return transaction, nil
}

func (r *Repository) ListByOwner(ctx context.Context, ownerUserID int64) ([]Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ownerUserID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT t.id, t.account_id, t.counterparty_name, t.counterparty_country, t.counterparty_fingerprint, t.amount::text, t.currency, t.direction, t.purpose, t.trace_id, t.created_at, t.risk_score
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN customers c ON c.id = a.customer_id
WHERE c.owner_user_id = $1
ORDER BY t.id`,
		ownerUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("list transactions by owner: %w", err)
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var transaction Transaction
		if err := rows.Scan(&transaction.ID, &transaction.AccountID, &transaction.CounterpartyName, &transaction.CounterpartyCountry, &transaction.CounterpartyFingerprint, &transaction.Amount, &transaction.Currency, &transaction.Direction, &transaction.Purpose, &transaction.TraceID, &transaction.CreatedAt, &transaction.RiskScore); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		transactions = append(transactions, transaction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}
	return transactions, nil
}

func (r *Repository) ListByCustomerOwner(ctx context.Context, ownerUserID, customerID int64) ([]Transaction, error) {
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
		return nil, ErrTransactionNotFound
	} else if err != nil {
		return nil, fmt.Errorf("verify customer owner for transactions: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT t.id, t.account_id, t.counterparty_name, t.counterparty_country, t.counterparty_fingerprint, t.amount::text, t.currency, t.direction, t.purpose, t.trace_id, t.created_at, t.risk_score
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN customers c ON c.id = a.customer_id
WHERE c.owner_user_id = $1 AND c.id = $2
ORDER BY t.id`,
		ownerUserID,
		customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list transactions by customer owner: %w", err)
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var transaction Transaction
		if err := rows.Scan(&transaction.ID, &transaction.AccountID, &transaction.CounterpartyName, &transaction.CounterpartyCountry, &transaction.CounterpartyFingerprint, &transaction.Amount, &transaction.Currency, &transaction.Direction, &transaction.Purpose, &transaction.TraceID, &transaction.CreatedAt, &transaction.RiskScore); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		transactions = append(transactions, transaction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}
	return transactions, nil
}

func validateCreateParams(params CreateParams) error {
	if params.OwnerUserID <= 0 || params.AccountID <= 0 {
		return ErrInvalidInput
	}
	if strings.TrimSpace(params.CounterpartyName) == "" ||
		strings.TrimSpace(params.CounterpartyCountry) == "" ||
		strings.TrimSpace(params.CounterpartyFingerprint) == "" ||
		!validCurrency(params.Currency) ||
		strings.TrimSpace(params.Purpose) == "" ||
		strings.TrimSpace(params.TraceID) == "" {
		return ErrInvalidInput
	}
	if err := ValidateAmount(params.Amount); err != nil {
		return err
	}
	if _, ok := allowedDirections[params.Direction]; !ok {
		return ErrInvalidInput
	}
	return nil
}

func ValidateAmount(amount string) error {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return ErrInvalidInput
	}

	parts := strings.Split(amount, ".")
	if len(parts) > 2 || parts[0] == "" {
		return ErrInvalidInput
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) == 0 || len(fraction) > 2 {
			return ErrInvalidInput
		}
	}
	if !allDigits(parts[0]) || (fraction != "" && !allDigits(fraction)) {
		return ErrInvalidInput
	}
	if len(strings.TrimLeft(parts[0], "0")) > 16 {
		return ErrInvalidInput
	}
	if strings.Trim(parts[0]+fraction, "0") == "" {
		return ErrInvalidInput
	}
	return nil
}

func normalizeCreateParams(params CreateParams) CreateParams {
	params.CounterpartyName = strings.TrimSpace(params.CounterpartyName)
	params.CounterpartyCountry = strings.TrimSpace(params.CounterpartyCountry)
	params.CounterpartyFingerprint = strings.TrimSpace(params.CounterpartyFingerprint)
	params.Amount = strings.TrimSpace(params.Amount)
	params.Currency = strings.TrimSpace(params.Currency)
	params.Purpose = strings.TrimSpace(params.Purpose)
	params.TraceID = strings.TrimSpace(params.TraceID)
	return params
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

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
