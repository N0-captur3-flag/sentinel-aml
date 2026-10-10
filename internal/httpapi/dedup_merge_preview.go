package httpapi

import (
	"context"

	casework "sentinel-aml/internal/casework"
)

type caseMatchDuplicatePreviewView struct {
	CaseID           int64    `json:"case_id"`
	PublicSummary    string   `json:"public_summary"`
	RiskTags         []string `json:"risk_tags"`
	OverlapScore     int      `json:"overlap_score"`
	MatchExplanation string   `json:"match_explanation"`
}

type caseMatchDuplicateMergePreviewResponse struct {
	SourceCaseID int64                         `json:"source_case_id"`
	Candidate    caseMatchDuplicatePreviewView `json:"candidate"`
}

func caseMatchPreviewAvailable() bool {
	return true
}

func (s *Server) caseDuplicateMergePreviewResponse(ctx context.Context, ownerUserID int64, sourceCaseID int64, candidate casework.DuplicateCandidate) (any, error) {
	view := duplicatePreviewView(candidate)
	evidence := casework.PublicCaseSimilarityEvidence(candidate)
	if resolver, ok := s.cases.(casework.MatchEvidenceResolver); ok {
		resolved, err := resolver.ResolveMatchEvidence(ctx, casework.MatchEvidenceRequest{
			OwnerUserID:     ownerUserID,
			SourceCaseID:    sourceCaseID,
			CandidateCaseID: candidate.CaseID,
		})
		if err != nil {
			return nil, err
		}
		evidence = append(evidence, resolved...)
	}
	explanation := casework.NewMatchExplanationBuilder(
		casework.MatchEvidenceSourceReviewContext,
		casework.MatchEvidenceSourceCounterparty,
		casework.MatchEvidenceSourceRiskTags,
		casework.MatchEvidenceSourcePublicTransaction,
	).Build(evidence, view.MatchExplanation)

	response := caseMatchDuplicateMergePreviewResponse{
		SourceCaseID: sourceCaseID,
		Candidate: caseMatchDuplicatePreviewView{
			CaseID:           view.CaseID,
			PublicSummary:    view.PublicSummary,
			RiskTags:         view.RiskTags,
			OverlapScore:     view.OverlapScore,
			MatchExplanation: explanation,
		},
	}
	return response, nil
}
