package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sentinel-aml/internal/classification"
	"strings"
	"time"
)

const RedactedValue = "[redacted]"

var (
	ErrEventNotFound = errors.New("audit event not found")
	ErrInvalidInput  = errors.New("invalid audit input")
)

var allowedObjectTypes = map[string]struct{}{
	"customer":    {},
	"account":     {},
	"transaction": {},
	"alert":       {},
	"case":        {},
	"case_note":   {},
	"edd_record":  {},
}

var allowedEventTypes = map[string]struct{}{
	"created":        {},
	"updated":        {},
	"status_changed": {},
	"claimed":        {},
	"triaged":        {},
	"reviewed":       {},
}

var allowedVisibilityScopes = map[string]struct{}{
	"owner":    {},
	"internal": {},
	"system":   {},
}

var restrictedDiffFields = map[string]struct{}{
	"body":                            {},
	"case_note.body":                  {},
	"confidential_comment":            {},
	"edd_record.confidential_comment": {},
	"password":                        {},
	"password_hash":                   {},
	"token":                           {},
	"token_hash":                      {},
	"authorization":                   {},
	"session":                         {},
	"session_secret":                  {},
	"raw_token":                       {},
	"external_reference_value":        {},
	"verification_marker":             {},
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

type Event struct {
	ID              int64
	ActorUserID     int64
	ObjectType      string
	ObjectID        int64
	EventType       string
	PublicMessage   string
	DiffJSON        string
	CreatedAt       time.Time
	VisibilityScope string
}

type EntityActivityEvent struct {
	Event
	CaseID        int64
	CustomerID    int64
	EntityHint    string
	PublicSummary string
}

type CreateParams struct {
	ActorUserID     int64
	ObjectType      string
	ObjectID        int64
	EventType       string
	PublicMessage   string
	FieldChanges    []FieldChange
	VisibilityScope string
}

type FieldChange struct {
	Field    string
	OldValue string
	NewValue string
}

type RedactedFieldChange struct {
	Field    string `json:"field"`
	Old      string `json:"old,omitempty"`
	New      string `json:"new,omitempty"`
	OldHash  string `json:"old_hash,omitempty"`
	NewHash  string `json:"new_hash,omitempty"`
	Redacted bool   `json:"redacted"`
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

func (r *Repository) Create(ctx context.Context, params CreateParams) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}
	if err := validateCreateParams(params); err != nil {
		return Event{}, err
	}
	params = normalizeCreateParams(params)

	diffJSON, err := RedactedDiffJSON(params.FieldChanges)
	if err != nil {
		return Event{}, err
	}

	var event Event
	err = r.db.QueryRowContext(ctx, `
INSERT INTO audit_events (actor_user_id, object_type, object_id, event_type, public_message, diff_json, visibility_scope)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
RETURNING id, actor_user_id, object_type, object_id, event_type, public_message, diff_json::text, created_at, visibility_scope`,
		params.ActorUserID,
		params.ObjectType,
		params.ObjectID,
		params.EventType,
		params.PublicMessage,
		diffJSON,
		params.VisibilityScope,
	).Scan(&event.ID, &event.ActorUserID, &event.ObjectType, &event.ObjectID, &event.EventType, &event.PublicMessage, &event.DiffJSON, &event.CreatedAt, &event.VisibilityScope)
	if err != nil {
		return Event{}, fmt.Errorf("create audit event: %w", err)
	}
	return event, nil
}

func (r *Repository) ListCaseTimelineByOwner(ctx context.Context, ownerUserID, caseID int64) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ownerUserID <= 0 || caseID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT ae.id, ae.actor_user_id, ae.object_type, ae.object_id, ae.event_type, ae.public_message, ae.diff_json::text, ae.created_at, ae.visibility_scope
FROM audit_events ae
JOIN cases c ON c.owner_user_id = $1 AND c.id = $2
WHERE (ae.object_type = 'case' AND ae.object_id = c.id)
   OR (ae.object_type = 'alert' AND ae.object_id = c.alert_id)
   OR (ae.object_type = 'case_note' AND ae.object_id IN (
       SELECT cn.id FROM case_notes cn WHERE cn.case_id = c.id
   ))
ORDER BY ae.created_at, ae.id`,
		ownerUserID,
		caseID,
	)
	if err != nil {
		return nil, fmt.Errorf("list case audit timeline by owner: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.ActorUserID, &event.ObjectType, &event.ObjectID, &event.EventType, &event.PublicMessage, &event.DiffJSON, &event.CreatedAt, &event.VisibilityScope); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}

func (r *Repository) ListEntityActivityByRequester(ctx context.Context, requesterUserID int64, entityHint string) ([]EntityActivityEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hint, err := normalizeEntityHint(entityHint)
	if err != nil {
		return nil, err
	}
	if requesterUserID <= 0 {
		return nil, ErrInvalidInput
	}

	var allowed bool
	err = r.db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM customers
    WHERE owner_user_id = $1
      AND lower(btrim(entity_hint)) = $2
)`,
		requesterUserID,
		hint,
	).Scan(&allowed)
	if err != nil {
		return nil, fmt.Errorf("check entity activity access: %w", err)
	}
	if !allowed {
		return nil, ErrEventNotFound
	}

	rows, err := r.db.QueryContext(ctx, `
WITH related_cases AS (
    SELECT ca.id AS case_id,
           ca.customer_id,
           ca.alert_id,
           lower(btrim(c.entity_hint)) AS entity_hint,
           ca.public_summary
    FROM cases ca
    JOIN customers c ON c.id = ca.customer_id
    WHERE lower(btrim(c.entity_hint)) = $1
)
SELECT ae.id, ae.actor_user_id, ae.object_type, ae.object_id, ae.event_type, ae.public_message, ae.diff_json::text, ae.created_at, ae.visibility_scope,
       rc.case_id, rc.customer_id, rc.entity_hint, rc.public_summary
FROM audit_events ae
JOIN related_cases rc ON
       (ae.object_type = 'case' AND ae.object_id = rc.case_id)
    OR (ae.object_type = 'alert' AND ae.object_id = rc.alert_id)
    OR (ae.object_type = 'case_note' AND ae.object_id IN (
        SELECT note.id FROM case_notes note WHERE note.case_id = rc.case_id
    ))
ORDER BY ae.created_at, ae.id`,
		hint,
	)
	if err != nil {
		return nil, fmt.Errorf("list entity audit activity: %w", err)
	}
	defer rows.Close()

	var events []EntityActivityEvent
	for rows.Next() {
		var event EntityActivityEvent
		if err := rows.Scan(
			&event.ID,
			&event.ActorUserID,
			&event.ObjectType,
			&event.ObjectID,
			&event.EventType,
			&event.PublicMessage,
			&event.DiffJSON,
			&event.CreatedAt,
			&event.VisibilityScope,
			&event.CaseID,
			&event.CustomerID,
			&event.EntityHint,
			&event.PublicSummary,
		); err != nil {
			return nil, fmt.Errorf("scan entity audit activity: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate entity audit activity: %w", err)
	}
	return events, nil
}

func (r *Repository) ActivityReviewContext(ctx context.Context, event EntityActivityEvent) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if event.ObjectType != "case_note" || event.ObjectID <= 0 || event.CaseID <= 0 {
		return "", nil
	}
	hint, err := normalizeEntityHint(event.EntityHint)
	if err != nil {
		return "", err
	}

	var excerpt string
	err = r.db.QueryRowContext(ctx, `
SELECT cn.body
FROM case_notes cn
JOIN cases ca ON ca.id = cn.case_id
JOIN customers c ON c.id = ca.customer_id
WHERE cn.id = $1
  AND ca.id = $2
  AND lower(btrim(c.entity_hint)) = $3`,
		event.ObjectID,
		event.CaseID,
		hint,
	).Scan(&excerpt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve activity narrative: %w", err)
	}
	return strings.TrimSpace(excerpt), nil
}

func RedactedDiffJSON(changes []FieldChange) (string, error) {
	redacted := make([]RedactedFieldChange, 0, len(changes))
	for _, change := range changes {
		field := classification.PublicFieldName(change.Field)
		if field == "" {
			return "", ErrInvalidInput
		}
		entry := RedactedFieldChange{Field: field}
		if IsRestrictedField(field) {
			entry.Old = RedactedValue
			entry.New = RedactedValue
			entry.OldHash = HashValue(change.OldValue)
			entry.NewHash = HashValue(change.NewValue)
			entry.Redacted = true
		} else {
			entry.Old = strings.TrimSpace(change.OldValue)
			entry.New = strings.TrimSpace(change.NewValue)
		}
		redacted = append(redacted, entry)
	}
	data, err := json.Marshal(redacted)
	if err != nil {
		return "", fmt.Errorf("marshal redacted audit diff: %w", err)
	}
	return string(data), nil
}

func IsRestrictedField(field string) bool {
	_, ok := restrictedDiffFields[normalizeFieldName(field)]
	return ok
}

func HashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validateCreateParams(params CreateParams) error {
	if params.ActorUserID <= 0 || params.ObjectID <= 0 {
		return ErrInvalidInput
	}
	if _, ok := allowedObjectTypes[normalizeFieldName(params.ObjectType)]; !ok {
		return ErrInvalidInput
	}
	if _, ok := allowedEventTypes[normalizeFieldName(params.EventType)]; !ok {
		return ErrInvalidInput
	}
	scope := params.VisibilityScope
	if scope == "" {
		scope = "owner"
	}
	if _, ok := allowedVisibilityScopes[normalizeFieldName(scope)]; !ok {
		return ErrInvalidInput
	}
	if strings.TrimSpace(params.PublicMessage) == "" {
		return ErrInvalidInput
	}
	return nil
}

func normalizeCreateParams(params CreateParams) CreateParams {
	params.ObjectType = normalizeFieldName(params.ObjectType)
	params.EventType = normalizeFieldName(params.EventType)
	params.PublicMessage = strings.TrimSpace(params.PublicMessage)
	if params.VisibilityScope == "" {
		params.VisibilityScope = "owner"
	}
	params.VisibilityScope = normalizeFieldName(params.VisibilityScope)
	return params
}

func normalizeFieldName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeEntityHint(value string) (string, error) {
	hint := strings.ToLower(strings.TrimSpace(value))
	if hint == "" || len(hint) > 256 || strings.ContainsAny(hint, "\x00\r\n\t") {
		return "", ErrInvalidInput
	}
	return hint, nil
}
