package exporting

import (
	"sentinel-aml/internal/classification"
	"sort"
	"strings"
	"time"
)

const RedactionPolicyPublic = "public_redacted"

var allowedExportScopes = map[string]struct{}{
	"root_case":     {},
	"linked_cases":  {},
	"audit_summary": {},
	"edd_summary":   {},
	"item_metadata": {},
}

var sensitiveExportFields = map[string]struct{}{
	"body":                            {},
	"case_note.body":                  {},
	"confidential_comment":            {},
	"edd_record.confidential_comment": {},
	"source_of_funds":                 {},
	"risk_rationale":                  {},
	"diff_json":                       {},
	"old_hash":                        {},
	"new_hash":                        {},
	"value_hash":                      {},
	"password":                        {},
	"password_hash":                   {},
	"token":                           {},
	"token_hash":                      {},
	"session":                         {},
	"internal_review_marker":          {},
	"external_reference_value":        {},
	"verification_marker":             {},
}

type ExportContext struct {
	RequesterUserID int64
	RootCaseID      int64
	DisputeID       int64
	Purpose         string
	AllowedScopes   []string
	Scope           ExportScope
}

type ExportScope string

const (
	ExportScopeRootCaseDetail    ExportScope = classification.ExportScopeRootCaseDetail
	ExportScopeLinkedCaseSummary ExportScope = classification.ExportScopeLinkedCaseSummary
)

type ExportCaseTraversal struct {
	Root   ExportContext
	Linked ExportContext
}

type ExportSourceMaterial struct {
	ItemType        string
	ObjectType      string
	ObjectID        int64
	CaseID          int64
	Relation        string
	SourceClass     string
	Content         string
	EvidenceExcerpt string
	Caption         string
	CoveragePolicy  ExportScope
}

type ExportItemBuilder struct {
	Context ExportContext
}

type CaseSnapshot struct {
	CaseID          int64
	CustomerID      int64
	OwnerUserID     int64
	Status          string
	PublicSummary   string
	RiskScore       int
	TypologyHint    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Notes           []CaseNoteSnapshot
	AuditEvents     []AuditEventSnapshot
	EDDRecords      []EDDRecordSnapshot
	LinkedCases     []CaseSnapshot
	SourceMaterials []ExportSourceMaterial
}

type CaseNoteSnapshot struct {
	ID         int64
	Visibility string
	Body       string
	CreatedAt  time.Time
}

type AuditEventSnapshot struct {
	ID            int64
	ObjectType    string
	ObjectID      int64
	EventType     string
	PublicMessage string
	DiffJSON      string
	Changes       []AuditChangeSnapshot
	CreatedAt     time.Time
}

type AuditChangeSnapshot struct {
	Field    string
	Old      string
	New      string
	OldHash  string
	NewHash  string
	Redacted bool
}

type EDDRecordSnapshot struct {
	ID                  int64
	CustomerID          int64
	ReviewStatus        string
	RiskLevel           string
	SourceOfFunds       string
	RiskRationale       string
	ConfidentialComment string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type ExportBundle struct {
	Purpose         string               `json:"purpose"`
	RedactionPolicy string               `json:"redaction_policy"`
	GeneratedAt     time.Time            `json:"generated_at"`
	RootCase        ExportCaseShell      `json:"root_case"`
	LinkedCases     []ExportCaseShell    `json:"linked_cases"`
	LinkedEvidence  []map[string]any     `json:"linked_evidence,omitempty"`
	AuditSummaries  []ExportAuditSummary `json:"audit_summaries"`
	EDDSummaries    []ExportEDDSummary   `json:"edd_summaries"`
	Items           []ExportItemMetadata `json:"items"`
}

type ExportCaseShell struct {
	CaseID        int64     `json:"case_id"`
	CustomerID    int64     `json:"customer_id"`
	Status        string    `json:"status"`
	PublicSummary string    `json:"public_summary"`
	RiskScore     int       `json:"risk_score"`
	TypologyHint  string    `json:"typology_hint"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ExportAuditSummary struct {
	EventID       int64     `json:"event_id"`
	ObjectType    string    `json:"object_type"`
	ObjectID      int64     `json:"object_id"`
	EventType     string    `json:"event_type"`
	PublicMessage string    `json:"public_message"`
	ChangedFields []string  `json:"changed_fields"`
	CreatedAt     time.Time `json:"created_at"`
}

type ExportEDDSummary struct {
	RecordID     int64     `json:"record_id"`
	CustomerID   int64     `json:"customer_id"`
	ReviewStatus string    `json:"review_status"`
	RiskTags     []string  `json:"risk_tags"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ExportItemMetadata struct {
	ItemType        string `json:"item_type"`
	ObjectType      string `json:"object_type"`
	ObjectID        int64  `json:"object_id"`
	Relation        string `json:"relation"`
	RedactionPolicy string `json:"redaction_policy"`
}

func BuildBundle(ctx ExportContext, root CaseSnapshot, generatedAt time.Time) (ExportBundle, error) {
	ctx = normalizeExportContext(ctx)
	if err := validateExportContext(ctx); err != nil {
		return ExportBundle{}, err
	}
	if root.CaseID != ctx.RootCaseID || root.OwnerUserID != ctx.RequesterUserID {
		return ExportBundle{}, ErrExportNotFound
	}
	traversal := NewExportCaseTraversal(ctx)

	bundle := ExportBundle{
		Purpose:         ctx.Purpose,
		RedactionPolicy: RedactionPolicyPublic,
		GeneratedAt:     generatedAt.UTC(),
		RootCase:        safeCaseShell(root),
	}
	scopes := scopeSet(ctx.AllowedScopes)
	if scopes["item_metadata"] {
		bundle.Items = append(bundle.Items, itemMetadata("root_case", "case", root.CaseID, "root"))
	}
	visited := map[int64]bool{root.CaseID: true}
	appendSafeCaseData(&bundle, root, scopes, "root")
	linkedCases := flattenLinkedCases(root.LinkedCases, visited)
	for _, linked := range linkedCases {
		if scopes["linked_cases"] {
			bundle.LinkedCases = append(bundle.LinkedCases, safeCaseShell(linked))
		}
		appendSafeCaseData(&bundle, linked, scopes, "linked")
	}
	appendExportLinkedCaseData(&bundle, root, linkedCases, scopes, traversal)

	sort.Slice(bundle.LinkedCases, func(i, j int) bool { return bundle.LinkedCases[i].CaseID < bundle.LinkedCases[j].CaseID })
	sort.Slice(bundle.LinkedEvidence, func(i, j int) bool {
		left := bundle.LinkedEvidence[i]
		right := bundle.LinkedEvidence[j]
		leftCaseID := exportMapInt64(left, "case_id")
		rightCaseID := exportMapInt64(right, "case_id")
		if leftCaseID != rightCaseID {
			return leftCaseID < rightCaseID
		}
		leftObjectID := exportMapInt64(left, "object_id")
		rightObjectID := exportMapInt64(right, "object_id")
		if leftObjectID != rightObjectID {
			return leftObjectID < rightObjectID
		}
		leftItemType, _ := left["item_type"].(string)
		rightItemType, _ := right["item_type"].(string)
		return leftItemType < rightItemType
	})
	sort.Slice(bundle.AuditSummaries, func(i, j int) bool {
		if bundle.AuditSummaries[i].CreatedAt.Equal(bundle.AuditSummaries[j].CreatedAt) {
			return bundle.AuditSummaries[i].EventID < bundle.AuditSummaries[j].EventID
		}
		return bundle.AuditSummaries[i].CreatedAt.Before(bundle.AuditSummaries[j].CreatedAt)
	})
	sort.Slice(bundle.EDDSummaries, func(i, j int) bool {
		if bundle.EDDSummaries[i].CustomerID == bundle.EDDSummaries[j].CustomerID {
			return bundle.EDDSummaries[i].RecordID < bundle.EDDSummaries[j].RecordID
		}
		return bundle.EDDSummaries[i].CustomerID < bundle.EDDSummaries[j].CustomerID
	})
	sort.Slice(bundle.Items, func(i, j int) bool {
		left := bundle.Items[i]
		right := bundle.Items[j]
		if left.ItemType != right.ItemType {
			return left.ItemType < right.ItemType
		}
		if left.ObjectType != right.ObjectType {
			return left.ObjectType < right.ObjectType
		}
		if left.ObjectID != right.ObjectID {
			return left.ObjectID < right.ObjectID
		}
		return left.Relation < right.Relation
	})
	return bundle, nil
}

func NewExportCaseTraversal(ctx ExportContext) ExportCaseTraversal {
	return ExportCaseTraversal{
		Root:   RebindExportContext(ctx, ExportScopeRootCaseDetail),
		Linked: RebindExportContext(ctx, ExportScopeLinkedCaseSummary),
	}
}

func RebindExportContext(ctx ExportContext, scope ExportScope) ExportContext {
	ctx.Scope = scope
	return ctx
}

func appendExportSourceMaterials(bundle *ExportBundle, materials []ExportSourceMaterial, ctx ExportContext) {
	builder := ExportItemBuilder{Context: ctx}
	for _, material := range materials {
		item, ok := builder.Build(material)
		if !ok {
			continue
		}
		bundle.LinkedEvidence = append(bundle.LinkedEvidence, item)
	}
}

func (b ExportItemBuilder) Build(material ExportSourceMaterial) (map[string]any, bool) {
	material = normalizeExportSourceMaterial(material)
	if !b.allows(material) {
		return nil, false
	}
	item := map[string]any{
		"item_type":       material.ItemType,
		"object_type":     material.ObjectType,
		"object_id":       material.ObjectID,
		"relation":        material.Relation,
		"coverage_policy": string(b.Context.Scope),
	}
	if material.CaseID > 0 {
		item["case_id"] = material.CaseID
	}
	if material.Content != "" {
		item["content"] = material.Content
	}
	if material.EvidenceExcerpt != "" {
		item["evidence_excerpt"] = material.EvidenceExcerpt
	}
	if material.Caption != "" {
		item["caption"] = material.Caption
	}
	if material.Content == "" && material.EvidenceExcerpt == "" && material.Caption == "" {
		return nil, false
	}
	return item, true
}

func (b ExportItemBuilder) allows(material ExportSourceMaterial) bool {
	scope := b.Context.Scope
	if scope == "" {
		scope = ExportScopeRootCaseDetail
	}
	switch scope {
	case ExportScopeRootCaseDetail:
		return material.CoveragePolicy == ExportScopeRootCaseDetail || material.CoveragePolicy == ExportScopeLinkedCaseSummary
	case ExportScopeLinkedCaseSummary:
		return material.CoveragePolicy == ExportScopeLinkedCaseSummary
	default:
		return false
	}
}

func normalizeExportSourceMaterial(material ExportSourceMaterial) ExportSourceMaterial {
	material.ItemType = normalizeToken(material.ItemType)
	material.ObjectType = normalizeToken(material.ObjectType)
	material.Relation = normalizeToken(material.Relation)
	material.SourceClass = normalizeToken(material.SourceClass)
	material.Content = strings.TrimSpace(material.Content)
	material.EvidenceExcerpt = strings.TrimSpace(material.EvidenceExcerpt)
	material.Caption = strings.TrimSpace(material.Caption)
	if material.ItemType == "" {
		material.ItemType = "source_material"
	}
	if material.ObjectType == "" {
		material.ObjectType = "case"
	}
	if material.Relation == "" {
		material.Relation = "linked"
	}
	if material.CoveragePolicy == "" {
		material.CoveragePolicy = ExportScopeLinkedCaseSummary
	}
	return material
}

func exportMapInt64(item map[string]any, key string) int64 {
	switch value := item[key].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func appendSafeCaseData(bundle *ExportBundle, c CaseSnapshot, scopes map[string]bool, relation string) {
	if scopes["audit_summary"] {
		for _, event := range c.AuditEvents {
			bundle.AuditSummaries = append(bundle.AuditSummaries, safeAuditSummary(event))
			if scopes["item_metadata"] {
				bundle.Items = append(bundle.Items, itemMetadata("audit_summary", "audit_event", event.ID, "audit"))
			}
		}
	}
	if scopes["edd_summary"] {
		for _, record := range c.EDDRecords {
			bundle.EDDSummaries = append(bundle.EDDSummaries, safeEDDSummary(record))
			if scopes["item_metadata"] {
				bundle.Items = append(bundle.Items, itemMetadata("edd_summary", "edd_record", record.ID, "edd"))
			}
		}
	}
	if relation == "linked" && scopes["item_metadata"] {
		bundle.Items = append(bundle.Items, itemMetadata("linked_case", "case", c.CaseID, "linked"))
	}
}

func flattenLinkedCases(cases []CaseSnapshot, visited map[int64]bool) []CaseSnapshot {
	var flattened []CaseSnapshot
	for _, c := range cases {
		if c.CaseID <= 0 || visited[c.CaseID] {
			continue
		}
		visited[c.CaseID] = true
		flattened = append(flattened, c)
		flattened = append(flattened, flattenLinkedCases(c.LinkedCases, visited)...)
	}
	return flattened
}

func safeCaseShell(c CaseSnapshot) ExportCaseShell {
	return ExportCaseShell{
		CaseID:        c.CaseID,
		CustomerID:    c.CustomerID,
		Status:        strings.TrimSpace(c.Status),
		PublicSummary: strings.TrimSpace(c.PublicSummary),
		RiskScore:     c.RiskScore,
		TypologyHint:  strings.TrimSpace(c.TypologyHint),
		CreatedAt:     c.CreatedAt.UTC(),
		UpdatedAt:     c.UpdatedAt.UTC(),
	}
}

func safeAuditSummary(event AuditEventSnapshot) ExportAuditSummary {
	fields := make([]string, 0, len(event.Changes))
	for _, change := range event.Changes {
		field := normalizeToken(change.Field)
		if field == "" {
			continue
		}
		if IsSensitiveExportField(field) || change.Redacted {
			field = "restricted_field"
		} else {
			field = classification.PublicFieldName(field)
		}
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return ExportAuditSummary{
		EventID:       event.ID,
		ObjectType:    normalizeToken(event.ObjectType),
		ObjectID:      event.ObjectID,
		EventType:     normalizeToken(event.EventType),
		PublicMessage: strings.TrimSpace(event.PublicMessage),
		ChangedFields: fields,
		CreatedAt:     event.CreatedAt.UTC(),
	}
}

func safeEDDSummary(record EDDRecordSnapshot) ExportEDDSummary {
	return ExportEDDSummary{
		RecordID:     record.ID,
		CustomerID:   record.CustomerID,
		ReviewStatus: strings.TrimSpace(record.ReviewStatus),
		RiskTags:     eddRiskTags(record),
		CreatedAt:    record.CreatedAt.UTC(),
		UpdatedAt:    record.UpdatedAt.UTC(),
	}
}

func eddRiskTags(record EDDRecordSnapshot) []string {
	var tags []string
	if status := normalizeToken(record.ReviewStatus); status != "" {
		tags = append(tags, "edd_status:"+status)
	}
	if risk := normalizeToken(record.RiskLevel); risk != "" {
		tags = append(tags, "risk:"+risk)
	}
	sort.Strings(tags)
	return tags
}

func itemMetadata(itemType, objectType string, objectID int64, relation string) ExportItemMetadata {
	return ExportItemMetadata{
		ItemType:        itemType,
		ObjectType:      objectType,
		ObjectID:        objectID,
		Relation:        relation,
		RedactionPolicy: RedactionPolicyPublic,
	}
}

func validateExportContext(ctx ExportContext) error {
	if ctx.RequesterUserID <= 0 || ctx.RootCaseID <= 0 {
		return ErrInvalidInput
	}
	if _, ok := allowedExportPurposes[ctx.Purpose]; !ok {
		return ErrInvalidInput
	}
	if len(ctx.AllowedScopes) == 0 {
		return ErrInvalidInput
	}
	for _, scope := range ctx.AllowedScopes {
		if _, ok := allowedExportScopes[scope]; !ok {
			return ErrInvalidInput
		}
	}
	return nil
}

func normalizeExportContext(ctx ExportContext) ExportContext {
	ctx.Purpose = normalizeToken(ctx.Purpose)
	normalizedScopes := make([]string, 0, len(ctx.AllowedScopes))
	for _, scope := range ctx.AllowedScopes {
		normalizedScopes = append(normalizedScopes, normalizeToken(scope))
	}
	ctx.AllowedScopes = normalizedScopes
	return ctx
}

func scopeSet(scopes []string) map[string]bool {
	set := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		set[scope] = true
	}
	return set
}

func IsSensitiveExportField(field string) bool {
	normalized := normalizeToken(field)
	if classification.IsProtectedSourceField(normalized) {
		return true
	}
	_, ok := sensitiveExportFields[normalized]
	return ok
}
