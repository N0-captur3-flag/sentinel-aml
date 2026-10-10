package httpapi

import (
	"context"
	"time"

	networkrepo "sentinel-aml/internal/networkroom"
)

type networkRoomDetailDossierRepository interface {
	ListExtendedDossierCases(context.Context, int64, int64) ([]networkrepo.ExtendedCaseDossierInput, error)
	ListDossierCases(context.Context, int64, int64) ([]networkrepo.CaseDossierInput, error)
}

type networkRoomDetailDossierView struct {
	Room           NetworkRoomSummaryView      `json:"room"`
	Cases          []networkRoomDetailCaseView `json:"cases"`
	ScopePolicy    string                      `json:"scope_policy"`
	CoveragePolicy string                      `json:"coverage_policy"`
}

type networkRoomDetailCaseView struct {
	CaseID                int64                                `json:"case_id"`
	CustomerID            int64                                `json:"customer_id"`
	OwnedByRequester      bool                                 `json:"owned_by_requester"`
	Status                string                               `json:"status"`
	PublicSummary         string                               `json:"public_summary"`
	RiskScore             int                                  `json:"risk_score"`
	TypologyHint          string                               `json:"typology_hint"`
	PublicRiskTags        []string                             `json:"public_risk_tags"`
	ScopeCoverageComplete bool                                 `json:"scope_coverage_complete"`
	ContextBlocks         []networkrepo.DossierContextBlock    `json:"context_blocks,omitempty"`
	EvaluationBlocks      []networkrepo.DossierEvaluationBlock `json:"evaluation_blocks,omitempty"`
	CreatedAt             time.Time                            `json:"created_at"`
	UpdatedAt             time.Time                            `json:"updated_at"`
}

func networkRoomContextAvailable() bool {
	return true
}

func (s *Server) networkRoomDossierResponseForSharedRoom(ctx context.Context, userID, roomID int64, room networkrepo.RoomSummary) (any, error) {
	reader, ok := s.networkRooms.(networkRoomDetailDossierRepository)
	if !ok {
		cases, err := s.networkRooms.ListDossierCases(ctx, userID, roomID)
		if err != nil {
			return nil, err
		}
		dossier, err := networkrepo.BuildSafeDossier(userID, room, cases)
		if err != nil {
			return nil, err
		}
		return networkRoomDossierView(dossier), nil
	}

	cases, err := reader.ListExtendedDossierCases(ctx, userID, roomID)
	if err != nil {
		return nil, err
	}
	dossier, err := networkrepo.BuildExtendedDossier(userID, room, cases)
	if err != nil {
		return nil, err
	}
	return networkRoomDetailDossierView{
		Room:           networkRoomSummaryView(dossier.Room),
		Cases:          networkRoomDetailCaseViews(dossier.Cases),
		ScopePolicy:    dossier.ScopePolicy,
		CoveragePolicy: dossier.CoveragePolicy,
	}, nil
}

func networkRoomDetailCaseViews(cases []networkrepo.ExtendedCaseDossierSummary) []networkRoomDetailCaseView {
	views := make([]networkRoomDetailCaseView, 0, len(cases))
	for _, c := range cases {
		views = append(views, networkRoomDetailCaseView{
			CaseID:                c.CaseID,
			CustomerID:            c.CustomerID,
			OwnedByRequester:      c.OwnedByRequester,
			Status:                c.Status,
			PublicSummary:         c.PublicSummary,
			RiskScore:             c.RiskScore,
			TypologyHint:          c.TypologyHint,
			PublicRiskTags:        append([]string(nil), c.PublicRiskTags...),
			ScopeCoverageComplete: c.ScopeCoverageComplete,
			ContextBlocks:         append([]networkrepo.DossierContextBlock(nil), c.ContextBlocks...),
			EvaluationBlocks:      append([]networkrepo.DossierEvaluationBlock(nil), c.EvaluationBlocks...),
			CreatedAt:             c.CreatedAt,
			UpdatedAt:             c.UpdatedAt,
		})
	}
	return views
}
