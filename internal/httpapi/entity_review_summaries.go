package httpapi

import (
	"context"
	"time"

	graphrepo "sentinel-aml/internal/graph"
)

type entityReviewEDDSummaryReader interface {
	ListReviewEDDSummaries(context.Context, int64, string) ([]graphrepo.ReviewEDDSummary, error)
}

type entityReviewEDDGraphSummaryView struct {
	CustomerID        int64     `json:"customer_id"`
	EntityHint        string    `json:"entity_hint"`
	ReviewStatus      string    `json:"review_status"`
	RiskTags          []string  `json:"risk_tags"`
	AssessmentSummary string    `json:"assessment_summary,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type entityReviewEDDGraphSummariesResponse struct {
	Summaries []entityReviewEDDGraphSummaryView `json:"summaries"`
}

func entityReviewSummariesAvailable() bool {
	return true
}

func (s *Server) graphEDDSummariesResponseForEntityReview(ctx context.Context, ownerUserID int64, entityHint string, summaries []graphrepo.EDDSummary) (any, error) {
	reader, ok := s.graph.(entityReviewEDDSummaryReader)
	if !ok {
		return EDDGraphSummariesResponse{Summaries: eddGraphSummaryViews(summaries)}, nil
	}
	reviewSummaries, err := reader.ListReviewEDDSummaries(ctx, ownerUserID, entityHint)
	if err != nil {
		return nil, err
	}
	return entityReviewEDDGraphSummariesResponse{Summaries: entityReviewEDDGraphSummaryViews(reviewSummaries)}, nil
}

func entityReviewEDDGraphSummaryViews(summaries []graphrepo.ReviewEDDSummary) []entityReviewEDDGraphSummaryView {
	responses := make([]entityReviewEDDGraphSummaryView, 0, len(summaries))
	for _, summary := range summaries {
		responses = append(responses, entityReviewEDDGraphSummaryView{
			CustomerID:        summary.CustomerID,
			EntityHint:        summary.EntityHint,
			ReviewStatus:      summary.ReviewStatus,
			RiskTags:          eddGraphRiskTags(summary.EDDSummary),
			AssessmentSummary: summary.AssessmentSummary,
			CreatedAt:         summary.CreatedAt,
			UpdatedAt:         summary.UpdatedAt,
		})
	}
	return responses
}
