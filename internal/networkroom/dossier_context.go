package networkroom

import (
	"sentinel-aml/internal/classification"
	"sort"
	"strings"
	"time"
)

type DossierContextBlock struct {
	BlockID        int64  `json:"block_id"`
	CaseID         int64  `json:"case_id"`
	CoveragePolicy string `json:"coverage_policy"`
	Content        string `json:"content"`
}

type DossierEvaluationBlock struct {
	BlockID        int64  `json:"block_id"`
	CustomerID     int64  `json:"customer_id"`
	CoveragePolicy string `json:"coverage_policy"`
	Content        string `json:"content"`
}

type ExtendedCaseDossierInput struct {
	CaseDossierInput
	ContextBlocks    []DossierContextBlock
	EvaluationBlocks []DossierEvaluationBlock
}

type ExtendedCaseDossierSummary struct {
	CaseID                int64                    `json:"case_id"`
	CustomerID            int64                    `json:"customer_id"`
	OwnedByRequester      bool                     `json:"owned_by_requester"`
	Status                string                   `json:"status"`
	PublicSummary         string                   `json:"public_summary"`
	RiskScore             int                      `json:"risk_score"`
	TypologyHint          string                   `json:"typology_hint"`
	PublicRiskTags        []string                 `json:"public_risk_tags"`
	ScopeCoverageComplete bool                     `json:"scope_coverage_complete"`
	ContextBlocks         []DossierContextBlock    `json:"context_blocks,omitempty"`
	EvaluationBlocks      []DossierEvaluationBlock `json:"evaluation_blocks,omitempty"`
	CreatedAt             time.Time                `json:"created_at"`
	UpdatedAt             time.Time                `json:"updated_at"`
}

type ExtendedDossier struct {
	Room           RoomSummary                  `json:"room"`
	Cases          []ExtendedCaseDossierSummary `json:"cases"`
	ScopePolicy    string                       `json:"scope_policy"`
	CoveragePolicy string                       `json:"coverage_policy"`
}

func BuildExtendedDossier(requesterUserID int64, room RoomSummary, cases []ExtendedCaseDossierInput) (ExtendedDossier, error) {
	if requesterUserID <= 0 || room.ID <= 0 || strings.TrimSpace(room.RoomKey) == "" {
		return ExtendedDossier{}, ErrInvalidInput
	}

	summaries := make([]ExtendedCaseDossierSummary, 0, len(cases))
	for _, c := range cases {
		if c.CaseID <= 0 || c.CustomerID <= 0 {
			return ExtendedDossier{}, ErrInvalidInput
		}
		summary := ExtendedCaseDossierSummary{
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
		}
		if !summary.OwnedByRequester {
			summary.ContextBlocks = boundedContextBlocks(c.ContextBlocks)
			summary.EvaluationBlocks = boundedEvaluationBlocks(c.EvaluationBlocks)
			summary.ScopeCoverageComplete = len(summary.ContextBlocks) == 0 && len(summary.EvaluationBlocks) == 0
		}
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].CaseID < summaries[j].CaseID })

	room.PublicRiskTags = normalizeTags(room.PublicRiskTags)
	room.CreatedAt = room.CreatedAt.UTC()
	room.UpdatedAt = room.UpdatedAt.UTC()
	return ExtendedDossier{
		Room:           room,
		Cases:          summaries,
		ScopePolicy:    classification.RoomScopeParticipantContext,
		CoveragePolicy: classification.RoomCoverageSharedCase,
	}, nil
}

func boundedContextBlocks(blocks []DossierContextBlock) []DossierContextBlock {
	bounded := make([]DossierContextBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.BlockID <= 0 || block.CaseID <= 0 || strings.TrimSpace(block.Content) == "" {
			continue
		}
		bounded = append(bounded, DossierContextBlock{
			BlockID:        block.BlockID,
			CaseID:         block.CaseID,
			CoveragePolicy: normalizeArtifactCoverage(block.CoveragePolicy),
			Content:        strings.TrimSpace(block.Content),
		})
	}
	sort.Slice(bounded, func(i, j int) bool { return bounded[i].BlockID < bounded[j].BlockID })
	return bounded
}

func boundedEvaluationBlocks(blocks []DossierEvaluationBlock) []DossierEvaluationBlock {
	bounded := make([]DossierEvaluationBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.BlockID <= 0 || block.CustomerID <= 0 || strings.TrimSpace(block.Content) == "" {
			continue
		}
		bounded = append(bounded, DossierEvaluationBlock{
			BlockID:        block.BlockID,
			CustomerID:     block.CustomerID,
			CoveragePolicy: normalizeArtifactCoverage(block.CoveragePolicy),
			Content:        strings.TrimSpace(block.Content),
		})
	}
	sort.Slice(bounded, func(i, j int) bool { return bounded[i].BlockID < bounded[j].BlockID })
	return bounded
}

func normalizeArtifactCoverage(value string) string {
	value = normalizeToken(value)
	if value == "" {
		return classification.RoomCoverageSummaryMaterial
	}
	return value
}
