package networkroom

import (
	"context"
	"errors"
	"fmt"
	"sentinel-aml/internal/classification"
	"time"
)

var errDirtyBatchBudgetExhausted = errors.New("network room dirty batch budget exhausted")

const dirtyBatchBudgetMargin = 500 * time.Millisecond

type LifecycleStats struct {
	GroupsProcessed       int
	CasesAttached         int
	ParticipantsJoined    int
	DuplicateCaseLinks    int
	DuplicateParticipants int
	ArtifactsMaterialized int
	DuplicateArtifacts    int
}

func (r *Repository) ProcessDirtyBatch(ctx context.Context, batchSize int, lockTTL time.Duration) (LifecycleStats, error) {
	if err := ctx.Err(); err != nil {
		return LifecycleStats{}, err
	}
	candidates, err := r.ClaimDirtyCandidates(ctx, batchSize, lockTTL)
	if err != nil {
		return LifecycleStats{}, err
	}

	var stats LifecycleStats
	var firstErr error
	for index, candidate := range candidates {
		if dirtyBatchBudgetLow(ctx) {
			requeueDirtyCandidates(ctx, r, candidates[index:])
			return stats, nil
		}
		requeue, err := r.processDirtyCandidate(ctx, candidate, &stats)
		if err != nil {
			if errors.Is(err, errDirtyBatchBudgetExhausted) {
				if markErr := r.MarkDirtyCandidatePending(ctx, candidate.ID); markErr != nil && firstErr == nil {
					firstErr = markErr
				}
				return stats, firstErr
			}
			if markErr := r.MarkDirtyCandidateFailed(ctx, candidate.ID, err); markErr != nil && firstErr == nil {
				firstErr = markErr
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if requeue {
			if err := r.MarkDirtyCandidatePending(ctx, candidate.ID); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := r.MarkDirtyCandidateDone(ctx, candidate.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return stats, firstErr
}

func (r *Repository) processDirtyCandidate(ctx context.Context, candidate DirtyCandidate, stats *LifecycleStats) (bool, error) {
	summary, err := r.GetCandidateClusterSummary(ctx, candidate.EntityHintNorm, candidate.TypologyHintNorm)
	if err != nil {
		return false, err
	}
	if summary.TotalCases < 2 {
		return false, nil
	}
	room, err := r.CreateOrFindRoom(ctx, CreateOrFindRoomParams{
		ActorUserID:   summary.ActorUserID,
		EntityHint:    candidate.EntityHintNorm,
		TypologyHint:  candidate.TypologyHintNorm,
		PublicSummary: fmt.Sprintf("Related activity for %s / %s", candidate.EntityHintNorm, candidate.TypologyHintNorm),
		PublicRiskTags: []string{
			"shared_entity",
			"typology:" + candidate.TypologyHintNorm,
		},
	})
	if err != nil {
		return false, err
	}
	cases, err := r.ListUnattachedCasesForCandidate(ctx, room.ID, candidate.EntityHintNorm, candidate.TypologyHintNorm, r.caseLimitPerCandidate)
	if err != nil {
		return false, err
	}
	if len(cases) == 0 {
		return false, nil
	}
	for _, item := range cases {
		if dirtyBatchBudgetLow(ctx) {
			return true, errDirtyBatchBudgetExhausted
		}
		caseAttached := false
		_, err := r.AttachCase(ctx, AttachCaseParams{
			ActorUserID:  item.OwnerUserID,
			RoomID:       room.ID,
			CaseID:       item.CaseID,
			RelationType: "typology_match",
		})
		if err == nil {
			stats.CasesAttached++
			caseAttached = true
		} else if errors.Is(err, ErrDuplicate) {
			stats.DuplicateCaseLinks++
		} else {
			return false, err
		}

		_, err = r.JoinRoom(ctx, JoinRoomParams{
			RoomID:          room.ID,
			UserID:          item.OwnerUserID,
			ParticipantRole: "analyst",
		})
		if err == nil {
			stats.ParticipantsJoined++
		} else if errors.Is(err, ErrDuplicate) {
			stats.DuplicateParticipants++
		} else {
			return false, err
		}

		if caseAttached {
			created, duplicates, err := r.materializeCandidateArtifactsWithLimit(ctx, room.ID, item.CaseID, r.artifactLimitPerCase)
			if err != nil {
				return false, err
			}
			stats.ArtifactsMaterialized += created
			stats.DuplicateArtifacts += duplicates
		}
	}
	stats.GroupsProcessed++
	remaining, err := r.CountUnattachedCasesForCandidate(ctx, room.ID, candidate.EntityHintNorm, candidate.TypologyHintNorm)
	if err != nil {
		return false, err
	}
	return remaining > 0, nil
}

func requeueDirtyCandidates(ctx context.Context, r *Repository, candidates []DirtyCandidate) {
	for _, candidate := range candidates {
		_ = r.MarkDirtyCandidatePending(ctx, candidate.ID)
	}
}

func effectiveLimit(configured int, fallback int) int {
	if configured > 0 {
		return configured
	}
	return fallback
}

func dirtyBatchBudgetLow(ctx context.Context) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return false
	}
	return time.Until(deadline) <= dirtyBatchBudgetMargin
}

func (r *Repository) materializeCandidateArtifacts(ctx context.Context, roomID, caseID int64) (int, int, error) {
	return r.materializeCandidateArtifactsWithLimit(ctx, roomID, caseID, r.artifactLimitPerCase)
}

func (r *Repository) materializeCandidateArtifactsWithLimit(ctx context.Context, roomID, caseID int64, limit int) (int, int, error) {
	if roomID <= 0 || caseID <= 0 {
		return 0, 0, ErrInvalidInput
	}
	if limit <= 0 {
		limit = DefaultArtifactLimitPerCase
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT ca.id, ca.customer_id, 'room_context', 'collaboration_context', $2, cn.body
FROM cases ca
JOIN case_notes cn ON cn.case_id = ca.id
WHERE ca.id = $1
  AND cn.visibility = 'private'
  AND btrim(cn.body) <> ''
UNION ALL
SELECT ca.id, ca.customer_id, 'assessment_summary', 'evaluation_summary', $2, er.confidential_comment
FROM cases ca
JOIN edd_records er ON er.customer_id = ca.customer_id
WHERE ca.id = $1
  AND btrim(er.confidential_comment) <> ''
ORDER BY 1, 3, 4
LIMIT $3`,
		caseID,
		classification.RoomCoverageSourceMaterial,
		limit,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("list network room source material: %w", err)
	}
	defer rows.Close()

	var created int
	var duplicates int
	for rows.Next() {
		var params MaterializeArtifactParams
		params.RoomID = roomID
		if err := rows.Scan(&params.CaseID, &params.CustomerID, &params.ArtifactKind, &params.SourceClass, &params.CoveragePolicy, &params.Content); err != nil {
			return created, duplicates, fmt.Errorf("scan network room source material: %w", err)
		}
		if _, err := r.MaterializeArtifact(ctx, params); err == nil {
			created++
		} else if errors.Is(err, ErrDuplicate) {
			duplicates++
		} else {
			return created, duplicates, err
		}
	}
	if err := rows.Err(); err != nil {
		return created, duplicates, fmt.Errorf("iterate network room source material: %w", err)
	}
	return created, duplicates, nil
}
