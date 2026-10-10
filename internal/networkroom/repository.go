package networkroom

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sentinel-aml/internal/classification"
	"sort"
	"strings"
	"time"
)

var (
	ErrRoomNotFound = errors.New("network room not found")
	ErrDuplicate    = errors.New("duplicate network room object")
	ErrInvalidInput = errors.New("invalid network room input")
)

var allowedRoomStatuses = map[string]struct{}{
	"OPEN":       {},
	"MONITORING": {},
	"CLOSED":     {},
}

var allowedRelations = map[string]struct{}{
	"root_case":      {},
	"related_case":   {},
	"typology_match": {},
	"entity_match":   {},
}

var allowedParticipantRoles = map[string]struct{}{
	"analyst":  {},
	"observer": {},
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
	db                    queryer
	caseLimitPerCandidate int
	artifactLimitPerCase  int
}

const (
	DefaultDirtyBatchSize           = 100
	DefaultDirtyLockTTL             = 30 * time.Second
	DefaultCaseLimitPerCandidate    = 500
	DefaultArtifactLimitPerCase     = 20
	DefaultDirtyCandidateReason     = "case_changed"
	maxDirtyCandidateErrorBytes     = 512
	defaultDirtyCandidateQueryLimit = 1
)

type Room struct {
	ID              int64
	RoomKey         string
	EntityHint      string
	TypologyHint    string
	Status          string
	PublicSummary   string
	PublicRiskTags  []string
	CreatedByUserID int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type RoomCase struct {
	ID            int64
	RoomID        int64
	CaseID        int64
	RelationType  string
	AddedByUserID int64
	CreatedAt     time.Time
}

type Participant struct {
	ID              int64
	RoomID          int64
	UserID          int64
	ParticipantRole string
	JoinedAt        time.Time
}

type RoomSummary struct {
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

type CaseDossierInput struct {
	CaseID           int64
	CustomerID       int64
	OwnerUserID      int64
	Status           string
	PublicSummary    string
	RiskScore        int
	TypologyHint     string
	PublicRiskTags   []string
	SensitiveSamples []string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CollaborationArtifact struct {
	ID             int64
	RoomID         int64
	CaseID         int64
	CustomerID     int64
	ArtifactKind   string
	SourceClass    string
	CoveragePolicy string
	Content        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DirtyCandidate struct {
	ID               int64
	EntityHintNorm   string
	TypologyHintNorm string
	Reason           string
	Status           string
	Attempts         int
	LockedUntil      sql.NullTime
	LastError        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CandidateCase struct {
	CaseID      int64
	OwnerUserID int64
}

type CandidateClusterSummary struct {
	ActorUserID int64
	TotalCases  int
}

type CaseDossierSummary struct {
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

type SafeDossier struct {
	Room           RoomSummary          `json:"room"`
	Cases          []CaseDossierSummary `json:"cases"`
	ScopePolicy    string               `json:"scope_policy"`
	CoveragePolicy string               `json:"coverage_policy"`
}

type CreateOrFindRoomParams struct {
	ActorUserID    int64
	EntityHint     string
	TypologyHint   string
	PublicSummary  string
	PublicRiskTags []string
}

type AttachCaseParams struct {
	ActorUserID  int64
	RoomID       int64
	CaseID       int64
	RelationType string
}

type JoinRoomParams struct {
	RoomID          int64
	UserID          int64
	ParticipantRole string
}

type MaterializeArtifactParams struct {
	RoomID         int64
	CaseID         int64
	CustomerID     int64
	ArtifactKind   string
	SourceClass    string
	CoveragePolicy string
	Content        string
}

func NewRepository(db *sql.DB) *Repository {
	return newRepository(sqlDBAdapter{db: db})
}

func newRepository(db queryer) *Repository {
	return &Repository{
		db:                    db,
		caseLimitPerCandidate: DefaultCaseLimitPerCandidate,
		artifactLimitPerCase:  DefaultArtifactLimitPerCase,
	}
}

func (r *Repository) SetLifecycleLimits(caseLimitPerCandidate, artifactLimitPerCase int) {
	if caseLimitPerCandidate > 0 {
		r.caseLimitPerCandidate = caseLimitPerCandidate
	}
	if artifactLimitPerCase > 0 {
		r.artifactLimitPerCase = artifactLimitPerCase
	}
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

func (r *Repository) CreateOrFindRoom(ctx context.Context, params CreateOrFindRoomParams) (Room, error) {
	if err := ctx.Err(); err != nil {
		return Room{}, err
	}
	params = normalizeCreateOrFindRoomParams(params)
	if err := validateCreateOrFindRoomParams(params); err != nil {
		return Room{}, err
	}
	roomKey := deterministicRoomKey(params.EntityHint, params.TypologyHint)
	riskTags := strings.Join(normalizeTags(params.PublicRiskTags), ",")

	var room Room
	var tags string
	err := r.db.QueryRowContext(ctx, `
INSERT INTO network_rooms (room_key, entity_hint, typology_hint, public_summary, public_risk_tags, created_by_user_id)
VALUES ($1, $2, $3, $4, CASE WHEN btrim($5) = '' THEN '{}'::text[] ELSE string_to_array($5, ',') END, $6)
ON CONFLICT (room_key) DO UPDATE
SET updated_at = network_rooms.updated_at
RETURNING id, room_key, entity_hint, typology_hint, status, public_summary, array_to_string(public_risk_tags, ','), created_by_user_id, created_at, updated_at`,
		roomKey,
		params.EntityHint,
		params.TypologyHint,
		params.PublicSummary,
		riskTags,
		params.ActorUserID,
	).Scan(&room.ID, &room.RoomKey, &room.EntityHint, &room.TypologyHint, &room.Status, &room.PublicSummary, &tags, &room.CreatedByUserID, &room.CreatedAt, &room.UpdatedAt)
	if err != nil {
		return Room{}, fmt.Errorf("create or find network room: %w", err)
	}
	room.PublicRiskTags = splitTags(tags)
	return room, nil
}

func (r *Repository) AttachCase(ctx context.Context, params AttachCaseParams) (RoomCase, error) {
	if err := ctx.Err(); err != nil {
		return RoomCase{}, err
	}
	params = normalizeAttachCaseParams(params)
	if err := validateAttachCaseParams(params); err != nil {
		return RoomCase{}, err
	}

	var link RoomCase
	err := r.db.QueryRowContext(ctx, `
INSERT INTO network_room_cases (room_id, case_id, relation_type, added_by_user_id)
SELECT nr.id, c.id, $4, $1
FROM network_rooms nr
JOIN cases c ON c.id = $3
WHERE nr.id = $2
RETURNING id, room_id, case_id, relation_type, added_by_user_id, created_at`,
		params.ActorUserID,
		params.RoomID,
		params.CaseID,
		params.RelationType,
	).Scan(&link.ID, &link.RoomID, &link.CaseID, &link.RelationType, &link.AddedByUserID, &link.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RoomCase{}, ErrRoomNotFound
	}
	if isUniqueViolation(err) {
		return RoomCase{}, ErrDuplicate
	}
	if err != nil {
		return RoomCase{}, fmt.Errorf("attach network room case: %w", err)
	}
	return link, nil
}

func (r *Repository) JoinRoom(ctx context.Context, params JoinRoomParams) (Participant, error) {
	if err := ctx.Err(); err != nil {
		return Participant{}, err
	}
	params = normalizeJoinRoomParams(params)
	if err := validateJoinRoomParams(params); err != nil {
		return Participant{}, err
	}

	var participant Participant
	err := r.db.QueryRowContext(ctx, `
INSERT INTO network_room_participants (room_id, user_id, participant_role)
SELECT nr.id, u.id, $3
FROM network_rooms nr
JOIN users u ON u.id = $2
WHERE nr.id = $1
  AND EXISTS (
      SELECT 1
      FROM network_room_cases nrc
      JOIN cases ca ON ca.id = nrc.case_id
      WHERE nrc.room_id = nr.id
        AND ca.owner_user_id = u.id
  )
RETURNING id, room_id, user_id, participant_role, joined_at`,
		params.RoomID,
		params.UserID,
		params.ParticipantRole,
	).Scan(&participant.ID, &participant.RoomID, &participant.UserID, &participant.ParticipantRole, &participant.JoinedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Participant{}, ErrRoomNotFound
	}
	if isUniqueViolation(err) {
		return Participant{}, ErrDuplicate
	}
	if err != nil {
		return Participant{}, fmt.Errorf("join network room: %w", err)
	}
	return participant, nil
}

func (r *Repository) MaterializeArtifact(ctx context.Context, params MaterializeArtifactParams) (CollaborationArtifact, error) {
	if err := ctx.Err(); err != nil {
		return CollaborationArtifact{}, err
	}
	params = normalizeMaterializeArtifactParams(params)
	if err := validateMaterializeArtifactParams(params); err != nil {
		return CollaborationArtifact{}, err
	}

	var artifact CollaborationArtifact
	err := r.db.QueryRowContext(ctx, `
INSERT INTO network_room_artifacts (room_id, case_id, customer_id, artifact_kind, source_class, coverage_policy, content)
SELECT nr.id, ca.id, ca.customer_id, $4, $5, $6, $7
FROM network_rooms nr
JOIN network_room_cases nrc ON nrc.room_id = nr.id AND nrc.case_id = $2
JOIN cases ca ON ca.id = nrc.case_id AND ca.customer_id = $3
WHERE nr.id = $1
ON CONFLICT (room_id, case_id, artifact_kind, source_class) DO UPDATE
SET customer_id = EXCLUDED.customer_id,
    coverage_policy = EXCLUDED.coverage_policy,
    content = EXCLUDED.content,
    updated_at = now()
RETURNING id, room_id, case_id, customer_id, artifact_kind, source_class, coverage_policy, content, created_at, updated_at`,
		params.RoomID,
		params.CaseID,
		params.CustomerID,
		params.ArtifactKind,
		params.SourceClass,
		params.CoveragePolicy,
		params.Content,
	).Scan(&artifact.ID, &artifact.RoomID, &artifact.CaseID, &artifact.CustomerID, &artifact.ArtifactKind, &artifact.SourceClass, &artifact.CoveragePolicy, &artifact.Content, &artifact.CreatedAt, &artifact.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CollaborationArtifact{}, ErrRoomNotFound
	}
	if err != nil {
		return CollaborationArtifact{}, fmt.Errorf("materialize network room artifact: %w", err)
	}
	return artifact, nil
}

func (r *Repository) EnqueueCaseCandidate(ctx context.Context, caseID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	caseID = strings.TrimSpace(caseID)
	if caseID == "" {
		return ErrInvalidInput
	}

	var id int64
	err := r.db.QueryRowContext(ctx, `
WITH candidate AS (
    SELECT replace(lower(btrim(cust.entity_hint)), ' ', '_') AS entity_hint_norm,
           replace(lower(btrim(ca.typology_hint)), ' ', '_') AS typology_hint_norm
    FROM cases ca
    JOIN customers cust ON cust.id = ca.customer_id
    WHERE ca.id::text = $1
      AND btrim(cust.entity_hint) <> ''
      AND btrim(ca.typology_hint) <> ''
),
upserted AS (
    INSERT INTO network_room_dirty_candidates (entity_hint_norm, typology_hint_norm, reason, status, locked_until, last_error)
    SELECT entity_hint_norm, typology_hint_norm, $2, 'pending', NULL, ''
    FROM candidate
    ON CONFLICT (entity_hint_norm, typology_hint_norm) DO UPDATE
    SET reason = EXCLUDED.reason,
        status = 'pending',
        locked_until = NULL,
        last_error = '',
        updated_at = now()
    RETURNING id
)
SELECT coalesce((SELECT id FROM upserted LIMIT 1), 0)`,
		caseID,
		DefaultDirtyCandidateReason,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("enqueue network room dirty candidate: %w", err)
	}
	return nil
}

func (r *Repository) ClaimDirtyCandidates(ctx context.Context, limit int, lockFor time.Duration) ([]DirtyCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultDirtyBatchSize
	}
	if lockFor <= 0 {
		lockFor = DefaultDirtyLockTTL
	}

	rows, err := r.db.QueryContext(ctx, `
WITH due AS (
    SELECT id
    FROM network_room_dirty_candidates
    WHERE status = 'pending'
       OR (status = 'processing' AND locked_until <= now())
    ORDER BY updated_at, id
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE network_room_dirty_candidates dirty
SET status = 'processing',
    attempts = dirty.attempts + 1,
    locked_until = now() + ($2::double precision * interval '1 second'),
    updated_at = now()
FROM due
WHERE dirty.id = due.id
RETURNING dirty.id, dirty.entity_hint_norm, dirty.typology_hint_norm, dirty.reason, dirty.status,
          dirty.attempts, dirty.locked_until, dirty.last_error, dirty.created_at, dirty.updated_at`,
		limit,
		lockFor.Seconds(),
	)
	if err != nil {
		return nil, fmt.Errorf("claim network room dirty candidates: %w", err)
	}
	defer rows.Close()

	var candidates []DirtyCandidate
	for rows.Next() {
		var candidate DirtyCandidate
		if err := rows.Scan(
			&candidate.ID,
			&candidate.EntityHintNorm,
			&candidate.TypologyHintNorm,
			&candidate.Reason,
			&candidate.Status,
			&candidate.Attempts,
			&candidate.LockedUntil,
			&candidate.LastError,
			&candidate.CreatedAt,
			&candidate.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan network room dirty candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate network room dirty candidates: %w", err)
	}
	return candidates, nil
}

func (r *Repository) MarkDirtyCandidateDone(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id <= 0 {
		return ErrInvalidInput
	}
	var updatedID int64
	err := r.db.QueryRowContext(ctx, `
UPDATE network_room_dirty_candidates
SET status = 'done',
    locked_until = NULL,
    last_error = '',
    updated_at = now()
WHERE id = $1
RETURNING id`,
		id,
	).Scan(&updatedID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidInput
	}
	if err != nil {
		return fmt.Errorf("mark network room dirty candidate done: %w", err)
	}
	return nil
}

func (r *Repository) MarkDirtyCandidateFailed(ctx context.Context, id int64, cause error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id <= 0 {
		return ErrInvalidInput
	}
	message := "network room candidate processing failed"
	if cause != nil {
		message = strings.TrimSpace(cause.Error())
	}
	if len(message) > maxDirtyCandidateErrorBytes {
		message = message[:maxDirtyCandidateErrorBytes]
	}
	var updatedID int64
	err := r.db.QueryRowContext(ctx, `
UPDATE network_room_dirty_candidates
SET status = 'pending',
    locked_until = NULL,
    last_error = $2,
    updated_at = now()
WHERE id = $1
RETURNING id`,
		id,
		message,
	).Scan(&updatedID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidInput
	}
	if err != nil {
		return fmt.Errorf("mark network room dirty candidate failed: %w", err)
	}
	return nil
}

func (r *Repository) MarkDirtyCandidatePending(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id <= 0 {
		return ErrInvalidInput
	}
	var updatedID int64
	err := r.db.QueryRowContext(ctx, `
UPDATE network_room_dirty_candidates
SET status = 'pending',
    locked_until = NULL,
    updated_at = now()
WHERE id = $1
RETURNING id`,
		id,
	).Scan(&updatedID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidInput
	}
	if err != nil {
		return fmt.Errorf("mark network room dirty candidate pending: %w", err)
	}
	return nil
}

func (r *Repository) GetCandidateClusterSummary(ctx context.Context, entityHint, typologyHint string) (CandidateClusterSummary, error) {
	if err := ctx.Err(); err != nil {
		return CandidateClusterSummary{}, err
	}
	entityHint = normalizeToken(entityHint)
	typologyHint = normalizeToken(typologyHint)
	if entityHint == "" || typologyHint == "" {
		return CandidateClusterSummary{}, ErrInvalidInput
	}

	var summary CandidateClusterSummary
	err := r.db.QueryRowContext(ctx, `
SELECT coalesce(min(ca.owner_user_id), 0), count(*)::int
FROM cases ca
JOIN customers cust ON cust.id = ca.customer_id
WHERE replace(lower(btrim(cust.entity_hint)), ' ', '_') = $1
  AND replace(lower(btrim(ca.typology_hint)), ' ', '_') = $2`,
		entityHint,
		typologyHint,
	).Scan(&summary.ActorUserID, &summary.TotalCases)
	if err != nil {
		return CandidateClusterSummary{}, fmt.Errorf("get network room candidate summary: %w", err)
	}
	return summary, nil
}

func (r *Repository) ListUnattachedCasesForCandidate(ctx context.Context, roomID int64, entityHint, typologyHint string, limit int) ([]CandidateCase, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entityHint = normalizeToken(entityHint)
	typologyHint = normalizeToken(typologyHint)
	if roomID <= 0 || entityHint == "" || typologyHint == "" {
		return nil, ErrInvalidInput
	}
	if limit <= 0 {
		limit = r.caseLimitPerCandidate
	}
	if limit <= 0 {
		limit = DefaultCaseLimitPerCandidate
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT ca.id, ca.owner_user_id
FROM cases ca
JOIN customers cust ON cust.id = ca.customer_id
WHERE replace(lower(btrim(cust.entity_hint)), ' ', '_') = $1
  AND replace(lower(btrim(ca.typology_hint)), ' ', '_') = $2
  AND NOT EXISTS (
      SELECT 1
      FROM network_room_cases nrc
      WHERE nrc.room_id = $3
        AND nrc.case_id = ca.id
  )
ORDER BY ca.id
LIMIT $4`,
		entityHint,
		typologyHint,
		roomID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list unattached network room cases: %w", err)
	}
	defer rows.Close()

	var cases []CandidateCase
	for rows.Next() {
		var item CandidateCase
		if err := rows.Scan(&item.CaseID, &item.OwnerUserID); err != nil {
			return nil, fmt.Errorf("scan unattached network room case: %w", err)
		}
		cases = append(cases, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unattached network room cases: %w", err)
	}
	return cases, nil
}

func (r *Repository) CountUnattachedCasesForCandidate(ctx context.Context, roomID int64, entityHint, typologyHint string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	entityHint = normalizeToken(entityHint)
	typologyHint = normalizeToken(typologyHint)
	if roomID <= 0 || entityHint == "" || typologyHint == "" {
		return 0, ErrInvalidInput
	}

	var count int
	err := r.db.QueryRowContext(ctx, `
SELECT count(*)::int
FROM cases ca
JOIN customers cust ON cust.id = ca.customer_id
WHERE replace(lower(btrim(cust.entity_hint)), ' ', '_') = $1
  AND replace(lower(btrim(ca.typology_hint)), ' ', '_') = $2
  AND NOT EXISTS (
      SELECT 1
      FROM network_room_cases nrc
      WHERE nrc.room_id = $3
        AND nrc.case_id = ca.id
  )`,
		entityHint,
		typologyHint,
		roomID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unattached network room cases: %w", err)
	}
	return count, nil
}

func (r *Repository) ListParticipants(ctx context.Context, roomID int64) ([]Participant, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if roomID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT id, room_id, user_id, participant_role, joined_at
FROM network_room_participants
WHERE room_id = $1
ORDER BY joined_at, id`,
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("list network room participants: %w", err)
	}
	defer rows.Close()

	var participants []Participant
	for rows.Next() {
		var participant Participant
		if err := rows.Scan(&participant.ID, &participant.RoomID, &participant.UserID, &participant.ParticipantRole, &participant.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan network room participant: %w", err)
		}
		participants = append(participants, participant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate network room participants: %w", err)
	}
	return participants, nil
}

func (r *Repository) GetRoomForUser(ctx context.Context, userID, roomID int64) (RoomSummary, error) {
	if err := ctx.Err(); err != nil {
		return RoomSummary{}, err
	}
	if userID <= 0 || roomID <= 0 {
		return RoomSummary{}, ErrInvalidInput
	}

	var summary RoomSummary
	var tags string
	err := r.db.QueryRowContext(ctx, `
SELECT nr.id, nr.room_key, nr.entity_hint, nr.typology_hint, nr.status, nr.public_summary,
       array_to_string(nr.public_risk_tags, ',') AS public_risk_tags,
       count(DISTINCT nrc.case_id)::int AS case_count,
       count(DISTINCT nrp.user_id)::int AS participant_count,
       nr.created_at, nr.updated_at
FROM network_rooms nr
JOIN network_room_participants owned ON owned.room_id = nr.id AND owned.user_id = $1
LEFT JOIN network_room_cases nrc ON nrc.room_id = nr.id
LEFT JOIN network_room_participants nrp ON nrp.room_id = nr.id
WHERE nr.id = $2
GROUP BY nr.id`,
		userID,
		roomID,
	).Scan(&summary.ID, &summary.RoomKey, &summary.EntityHint, &summary.TypologyHint, &summary.Status, &summary.PublicSummary, &tags, &summary.CaseCount, &summary.ParticipantCount, &summary.CreatedAt, &summary.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RoomSummary{}, ErrRoomNotFound
	}
	if err != nil {
		return RoomSummary{}, fmt.Errorf("get network room for user: %w", err)
	}
	summary.PublicRiskTags = splitTags(tags)
	return summary, nil
}

func (r *Repository) ListDossierCases(ctx context.Context, userID, roomID int64) ([]CaseDossierInput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if userID <= 0 || roomID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT ca.id, ca.customer_id, ca.owner_user_id, ca.status, ca.public_summary, ca.risk_score, ca.typology_hint,
       coalesce(nullif(ca.typology_hint, ''), 'case') AS public_risk_tags,
       ca.created_at, ca.updated_at
FROM network_room_cases nrc
JOIN network_room_participants owned ON owned.room_id = nrc.room_id AND owned.user_id = $1
JOIN cases ca ON ca.id = nrc.case_id
WHERE nrc.room_id = $2
ORDER BY ca.id`,
		userID,
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("list network room dossier cases: %w", err)
	}
	defer rows.Close()

	var cases []CaseDossierInput
	for rows.Next() {
		var c CaseDossierInput
		var tags string
		if err := rows.Scan(&c.CaseID, &c.CustomerID, &c.OwnerUserID, &c.Status, &c.PublicSummary, &c.RiskScore, &c.TypologyHint, &tags, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan network room dossier case: %w", err)
		}
		c.PublicRiskTags = splitTags(tags)
		cases = append(cases, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate network room dossier cases: %w", err)
	}
	return cases, nil
}

func (r *Repository) ListRoomsForUser(ctx context.Context, userID int64) ([]RoomSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT nr.id, nr.room_key, nr.entity_hint, nr.typology_hint, nr.status, nr.public_summary,
       array_to_string(nr.public_risk_tags, ',') AS public_risk_tags,
       count(DISTINCT nrc.case_id)::int AS case_count,
       count(DISTINCT nrp.user_id)::int AS participant_count,
       nr.created_at, nr.updated_at
FROM network_rooms nr
JOIN network_room_participants owned ON owned.room_id = nr.id AND owned.user_id = $1
LEFT JOIN network_room_cases nrc ON nrc.room_id = nr.id
LEFT JOIN network_room_participants nrp ON nrp.room_id = nr.id
GROUP BY nr.id
ORDER BY nr.updated_at DESC, nr.id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list network rooms: %w", err)
	}
	defer rows.Close()

	var rooms []RoomSummary
	for rows.Next() {
		var summary RoomSummary
		var tags string
		if err := rows.Scan(&summary.ID, &summary.RoomKey, &summary.EntityHint, &summary.TypologyHint, &summary.Status, &summary.PublicSummary, &tags, &summary.CaseCount, &summary.ParticipantCount, &summary.CreatedAt, &summary.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan network room summary: %w", err)
		}
		summary.PublicRiskTags = splitTags(tags)
		rooms = append(rooms, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate network rooms: %w", err)
	}
	return rooms, nil
}

func BuildSafeDossier(requesterUserID int64, room RoomSummary, cases []CaseDossierInput) (SafeDossier, error) {
	if requesterUserID <= 0 || room.ID <= 0 || strings.TrimSpace(room.RoomKey) == "" {
		return SafeDossier{}, ErrInvalidInput
	}

	summaries := make([]CaseDossierSummary, 0, len(cases))
	for _, c := range cases {
		if c.CaseID <= 0 || c.CustomerID <= 0 {
			return SafeDossier{}, ErrInvalidInput
		}
		summaries = append(summaries, CaseDossierSummary{
			CaseID:                c.CaseID,
			CustomerID:            c.CustomerID,
			OwnedByRequester:      c.OwnerUserID == requesterUserID,
			Status:                strings.TrimSpace(c.Status),
			PublicSummary:         strings.TrimSpace(c.PublicSummary),
			RiskScore:             c.RiskScore,
			TypologyHint:          strings.TrimSpace(c.TypologyHint),
			PublicRiskTags:        normalizeTags(c.PublicRiskTags),
			ScopeCoverageComplete: true,
			CreatedAt:             c.CreatedAt.UTC(),
			UpdatedAt:             c.UpdatedAt.UTC(),
		})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].CaseID < summaries[j].CaseID })

	room.PublicRiskTags = normalizeTags(room.PublicRiskTags)
	room.CreatedAt = room.CreatedAt.UTC()
	room.UpdatedAt = room.UpdatedAt.UTC()
	return SafeDossier{
		Room:           room,
		Cases:          summaries,
		ScopePolicy:    classification.RoomScopeCaseReference,
		CoveragePolicy: classification.RoomCoverageSummary,
	}, nil
}

func deterministicRoomKey(entityHint, typologyHint string) string {
	if typologyHint == "" {
		typologyHint = "general"
	}
	return normalizeToken(entityHint) + ":" + normalizeToken(typologyHint)
}

func normalizeCreateOrFindRoomParams(params CreateOrFindRoomParams) CreateOrFindRoomParams {
	params.EntityHint = normalizeToken(params.EntityHint)
	params.TypologyHint = normalizeToken(params.TypologyHint)
	params.PublicSummary = strings.TrimSpace(params.PublicSummary)
	params.PublicRiskTags = normalizeTags(params.PublicRiskTags)
	return params
}

func validateCreateOrFindRoomParams(params CreateOrFindRoomParams) error {
	if params.ActorUserID <= 0 || params.EntityHint == "" {
		return ErrInvalidInput
	}
	for _, tag := range params.PublicRiskTags {
		if tag == "" {
			return ErrInvalidInput
		}
	}
	return nil
}

func normalizeAttachCaseParams(params AttachCaseParams) AttachCaseParams {
	params.RelationType = normalizeToken(params.RelationType)
	if params.RelationType == "" {
		params.RelationType = "related_case"
	}
	return params
}

func validateAttachCaseParams(params AttachCaseParams) error {
	if params.ActorUserID <= 0 || params.RoomID <= 0 || params.CaseID <= 0 {
		return ErrInvalidInput
	}
	if _, ok := allowedRelations[params.RelationType]; !ok {
		return ErrInvalidInput
	}
	return nil
}

func normalizeJoinRoomParams(params JoinRoomParams) JoinRoomParams {
	params.ParticipantRole = normalizeToken(params.ParticipantRole)
	if params.ParticipantRole == "" {
		params.ParticipantRole = "analyst"
	}
	return params
}

func normalizeMaterializeArtifactParams(params MaterializeArtifactParams) MaterializeArtifactParams {
	params.ArtifactKind = normalizeToken(params.ArtifactKind)
	params.SourceClass = normalizeToken(params.SourceClass)
	params.CoveragePolicy = normalizeToken(params.CoveragePolicy)
	params.Content = strings.TrimSpace(params.Content)
	return params
}

func validateMaterializeArtifactParams(params MaterializeArtifactParams) error {
	if params.RoomID <= 0 || params.CaseID <= 0 || params.CustomerID <= 0 || params.Content == "" || params.SourceClass == "" {
		return ErrInvalidInput
	}
	if params.ArtifactKind != "room_context" && params.ArtifactKind != "assessment_summary" {
		return ErrInvalidInput
	}
	if params.CoveragePolicy != classification.RoomCoverageSummaryMaterial && params.CoveragePolicy != classification.RoomCoverageSourceMaterial {
		return ErrInvalidInput
	}
	return nil
}

func validateJoinRoomParams(params JoinRoomParams) error {
	if params.RoomID <= 0 || params.UserID <= 0 {
		return ErrInvalidInput
	}
	if _, ok := allowedParticipantRoles[params.ParticipantRole]; !ok {
		return ErrInvalidInput
	}
	return nil
}

func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = normalizeToken(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		normalized = append(normalized, tag)
	}
	sort.Strings(normalized)
	return normalized
}

func splitTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	return normalizeTags(strings.Split(value, ","))
}

func normalizeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique")
}
