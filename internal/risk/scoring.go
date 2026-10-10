package risk

import (
	"context"
	"errors"
	"strconv"
	"strings"

	alertrepo "sentinel-aml/internal/alert"
	txn "sentinel-aml/internal/transaction"
)

const (
	RuleDeterministicThreshold = "DETERMINISTIC_RISK_THRESHOLD"
	SuspiciousThreshold        = 70
)

var ErrInvalidInput = errors.New("invalid risk input")

type TransactionRepository interface {
	GetByOwner(context.Context, int64, int64) (txn.Transaction, error)
	UpdateRiskScoreByOwner(context.Context, txn.RiskScoreParams) (txn.Transaction, error)
}

type AlertRepository interface {
	Create(context.Context, alertrepo.CreateParams) (alertrepo.Alert, error)
}

type Service struct {
	transactions TransactionRepository
	alerts       AlertRepository
}

type Input struct {
	Amount                  string
	Country                 string
	CounterpartyFingerprint string
	TypologyHint            string
}

type Decision struct {
	Transaction  txn.Transaction
	Alert        alertrepo.Alert
	RiskScore    int
	AmountBucket string
	TypologyHint string
	RuleCode     string
	Suspicious   bool
	AlertCreated bool
}

func NewService(transactions TransactionRepository, alerts AlertRepository) *Service {
	return &Service{transactions: transactions, alerts: alerts}
}

func (s *Service) ScoreTransaction(ctx context.Context, ownerUserID, transactionID int64) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	if s == nil || s.transactions == nil || s.alerts == nil || ownerUserID <= 0 || transactionID <= 0 {
		return Decision{}, ErrInvalidInput
	}

	transaction, err := s.transactions.GetByOwner(ctx, ownerUserID, transactionID)
	if err != nil {
		return Decision{}, err
	}
	decision, err := Score(Input{
		Amount:                  transaction.Amount,
		Country:                 transaction.CounterpartyCountry,
		CounterpartyFingerprint: transaction.CounterpartyFingerprint,
	})
	if err != nil {
		return Decision{}, err
	}

	updatedTransaction, err := s.transactions.UpdateRiskScoreByOwner(ctx, txn.RiskScoreParams{
		OwnerUserID:   ownerUserID,
		TransactionID: transaction.ID,
		RiskScore:     decision.RiskScore,
	})
	if err != nil {
		return Decision{}, err
	}
	decision.Transaction = updatedTransaction

	if !decision.Suspicious {
		return decision, nil
	}
	alert, err := s.alerts.Create(ctx, alertrepo.CreateParams{
		OwnerUserID:   ownerUserID,
		TransactionID: transaction.ID,
		RuleCode:      decision.RuleCode,
		RiskScore:     decision.RiskScore,
		TypologyHint:  decision.TypologyHint,
	})
	if errors.Is(err, alertrepo.ErrDuplicate) {
		return decision, nil
	}
	if err != nil {
		return Decision{}, err
	}
	decision.Alert = alert
	decision.AlertCreated = true
	return decision, nil
}

func Score(input Input) (Decision, error) {
	if err := txn.ValidateAmount(input.Amount); err != nil {
		return Decision{}, ErrInvalidInput
	}
	bucket, amountWeight, err := amountBucket(input.Amount)
	if err != nil {
		return Decision{}, err
	}

	country := strings.ToUpper(strings.TrimSpace(input.Country))
	fingerprint := strings.ToLower(strings.TrimSpace(input.CounterpartyFingerprint))
	typology := strings.TrimSpace(input.TypologyHint)
	if typology == "" {
		typology = deriveTypology(bucket, country, fingerprint)
	}

	score := amountWeight + countryWeight(country) + fingerprintWeight(fingerprint) + typologyWeight(typology)
	if score > 100 {
		score = 100
	}

	decision := Decision{
		RiskScore:    score,
		AmountBucket: bucket,
		TypologyHint: typology,
		Suspicious:   score >= SuspiciousThreshold,
	}
	if decision.Suspicious {
		decision.RuleCode = RuleDeterministicThreshold
	}
	return decision, nil
}

func amountBucket(amount string) (string, int, error) {
	cents, err := amountCents(amount)
	if err != nil {
		return "", 0, err
	}
	switch {
	case cents < 100000:
		return "small", 5, nil
	case cents < 1000000:
		return "medium", 20, nil
	case cents < 5000000:
		return "large", 45, nil
	default:
		return "very_large", 65, nil
	}
}

func amountCents(amount string) (int64, error) {
	amount = strings.TrimSpace(amount)
	parts := strings.Split(amount, ".")
	whole := parts[0]
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	cents, err := strconv.ParseInt(whole+fraction, 10, 64)
	if err != nil {
		return 0, ErrInvalidInput
	}
	return cents, nil
}

func deriveTypology(bucket, country, fingerprint string) string {
	if countryWeight(country) > 0 {
		return "high-risk-country"
	}
	if fingerprintKeywordWeight(fingerprint) > 0 {
		return "counterparty-watchlist"
	}
	if bucket == "large" || bucket == "very_large" {
		return "large-value-transfer"
	}
	return "standard-transaction"
}

func countryWeight(country string) int {
	switch country {
	case "IR", "KP", "SY":
		return 20
	default:
		return 0
	}
}

func fingerprintWeight(fingerprint string) int {
	if weight := fingerprintKeywordWeight(fingerprint); weight > 0 {
		return weight
	}
	var sum int
	for _, r := range fingerprint {
		sum += int(r)
	}
	return (sum % 3) * 5
}

func fingerprintKeywordWeight(fingerprint string) int {
	for _, marker := range []string{"sanction", "watchlist", "shell", "mule"} {
		if strings.Contains(fingerprint, marker) {
			return 20
		}
	}
	return 0
}

func typologyWeight(typology string) int {
	switch typology {
	case "high-risk-country":
		return 10
	case "counterparty-watchlist":
		return 20
	case "large-value-transfer":
		return 15
	case "standard-transaction":
		return 0
	default:
		return 5
	}
}
