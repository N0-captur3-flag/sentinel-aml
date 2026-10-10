package exporting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrExportNotFound = errors.New("export object not found")
	ErrDuplicate      = errors.New("duplicate export object")
	ErrInvalidInput   = errors.New("invalid export input")
)

var allowedDisputeStatuses = map[string]struct{}{
	"OPEN":         {},
	"UNDER_REVIEW": {},
	"RESOLVED":     {},
	"REJECTED":     {},
}

var allowedExportPurposes = map[string]struct{}{
	"case_review":       {},
	"regulatory_report": {},
	"internal_review":   {},
}

var allowedExportStatuses = map[string]struct{}{
	"PENDING":   {},
	"GENERATED": {},
	"FAILED":    {},
	"EXPIRED":   {},
}

var allowedItemTypes = map[string]struct{}{
	"root_case":     {},
	"linked_case":   {},
	"audit_summary": {},
	"edd_summary":   {},
	"metadata":      {},
}

var allowedObjectTypes = map[string]struct{}{
	"case":        {},
	"audit_event": {},
	"edd_record":  {},
	"export":      {},
}

var allowedRelations = map[string]struct{}{
	"root":     {},
	"linked":   {},
	"audit":    {},
	"edd":      {},
	"metadata": {},
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

type Dispute struct {
	ID                int64
	OwnerUserID       int64
	CaseID            int64
	Status            string
	Reason            string
	ResolutionSummary string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ExportRecord struct {
	ID              int64
	OwnerUserID     int64
	RootCaseID      int64
	DisputeID       int64
	Purpose         string
	Status          string
	BundleJSON      string
	RedactionPolicy string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ExpiresAt       time.Time
}

type ExportItem struct {
	ID              int64
	ExportID        int64
	ItemType        string
	ObjectType      string
	ObjectID        int64
	Relation        string
	RedactionPolicy string
	PayloadJSON     string
	CreatedAt       time.Time
}

type CreateDisputeParams struct {
	OwnerUserID int64
	CaseID      int64
	Status      string
	Reason      string
}

type CreateExportParams struct {
	OwnerUserID int64
	RootCaseID  int64
	DisputeID   int64
	Purpose     string
	Status      string
	BundleJSON  string
}

type CreateExportItemParams struct {
	OwnerUserID int64
	ExportID    int64
	ItemType    string
	ObjectType  string
	ObjectID    int64
	Relation    string
	PayloadJSON string
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

func (r *Repository) CreateDispute(ctx context.Context, params CreateDisputeParams) (Dispute, error) {
	if err := ctx.Err(); err != nil {
		return Dispute{}, err
	}
	params = normalizeCreateDisputeParams(params)
	if err := validateCreateDisputeParams(params); err != nil {
		return Dispute{}, err
	}

	var dispute Dispute
	err := r.db.QueryRowContext(ctx, `
INSERT INTO disputes (owner_user_id, case_id, status, reason)
SELECT c.owner_user_id, c.id, $3, $4
FROM cases c
WHERE c.owner_user_id = $1 AND c.id = $2
RETURNING id, owner_user_id, case_id, status, reason, resolution_summary, created_at, updated_at`,
		params.OwnerUserID,
		params.CaseID,
		params.Status,
		params.Reason,
	).Scan(&dispute.ID, &dispute.OwnerUserID, &dispute.CaseID, &dispute.Status, &dispute.Reason, &dispute.ResolutionSummary, &dispute.CreatedAt, &dispute.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Dispute{}, ErrExportNotFound
	}
	if isUniqueViolation(err) {
		return Dispute{}, ErrDuplicate
	}
	if err != nil {
		return Dispute{}, fmt.Errorf("create dispute: %w", err)
	}
	return dispute, nil
}

func (r *Repository) CreateExport(ctx context.Context, params CreateExportParams) (ExportRecord, error) {
	if err := ctx.Err(); err != nil {
		return ExportRecord{}, err
	}
	params = normalizeCreateExportParams(params)
	if err := validateCreateExportParams(params); err != nil {
		return ExportRecord{}, err
	}

	var record ExportRecord
	err := r.db.QueryRowContext(ctx, `
INSERT INTO exports (owner_user_id, root_case_id, dispute_id, purpose, status, bundle_json, redaction_policy)
SELECT c.owner_user_id, c.id, nullif($3, 0), $4, $5, $6::jsonb, 'public_redacted'
FROM cases c
WHERE c.owner_user_id = $1 AND c.id = $2
RETURNING id, owner_user_id, root_case_id, coalesce(dispute_id, 0), purpose, status, bundle_json::text, redaction_policy, created_at, updated_at, expires_at`,
		params.OwnerUserID,
		params.RootCaseID,
		params.DisputeID,
		params.Purpose,
		params.Status,
		params.BundleJSON,
	).Scan(&record.ID, &record.OwnerUserID, &record.RootCaseID, &record.DisputeID, &record.Purpose, &record.Status, &record.BundleJSON, &record.RedactionPolicy, &record.CreatedAt, &record.UpdatedAt, &record.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ExportRecord{}, ErrExportNotFound
	}
	if err != nil {
		return ExportRecord{}, fmt.Errorf("create export: %w", err)
	}
	return record, nil
}

func (r *Repository) GetExportByOwner(ctx context.Context, ownerUserID, exportID int64) (ExportRecord, error) {
	if err := ctx.Err(); err != nil {
		return ExportRecord{}, err
	}
	if ownerUserID <= 0 || exportID <= 0 {
		return ExportRecord{}, ErrInvalidInput
	}

	var record ExportRecord
	err := r.db.QueryRowContext(ctx, `
SELECT id, owner_user_id, root_case_id, coalesce(dispute_id, 0), purpose, status, bundle_json::text, redaction_policy, created_at, updated_at, expires_at
FROM exports
WHERE owner_user_id = $1 AND id = $2`,
		ownerUserID,
		exportID,
	).Scan(&record.ID, &record.OwnerUserID, &record.RootCaseID, &record.DisputeID, &record.Purpose, &record.Status, &record.BundleJSON, &record.RedactionPolicy, &record.CreatedAt, &record.UpdatedAt, &record.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ExportRecord{}, ErrExportNotFound
	}
	if err != nil {
		return ExportRecord{}, fmt.Errorf("get export by owner: %w", err)
	}
	return record, nil
}

func (r *Repository) GetDisputeByOwner(ctx context.Context, ownerUserID, disputeID int64) (Dispute, error) {
	if err := ctx.Err(); err != nil {
		return Dispute{}, err
	}
	if ownerUserID <= 0 || disputeID <= 0 {
		return Dispute{}, ErrInvalidInput
	}

	var dispute Dispute
	err := r.db.QueryRowContext(ctx, `
SELECT id, owner_user_id, case_id, status, reason, resolution_summary, created_at, updated_at
FROM disputes
WHERE owner_user_id = $1 AND id = $2`,
		ownerUserID,
		disputeID,
	).Scan(&dispute.ID, &dispute.OwnerUserID, &dispute.CaseID, &dispute.Status, &dispute.Reason, &dispute.ResolutionSummary, &dispute.CreatedAt, &dispute.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Dispute{}, ErrExportNotFound
	}
	if err != nil {
		return Dispute{}, fmt.Errorf("get dispute by owner: %w", err)
	}
	return dispute, nil
}

func (r *Repository) CreateExportItem(ctx context.Context, params CreateExportItemParams) (ExportItem, error) {
	if err := ctx.Err(); err != nil {
		return ExportItem{}, err
	}
	params = normalizeCreateExportItemParams(params)
	if err := validateCreateExportItemParams(params); err != nil {
		return ExportItem{}, err
	}

	var item ExportItem
	err := r.db.QueryRowContext(ctx, `
INSERT INTO export_items (export_id, item_type, object_type, object_id, relation, redaction_policy, payload_json)
SELECT e.id, $3, $4, $5, $6, 'public_redacted', $7::jsonb
FROM exports e
WHERE e.owner_user_id = $1 AND e.id = $2
RETURNING id, export_id, item_type, object_type, object_id, relation, redaction_policy, payload_json::text, created_at`,
		params.OwnerUserID,
		params.ExportID,
		params.ItemType,
		params.ObjectType,
		params.ObjectID,
		params.Relation,
		params.PayloadJSON,
	).Scan(&item.ID, &item.ExportID, &item.ItemType, &item.ObjectType, &item.ObjectID, &item.Relation, &item.RedactionPolicy, &item.PayloadJSON, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ExportItem{}, ErrExportNotFound
	}
	if isUniqueViolation(err) {
		return ExportItem{}, ErrDuplicate
	}
	if err != nil {
		return ExportItem{}, fmt.Errorf("create export item: %w", err)
	}
	return item, nil
}

func validateCreateDisputeParams(params CreateDisputeParams) error {
	if params.OwnerUserID <= 0 || params.CaseID <= 0 || strings.TrimSpace(params.Reason) == "" {
		return ErrInvalidInput
	}
	if _, ok := allowedDisputeStatuses[params.Status]; !ok {
		return ErrInvalidInput
	}
	return nil
}

func normalizeCreateDisputeParams(params CreateDisputeParams) CreateDisputeParams {
	params.Status = strings.ToUpper(strings.TrimSpace(params.Status))
	if params.Status == "" {
		params.Status = "OPEN"
	}
	params.Reason = strings.TrimSpace(params.Reason)
	return params
}

func validateCreateExportParams(params CreateExportParams) error {
	if params.OwnerUserID <= 0 || params.RootCaseID <= 0 {
		return ErrInvalidInput
	}
	if _, ok := allowedExportPurposes[params.Purpose]; !ok {
		return ErrInvalidInput
	}
	if _, ok := allowedExportStatuses[params.Status]; !ok {
		return ErrInvalidInput
	}
	return validateJSONObject(params.BundleJSON)
}

func normalizeCreateExportParams(params CreateExportParams) CreateExportParams {
	params.Purpose = normalizeToken(params.Purpose)
	params.Status = strings.ToUpper(strings.TrimSpace(params.Status))
	if params.Status == "" {
		params.Status = "PENDING"
	}
	params.BundleJSON = strings.TrimSpace(params.BundleJSON)
	if params.BundleJSON == "" {
		params.BundleJSON = "{}"
	}
	return params
}

func validateCreateExportItemParams(params CreateExportItemParams) error {
	if params.OwnerUserID <= 0 || params.ExportID <= 0 || params.ObjectID <= 0 {
		return ErrInvalidInput
	}
	if _, ok := allowedItemTypes[params.ItemType]; !ok {
		return ErrInvalidInput
	}
	if _, ok := allowedObjectTypes[params.ObjectType]; !ok {
		return ErrInvalidInput
	}
	if _, ok := allowedRelations[params.Relation]; !ok {
		return ErrInvalidInput
	}
	return validateJSONObject(params.PayloadJSON)
}

func normalizeCreateExportItemParams(params CreateExportItemParams) CreateExportItemParams {
	params.ItemType = normalizeToken(params.ItemType)
	params.ObjectType = normalizeToken(params.ObjectType)
	params.Relation = normalizeToken(params.Relation)
	params.PayloadJSON = strings.TrimSpace(params.PayloadJSON)
	if params.PayloadJSON == "" {
		params.PayloadJSON = "{}"
	}
	return params
}

func validateJSONObject(value string) error {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return ErrInvalidInput
	}
	return nil
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "SQLSTATE 23505") || strings.Contains(err.Error(), "duplicate key")
}
