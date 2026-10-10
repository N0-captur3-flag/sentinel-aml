package classification

import (
	"sort"
	"strings"
)

type DataClass string

const (
	DataClassAccount       DataClass = "account"
	DataClassActivity      DataClass = "activity"
	DataClassCase          DataClass = "case"
	DataClassCustomer      DataClass = "customer"
	DataClassDocument      DataClass = "document"
	DataClassEDD           DataClass = "edd"
	DataClassEntity        DataClass = "entity"
	DataClassEvidence      DataClass = "evidence"
	DataClassExport        DataClass = "export"
	DataClassInvestigation DataClass = "investigation"
	DataClassNetwork       DataClass = "network"
	DataClassPolicy        DataClass = "policy"
	DataClassRisk          DataClass = "risk"
	DataClassScreening     DataClass = "screening"
	DataClassTransaction   DataClass = "transaction"
	DataClassWorkflow      DataClass = "workflow"
)

type Audience string

const (
	AudienceAnalyst       Audience = "analyst"
	AudienceCaseTeam      Audience = "case_team"
	AudienceExport        Audience = "export"
	AudienceInvestigation Audience = "investigation"
	AudienceNetwork       Audience = "network"
	AudienceReview        Audience = "review"
	AudienceSystem        Audience = "system"
)

type Disposition string

const (
	DispositionLabelOnly Disposition = "label_only"
	DispositionReference Disposition = "reference"
	DispositionSummary   Disposition = "summary"
	DispositionScoped    Disposition = "scoped"
	DispositionOmit      Disposition = "omit"
)

type FieldProfile struct {
	Name                  string
	DataDomain            DataClass
	DefaultAudience       Audience
	ExportDisposition     Disposition
	AuditDisposition      Disposition
	ProjectionDisposition Disposition
	PublicLabel           string
}

var profiles = []FieldProfile{
	profile("account_opening_note", DataClassAccount, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Account opening note"),
	profile("account_review_marker", DataClassAccount, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Account review marker"),
	profile("activity_subject", DataClassActivity, AudienceAnalyst, DispositionReference, DispositionReference, DispositionScoped, "Activity subject"),
	profile("adverse_media_label", DataClassScreening, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Adverse media label"),
	profile("alert_decision_reason", DataClassRisk, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Alert decision reason"),
	profile("approval_chain_marker", DataClassWorkflow, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Approval chain marker"),
	profile("assessment_summary", DataClassRisk, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Assessment summary"),
	profile("attachment_reference", DataClassDocument, AudienceCaseTeam, DispositionReference, DispositionReference, DispositionScoped, "Attachment reference"),
	profile("beneficial_owner_hint", DataClassCustomer, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Beneficial owner hint"),
	profile("business_profile_claim", DataClassCustomer, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Business profile claim"),
	profile("case_context_excerpt", DataClassCase, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Case context excerpt"),
	profile("case_disposition_reason", DataClassCase, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Case disposition reason"),
	profile("case_evidence_caption", DataClassEvidence, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Case evidence caption"),
	profile("case_match_explanation", DataClassCase, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Case match explanation"),
	profile("case_reference_alias", DataClassCase, AudienceCaseTeam, DispositionReference, DispositionReference, DispositionScoped, "Case reference alias"),
	profile("case_resolution_note", DataClassCase, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Case resolution note"),
	profile("case_working_observation", DataClassCase, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Case working observation"),
	profile("collaboration_context", DataClassInvestigation, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Collaboration context"),
	profile("collaboration_digest", DataClassInvestigation, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Collaboration digest"),
	profile("counterparty_profile_hint", DataClassCustomer, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Counterparty profile hint"),
	profile("country_risk_label", DataClassRisk, AudienceAnalyst, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Country risk label"),
	profile("customer_claim_reference", DataClassCustomer, AudienceAnalyst, DispositionReference, DispositionReference, DispositionScoped, "Customer claim reference"),
	profile("customer_document_label", DataClassDocument, AudienceAnalyst, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Customer document label"),
	profile("decision_support_excerpt", DataClassWorkflow, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Decision support excerpt"),
	profile("dispute_evidence_caption", DataClassEvidence, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Dispute evidence caption"),
	profile("dispute_resolution_summary", DataClassWorkflow, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Dispute resolution summary"),
	profile("dossier_context_excerpt", DataClassNetwork, AudienceNetwork, DispositionSummary, DispositionSummary, DispositionScoped, "Dossier context excerpt"),
	profile("edd_assessment_summary", DataClassEDD, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "EDD assessment summary"),
	profile("edd_review_caption", DataClassEDD, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "EDD review caption"),
	profile("entity_assessment_label", DataClassEntity, AudienceAnalyst, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Entity assessment label"),
	profile("entity_cluster_reason", DataClassEntity, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Entity cluster reason"),
	profile("entity_link_caption", DataClassEntity, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Entity link caption"),
	profile("entity_risk_indicator", DataClassRisk, AudienceAnalyst, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Entity risk indicator"),
	profile("evidence_chain_marker", DataClassEvidence, AudienceCaseTeam, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Evidence chain marker"),
	profile("evidence_excerpt", DataClassEvidence, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Evidence excerpt"),
	profile("evidence_origin_label", DataClassEvidence, AudienceCaseTeam, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Evidence origin label"),
	profile("export_coverage_note", DataClassExport, AudienceExport, DispositionSummary, DispositionSummary, DispositionScoped, "Export coverage note"),
	profile("export_item_caption", DataClassExport, AudienceExport, DispositionSummary, DispositionSummary, DispositionScoped, "Export item caption"),
	profile("export_scope_label", DataClassExport, AudienceExport, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Export scope label"),
	profile("external_reference_value", DataClassDocument, AudienceReview, DispositionReference, DispositionReference, DispositionScoped, "External reference value"),
	profile("financial_profile_hint", DataClassCustomer, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Financial profile hint"),
	profile("funds_flow_caption", DataClassTransaction, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Funds flow caption"),
	profile("identity_document_label", DataClassDocument, AudienceAnalyst, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Identity document label"),
	profile("internal_review_marker", DataClassWorkflow, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Internal review marker"),
	profile("investigation_context", DataClassInvestigation, AudienceInvestigation, DispositionSummary, DispositionSummary, DispositionScoped, "Investigation context"),
	profile("investigation_digest", DataClassInvestigation, AudienceInvestigation, DispositionSummary, DispositionSummary, DispositionScoped, "Investigation digest"),
	profile("kyc_document_caption", DataClassDocument, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "KYC document caption"),
	profile("legal_hold_marker", DataClassWorkflow, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Legal hold marker"),
	profile("linkage_reason", DataClassEntity, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Linkage reason"),
	profile("match_evidence_excerpt", DataClassEvidence, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Match evidence excerpt"),
	profile("monitoring_rule_caption", DataClassRisk, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Monitoring rule caption"),
	profile("network_context_digest", DataClassNetwork, AudienceNetwork, DispositionSummary, DispositionSummary, DispositionScoped, "Network context digest"),
	profile("network_room_caption", DataClassNetwork, AudienceNetwork, DispositionSummary, DispositionSummary, DispositionScoped, "Network room caption"),
	profile("operator_annotation", DataClassWorkflow, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Operator annotation"),
	profile("operator_marker", DataClassWorkflow, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Operator marker"),
	profile("party_reference_alias", DataClassCustomer, AudienceAnalyst, DispositionReference, DispositionReference, DispositionScoped, "Party reference alias"),
	profile("policy_evaluation_note", DataClassPolicy, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Policy evaluation note"),
	profile("provenance_caption", DataClassEvidence, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Provenance caption"),
	profile("quality_review_marker", DataClassWorkflow, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Quality review marker"),
	profile("reconciliation_note", DataClassWorkflow, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Reconciliation note"),
	profile("reference_family", DataClassDocument, AudienceReview, DispositionReference, DispositionReference, DispositionScoped, "Reference family"),
	profile("related_activity_caption", DataClassActivity, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Related activity caption"),
	profile("review_assignment_note", DataClassWorkflow, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Review assignment note"),
	profile("review_checkpoint", DataClassWorkflow, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Review checkpoint"),
	profile("review_context_excerpt", DataClassWorkflow, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Review context excerpt"),
	profile("review_scope_label", DataClassWorkflow, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Review scope label"),
	profile("risk_assessment_caption", DataClassRisk, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Risk assessment caption"),
	profile("risk_indicator_label", DataClassRisk, AudienceAnalyst, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Risk indicator label"),
	profile("risk_model_reason", DataClassRisk, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Risk model reason"),
	profile("risk_score_explanation", DataClassRisk, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Risk score explanation"),
	profile("sanctions_screening_label", DataClassScreening, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Sanctions screening label"),
	profile("screening_decision_note", DataClassScreening, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Screening decision note"),
	profile("source_material_caption", DataClassEvidence, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Source material caption"),
	profile("source_quality_marker", DataClassEvidence, AudienceCaseTeam, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Source quality marker"),
	profile("transaction_cluster_reason", DataClassTransaction, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Transaction cluster reason"),
	profile("transaction_evidence_caption", DataClassTransaction, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Transaction evidence caption"),
	profile("transfer_pattern_label", DataClassTransaction, AudienceAnalyst, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Transfer pattern label"),
	profile("verification_marker", DataClassWorkflow, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Verification marker"),
	profile("watchlist_reason", DataClassScreening, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Watchlist reason"),
	profile("workflow_decision_note", DataClassWorkflow, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Workflow decision note"),
	profile("workflow_transition_caption", DataClassWorkflow, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Workflow transition caption"),
	profile("account_status_caption", DataClassAccount, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Account status caption"),
	profile("case_queue_reason", DataClassCase, AudienceCaseTeam, DispositionSummary, DispositionSummary, DispositionScoped, "Case queue reason"),
	profile("customer_profile_hint", DataClassCustomer, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Customer profile hint"),
	profile("document_review_marker", DataClassDocument, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Document review marker"),
	profile("relationship_context", DataClassEntity, AudienceAnalyst, DispositionSummary, DispositionSummary, DispositionScoped, "Relationship context"),
	profile("room_coverage_note", DataClassNetwork, AudienceNetwork, DispositionSummary, DispositionSummary, DispositionScoped, "Room coverage note"),
	profile("screening_context_excerpt", DataClassScreening, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Screening context excerpt"),
	profile("transaction_route_label", DataClassTransaction, AudienceAnalyst, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Transaction route label"),
	profile("workflow_evidence_caption", DataClassWorkflow, AudienceReview, DispositionSummary, DispositionSummary, DispositionScoped, "Workflow evidence caption"),
	profile("review_outcome_label", DataClassWorkflow, AudienceReview, DispositionLabelOnly, DispositionLabelOnly, DispositionScoped, "Review outcome label"),
}

var profilesByName = indexProfiles(profiles)

var protectedSourceFields = map[string]struct{}{
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
	"authorization":                   {},
	"session":                         {},
	"session_secret":                  {},
	"raw_token":                       {},
}

func Catalog() []FieldProfile {
	out := append([]FieldProfile(nil), profiles...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func Lookup(name string) (FieldProfile, bool) {
	profile, ok := profilesByName[NormalizeName(name)]
	return profile, ok
}

func NormalizeName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	return normalized
}

func PublicFieldName(name string) string {
	normalized := NormalizeName(name)
	if profile, ok := Lookup(normalized); ok {
		return profile.Name
	}
	return normalized
}

func PublicFieldLabel(name string) string {
	if profile, ok := Lookup(name); ok {
		return profile.PublicLabel
	}
	return strings.TrimSpace(name)
}

func ProjectionProfiles() []FieldProfile {
	var out []FieldProfile
	for _, profile := range Catalog() {
		if profile.ProjectionDisposition != DispositionOmit {
			out = append(out, profile)
		}
	}
	return out
}

func IsProtectedSourceField(name string) bool {
	_, ok := protectedSourceFields[NormalizeName(name)]
	return ok
}

func AllowsProtectedSourceField(name string) bool {
	if IsProtectedSourceField(name) {
		return false
	}
	_, ok := Lookup(name)
	return ok
}

func profile(name string, domain DataClass, audience Audience, exportDisposition, auditDisposition, projectionDisposition Disposition, label string) FieldProfile {
	return FieldProfile{
		Name:                  name,
		DataDomain:            domain,
		DefaultAudience:       audience,
		ExportDisposition:     exportDisposition,
		AuditDisposition:      auditDisposition,
		ProjectionDisposition: projectionDisposition,
		PublicLabel:           label,
	}
}

func indexProfiles(values []FieldProfile) map[string]FieldProfile {
	index := make(map[string]FieldProfile, len(values))
	for _, profile := range values {
		index[NormalizeName(profile.Name)] = profile
	}
	return index
}
