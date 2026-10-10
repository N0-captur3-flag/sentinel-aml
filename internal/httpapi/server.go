package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	acct "sentinel-aml/internal/account"
	alertrepo "sentinel-aml/internal/alert"
	auditrepo "sentinel-aml/internal/audit"
	"sentinel-aml/internal/auth"
	"sentinel-aml/internal/authz"
	casework "sentinel-aml/internal/casework"
	cust "sentinel-aml/internal/customer"
	"sentinel-aml/internal/edd"
	exportrepo "sentinel-aml/internal/exporting"
	graphrepo "sentinel-aml/internal/graph"
	networkrepo "sentinel-aml/internal/networkroom"
	"sentinel-aml/internal/risk"
	txn "sentinel-aml/internal/transaction"
)

const (
	requestTimeout          = 5 * time.Second
	maxJSONRequestBodyBytes = 1 << 20
)

type AuthService interface {
	Register(context.Context, string, string) (auth.AuthResult, error)
	Login(context.Context, string, string) (auth.AuthResult, error)
	CurrentUser(context.Context, string) (auth.User, error)
}

type CustomerRepository interface {
	Create(context.Context, cust.CreateParams) (cust.Customer, error)
	GetByOwner(context.Context, int64, int64) (cust.Customer, error)
}

type AccountRepository interface {
	Create(context.Context, acct.CreateParams) (acct.Account, error)
	GetByOwner(context.Context, int64, int64) (acct.Account, error)
	ListByCustomerOwner(context.Context, int64, int64) ([]acct.Account, error)
}

type TransactionRepository interface {
	Create(context.Context, txn.CreateParams) (txn.Transaction, error)
	GetByOwner(context.Context, int64, int64) (txn.Transaction, error)
	ListByCustomerOwner(context.Context, int64, int64) ([]txn.Transaction, error)
}

type TransactionRiskService interface {
	ScoreTransaction(context.Context, int64, int64) (risk.Decision, error)
}

type AlertRepository interface {
	ListByOwner(context.Context, int64) ([]alertrepo.Alert, error)
	GetByOwner(context.Context, int64, int64) (alertrepo.Alert, error)
	Claim(context.Context, alertrepo.ClaimParams) (alertrepo.Alert, error)
	Triage(context.Context, alertrepo.TriageParams) (alertrepo.Alert, error)
}

type CaseworkRepository interface {
	CreateFromAlert(context.Context, casework.CreateCaseParams) (casework.Case, error)
	GetByOwner(context.Context, int64, int64) (casework.Case, error)
	UpdateStatus(context.Context, casework.UpdateCaseStatusParams) (casework.Case, error)
	FindDuplicateCandidatesByOwner(context.Context, int64, int64) ([]casework.DuplicateCandidate, error)
	CreateNote(context.Context, casework.CreateNoteParams) (casework.CaseNote, error)
	ListNotesByOwner(context.Context, int64, int64) ([]casework.CaseNote, error)
}

type EDDRepository interface {
	Create(context.Context, edd.CreateParams) (edd.EDDRecord, error)
	GetByOwnerCustomer(context.Context, int64, int64) (edd.EDDRecord, error)
	UpdateByOwnerCustomer(context.Context, edd.UpdateParams) (edd.EDDRecord, error)
}

type AuditRepository interface {
	Create(context.Context, auditrepo.CreateParams) (auditrepo.Event, error)
	ListCaseTimelineByOwner(context.Context, int64, int64) ([]auditrepo.Event, error)
	ListEntityActivityByRequester(context.Context, int64, string) ([]auditrepo.EntityActivityEvent, error)
}

type GraphRepository interface {
	HasEntityAccess(context.Context, int64, string) (bool, error)
	GetEntitySummary(context.Context, int64, string) (graphrepo.EntitySummary, error)
	ListRelatedCases(context.Context, int64, string) ([]graphrepo.RelatedCaseSummary, error)
	ListEDDSummaries(context.Context, int64, string) ([]graphrepo.EDDSummary, error)
}

type ExportRepository interface {
	CreateDispute(context.Context, exportrepo.CreateDisputeParams) (exportrepo.Dispute, error)
	GetDisputeByOwner(context.Context, int64, int64) (exportrepo.Dispute, error)
	CreateExport(context.Context, exportrepo.CreateExportParams) (exportrepo.ExportRecord, error)
	GetExportByOwner(context.Context, int64, int64) (exportrepo.ExportRecord, error)
	CreateExportItem(context.Context, exportrepo.CreateExportItemParams) (exportrepo.ExportItem, error)
}

type NetworkRoomRepository interface {
	JoinRoom(context.Context, networkrepo.JoinRoomParams) (networkrepo.Participant, error)
	ListParticipants(context.Context, int64) ([]networkrepo.Participant, error)
	ListRoomsForUser(context.Context, int64) ([]networkrepo.RoomSummary, error)
	GetRoomForUser(context.Context, int64, int64) (networkrepo.RoomSummary, error)
	ListDossierCases(context.Context, int64, int64) ([]networkrepo.CaseDossierInput, error)
}

type NetworkRoomDirtyCandidateQueue interface {
	EnqueueCaseCandidate(context.Context, string) error
}

type Server struct {
	auth         AuthService
	customers    CustomerRepository
	accounts     AccountRepository
	transactions TransactionRepository
	risk         TransactionRiskService
	alerts       AlertRepository
	cases        CaseworkRepository
	edd          EDDRepository
	audit        AuditRepository
	graph        GraphRepository
	exports      ExportRepository
	networkRooms NetworkRoomRepository
}

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	User    UserResponse    `json:"user"`
	Session SessionResponse `json:"session"`
}

type MeResponse struct {
	User UserResponse `json:"user"`
}

type UserResponse struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type SessionResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type CreateCustomerRequest struct {
	DisplayName        string `json:"display_name"`
	CustomerType       string `json:"customer_type"`
	Country            string `json:"country"`
	RegistrationNumber string `json:"registration_number"`
	RiskLevel          string `json:"risk_level,omitempty"`
	EntityHint         string `json:"entity_hint,omitempty"`
	Status             string `json:"status,omitempty"`
}

type CustomerResponse struct {
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

type CreateAccountRequest struct {
	IBANLike string `json:"iban_like"`
	Currency string `json:"currency"`
	Status   string `json:"status,omitempty"`
}

type AccountResponse struct {
	ID         int64     `json:"id"`
	CustomerID int64     `json:"customer_id"`
	IBANLike   string    `json:"iban_like"`
	Currency   string    `json:"currency"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type AccountsResponse struct {
	Accounts []AccountResponse `json:"accounts"`
}

type CreateTransactionRequest struct {
	AccountID               int64  `json:"account_id"`
	CounterpartyName        string `json:"counterparty_name"`
	CounterpartyCountry     string `json:"counterparty_country"`
	CounterpartyFingerprint string `json:"counterparty_fingerprint"`
	Amount                  string `json:"amount"`
	Currency                string `json:"currency"`
	Direction               string `json:"direction"`
	Purpose                 string `json:"purpose"`
	TraceID                 string `json:"trace_id"`
}

type TransactionResponse struct {
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

type TransactionsResponse struct {
	Transactions []TransactionResponse `json:"transactions"`
}

type AlertResponse struct {
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

type AlertsResponse struct {
	Alerts []AlertResponse `json:"alerts"`
}

type TriageAlertRequest struct {
	Status string `json:"status"`
}

type CreateCaseRequest struct {
	PublicSummary string `json:"public_summary,omitempty"`
	Status        string `json:"status,omitempty"`
	DedupHint     string `json:"dedup_hint,omitempty"`
}

type CaseResponse struct {
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

type UpdateCaseStatusRequest struct {
	Status string `json:"status"`
}

type CreateCaseNoteRequest struct {
	Visibility string `json:"visibility,omitempty"`
	Body       string `json:"body"`
}

type CaseNoteResponse struct {
	ID           int64     `json:"id"`
	CaseID       int64     `json:"case_id"`
	AuthorUserID int64     `json:"author_user_id"`
	Visibility   string    `json:"visibility"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CaseNotesResponse struct {
	Notes []CaseNoteResponse `json:"notes"`
}

type CaseDuplicatePreviewView struct {
	CaseID           int64    `json:"case_id"`
	PublicSummary    string   `json:"public_summary"`
	RiskTags         []string `json:"risk_tags"`
	OverlapScore     int      `json:"overlap_score"`
	MatchExplanation string   `json:"match_explanation"`
}

type CaseDuplicateCandidatesResponse struct {
	Candidates []CaseDuplicatePreviewView `json:"candidates"`
}

type CaseDuplicateMergePreviewResponse struct {
	SourceCaseID int64                    `json:"source_case_id"`
	Candidate    CaseDuplicatePreviewView `json:"candidate"`
}

type CreateEDDRequest struct {
	SourceOfFunds       string `json:"source_of_funds"`
	RiskRationale       string `json:"risk_rationale"`
	ConfidentialComment string `json:"confidential_comment"`
	ReviewStatus        string `json:"review_status,omitempty"`
}

type UpdateEDDRequest struct {
	SourceOfFunds       string `json:"source_of_funds"`
	RiskRationale       string `json:"risk_rationale"`
	ConfidentialComment string `json:"confidential_comment"`
	ReviewStatus        string `json:"review_status"`
}

type EDDOwnerView struct {
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

type CaseAuditChangeResponse struct {
	Field    string `json:"field"`
	Old      string `json:"old,omitempty"`
	New      string `json:"new,omitempty"`
	Redacted bool   `json:"redacted"`
}

type CaseAuditView struct {
	ID              int64                     `json:"id"`
	ObjectType      string                    `json:"object_type"`
	ObjectID        int64                     `json:"object_id"`
	EventType       string                    `json:"event_type"`
	PublicMessage   string                    `json:"public_message"`
	Changes         []CaseAuditChangeResponse `json:"changes"`
	CreatedAt       time.Time                 `json:"created_at"`
	VisibilityScope string                    `json:"visibility_scope"`
}

type CaseTimelineResponse struct {
	Events []CaseAuditView `json:"events"`
}

type EntityActivityView struct {
	CaseID          int64                     `json:"case_id"`
	CustomerID      int64                     `json:"customer_id"`
	EntityHint      string                    `json:"entity_hint"`
	PublicSummary   string                    `json:"public_summary"`
	ActivitySubject string                    `json:"activity_subject,omitempty"`
	ActivityLevel   string                    `json:"activity_detail_level,omitempty"`
	ActivityText    string                    `json:"activity_narrative,omitempty"`
	ActivitySource  string                    `json:"activity_provenance,omitempty"`
	ObjectType      string                    `json:"object_type"`
	ObjectID        int64                     `json:"object_id"`
	EventType       string                    `json:"event_type"`
	PublicMessage   string                    `json:"public_message"`
	Changes         []CaseAuditChangeResponse `json:"changes"`
	CreatedAt       time.Time                 `json:"created_at"`
	VisibilityScope string                    `json:"visibility_scope"`
}

type EntityActivityResponse struct {
	Activities []EntityActivityView `json:"activities"`
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

type GraphRelatedCasesResponse struct {
	Cases []GraphRelatedCaseView `json:"cases"`
}

type EDDGraphSummaryView struct {
	CustomerID   int64     `json:"customer_id"`
	EntityHint   string    `json:"entity_hint"`
	ReviewStatus string    `json:"review_status"`
	RiskTags     []string  `json:"risk_tags"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type EDDGraphSummariesResponse struct {
	Summaries []EDDGraphSummaryView `json:"summaries"`
}

type CreateExportRequest struct {
	Purpose       string   `json:"purpose"`
	AllowedScopes []string `json:"allowed_scopes,omitempty"`
}

type CreateDisputeRequest struct {
	CaseID int64  `json:"case_id"`
	Reason string `json:"reason"`
	Status string `json:"status,omitempty"`
}

type DisputeResponse struct {
	ID                int64     `json:"id"`
	CaseID            int64     `json:"case_id"`
	Status            string    `json:"status"`
	Reason            string    `json:"reason"`
	ResolutionSummary string    `json:"resolution_summary"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ExportResponse struct {
	ID              int64            `json:"id"`
	RootCaseID      int64            `json:"root_case_id"`
	DisputeID       int64            `json:"dispute_id,omitempty"`
	Purpose         string           `json:"purpose"`
	Status          string           `json:"status"`
	RedactionPolicy string           `json:"redaction_policy"`
	Bundle          ExportBundleView `json:"bundle"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	ExpiresAt       time.Time        `json:"expires_at"`
}

type ExportBundleView struct {
	Purpose         string                   `json:"purpose"`
	RedactionPolicy string                   `json:"redaction_policy"`
	GeneratedAt     time.Time                `json:"generated_at"`
	RootCase        ExportCaseShellView      `json:"root_case"`
	LinkedCases     []ExportCaseShellView    `json:"linked_cases"`
	LinkedEvidence  []map[string]any         `json:"linked_evidence,omitempty"`
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

type NetworkRoomsResponse struct {
	Rooms []NetworkRoomSummaryView `json:"rooms"`
}

type NetworkRoomDetailResponse struct {
	Room         NetworkRoomSummaryView       `json:"room"`
	Participants []NetworkRoomParticipantView `json:"participants"`
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

type NetworkRoomParticipantView struct {
	ID              int64     `json:"id"`
	RoomID          int64     `json:"room_id"`
	UserID          int64     `json:"user_id"`
	ParticipantRole string    `json:"participant_role"`
	JoinedAt        time.Time `json:"joined_at"`
}

type NetworkRoomDossierView struct {
	Room           NetworkRoomSummaryView `json:"room"`
	Cases          []NetworkRoomCaseView  `json:"cases"`
	ScopePolicy    string                 `json:"scope_policy"`
	CoveragePolicy string                 `json:"coverage_policy"`
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

type ErrorResponse struct {
	Error string `json:"error"`
}

type Option func(*Server)

func WithDomainRepositories(customers CustomerRepository, accounts AccountRepository, transactions TransactionRepository) Option {
	return func(s *Server) {
		s.customers = customers
		s.accounts = accounts
		s.transactions = transactions
	}
}

func WithTransactionRiskService(transactionRisk TransactionRiskService) Option {
	return func(s *Server) {
		s.risk = transactionRisk
	}
}

func WithInvestigationRepositories(alerts AlertRepository, cases CaseworkRepository) Option {
	return func(s *Server) {
		s.alerts = alerts
		s.cases = cases
	}
}

func WithEDDRepository(repository EDDRepository) Option {
	return func(s *Server) {
		s.edd = repository
	}
}

func WithAuditRepository(repository AuditRepository) Option {
	return func(s *Server) {
		s.audit = repository
	}
}

func WithGraphRepository(repository GraphRepository) Option {
	return func(s *Server) {
		s.graph = repository
	}
}

func WithExportRepository(repository ExportRepository) Option {
	return func(s *Server) {
		s.exports = repository
	}
}

func WithNetworkRoomRepository(repository NetworkRoomRepository) Option {
	return func(s *Server) {
		s.networkRooms = repository
	}
}

func NewServer(authService AuthService, options ...Option) *Server {
	server := &Server{auth: authService}
	for _, option := range options {
		option(server)
	}
	return server
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", HealthHandler)
	mux.HandleFunc("/readyz", HealthHandler)
	mux.HandleFunc("/api/auth/register", s.handleRegister)
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/me", s.requireAuth(s.handleMe))
	if s.domainReady() {
		mux.HandleFunc("/api/customers", s.requireAuth(s.handleCustomersRoot))
		mux.HandleFunc("/api/customers/", s.requireAuth(s.handleCustomersSubtree))
		mux.HandleFunc("/api/accounts/", s.requireAuth(s.handleAccountsSubtree))
		mux.HandleFunc("/api/transactions", s.requireAuth(s.handleTransactionsRoot))
		mux.HandleFunc("/api/transactions/", s.requireAuth(s.handleTransactionsSubtree))
	}
	if s.investigationReady() {
		mux.HandleFunc("/api/alerts/queue", s.requireAuth(s.handleAlertsQueue))
		mux.HandleFunc("/api/alerts/", s.requireAuth(s.handleAlertsSubtree))
		mux.HandleFunc("/api/cases/", s.requireAuth(s.handleCasesSubtree))
	}
	if s.audit != nil {
		mux.HandleFunc("/api/entities/", s.requireAuth(s.handleEntitiesSubtree))
	}
	if s.graphReady() {
		mux.HandleFunc("/api/graph/entities/", s.requireAuth(s.handleGraphEntitiesSubtree))
	}
	if s.exportReady() {
		mux.HandleFunc("/api/exports", s.requireAuth(s.handleExportsRoot))
		mux.HandleFunc("/api/exports/", s.requireAuth(s.handleExportsSubtree))
		mux.HandleFunc("/api/disputes", s.requireAuth(s.handleDisputesRoot))
		mux.HandleFunc("/api/disputes/", s.requireAuth(s.handleDisputesSubtree))
	}
	if s.networkRoomReady() {
		mux.HandleFunc("/api/network/rooms", s.requireAuth(s.handleNetworkRoomsRoot))
		mux.HandleFunc("/api/network/rooms/", s.requireAuth(s.handleNetworkRoomsSubtree))
	}
	return mux
}

func (s *Server) domainReady() bool {
	return s.customers != nil && s.accounts != nil && s.transactions != nil
}

func (s *Server) investigationReady() bool {
	return s.alerts != nil && s.cases != nil
}

func (s *Server) graphReady() bool {
	return s.graph != nil
}

func (s *Server) exportReady() bool {
	return s.exports != nil && s.cases != nil
}

func (s *Server) networkRoomReady() bool {
	return s.networkRooms != nil
}

func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "sentinel-aml",
		"status":  "ok",
		"scope":   "operational-health-only",
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}
	req, ok := decodeAuthRequest(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	result, err := s.auth.Register(ctx, req.Username, req.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, authResultResponse(result))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}
	req, ok := decodeAuthRequest(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	result, err := s.auth.Login(ctx, req.Username, req.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, authResultResponse(result))
}

func (s *Server) requireAuth(next func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		user, err := s.auth.CurrentUser(ctx, token)
		if err != nil {
			writeAuthError(w, err)
			return
		}

		principal := authz.Principal{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
			Status:   user.Status,
		}
		next(w, r.WithContext(authz.WithPrincipal(r.Context(), principal)), user)
	}
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}
	if _, ok := authz.PrincipalFromContext(r.Context()); !ok {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthenticated"})
		return
	}
	writeJSON(w, http.StatusOK, MeResponse{User: userResponse(user)})
}

func (s *Server) handleCustomersRoot(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.URL.Path != "/api/customers" {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	var req CreateCustomerRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	customer, err := s.customers.Create(ctx, cust.CreateParams{
		OwnerUserID:        user.ID,
		DisplayName:        req.DisplayName,
		CustomerType:       req.CustomerType,
		Country:            req.Country,
		RegistrationNumber: req.RegistrationNumber,
		RiskLevel:          req.RiskLevel,
		EntityHint:         req.EntityHint,
		Status:             req.Status,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, customerResponse(customer))
}

func (s *Server) handleCustomersSubtree(w http.ResponseWriter, r *http.Request, user auth.User) {
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/customers/"))
	if len(parts) == 0 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}

	customerID, ok := parseID(w, parts[0])
	if !ok {
		return
	}

	switch {
	case len(parts) == 1:
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
			return
		}
		s.handleGetCustomer(w, r, user, customerID)
	case len(parts) == 2 && parts[1] == "accounts":
		s.handleCustomerAccounts(w, r, user, customerID)
	case len(parts) == 2 && parts[1] == "transactions":
		s.handleCustomerTransactions(w, r, user, customerID)
	case len(parts) == 2 && parts[1] == "edd" && s.edd != nil:
		s.handleCustomerEDD(w, r, user, customerID)
	case len(parts) == 2 && parts[1] == "activity":
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
	default:
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
	}
}

func (s *Server) handleGetCustomer(w http.ResponseWriter, r *http.Request, user auth.User, customerID int64) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	customer, err := s.customers.GetByOwner(ctx, user.ID, customerID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, customerResponse(customer))
}

func (s *Server) handleCustomerAccounts(w http.ResponseWriter, r *http.Request, user auth.User, customerID int64) {
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		accounts, err := s.accounts.ListByCustomerOwner(ctx, user.ID, customerID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, AccountsResponse{Accounts: accountResponses(accounts)})
	case http.MethodPost:
		var req CreateAccountRequest
		if !decodeJSONRequest(w, r, &req) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		account, err := s.accounts.Create(ctx, acct.CreateParams{
			OwnerUserID: user.ID,
			CustomerID:  customerID,
			IBANLike:    req.IBANLike,
			Currency:    req.Currency,
			Status:      req.Status,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, accountResponse(account))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
	}
}

func (s *Server) handleCustomerTransactions(w http.ResponseWriter, r *http.Request, user auth.User, customerID int64) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	transactions, err := s.transactions.ListByCustomerOwner(ctx, user.ID, customerID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, TransactionsResponse{Transactions: transactionResponses(transactions)})
}

func (s *Server) handleCustomerEDD(w http.ResponseWriter, r *http.Request, user auth.User, customerID int64) {
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		record, err := s.edd.GetByOwnerCustomer(ctx, user.ID, customerID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, eddOwnerView(record))
	case http.MethodPost:
		var req CreateEDDRequest
		if !decodeJSONRequest(w, r, &req) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		record, err := s.edd.Create(ctx, edd.CreateParams{
			OwnerUserID:         user.ID,
			CustomerID:          customerID,
			AnalystUserID:       user.ID,
			SourceOfFunds:       req.SourceOfFunds,
			RiskRationale:       req.RiskRationale,
			ConfidentialComment: req.ConfidentialComment,
			ReviewStatus:        req.ReviewStatus,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		if err := s.recordAudit(ctx, auditrepo.CreateParams{
			ActorUserID:   user.ID,
			ObjectType:    "edd_record",
			ObjectID:      record.ID,
			EventType:     "created",
			PublicMessage: "edd record created",
			FieldChanges: []auditrepo.FieldChange{
				{Field: "source_of_funds", NewValue: record.SourceOfFunds},
				{Field: "risk_rationale", NewValue: record.RiskRationale},
				{Field: "edd_record.confidential_comment", NewValue: record.ConfidentialComment},
				{Field: "review_status", NewValue: record.ReviewStatus},
			},
		}); err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, eddOwnerView(record))
	case http.MethodPatch:
		var req UpdateEDDRequest
		if !decodeJSONRequest(w, r, &req) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		var previous edd.EDDRecord
		if s.audit != nil {
			var err error
			previous, err = s.edd.GetByOwnerCustomer(ctx, user.ID, customerID)
			if err != nil {
				writeDomainError(w, err)
				return
			}
		}
		record, err := s.edd.UpdateByOwnerCustomer(ctx, edd.UpdateParams{
			OwnerUserID:         user.ID,
			CustomerID:          customerID,
			AnalystUserID:       user.ID,
			SourceOfFunds:       req.SourceOfFunds,
			RiskRationale:       req.RiskRationale,
			ConfidentialComment: req.ConfidentialComment,
			ReviewStatus:        req.ReviewStatus,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		if err := s.recordAudit(ctx, auditrepo.CreateParams{
			ActorUserID:   user.ID,
			ObjectType:    "edd_record",
			ObjectID:      record.ID,
			EventType:     "updated",
			PublicMessage: "edd record updated",
			FieldChanges: []auditrepo.FieldChange{
				{Field: "source_of_funds", OldValue: previous.SourceOfFunds, NewValue: record.SourceOfFunds},
				{Field: "risk_rationale", OldValue: previous.RiskRationale, NewValue: record.RiskRationale},
				{Field: "edd_record.confidential_comment", OldValue: previous.ConfidentialComment, NewValue: record.ConfidentialComment},
				{Field: "review_status", OldValue: previous.ReviewStatus, NewValue: record.ReviewStatus},
			},
		}); err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, eddOwnerView(record))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
	}
}

func (s *Server) handleAccountsSubtree(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/accounts/"))
	if len(parts) != 1 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}
	accountID, ok := parseID(w, parts[0])
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	account, err := s.accounts.GetByOwner(ctx, user.ID, accountID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accountResponse(account))
}

func (s *Server) handleTransactionsRoot(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.URL.Path != "/api/transactions" {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	var req CreateTransactionRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	transaction, err := s.transactions.Create(ctx, txn.CreateParams{
		OwnerUserID:             user.ID,
		AccountID:               req.AccountID,
		CounterpartyName:        req.CounterpartyName,
		CounterpartyCountry:     req.CounterpartyCountry,
		CounterpartyFingerprint: req.CounterpartyFingerprint,
		Amount:                  req.Amount,
		Currency:                req.Currency,
		Direction:               req.Direction,
		Purpose:                 req.Purpose,
		TraceID:                 req.TraceID,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if s.risk != nil {
		decision, err := s.risk.ScoreTransaction(ctx, user.ID, transaction.ID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		if decision.Transaction.ID != 0 {
			transaction = decision.Transaction
		}
	}
	writeJSON(w, http.StatusCreated, transactionResponse(transaction))
}

func (s *Server) handleTransactionsSubtree(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/transactions/"))
	if len(parts) != 1 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}
	transactionID, ok := parseID(w, parts[0])
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	transaction, err := s.transactions.GetByOwner(ctx, user.ID, transactionID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, transactionResponse(transaction))
}

func (s *Server) handleAlertsQueue(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.URL.Path != "/api/alerts/queue" {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	alerts, err := s.alerts.ListByOwner(ctx, user.ID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, AlertsResponse{Alerts: alertResponses(alerts)})
}

func (s *Server) handleAlertsSubtree(w http.ResponseWriter, r *http.Request, user auth.User) {
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/alerts/"))
	if len(parts) == 0 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}

	alertID, ok := parseID(w, parts[0])
	if !ok {
		return
	}

	switch {
	case len(parts) == 1:
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
			return
		}
		s.handleGetAlert(w, r, user, alertID)
	case len(parts) == 2 && parts[1] == "claim":
		s.handleClaimAlert(w, r, user, alertID)
	case len(parts) == 2 && parts[1] == "triage":
		s.handleTriageAlert(w, r, user, alertID)
	case len(parts) == 2 && parts[1] == "case":
		s.handleCreateCaseFromAlert(w, r, user, alertID)
	default:
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
	}
}

func (s *Server) handleGetAlert(w http.ResponseWriter, r *http.Request, user auth.User, alertID int64) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	alert, err := s.alerts.GetByOwner(ctx, user.ID, alertID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alertResponse(alert))
}

func (s *Server) handleClaimAlert(w http.ResponseWriter, r *http.Request, user auth.User, alertID int64) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	alert, err := s.alerts.Claim(ctx, alertrepo.ClaimParams{
		OwnerUserID:    user.ID,
		AlertID:        alertID,
		AssignedUserID: user.ID,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alertResponse(alert))
}

func (s *Server) handleTriageAlert(w http.ResponseWriter, r *http.Request, user auth.User, alertID int64) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	var req TriageAlertRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	alert, err := s.alerts.Triage(ctx, alertrepo.TriageParams{
		OwnerUserID: user.ID,
		AlertID:     alertID,
		Status:      req.Status,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alertResponse(alert))
}

func (s *Server) handleCreateCaseFromAlert(w http.ResponseWriter, r *http.Request, user auth.User, alertID int64) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	var req CreateCaseRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	investigationCase, err := s.cases.CreateFromAlert(ctx, casework.CreateCaseParams{
		OwnerUserID:   user.ID,
		AlertID:       alertID,
		PublicSummary: req.PublicSummary,
		Status:        req.Status,
		DedupHint:     req.DedupHint,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := s.recordAudit(ctx, auditrepo.CreateParams{
		ActorUserID:   user.ID,
		ObjectType:    "case",
		ObjectID:      investigationCase.ID,
		EventType:     "created",
		PublicMessage: "case created",
		FieldChanges: []auditrepo.FieldChange{
			{Field: "status", NewValue: investigationCase.Status},
			{Field: "public_summary", NewValue: investigationCase.PublicSummary},
		},
	}); err != nil {
		writeDomainError(w, err)
		return
	}
	if queue, ok := s.networkRooms.(NetworkRoomDirtyCandidateQueue); ok {
		if err := queue.EnqueueCaseCandidate(ctx, strconv.FormatInt(investigationCase.ID, 10)); err != nil {
			slog.Default().Warn("network-room dirty candidate enqueue failed", "case_id", investigationCase.ID, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, caseResponse(investigationCase))
}

func (s *Server) handleCasesSubtree(w http.ResponseWriter, r *http.Request, user auth.User) {
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/cases/"))
	if len(parts) == 0 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}

	caseID, ok := parseID(w, parts[0])
	if !ok {
		return
	}

	switch {
	case len(parts) == 1:
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
			return
		}
		s.handleGetCase(w, r, user, caseID)
	case len(parts) == 2 && parts[1] == "notes":
		s.handleCaseNotes(w, r, user, caseID)
	case len(parts) == 2 && parts[1] == "status":
		s.handleUpdateCaseStatus(w, r, user, caseID)
	case len(parts) == 2 && parts[1] == "duplicates":
		s.handleCaseDuplicateCandidates(w, r, user, caseID)
	case len(parts) == 2 && parts[1] == "timeline" && s.audit != nil:
		s.handleCaseTimeline(w, r, user, caseID)
	case len(parts) == 2 && parts[1] == "exports" && s.exports != nil:
		s.handleCreateCaseExport(w, r, user, caseID, 0)
	case len(parts) == 4 && parts[1] == "duplicates" && parts[3] == "merge-preview":
		duplicateCaseID, ok := parseID(w, parts[2])
		if !ok {
			return
		}
		s.handleCaseDuplicateMergePreview(w, r, user, caseID, duplicateCaseID)
	default:
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
	}
}

func (s *Server) handleGetCase(w http.ResponseWriter, r *http.Request, user auth.User, caseID int64) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	investigationCase, err := s.cases.GetByOwner(ctx, user.ID, caseID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, caseResponse(investigationCase))
}

func (s *Server) handleCaseTimeline(w http.ResponseWriter, r *http.Request, user auth.User, caseID int64) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	if _, err := s.cases.GetByOwner(ctx, user.ID, caseID); err != nil {
		writeDomainError(w, err)
		return
	}
	events, err := s.audit.ListCaseTimelineByOwner(ctx, user.ID, caseID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, CaseTimelineResponse{Events: caseAuditViews(events)})
}

func (s *Server) handleEntitiesSubtree(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/entities/"))
	if len(parts) != 2 || parts[1] != "activity" {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}
	entityHint, err := urlPathUnescape(parts[0])
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid_path"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	events, err := s.audit.ListEntityActivityByRequester(ctx, user.ID, entityHint)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	activities, err := s.entityActivityViewsForActivityStream(ctx, user.ID, entityHint, events)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, EntityActivityResponse{Activities: activities})
}

func (s *Server) handleCaseDuplicateCandidates(w http.ResponseWriter, r *http.Request, user auth.User, caseID int64) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	candidates, err := s.cases.FindDuplicateCandidatesByOwner(ctx, user.ID, caseID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, CaseDuplicateCandidatesResponse{Candidates: duplicatePreviewViews(candidates)})
}

func (s *Server) handleCaseDuplicateMergePreview(w http.ResponseWriter, r *http.Request, user auth.User, sourceCaseID, duplicateCaseID int64) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	candidates, err := s.cases.FindDuplicateCandidatesByOwner(ctx, user.ID, sourceCaseID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	for _, candidate := range candidates {
		if candidate.CaseID == duplicateCaseID {
			response, err := s.caseDuplicateMergePreviewResponse(ctx, user.ID, sourceCaseID, candidate)
			if err != nil {
				writeDomainError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
}

func (s *Server) handleCaseNotes(w http.ResponseWriter, r *http.Request, user auth.User, caseID int64) {
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		notes, err := s.cases.ListNotesByOwner(ctx, user.ID, caseID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, CaseNotesResponse{Notes: caseNoteResponses(notes)})
	case http.MethodPost:
		var req CreateCaseNoteRequest
		if !decodeJSONRequest(w, r, &req) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		note, err := s.cases.CreateNote(ctx, casework.CreateNoteParams{
			OwnerUserID:  user.ID,
			CaseID:       caseID,
			AuthorUserID: user.ID,
			Visibility:   req.Visibility,
			Body:         req.Body,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		if err := s.recordAudit(ctx, auditrepo.CreateParams{
			ActorUserID:   user.ID,
			ObjectType:    "case_note",
			ObjectID:      note.ID,
			EventType:     "created",
			PublicMessage: "case note created",
			FieldChanges: []auditrepo.FieldChange{
				{Field: "case_note.body", NewValue: note.Body},
				{Field: "visibility", NewValue: note.Visibility},
			},
		}); err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, caseNoteResponse(note))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
	}
}

func (s *Server) handleUpdateCaseStatus(w http.ResponseWriter, r *http.Request, user auth.User, caseID int64) {
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	var req UpdateCaseStatusRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	investigationCase, err := s.cases.UpdateStatus(ctx, casework.UpdateCaseStatusParams{
		OwnerUserID: user.ID,
		CaseID:      caseID,
		Status:      req.Status,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := s.recordAudit(ctx, auditrepo.CreateParams{
		ActorUserID:   user.ID,
		ObjectType:    "case",
		ObjectID:      investigationCase.ID,
		EventType:     "status_changed",
		PublicMessage: "case status changed",
		FieldChanges: []auditrepo.FieldChange{
			{Field: "status", NewValue: investigationCase.Status},
		},
	}); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, caseResponse(investigationCase))
}

func (s *Server) handleCreateCaseExport(w http.ResponseWriter, r *http.Request, user auth.User, caseID, disputeID int64) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	var req CreateExportRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	bundle, err := s.buildExportBundle(ctx, user.ID, caseID, disputeID, req)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		writeDomainError(w, exportrepo.ErrInvalidInput)
		return
	}
	record, err := s.exports.CreateExport(ctx, exportrepo.CreateExportParams{
		OwnerUserID: user.ID,
		RootCaseID:  caseID,
		DisputeID:   disputeID,
		Purpose:     req.Purpose,
		Status:      "GENERATED",
		BundleJSON:  string(bundleJSON),
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	for _, item := range bundle.Items {
		itemJSON, err := json.Marshal(item)
		if err != nil {
			writeDomainError(w, exportrepo.ErrInvalidInput)
			return
		}
		if _, err := s.exports.CreateExportItem(ctx, exportrepo.CreateExportItemParams{
			OwnerUserID: user.ID,
			ExportID:    record.ID,
			ItemType:    item.ItemType,
			ObjectType:  item.ObjectType,
			ObjectID:    item.ObjectID,
			Relation:    item.Relation,
			PayloadJSON: string(itemJSON),
		}); err != nil {
			writeDomainError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, exportResponse(record, bundle))
}

func (s *Server) handleExportsSubtree(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/exports/"))
	if len(parts) != 1 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}
	exportID, ok := parseID(w, parts[0])
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	record, err := s.exports.GetExportByOwner(ctx, user.ID, exportID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	bundle, err := decodeExportBundle(record.BundleJSON)
	if err != nil {
		writeDomainError(w, exportrepo.ErrInvalidInput)
		return
	}
	writeJSON(w, http.StatusOK, exportResponse(record, bundle))
}

func (s *Server) handleExportsRoot(w http.ResponseWriter, _ *http.Request, _ auth.User) {
	writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
}

func (s *Server) handleDisputesRoot(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.URL.Path != "/api/disputes" {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}
	var req CreateDisputeRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	dispute, err := s.exports.CreateDispute(ctx, exportrepo.CreateDisputeParams{
		OwnerUserID: user.ID,
		CaseID:      req.CaseID,
		Status:      req.Status,
		Reason:      req.Reason,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, disputeResponse(dispute))
}

func (s *Server) handleDisputesSubtree(w http.ResponseWriter, r *http.Request, user auth.User) {
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/disputes/"))
	if len(parts) != 2 || parts[1] != "export" {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}
	disputeID, ok := parseID(w, parts[0])
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	dispute, err := s.exports.GetDisputeByOwner(ctx, user.ID, disputeID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.handleCreateCaseExport(w, r.WithContext(ctx), user, dispute.CaseID, dispute.ID)
}

func (s *Server) handleGraphEntitiesSubtree(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}

	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/graph/entities/"))
	if len(parts) == 0 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}

	entityHint := parts[0]
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	allowed, err := s.graph.HasEntityAccess(ctx, user.ID, entityHint)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if !allowed {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}

	switch {
	case len(parts) == 1:
		s.handleGraphEntity(w, ctx, user, entityHint)
	case len(parts) == 2 && parts[1] == "related-cases":
		s.handleGraphRelatedCases(w, ctx, user, entityHint)
	case len(parts) == 2 && parts[1] == "edd-summaries":
		s.handleGraphEDDSummaries(w, ctx, user, entityHint)
	default:
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
	}
}

func (s *Server) handleGraphEntity(w http.ResponseWriter, ctx context.Context, user auth.User, entityHint string) {
	summary, err := s.graph.GetEntitySummary(ctx, user.ID, entityHint)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graphEntityView(summary))
}

func (s *Server) handleGraphRelatedCases(w http.ResponseWriter, ctx context.Context, user auth.User, entityHint string) {
	cases, err := s.graph.ListRelatedCases(ctx, user.ID, entityHint)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, GraphRelatedCasesResponse{Cases: graphRelatedCaseViews(cases)})
}

func (s *Server) handleGraphEDDSummaries(w http.ResponseWriter, ctx context.Context, user auth.User, entityHint string) {
	summaries, err := s.graph.ListEDDSummaries(ctx, user.ID, entityHint)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	response, err := s.graphEDDSummariesResponseForEntityReview(ctx, user.ID, entityHint, summaries)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleNetworkRoomsRoot(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	rooms, err := s.networkRooms.ListRoomsForUser(ctx, user.ID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, NetworkRoomsResponse{Rooms: networkRoomSummaryViews(rooms)})
}

func (s *Server) handleNetworkRoomsSubtree(w http.ResponseWriter, r *http.Request, user auth.User) {
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/network/rooms/"))
	if len(parts) < 1 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
		return
	}
	roomID, ok := parseID(w, parts[0])
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	switch {
	case len(parts) == 1:
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
			return
		}
		room, err := s.networkRooms.GetRoomForUser(ctx, user.ID, roomID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		participants, err := s.networkRooms.ListParticipants(ctx, roomID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, NetworkRoomDetailResponse{
			Room:         networkRoomSummaryView(room),
			Participants: networkRoomParticipantViews(participants),
		})
	case len(parts) == 2 && parts[1] == "join":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
			return
		}
		participant, err := s.networkRooms.JoinRoom(ctx, networkrepo.JoinRoomParams{RoomID: roomID, UserID: user.ID})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, networkRoomParticipantView(participant))
	case len(parts) == 2 && parts[1] == "dossier":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed"})
			return
		}
		room, err := s.networkRooms.GetRoomForUser(ctx, user.ID, roomID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		dossier, err := s.networkRoomDossierResponseForSharedRoom(ctx, user.ID, roomID, room)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dossier)
	default:
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
	}
}

func decodeAuthRequest(w http.ResponseWriter, r *http.Request) (RegisterRequest, bool) {
	var req RegisterRequest
	if !decodeJSONRequest(w, r, &req) {
		return RegisterRequest{}, false
	}
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid_request"})
		return RegisterRequest{}, false
	}
	return req, true
}

func decodeJSONRequest(w http.ResponseWriter, r *http.Request, dest any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid_json"})
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid_json"})
		return false
	}
	return true
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return parts[1], true
}

func authResultResponse(result auth.AuthResult) AuthResponse {
	return AuthResponse{
		User: userResponse(result.User),
		Session: SessionResponse{
			Token:     result.RawToken,
			ExpiresAt: result.ExpiresAt,
		},
	}
}

func userResponse(user auth.User) UserResponse {
	return UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
	}
}

func customerResponse(customer cust.Customer) CustomerResponse {
	return CustomerResponse{
		ID:                 customer.ID,
		DisplayName:        customer.DisplayName,
		CustomerType:       customer.CustomerType,
		Country:            customer.Country,
		RegistrationNumber: customer.RegistrationNumber,
		RiskLevel:          customer.RiskLevel,
		EntityHint:         customer.EntityHint,
		CreatedAt:          customer.CreatedAt,
		Status:             customer.Status,
	}
}

func accountResponse(account acct.Account) AccountResponse {
	return AccountResponse{
		ID:         account.ID,
		CustomerID: account.CustomerID,
		IBANLike:   account.IBANLike,
		Currency:   account.Currency,
		Status:     account.Status,
		CreatedAt:  account.CreatedAt,
	}
}

func accountResponses(accounts []acct.Account) []AccountResponse {
	responses := make([]AccountResponse, 0, len(accounts))
	for _, account := range accounts {
		responses = append(responses, accountResponse(account))
	}
	return responses
}

func transactionResponse(transaction txn.Transaction) TransactionResponse {
	return TransactionResponse{
		ID:                      transaction.ID,
		AccountID:               transaction.AccountID,
		CounterpartyName:        transaction.CounterpartyName,
		CounterpartyCountry:     transaction.CounterpartyCountry,
		CounterpartyFingerprint: transaction.CounterpartyFingerprint,
		Amount:                  transaction.Amount,
		Currency:                transaction.Currency,
		Direction:               transaction.Direction,
		Purpose:                 transaction.Purpose,
		TraceID:                 transaction.TraceID,
		CreatedAt:               transaction.CreatedAt,
		RiskScore:               transaction.RiskScore,
	}
}

func transactionResponses(transactions []txn.Transaction) []TransactionResponse {
	responses := make([]TransactionResponse, 0, len(transactions))
	for _, transaction := range transactions {
		responses = append(responses, transactionResponse(transaction))
	}
	return responses
}

func alertResponse(alert alertrepo.Alert) AlertResponse {
	var assigned *int64
	if alert.AssignedUserID.Valid {
		value := alert.AssignedUserID.Int64
		assigned = &value
	}
	return AlertResponse{
		ID:             alert.ID,
		TransactionID:  alert.TransactionID,
		CustomerID:     alert.CustomerID,
		RuleCode:       alert.RuleCode,
		RiskScore:      alert.RiskScore,
		Status:         alert.Status,
		AssignedUserID: assigned,
		TypologyHint:   alert.TypologyHint,
		CreatedAt:      alert.CreatedAt,
		UpdatedAt:      alert.UpdatedAt,
	}
}

func alertResponses(alerts []alertrepo.Alert) []AlertResponse {
	responses := make([]AlertResponse, 0, len(alerts))
	for _, alert := range alerts {
		responses = append(responses, alertResponse(alert))
	}
	return responses
}

func caseResponse(investigationCase casework.Case) CaseResponse {
	return CaseResponse{
		ID:            investigationCase.ID,
		CustomerID:    investigationCase.CustomerID,
		AlertID:       investigationCase.AlertID,
		Status:        investigationCase.Status,
		PublicSummary: investigationCase.PublicSummary,
		RiskScore:     investigationCase.RiskScore,
		TypologyHint:  investigationCase.TypologyHint,
		CreatedAt:     investigationCase.CreatedAt,
		UpdatedAt:     investigationCase.UpdatedAt,
	}
}

func caseNoteResponse(note casework.CaseNote) CaseNoteResponse {
	return CaseNoteResponse{
		ID:           note.ID,
		CaseID:       note.CaseID,
		AuthorUserID: note.AuthorUserID,
		Visibility:   note.Visibility,
		Body:         note.Body,
		CreatedAt:    note.CreatedAt,
		UpdatedAt:    note.UpdatedAt,
	}
}

func caseNoteResponses(notes []casework.CaseNote) []CaseNoteResponse {
	responses := make([]CaseNoteResponse, 0, len(notes))
	for _, note := range notes {
		responses = append(responses, caseNoteResponse(note))
	}
	return responses
}

func caseAuditViews(events []auditrepo.Event) []CaseAuditView {
	responses := make([]CaseAuditView, 0, len(events))
	for _, event := range events {
		responses = append(responses, caseAuditView(event))
	}
	return responses
}

func entityActivityViews(events []auditrepo.EntityActivityEvent) []EntityActivityView {
	responses := make([]EntityActivityView, 0, len(events))
	for _, event := range events {
		responses = append(responses, entityActivityView(event, caseAuditChangeResponses(event.DiffJSON), auditrepo.ActivityNarrative{}))
	}
	return responses
}

func (s *Server) entityActivityViewsWithDetail(ctx context.Context, events []auditrepo.EntityActivityEvent, audience auditrepo.ActivityAudience, detailForEvent func(auditrepo.EntityActivityEvent) auditrepo.ActivityDetailLevel) ([]EntityActivityView, error) {
	hydrator := auditrepo.NewActivityHydrator(activityReviewContextSource(s.audit))
	responses := make([]EntityActivityView, 0, len(events))
	for _, event := range events {
		detail := auditrepo.ActivityDetailRelated
		if detailForEvent != nil {
			if selected := detailForEvent(event); selected != "" {
				detail = selected
			}
		}
		narrative, err := hydrator.Hydrate(ctx, auditrepo.ActivityHydrationRequest{
			Event:       event,
			Audience:    audience,
			DetailLevel: detail,
		})
		if err != nil {
			return nil, err
		}
		responses = append(responses, entityActivityView(event, caseAuditChangeResponses(event.DiffJSON), narrative))
	}
	return responses, nil
}

func entityActivityView(event auditrepo.EntityActivityEvent, changes []CaseAuditChangeResponse, narrative auditrepo.ActivityNarrative) EntityActivityView {
	return EntityActivityView{
		CaseID:          event.CaseID,
		CustomerID:      event.CustomerID,
		EntityHint:      event.EntityHint,
		PublicSummary:   event.PublicSummary,
		ActivitySubject: string(narrative.Subject),
		ActivityLevel:   string(narrative.DetailLevel),
		ActivityText:    narrative.Narrative,
		ActivitySource:  string(narrative.Provenance),
		ObjectType:      event.ObjectType,
		ObjectID:        event.ObjectID,
		EventType:       event.EventType,
		PublicMessage:   event.PublicMessage,
		Changes:         changes,
		CreatedAt:       event.CreatedAt,
		VisibilityScope: event.VisibilityScope,
	}
}

func activityReviewContextSource(repository AuditRepository) auditrepo.ActivityReviewContextSource {
	source, ok := repository.(auditrepo.ActivityReviewContextSource)
	if !ok {
		return nil
	}
	return source
}

func caseAuditView(event auditrepo.Event) CaseAuditView {
	return CaseAuditView{
		ID:              event.ID,
		ObjectType:      event.ObjectType,
		ObjectID:        event.ObjectID,
		EventType:       event.EventType,
		PublicMessage:   event.PublicMessage,
		Changes:         caseAuditChangeResponses(event.DiffJSON),
		CreatedAt:       event.CreatedAt,
		VisibilityScope: event.VisibilityScope,
	}
}

func caseAuditChangeResponses(diffJSON string) []CaseAuditChangeResponse {
	var changes []auditrepo.RedactedFieldChange
	if err := json.Unmarshal([]byte(diffJSON), &changes); err != nil {
		return []CaseAuditChangeResponse{}
	}
	responses := make([]CaseAuditChangeResponse, 0, len(changes))
	for _, change := range changes {
		field := change.Field
		if change.Redacted && auditrepo.IsRestrictedField(field) {
			field = "restricted_field"
		}
		responses = append(responses, CaseAuditChangeResponse{
			Field:    field,
			Old:      change.Old,
			New:      change.New,
			Redacted: change.Redacted,
		})
	}
	return responses
}

func (s *Server) recordAudit(ctx context.Context, params auditrepo.CreateParams) error {
	if s.audit == nil {
		return nil
	}
	if params.VisibilityScope == "" {
		params.VisibilityScope = "owner"
	}
	_, err := s.audit.Create(ctx, params)
	return err
}

func duplicatePreviewViews(candidates []casework.DuplicateCandidate) []CaseDuplicatePreviewView {
	responses := make([]CaseDuplicatePreviewView, 0, len(candidates))
	for _, candidate := range candidates {
		responses = append(responses, duplicatePreviewView(candidate))
	}
	return responses
}

func duplicatePreviewView(candidate casework.DuplicateCandidate) CaseDuplicatePreviewView {
	return CaseDuplicatePreviewView{
		CaseID:           candidate.CaseID,
		PublicSummary:    candidate.PublicSummary,
		RiskTags:         duplicateRiskTags(candidate),
		OverlapScore:     duplicateOverlapScore(candidate),
		MatchExplanation: "shared_dedup_hint",
	}
}

func duplicateRiskTags(candidate casework.DuplicateCandidate) []string {
	tags := make([]string, 0, 3)
	if candidate.RiskLevel != "" {
		tags = append(tags, "risk_level:"+candidate.RiskLevel)
	}
	if candidate.TypologyHint != "" {
		tags = append(tags, "typology:"+candidate.TypologyHint)
	}
	if candidate.RiskScore >= 75 {
		tags = append(tags, "risk_score:high")
	}
	return tags
}

func duplicateOverlapScore(candidate casework.DuplicateCandidate) int {
	if candidate.DedupHint == "" {
		return 0
	}
	if candidate.RiskScore >= 75 {
		return 95
	}
	return 85
}

func graphEntityView(summary graphrepo.EntitySummary) GraphEntityView {
	return GraphEntityView{
		EntityHint:    summary.EntityHint,
		RelationType:  summary.RelationType,
		CustomerCount: summary.CustomerCount,
		CaseCount:     summary.CaseCount,
		HighestRisk:   summary.HighestRisk,
		RiskTags:      append([]string(nil), summary.PublicRiskTags...),
		UpdatedAt:     summary.UpdatedAt,
	}
}

func graphRelatedCaseViews(cases []graphrepo.RelatedCaseSummary) []GraphRelatedCaseView {
	responses := make([]GraphRelatedCaseView, 0, len(cases))
	for _, related := range cases {
		responses = append(responses, GraphRelatedCaseView{
			CaseID:        related.CaseID,
			CustomerID:    related.CustomerID,
			EntityHint:    related.EntityHint,
			DisplayName:   related.DisplayName,
			Country:       related.Country,
			Status:        related.Status,
			PublicSummary: related.PublicSummary,
			RiskScore:     related.RiskScore,
			TypologyHint:  related.TypologyHint,
			CreatedAt:     related.CreatedAt,
			UpdatedAt:     related.UpdatedAt,
		})
	}
	return responses
}

func eddGraphSummaryViews(summaries []graphrepo.EDDSummary) []EDDGraphSummaryView {
	responses := make([]EDDGraphSummaryView, 0, len(summaries))
	for _, summary := range summaries {
		responses = append(responses, EDDGraphSummaryView{
			CustomerID:   summary.CustomerID,
			EntityHint:   summary.EntityHint,
			ReviewStatus: summary.ReviewStatus,
			RiskTags:     eddGraphRiskTags(summary),
			CreatedAt:    summary.CreatedAt,
			UpdatedAt:    summary.UpdatedAt,
		})
	}
	return responses
}

func eddGraphRiskTags(summary graphrepo.EDDSummary) []string {
	if summary.RiskLevel == "" {
		return []string{}
	}
	return []string{"risk_level:" + summary.RiskLevel}
}

func eddOwnerView(record edd.EDDRecord) EDDOwnerView {
	return EDDOwnerView{
		ID:                  record.ID,
		CustomerID:          record.CustomerID,
		AnalystUserID:       record.AnalystUserID,
		SourceOfFunds:       record.SourceOfFunds,
		RiskRationale:       record.RiskRationale,
		ConfidentialComment: record.ConfidentialComment,
		ReviewStatus:        record.ReviewStatus,
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
	}
}

func (s *Server) buildExportBundle(ctx context.Context, ownerUserID, caseID, disputeID int64, req CreateExportRequest) (exportrepo.ExportBundle, error) {
	root, err := s.cases.GetByOwner(ctx, ownerUserID, caseID)
	if err != nil {
		return exportrepo.ExportBundle{}, err
	}
	snapshot := exportCaseSnapshot(root)
	if s.cases != nil {
		notes, err := s.cases.ListNotesByOwner(ctx, ownerUserID, caseID)
		if err != nil && !errors.Is(err, casework.ErrNoteNotFound) && !errors.Is(err, casework.ErrCaseNotFound) {
			return exportrepo.ExportBundle{}, err
		}
		for _, note := range notes {
			snapshot.Notes = append(snapshot.Notes, exportNoteSnapshot(note))
		}
		linked, err := s.cases.FindDuplicateCandidatesByOwner(ctx, ownerUserID, caseID)
		if err != nil {
			return exportrepo.ExportBundle{}, err
		}
		for _, candidate := range linked {
			linkedSnapshot := exportLinkedCaseSnapshot(candidate)
			if err := s.populateExportLinkedCaseData(ctx, ownerUserID, caseID, &linkedSnapshot); err != nil {
				return exportrepo.ExportBundle{}, err
			}
			snapshot.LinkedCases = append(snapshot.LinkedCases, linkedSnapshot)
		}
	}
	if s.edd != nil {
		record, err := s.edd.GetByOwnerCustomer(ctx, ownerUserID, root.CustomerID)
		if err == nil {
			snapshot.EDDRecords = append(snapshot.EDDRecords, exportEDDSnapshot(record, ""))
		} else if !errors.Is(err, edd.ErrEDDRecordNotFound) {
			return exportrepo.ExportBundle{}, err
		}
	}
	if s.audit != nil {
		events, err := s.audit.ListCaseTimelineByOwner(ctx, ownerUserID, caseID)
		if err != nil {
			return exportrepo.ExportBundle{}, err
		}
		for _, event := range events {
			snapshot.AuditEvents = append(snapshot.AuditEvents, exportAuditSnapshot(event))
		}
	}
	return exportrepo.BuildBundle(exportrepo.ExportContext{
		RequesterUserID: ownerUserID,
		RootCaseID:      caseID,
		DisputeID:       disputeID,
		Purpose:         req.Purpose,
		AllowedScopes:   exportAllowedScopes(req.AllowedScopes),
	}, snapshot, time.Now().UTC())
}

func exportAllowedScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{"root_case", "linked_cases", "audit_summary", "edd_summary", "item_metadata"}
	}
	return scopes
}

func exportCaseSnapshot(c casework.Case) exportrepo.CaseSnapshot {
	return exportrepo.CaseSnapshot{
		CaseID:        c.ID,
		CustomerID:    c.CustomerID,
		OwnerUserID:   c.OwnerUserID,
		Status:        c.Status,
		PublicSummary: c.PublicSummary,
		RiskScore:     c.RiskScore,
		TypologyHint:  c.TypologyHint,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

func exportLinkedCaseSnapshot(candidate casework.DuplicateCandidate) exportrepo.CaseSnapshot {
	return exportrepo.CaseSnapshot{
		CaseID:        candidate.CaseID,
		CustomerID:    candidate.CustomerID,
		Status:        candidate.Status,
		PublicSummary: candidate.PublicSummary,
		RiskScore:     candidate.RiskScore,
		TypologyHint:  candidate.TypologyHint,
		CreatedAt:     candidate.CreatedAt,
		UpdatedAt:     candidate.UpdatedAt,
	}
}

func exportNoteSnapshot(note casework.CaseNote) exportrepo.CaseNoteSnapshot {
	return exportrepo.CaseNoteSnapshot{
		ID:         note.ID,
		Visibility: note.Visibility,
		Body:       note.Body,
		CreatedAt:  note.CreatedAt,
	}
}

func exportEDDSnapshot(record edd.EDDRecord, riskLevel string) exportrepo.EDDRecordSnapshot {
	return exportrepo.EDDRecordSnapshot{
		ID:                  record.ID,
		CustomerID:          record.CustomerID,
		ReviewStatus:        record.ReviewStatus,
		RiskLevel:           riskLevel,
		SourceOfFunds:       record.SourceOfFunds,
		RiskRationale:       record.RiskRationale,
		ConfidentialComment: record.ConfidentialComment,
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
	}
}

func exportAuditSnapshot(event auditrepo.Event) exportrepo.AuditEventSnapshot {
	return exportrepo.AuditEventSnapshot{
		ID:            event.ID,
		ObjectType:    event.ObjectType,
		ObjectID:      event.ObjectID,
		EventType:     event.EventType,
		PublicMessage: event.PublicMessage,
		DiffJSON:      event.DiffJSON,
		Changes:       exportAuditChanges(event.DiffJSON),
		CreatedAt:     event.CreatedAt,
	}
}

func exportAuditChanges(diffJSON string) []exportrepo.AuditChangeSnapshot {
	var changes []auditrepo.RedactedFieldChange
	if err := json.Unmarshal([]byte(diffJSON), &changes); err != nil {
		return nil
	}
	responses := make([]exportrepo.AuditChangeSnapshot, 0, len(changes))
	for _, change := range changes {
		responses = append(responses, exportrepo.AuditChangeSnapshot{
			Field:    change.Field,
			Old:      change.Old,
			New:      change.New,
			OldHash:  change.OldHash,
			NewHash:  change.NewHash,
			Redacted: change.Redacted,
		})
	}
	return responses
}

func exportResponse(record exportrepo.ExportRecord, bundle exportrepo.ExportBundle) ExportResponse {
	return ExportResponse{
		ID:              record.ID,
		RootCaseID:      record.RootCaseID,
		DisputeID:       record.DisputeID,
		Purpose:         record.Purpose,
		Status:          record.Status,
		RedactionPolicy: record.RedactionPolicy,
		Bundle:          exportBundleView(bundle),
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
		ExpiresAt:       record.ExpiresAt,
	}
}

func decodeExportBundle(bundleJSON string) (exportrepo.ExportBundle, error) {
	var bundle exportrepo.ExportBundle
	if err := json.Unmarshal([]byte(bundleJSON), &bundle); err != nil {
		return exportrepo.ExportBundle{}, err
	}
	return bundle, nil
}

func exportBundleView(bundle exportrepo.ExportBundle) ExportBundleView {
	return ExportBundleView{
		Purpose:         bundle.Purpose,
		RedactionPolicy: bundle.RedactionPolicy,
		GeneratedAt:     bundle.GeneratedAt,
		RootCase:        exportCaseShellView(bundle.RootCase),
		LinkedCases:     exportCaseShellViews(bundle.LinkedCases),
		LinkedEvidence:  cloneExportLinkedEvidenceMaps(bundle.LinkedEvidence),
		AuditSummaries:  exportAuditSummaryViews(bundle.AuditSummaries),
		EDDSummaries:    exportEDDSummaryViews(bundle.EDDSummaries),
		Items:           exportItemMetadataViews(bundle.Items),
	}
}

func exportCaseShellView(shell exportrepo.ExportCaseShell) ExportCaseShellView {
	return ExportCaseShellView{
		CaseID:        shell.CaseID,
		CustomerID:    shell.CustomerID,
		Status:        shell.Status,
		PublicSummary: shell.PublicSummary,
		RiskScore:     shell.RiskScore,
		TypologyHint:  shell.TypologyHint,
		CreatedAt:     shell.CreatedAt,
		UpdatedAt:     shell.UpdatedAt,
	}
}

func exportCaseShellViews(shells []exportrepo.ExportCaseShell) []ExportCaseShellView {
	views := make([]ExportCaseShellView, 0, len(shells))
	for _, shell := range shells {
		views = append(views, exportCaseShellView(shell))
	}
	return views
}

func cloneExportLinkedEvidenceMaps(items []map[string]any) []map[string]any {
	views := make([]map[string]any, 0, len(items))
	for _, item := range items {
		copied := make(map[string]any, len(item))
		for key, value := range item {
			copied[key] = value
		}
		views = append(views, copied)
	}
	return views
}

func exportAuditSummaryViews(summaries []exportrepo.ExportAuditSummary) []ExportAuditSummaryView {
	views := make([]ExportAuditSummaryView, 0, len(summaries))
	for _, summary := range summaries {
		views = append(views, ExportAuditSummaryView{
			EventID:       summary.EventID,
			ObjectType:    summary.ObjectType,
			ObjectID:      summary.ObjectID,
			EventType:     summary.EventType,
			PublicMessage: summary.PublicMessage,
			ChangedFields: append([]string(nil), summary.ChangedFields...),
			CreatedAt:     summary.CreatedAt,
		})
	}
	return views
}

func exportEDDSummaryViews(summaries []exportrepo.ExportEDDSummary) []ExportEDDSummaryView {
	views := make([]ExportEDDSummaryView, 0, len(summaries))
	for _, summary := range summaries {
		views = append(views, ExportEDDSummaryView{
			RecordID:     summary.RecordID,
			CustomerID:   summary.CustomerID,
			ReviewStatus: summary.ReviewStatus,
			RiskTags:     append([]string(nil), summary.RiskTags...),
			CreatedAt:    summary.CreatedAt,
			UpdatedAt:    summary.UpdatedAt,
		})
	}
	return views
}

func exportItemMetadataViews(items []exportrepo.ExportItemMetadata) []ExportItemMetadataView {
	views := make([]ExportItemMetadataView, 0, len(items))
	for _, item := range items {
		views = append(views, ExportItemMetadataView{
			ItemType:        item.ItemType,
			ObjectType:      item.ObjectType,
			ObjectID:        item.ObjectID,
			Relation:        item.Relation,
			RedactionPolicy: item.RedactionPolicy,
		})
	}
	return views
}

func disputeResponse(dispute exportrepo.Dispute) DisputeResponse {
	return DisputeResponse{
		ID:                dispute.ID,
		CaseID:            dispute.CaseID,
		Status:            dispute.Status,
		Reason:            dispute.Reason,
		ResolutionSummary: dispute.ResolutionSummary,
		CreatedAt:         dispute.CreatedAt,
		UpdatedAt:         dispute.UpdatedAt,
	}
}

func networkRoomSummaryViews(rooms []networkrepo.RoomSummary) []NetworkRoomSummaryView {
	views := make([]NetworkRoomSummaryView, 0, len(rooms))
	for _, room := range rooms {
		views = append(views, networkRoomSummaryView(room))
	}
	return views
}

func networkRoomSummaryView(room networkrepo.RoomSummary) NetworkRoomSummaryView {
	return NetworkRoomSummaryView{
		ID:               room.ID,
		RoomKey:          room.RoomKey,
		EntityHint:       room.EntityHint,
		TypologyHint:     room.TypologyHint,
		Status:           room.Status,
		PublicSummary:    room.PublicSummary,
		PublicRiskTags:   append([]string(nil), room.PublicRiskTags...),
		CaseCount:        room.CaseCount,
		ParticipantCount: room.ParticipantCount,
		CreatedAt:        room.CreatedAt,
		UpdatedAt:        room.UpdatedAt,
	}
}

func networkRoomParticipantViews(participants []networkrepo.Participant) []NetworkRoomParticipantView {
	views := make([]NetworkRoomParticipantView, 0, len(participants))
	for _, participant := range participants {
		views = append(views, networkRoomParticipantView(participant))
	}
	return views
}

func networkRoomParticipantView(participant networkrepo.Participant) NetworkRoomParticipantView {
	return NetworkRoomParticipantView{
		ID:              participant.ID,
		RoomID:          participant.RoomID,
		UserID:          participant.UserID,
		ParticipantRole: participant.ParticipantRole,
		JoinedAt:        participant.JoinedAt,
	}
}

func networkRoomDossierView(dossier networkrepo.SafeDossier) NetworkRoomDossierView {
	return NetworkRoomDossierView{
		Room:           networkRoomSummaryView(dossier.Room),
		Cases:          networkRoomCaseViews(dossier.Cases),
		ScopePolicy:    dossier.ScopePolicy,
		CoveragePolicy: dossier.CoveragePolicy,
	}
}

func networkRoomCaseViews(cases []networkrepo.CaseDossierSummary) []NetworkRoomCaseView {
	views := make([]NetworkRoomCaseView, 0, len(cases))
	for _, c := range cases {
		views = append(views, NetworkRoomCaseView{
			CaseID:                c.CaseID,
			CustomerID:            c.CustomerID,
			OwnedByRequester:      c.OwnedByRequester,
			Status:                c.Status,
			PublicSummary:         c.PublicSummary,
			RiskScore:             c.RiskScore,
			TypologyHint:          c.TypologyHint,
			PublicRiskTags:        append([]string(nil), c.PublicRiskTags...),
			ScopeCoverageComplete: c.ScopeCoverageComplete,
			CreatedAt:             c.CreatedAt,
			UpdatedAt:             c.UpdatedAt,
		})
	}
	return views
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid_request"})
	case errors.Is(err, auth.ErrDuplicateUsername):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "duplicate_username"})
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid_credentials"})
	case errors.Is(err, auth.ErrUnauthenticated):
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthenticated"})
	case errors.Is(err, auth.ErrDisabledUser):
		writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "user_disabled"})
	default:
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal_error"})
	}
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cust.ErrInvalidInput), errors.Is(err, acct.ErrInvalidInput), errors.Is(err, txn.ErrInvalidInput), errors.Is(err, alertrepo.ErrInvalidInput), errors.Is(err, casework.ErrInvalidInput), errors.Is(err, risk.ErrInvalidInput), errors.Is(err, edd.ErrInvalidInput), errors.Is(err, auditrepo.ErrInvalidInput), errors.Is(err, graphrepo.ErrInvalidInput), errors.Is(err, exportrepo.ErrInvalidInput), errors.Is(err, networkrepo.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid_request"})
	case errors.Is(err, cust.ErrDuplicate):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "duplicate_customer_registration"})
	case errors.Is(err, acct.ErrDuplicate):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "duplicate_account_iban"})
	case errors.Is(err, txn.ErrDuplicate):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "duplicate_transaction_trace"})
	case errors.Is(err, alertrepo.ErrDuplicate):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "duplicate_alert"})
	case errors.Is(err, casework.ErrDuplicate):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "duplicate_case"})
	case errors.Is(err, edd.ErrDuplicate):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "duplicate_edd_record"})
	case errors.Is(err, exportrepo.ErrDuplicate):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "duplicate_export_object"})
	case errors.Is(err, networkrepo.ErrDuplicate):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "duplicate_network_room_object"})
	case errors.Is(err, alertrepo.ErrInvalidTransition), errors.Is(err, casework.ErrInvalidTransition):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "invalid_transition"})
	case errors.Is(err, cust.ErrCustomerNotFound), errors.Is(err, acct.ErrAccountNotFound), errors.Is(err, txn.ErrTransactionNotFound), errors.Is(err, alertrepo.ErrAlertNotFound), errors.Is(err, casework.ErrCaseNotFound), errors.Is(err, casework.ErrNoteNotFound), errors.Is(err, edd.ErrEDDRecordNotFound), errors.Is(err, auditrepo.ErrEventNotFound), errors.Is(err, graphrepo.ErrEntityNotFound), errors.Is(err, exportrepo.ErrExportNotFound), errors.Is(err, networkrepo.ErrRoomNotFound):
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not_found"})
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeJSON(w, http.StatusRequestTimeout, ErrorResponse{Error: "request_timeout"})
	default:
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal_error"})
	}
}

func parseID(w http.ResponseWriter, value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid_path"})
		return 0, false
	}
	return id, true
}

func pathParts(path string) []string {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	clean := parts[:0]
	for _, part := range parts {
		if part != "" {
			clean = append(clean, part)
		}
	}
	return clean
}

func urlPathUnescape(value string) (string, error) {
	return url.PathUnescape(value)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		_, _ = fmt.Fprintln(w, `{"error":"internal_error"}`)
	}
}
