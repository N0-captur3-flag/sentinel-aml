package projection

import (
	"sentinel-aml/internal/classification"
	"sort"
	"time"
)

// Contract records the JSON allowlist for one projection surface.
type Contract struct {
	Name              string
	View              any
	AllowedJSONFields []string
	OwnerOnly         bool
}

type OwnerCustomerView struct {
	ID                 int64     `json:"id"`
	DisplayName        string    `json:"display_name"`
	CustomerType       string    `json:"customer_type"`
	Country            string    `json:"country"`
	RegistrationNumber string    `json:"registration_number"`
	RiskLevel          string    `json:"risk_level"`
	EntityHint         string    `json:"entity_hint"`
	CreatedAt          time.Time `json:"created_at"`
	Status             string    `json:"status"`
}

type OwnerAccountView struct {
	ID         int64     `json:"id"`
	CustomerID int64     `json:"customer_id"`
	IBANLike   string    `json:"iban_like"`
	Currency   string    `json:"currency"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type OwnerTransactionView struct {
	ID                      int64     `json:"id"`
	AccountID               int64     `json:"account_id"`
	CounterpartyName        string    `json:"counterparty_name"`
	CounterpartyCountry     string    `json:"counterparty_country"`
	CounterpartyFingerprint string    `json:"counterparty_fingerprint"`
	Amount                  string    `json:"amount"`
	Currency                string    `json:"currency"`
	Direction               string    `json:"direction"`
	Purpose                 string    `json:"purpose"`
	TraceID                 string    `json:"trace_id"`
	CreatedAt               time.Time `json:"created_at"`
	RiskScore               int       `json:"risk_score"`
}

type OwnerAlertView struct {
	ID             int64     `json:"id"`
	TransactionID  int64     `json:"transaction_id"`
	CustomerID     int64     `json:"customer_id"`
	RuleCode       string    `json:"rule_code"`
	RiskScore      int       `json:"risk_score"`
	Status         string    `json:"status"`
	AssignedUserID *int64    `json:"assigned_user_id,omitempty"`
	TypologyHint   string    `json:"typology_hint"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type OwnerCaseView struct {
	ID            int64     `json:"id"`
	CustomerID    int64     `json:"customer_id"`
	AlertID       int64     `json:"alert_id"`
	Status        string    `json:"status"`
	PublicSummary string    `json:"public_summary"`
	RiskScore     int       `json:"risk_score"`
	TypologyHint  string    `json:"typology_hint"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type OwnerCaseNoteView struct {
	ID           int64     `json:"id"`
	CaseID       int64     `json:"case_id"`
	AuthorUserID int64     `json:"author_user_id"`
	Visibility   string    `json:"visibility"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type OwnerEDDView struct {
	ID                  int64     `json:"id"`
	CustomerID          int64     `json:"customer_id"`
	AnalystUserID       int64     `json:"analyst_user_id"`
	SourceOfFunds       string    `json:"source_of_funds"`
	RiskRationale       string    `json:"risk_rationale"`
	ConfidentialComment string    `json:"confidential_comment"`
	ReviewStatus        string    `json:"review_status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type OwnerCaseAuditChangeView struct {
	Field    string `json:"field"`
	Old      string `json:"old,omitempty"`
	New      string `json:"new,omitempty"`
	Redacted bool   `json:"redacted"`
}

type OwnerCaseAuditView struct {
	ID              int64                      `json:"id"`
	ObjectType      string                     `json:"object_type"`
	ObjectID        int64                      `json:"object_id"`
	EventType       string                     `json:"event_type"`
	PublicMessage   string                     `json:"public_message"`
	Changes         []OwnerCaseAuditChangeView `json:"changes"`
	CreatedAt       time.Time                  `json:"created_at"`
	VisibilityScope string                     `json:"visibility_scope"`
}

type CaseDuplicatePreviewView struct {
	CaseID           int64    `json:"case_id"`
	PublicSummary    string   `json:"public_summary"`
	RiskTags         []string `json:"risk_tags"`
	OverlapScore     int      `json:"overlap_score"`
	MatchExplanation string   `json:"match_explanation"`
}

type GraphEntityView struct {
	EntityHint    string    `json:"entity_hint"`
	RelationType  string    `json:"relation_type"`
	CustomerCount int       `json:"customer_count"`
	CaseCount     int       `json:"case_count"`
	HighestRisk   int       `json:"highest_risk"`
	RiskTags      []string  `json:"risk_tags"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type GraphRelatedCaseView struct {
	CaseID        int64     `json:"case_id"`
	CustomerID    int64     `json:"customer_id"`
	EntityHint    string    `json:"entity_hint"`
	DisplayName   string    `json:"display_name"`
	Country       string    `json:"country"`
	Status        string    `json:"status"`
	PublicSummary string    `json:"public_summary"`
	RiskScore     int       `json:"risk_score"`
	TypologyHint  string    `json:"typology_hint"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type EDDGraphSummaryView struct {
	CustomerID   int64     `json:"customer_id"`
	EntityHint   string    `json:"entity_hint"`
	ReviewStatus string    `json:"review_status"`
	RiskTags     []string  `json:"risk_tags"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ExportBundleView struct {
	Purpose         string                   `json:"purpose"`
	RedactionPolicy string                   `json:"redaction_policy"`
	GeneratedAt     time.Time                `json:"generated_at"`
	RootCase        ExportCaseShellView      `json:"root_case"`
	LinkedCases     []ExportCaseShellView    `json:"linked_cases"`
	AuditSummaries  []ExportAuditSummaryView `json:"audit_summaries"`
	EDDSummaries    []ExportEDDSummaryView   `json:"edd_summaries"`
	Items           []ExportItemMetadataView `json:"items"`
}

type ExportCaseShellView struct {
	CaseID        int64     `json:"case_id"`
	CustomerID    int64     `json:"customer_id"`
	Status        string    `json:"status"`
	PublicSummary string    `json:"public_summary"`
	RiskScore     int       `json:"risk_score"`
	TypologyHint  string    `json:"typology_hint"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ExportAuditSummaryView struct {
	EventID       int64     `json:"event_id"`
	ObjectType    string    `json:"object_type"`
	ObjectID      int64     `json:"object_id"`
	EventType     string    `json:"event_type"`
	PublicMessage string    `json:"public_message"`
	ChangedFields []string  `json:"changed_fields"`
	CreatedAt     time.Time `json:"created_at"`
}

type ExportEDDSummaryView struct {
	RecordID     int64     `json:"record_id"`
	CustomerID   int64     `json:"customer_id"`
	ReviewStatus string    `json:"review_status"`
	RiskTags     []string  `json:"risk_tags"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ExportItemMetadataView struct {
	ItemType        string `json:"item_type"`
	ObjectType      string `json:"object_type"`
	ObjectID        int64  `json:"object_id"`
	Relation        string `json:"relation"`
	RedactionPolicy string `json:"redaction_policy"`
}

type NetworkRoomSummaryView struct {
	ID               int64     `json:"id"`
	RoomKey          string    `json:"room_key"`
	EntityHint       string    `json:"entity_hint"`
	TypologyHint     string    `json:"typology_hint"`
	Status           string    `json:"status"`
	PublicSummary    string    `json:"public_summary"`
	PublicRiskTags   []string  `json:"public_risk_tags"`
	CaseCount        int       `json:"case_count"`
	ParticipantCount int       `json:"participant_count"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type NetworkRoomCaseView struct {
	CaseID                int64     `json:"case_id"`
	CustomerID            int64     `json:"customer_id"`
	OwnedByRequester      bool      `json:"owned_by_requester"`
	Status                string    `json:"status"`
	PublicSummary         string    `json:"public_summary"`
	RiskScore             int       `json:"risk_score"`
	TypologyHint          string    `json:"typology_hint"`
	PublicRiskTags        []string  `json:"public_risk_tags"`
	ScopeCoverageComplete bool      `json:"scope_coverage_complete"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type NetworkRoomDossierView struct {
	Room           NetworkRoomSummaryView `json:"room"`
	Cases          []NetworkRoomCaseView  `json:"cases"`
	ScopePolicy    string                 `json:"scope_policy"`
	CoveragePolicy string                 `json:"coverage_policy"`
}

type FuturePublicCaseShell struct {
	ID            int64     `json:"id"`
	CustomerID    int64     `json:"customer_id"`
	Status        string    `json:"status"`
	PublicSummary string    `json:"public_summary"`
	RiskScore     int       `json:"risk_score"`
	TypologyHint  string    `json:"typology_hint"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type FutureDuplicatePreview struct {
	CandidateCustomerID int64  `json:"candidate_customer_id"`
	MatchCustomerID     int64  `json:"match_customer_id"`
	DisplayName         string `json:"display_name"`
	Country             string `json:"country"`
	RiskLevel           string `json:"risk_level"`
	MatchReason         string `json:"match_reason"`
	ConfidenceBucket    string `json:"confidence_bucket"`
}

type FutureAuditSummary struct {
	ID         int64     `json:"id"`
	EventType  string    `json:"event_type"`
	ObjectType string    `json:"object_type"`
	ObjectID   int64     `json:"object_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type FutureExportSummary struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func CatalogProjectionFields() []string {
	profiles := classification.ProjectionProfiles()
	fields := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		fields = append(fields, profile.Name)
	}
	sort.Strings(fields)
	return fields
}

func CurrentOwnerContracts() []Contract {
	return []Contract{
		{Name: "owner_customer", View: OwnerCustomerView{}, AllowedJSONFields: []string{"id", "display_name", "customer_type", "country", "registration_number", "risk_level", "entity_hint", "created_at", "status"}, OwnerOnly: true},
		{Name: "owner_account", View: OwnerAccountView{}, AllowedJSONFields: []string{"id", "customer_id", "iban_like", "currency", "status", "created_at"}, OwnerOnly: true},
		{Name: "owner_transaction", View: OwnerTransactionView{}, AllowedJSONFields: []string{"id", "account_id", "counterparty_name", "counterparty_country", "counterparty_fingerprint", "amount", "currency", "direction", "purpose", "trace_id", "created_at", "risk_score"}, OwnerOnly: true},
		{Name: "owner_alert", View: OwnerAlertView{}, AllowedJSONFields: []string{"id", "transaction_id", "customer_id", "rule_code", "risk_score", "status", "assigned_user_id", "typology_hint", "created_at", "updated_at"}, OwnerOnly: true},
		{Name: "owner_case", View: OwnerCaseView{}, AllowedJSONFields: []string{"id", "customer_id", "alert_id", "status", "public_summary", "risk_score", "typology_hint", "created_at", "updated_at"}, OwnerOnly: true},
		{Name: "owner_case_note", View: OwnerCaseNoteView{}, AllowedJSONFields: []string{"id", "case_id", "author_user_id", "visibility", "body", "created_at", "updated_at"}, OwnerOnly: true},
		{Name: "owner_edd", View: OwnerEDDView{}, AllowedJSONFields: []string{"id", "customer_id", "analyst_user_id", "source_of_funds", "risk_rationale", "confidential_comment", "review_status", "created_at", "updated_at"}, OwnerOnly: true},
		{Name: "owner_case_audit", View: OwnerCaseAuditView{}, AllowedJSONFields: []string{"id", "object_type", "object_id", "event_type", "public_message", "changes", "created_at", "visibility_scope"}, OwnerOnly: true},
	}
}

func CurrentSideViewContracts() []Contract {
	return []Contract{
		{Name: "case_duplicate_preview", View: CaseDuplicatePreviewView{}, AllowedJSONFields: []string{"case_id", "public_summary", "risk_tags", "overlap_score", "match_explanation"}},
		{Name: "graph_entity", View: GraphEntityView{}, AllowedJSONFields: []string{"entity_hint", "relation_type", "customer_count", "case_count", "highest_risk", "risk_tags", "updated_at"}},
		{Name: "graph_related_case", View: GraphRelatedCaseView{}, AllowedJSONFields: []string{"case_id", "customer_id", "entity_hint", "display_name", "country", "status", "public_summary", "risk_score", "typology_hint", "created_at", "updated_at"}},
		{Name: "edd_graph_summary", View: EDDGraphSummaryView{}, AllowedJSONFields: []string{"customer_id", "entity_hint", "review_status", "risk_tags", "created_at", "updated_at"}},
		{Name: "export_bundle", View: ExportBundleView{}, AllowedJSONFields: []string{"purpose", "redaction_policy", "generated_at", "root_case", "linked_cases", "audit_summaries", "edd_summaries", "items"}},
		{Name: "export_case_shell", View: ExportCaseShellView{}, AllowedJSONFields: []string{"case_id", "customer_id", "status", "public_summary", "risk_score", "typology_hint", "created_at", "updated_at"}},
		{Name: "export_audit_summary", View: ExportAuditSummaryView{}, AllowedJSONFields: []string{"event_id", "object_type", "object_id", "event_type", "public_message", "changed_fields", "created_at"}},
		{Name: "export_edd_summary", View: ExportEDDSummaryView{}, AllowedJSONFields: []string{"record_id", "customer_id", "review_status", "risk_tags", "created_at", "updated_at"}},
		{Name: "export_item_metadata", View: ExportItemMetadataView{}, AllowedJSONFields: []string{"item_type", "object_type", "object_id", "relation", "redaction_policy"}},
		{Name: "network_room_summary", View: NetworkRoomSummaryView{}, AllowedJSONFields: []string{"id", "room_key", "entity_hint", "typology_hint", "status", "public_summary", "public_risk_tags", "case_count", "participant_count", "created_at", "updated_at"}},
		{Name: "network_room_case", View: NetworkRoomCaseView{}, AllowedJSONFields: []string{"case_id", "customer_id", "owned_by_requester", "status", "public_summary", "risk_score", "typology_hint", "public_risk_tags", "scope_coverage_complete", "created_at", "updated_at"}},
		{Name: "network_room_dossier", View: NetworkRoomDossierView{}, AllowedJSONFields: []string{"room", "cases", "scope_policy", "coverage_policy"}},
	}
}

func FutureSafeContracts() []Contract {
	return []Contract{
		{Name: "future_public_case_shell", View: FuturePublicCaseShell{}, AllowedJSONFields: []string{"id", "customer_id", "status", "public_summary", "risk_score", "typology_hint", "created_at", "updated_at"}},
		{Name: "future_duplicate_preview", View: FutureDuplicatePreview{}, AllowedJSONFields: []string{"candidate_customer_id", "match_customer_id", "display_name", "country", "risk_level", "match_reason", "confidence_bucket"}},
		{Name: "future_audit_summary", View: FutureAuditSummary{}, AllowedJSONFields: []string{"id", "event_type", "object_type", "object_id", "created_at"}},
		{Name: "future_export_summary", View: FutureExportSummary{}, AllowedJSONFields: []string{"id", "kind", "status", "created_at", "expires_at"}},
	}
}
