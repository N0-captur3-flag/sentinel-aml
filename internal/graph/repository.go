package graph

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrEntityNotFound = errors.New("entity graph not found")
	ErrInvalidInput   = errors.New("invalid graph input")
)

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

type EntitySummary struct {
	EntityHint     string
	RelationType   string
	CustomerCount  int
	CaseCount      int
	HighestRisk    int
	PublicRiskTags []string
	UpdatedAt      time.Time
}

type RelatedCaseSummary struct {
	CaseID        int64
	CustomerID    int64
	EntityHint    string
	DisplayName   string
	Country       string
	Status        string
	PublicSummary string
	RiskScore     int
	TypologyHint  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type EDDSummary struct {
	EDDRecordID  int64
	CustomerID   int64
	EntityHint   string
	ReviewStatus string
	RiskLevel    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
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

func (r *Repository) HasEntityAccess(ctx context.Context, ownerUserID int64, entityHint string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	hint, err := normalizeEntityHint(entityHint)
	if err != nil {
		return false, err
	}
	if ownerUserID <= 0 {
		return false, ErrInvalidInput
	}

	var allowed bool
	err = r.db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM customers
    WHERE owner_user_id = $1 AND lower(btrim(entity_hint)) = $2
)`,
		ownerUserID,
		hint,
	).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check entity graph access: %w", err)
	}
	return allowed, nil
}

func (r *Repository) GetEntitySummary(ctx context.Context, ownerUserID int64, entityHint string) (EntitySummary, error) {
	if err := ctx.Err(); err != nil {
		return EntitySummary{}, err
	}
	hint, err := normalizeEntityHint(entityHint)
	if err != nil {
		return EntitySummary{}, err
	}
	if ownerUserID <= 0 {
		return EntitySummary{}, ErrInvalidInput
	}

	var summary EntitySummary
	var riskTags string
	err = r.db.QueryRowContext(ctx, `
SELECT lower(btrim(c.entity_hint)) AS entity_hint,
       'shared_entity_hint' AS relation_type,
       count(DISTINCT c.id)::int AS customer_count,
       count(DISTINCT ca.id)::int AS case_count,
       coalesce(max(ca.risk_score), 0)::int AS highest_risk,
       coalesce(string_agg(DISTINCT nullif(ca.typology_hint, ''), ',' ORDER BY nullif(ca.typology_hint, '')), '') AS public_risk_tags,
       coalesce(max(ca.updated_at), max(c.created_at)) AS updated_at
FROM customers c
LEFT JOIN cases ca ON ca.customer_id = c.id
WHERE lower(btrim(c.entity_hint)) = $2
  AND EXISTS (
      SELECT 1
      FROM customers owned
      WHERE owned.owner_user_id = $1 AND lower(btrim(owned.entity_hint)) = $2
  )
GROUP BY lower(btrim(c.entity_hint))`,
		ownerUserID,
		hint,
	).Scan(&summary.EntityHint, &summary.RelationType, &summary.CustomerCount, &summary.CaseCount, &summary.HighestRisk, &riskTags, &summary.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EntitySummary{}, ErrEntityNotFound
	}
	if err != nil {
		return EntitySummary{}, fmt.Errorf("get entity graph summary: %w", err)
	}
	summary.PublicRiskTags = splitPublicTags(riskTags)
	return summary, nil
}

func (r *Repository) ListRelatedCases(ctx context.Context, ownerUserID int64, entityHint string) ([]RelatedCaseSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hint, err := normalizeEntityHint(entityHint)
	if err != nil {
		return nil, err
	}
	if ownerUserID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT ca.id, ca.customer_id, lower(btrim(c.entity_hint)) AS entity_hint, c.display_name, c.country, ca.status, ca.public_summary, ca.risk_score, ca.typology_hint, ca.created_at, ca.updated_at
FROM cases ca
JOIN customers c ON c.id = ca.customer_id
WHERE lower(btrim(c.entity_hint)) = $2
  AND EXISTS (
      SELECT 1
      FROM customers owned
      WHERE owned.owner_user_id = $1 AND lower(btrim(owned.entity_hint)) = $2
  )
ORDER BY ca.updated_at DESC, ca.id`,
		ownerUserID,
		hint,
	)
	if err != nil {
		return nil, fmt.Errorf("list related graph cases: %w", err)
	}
	defer rows.Close()

	var cases []RelatedCaseSummary
	for rows.Next() {
		var c RelatedCaseSummary
		if err := rows.Scan(&c.CaseID, &c.CustomerID, &c.EntityHint, &c.DisplayName, &c.Country, &c.Status, &c.PublicSummary, &c.RiskScore, &c.TypologyHint, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan related graph case: %w", err)
		}
		cases = append(cases, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate related graph cases: %w", err)
	}
	return cases, nil
}

func (r *Repository) ListEDDSummaries(ctx context.Context, ownerUserID int64, entityHint string) ([]EDDSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hint, err := normalizeEntityHint(entityHint)
	if err != nil {
		return nil, err
	}
	if ownerUserID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT er.id, er.customer_id, lower(btrim(c.entity_hint)) AS entity_hint, er.review_status, c.risk_level, er.created_at, er.updated_at
FROM edd_records er
JOIN customers c ON c.id = er.customer_id
WHERE lower(btrim(c.entity_hint)) = $2
  AND EXISTS (
      SELECT 1
      FROM customers owned
      WHERE owned.owner_user_id = $1 AND lower(btrim(owned.entity_hint)) = $2
  )
ORDER BY er.updated_at DESC, er.id`,
		ownerUserID,
		hint,
	)
	if err != nil {
		return nil, fmt.Errorf("list graph edd summaries: %w", err)
	}
	defer rows.Close()

	var summaries []EDDSummary
	for rows.Next() {
		var summary EDDSummary
		if err := rows.Scan(&summary.EDDRecordID, &summary.CustomerID, &summary.EntityHint, &summary.ReviewStatus, &summary.RiskLevel, &summary.CreatedAt, &summary.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan graph edd summary: %w", err)
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate graph edd summaries: %w", err)
	}
	return summaries, nil
}

func normalizeEntityHint(value string) (string, error) {
	hint := strings.ToLower(strings.TrimSpace(value))
	if hint == "" || len(hint) > 256 || strings.ContainsAny(hint, "\x00\r\n\t") {
		return "", ErrInvalidInput
	}
	return hint, nil
}

func splitPublicTags(value string) []string {
	var tags []string
	for _, part := range strings.Split(value, ",") {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}
