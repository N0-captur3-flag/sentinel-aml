package casework

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrCaseNotFound      = errors.New("case not found")
	ErrNoteNotFound      = errors.New("case note not found")
	ErrDuplicate         = errors.New("duplicate case for alert")
	ErrInvalidInput      = errors.New("invalid casework input")
	ErrInvalidTransition = errors.New("invalid case status transition")
)

var allowedCaseStatuses = map[string]struct{}{
	"OPEN":         {},
	"UNDER_REVIEW": {},
	"ESCALATED":    {},
	"REPORTED":     {},
	"CLOSED":       {},
}

var allowedCaseTransitions = map[string]map[string]struct{}{
	"OPEN": {
		"UNDER_REVIEW": {},
		"ESCALATED":    {},
		"CLOSED":       {},
	},
	"UNDER_REVIEW": {
		"ESCALATED": {},
		"REPORTED":  {},
		"CLOSED":    {},
	},
	"ESCALATED": {
		"REPORTED": {},
		"CLOSED":   {},
	},
	"REPORTED": {
		"CLOSED": {},
	},
}

var allowedNoteVisibilities = map[string]struct{}{
	"private":  {},
	"internal": {},
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

type Case struct {
	ID            int64
	CustomerID    int64
	AlertID       int64
	OwnerUserID   int64
	Status        string
	PublicSummary string
	RiskScore     int
	TypologyHint  string
	DedupHint     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type DuplicateCandidate struct {
	CaseID        int64
	CustomerID    int64
	DisplayName   string
	Country       string
	RiskLevel     string
	Status        string
	PublicSummary string
	RiskScore     int
	TypologyHint  string
	DedupHint     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CaseNote struct {
	ID           int64
	CaseID       int64
	AuthorUserID int64
	Visibility   string
	Body         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CreateCaseParams struct {
	OwnerUserID   int64
	AlertID       int64
	PublicSummary string
	Status        string
	DedupHint     string
}

type UpdateCaseStatusParams struct {
	OwnerUserID int64
	CaseID      int64
	Status      string
}

type CreateNoteParams struct {
	OwnerUserID  int64
	CaseID       int64
	AuthorUserID int64
	Visibility   string
	Body         string
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

func (r *Repository) CreateFromAlert(ctx context.Context, params CreateCaseParams) (Case, error) {
	if err := ctx.Err(); err != nil {
		return Case{}, err
	}
	if err := validateCreateCaseParams(params); err != nil {
		return Case{}, err
	}
	params = normalizeCreateCaseParams(params)

	var c Case
	err := r.db.QueryRowContext(ctx, `
INSERT INTO cases (customer_id, alert_id, owner_user_id, status, public_summary, risk_score, typology_hint, dedup_hint)
SELECT al.customer_id, al.id, cust.owner_user_id, $3, $4, al.risk_score, al.typology_hint, $5
FROM alerts al
JOIN customers cust ON cust.id = al.customer_id
WHERE cust.owner_user_id = $1 AND al.id = $2
RETURNING id, customer_id, alert_id, owner_user_id, status, public_summary, risk_score, typology_hint, dedup_hint, created_at, updated_at`,
		params.OwnerUserID,
		params.AlertID,
		params.Status,
		params.PublicSummary,
		params.DedupHint,
	).Scan(&c.ID, &c.CustomerID, &c.AlertID, &c.OwnerUserID, &c.Status, &c.PublicSummary, &c.RiskScore, &c.TypologyHint, &c.DedupHint, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Case{}, ErrCaseNotFound
	}
	if isUniqueViolation(err) {
		return Case{}, ErrDuplicate
	}
	if err != nil {
		return Case{}, fmt.Errorf("create case from alert: %w", err)
	}
	return c, nil
}

func (r *Repository) GetByOwner(ctx context.Context, ownerUserID, caseID int64) (Case, error) {
	if err := ctx.Err(); err != nil {
		return Case{}, err
	}
	if ownerUserID <= 0 || caseID <= 0 {
		return Case{}, ErrInvalidInput
	}

	var c Case
	err := r.db.QueryRowContext(ctx, `
SELECT id, customer_id, alert_id, owner_user_id, status, public_summary, risk_score, typology_hint, dedup_hint, created_at, updated_at
FROM cases
WHERE owner_user_id = $1 AND id = $2`,
		ownerUserID,
		caseID,
	).Scan(&c.ID, &c.CustomerID, &c.AlertID, &c.OwnerUserID, &c.Status, &c.PublicSummary, &c.RiskScore, &c.TypologyHint, &c.DedupHint, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Case{}, ErrCaseNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("get case by owner: %w", err)
	}
	return c, nil
}

func (r *Repository) ListByOwner(ctx context.Context, ownerUserID int64) ([]Case, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ownerUserID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT id, customer_id, alert_id, owner_user_id, status, public_summary, risk_score, typology_hint, dedup_hint, created_at, updated_at
FROM cases
WHERE owner_user_id = $1
ORDER BY id`,
		ownerUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("list cases by owner: %w", err)
	}
	defer rows.Close()

	var cases []Case
	for rows.Next() {
		var c Case
		if err := rows.Scan(&c.ID, &c.CustomerID, &c.AlertID, &c.OwnerUserID, &c.Status, &c.PublicSummary, &c.RiskScore, &c.TypologyHint, &c.DedupHint, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan case: %w", err)
		}
		cases = append(cases, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cases: %w", err)
	}
	return cases, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, params UpdateCaseStatusParams) (Case, error) {
	if err := ctx.Err(); err != nil {
		return Case{}, err
	}
	if params.OwnerUserID <= 0 || params.CaseID <= 0 || !validCaseStatus(params.Status) {
		return Case{}, ErrInvalidInput
	}

	current, err := r.GetByOwner(ctx, params.OwnerUserID, params.CaseID)
	if err != nil {
		return Case{}, err
	}
	if !validCaseTransition(current.Status, params.Status) {
		return Case{}, ErrInvalidTransition
	}

	var c Case
	err = r.db.QueryRowContext(ctx, `
UPDATE cases
SET status = $3, updated_at = now()
WHERE owner_user_id = $1 AND id = $2
RETURNING id, customer_id, alert_id, owner_user_id, status, public_summary, risk_score, typology_hint, dedup_hint, created_at, updated_at`,
		params.OwnerUserID,
		params.CaseID,
		params.Status,
	).Scan(&c.ID, &c.CustomerID, &c.AlertID, &c.OwnerUserID, &c.Status, &c.PublicSummary, &c.RiskScore, &c.TypologyHint, &c.DedupHint, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Case{}, ErrCaseNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("update case status: %w", err)
	}
	return c, nil
}

func (r *Repository) FindDuplicateCandidatesByOwner(ctx context.Context, ownerUserID, sourceCaseID int64) ([]DuplicateCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ownerUserID <= 0 || sourceCaseID <= 0 {
		return nil, ErrInvalidInput
	}

	var sourceCustomerID int64
	var dedupHint string
	if err := r.db.QueryRowContext(ctx, `
SELECT customer_id, dedup_hint
FROM cases
WHERE owner_user_id = $1 AND id = $2`,
		ownerUserID,
		sourceCaseID,
	).Scan(&sourceCustomerID, &dedupHint); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCaseNotFound
	} else if err != nil {
		return nil, fmt.Errorf("verify source case for duplicate candidates: %w", err)
	}

	dedupHint = normalizeDedupHint(dedupHint)
	if dedupHint == "" {
		return []DuplicateCandidate{}, nil
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT c.id, c.customer_id, cust.display_name, cust.country, cust.risk_level, c.status, c.public_summary, c.risk_score, c.typology_hint, c.dedup_hint, c.created_at, c.updated_at
FROM cases c
JOIN customers cust ON cust.id = c.customer_id
WHERE c.dedup_hint = $1 AND c.id <> $2
ORDER BY c.id`,
		dedupHint,
		sourceCaseID,
	)
	if err != nil {
		return nil, fmt.Errorf("find duplicate case candidates: %w", err)
	}
	defer rows.Close()

	var candidates []DuplicateCandidate
	for rows.Next() {
		var candidate DuplicateCandidate
		if err := rows.Scan(
			&candidate.CaseID,
			&candidate.CustomerID,
			&candidate.DisplayName,
			&candidate.Country,
			&candidate.RiskLevel,
			&candidate.Status,
			&candidate.PublicSummary,
			&candidate.RiskScore,
			&candidate.TypologyHint,
			&candidate.DedupHint,
			&candidate.CreatedAt,
			&candidate.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan duplicate case candidate: %w", err)
		}
		candidate.DedupHint = normalizeDedupHint(candidate.DedupHint)
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate duplicate case candidates: %w", err)
	}
	return candidates, nil
}

func (r *Repository) ResolveMatchEvidence(ctx context.Context, request MatchEvidenceRequest) ([]CaseSimilarityEvidence, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.OwnerUserID <= 0 || request.SourceCaseID <= 0 || request.CandidateCaseID <= 0 || request.SourceCaseID == request.CandidateCaseID {
		return nil, ErrInvalidInput
	}

	var dedupHint string
	if err := r.db.QueryRowContext(ctx, `
SELECT dedup_hint
FROM cases
WHERE owner_user_id = $1 AND id = $2`,
		request.OwnerUserID,
		request.SourceCaseID,
	).Scan(&dedupHint); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCaseNotFound
	} else if err != nil {
		return nil, fmt.Errorf("verify source case for match evidence: %w", err)
	}

	dedupHint = normalizeDedupHint(dedupHint)
	if dedupHint == "" {
		return []CaseSimilarityEvidence{}, nil
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT cn.id, cn.case_id, cn.author_user_id, cn.visibility, cn.body, cn.created_at, cn.updated_at
FROM cases c
JOIN case_notes cn ON cn.case_id = c.id
WHERE c.id = $1 AND c.dedup_hint = $2 AND cn.visibility = 'private'
ORDER BY cn.id`,
		request.CandidateCaseID,
		dedupHint,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve case similarity evidence: %w", err)
	}
	defer rows.Close()

	var evidence []CaseSimilarityEvidence
	for rows.Next() {
		var note CaseNote
		if err := rows.Scan(&note.ID, &note.CaseID, &note.AuthorUserID, &note.Visibility, &note.Body, &note.CreatedAt, &note.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan case similarity evidence: %w", err)
		}
		evidence = append(evidence, CaseSimilarityEvidence{
			Source:  MatchEvidenceSourceReviewContext,
			Excerpt: strings.TrimSpace(note.Body),
			Weight:  90,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate case similarity evidence: %w", err)
	}
	return evidence, nil
}

func (r *Repository) CreateNote(ctx context.Context, params CreateNoteParams) (CaseNote, error) {
	if err := ctx.Err(); err != nil {
		return CaseNote{}, err
	}
	if err := validateCreateNoteParams(params); err != nil {
		return CaseNote{}, err
	}
	params = normalizeCreateNoteParams(params)

	var note CaseNote
	err := r.db.QueryRowContext(ctx, `
INSERT INTO case_notes (case_id, author_user_id, visibility, body)
SELECT c.id, $3, $4, $5
FROM cases c
WHERE c.owner_user_id = $1 AND c.id = $2
RETURNING id, case_id, author_user_id, visibility, body, created_at, updated_at`,
		params.OwnerUserID,
		params.CaseID,
		params.AuthorUserID,
		params.Visibility,
		params.Body,
	).Scan(&note.ID, &note.CaseID, &note.AuthorUserID, &note.Visibility, &note.Body, &note.CreatedAt, &note.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CaseNote{}, ErrCaseNotFound
	}
	if err != nil {
		return CaseNote{}, fmt.Errorf("create case note: %w", err)
	}
	return note, nil
}

func (r *Repository) ListNotesByOwner(ctx context.Context, ownerUserID, caseID int64) ([]CaseNote, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ownerUserID <= 0 || caseID <= 0 {
		return nil, ErrInvalidInput
	}

	var ownedCaseID int64
	if err := r.db.QueryRowContext(ctx, `
SELECT id
FROM cases
WHERE owner_user_id = $1 AND id = $2`,
		ownerUserID,
		caseID,
	).Scan(&ownedCaseID); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCaseNotFound
	} else if err != nil {
		return nil, fmt.Errorf("verify case owner for notes: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT cn.id, cn.case_id, cn.author_user_id, cn.visibility, cn.body, cn.created_at, cn.updated_at
FROM case_notes cn
JOIN cases c ON c.id = cn.case_id
WHERE c.owner_user_id = $1 AND c.id = $2
ORDER BY cn.id`,
		ownerUserID,
		caseID,
	)
	if err != nil {
		return nil, fmt.Errorf("list case notes by owner: %w", err)
	}
	defer rows.Close()

	var notes []CaseNote
	for rows.Next() {
		var note CaseNote
		if err := rows.Scan(&note.ID, &note.CaseID, &note.AuthorUserID, &note.Visibility, &note.Body, &note.CreatedAt, &note.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan case note: %w", err)
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate case notes: %w", err)
	}
	return notes, nil
}

func validateCreateCaseParams(params CreateCaseParams) error {
	if params.OwnerUserID <= 0 || params.AlertID <= 0 {
		return ErrInvalidInput
	}
	status := params.Status
	if status == "" {
		status = "OPEN"
	}
	if status != "OPEN" || !validCaseStatus(status) {
		return ErrInvalidInput
	}
	return nil
}

func normalizeCreateCaseParams(params CreateCaseParams) CreateCaseParams {
	params.PublicSummary = strings.TrimSpace(params.PublicSummary)
	if params.Status == "" {
		params.Status = "OPEN"
	}
	params.DedupHint = normalizeDedupHint(params.DedupHint)
	return params
}

func validateCreateNoteParams(params CreateNoteParams) error {
	if params.OwnerUserID <= 0 || params.CaseID <= 0 || params.AuthorUserID <= 0 {
		return ErrInvalidInput
	}
	if params.AuthorUserID != params.OwnerUserID || strings.TrimSpace(params.Body) == "" {
		return ErrInvalidInput
	}
	visibility := params.Visibility
	if visibility == "" {
		visibility = "private"
	}
	if _, ok := allowedNoteVisibilities[visibility]; !ok {
		return ErrInvalidInput
	}
	return nil
}

func normalizeCreateNoteParams(params CreateNoteParams) CreateNoteParams {
	if params.Visibility == "" {
		params.Visibility = "private"
	}
	params.Body = strings.TrimSpace(params.Body)
	return params
}

func normalizeDedupHint(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validCaseStatus(status string) bool {
	_, ok := allowedCaseStatuses[status]
	return ok
}

func validCaseTransition(from, to string) bool {
	next, ok := allowedCaseTransitions[from]
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
