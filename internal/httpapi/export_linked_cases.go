package httpapi

import (
	"context"

	casework "sentinel-aml/internal/casework"
	exportrepo "sentinel-aml/internal/exporting"
)

func linkedCaseExportAvailable() bool {
	return true
}

func (s *Server) populateExportLinkedCaseData(ctx context.Context, ownerUserID, sourceCaseID int64, linked *exportrepo.CaseSnapshot) error {
	resolver, ok := s.cases.(casework.MatchEvidenceResolver)
	if !ok {
		return nil
	}
	evidence, err := resolver.ResolveMatchEvidence(ctx, casework.MatchEvidenceRequest{
		OwnerUserID:     ownerUserID,
		SourceCaseID:    sourceCaseID,
		CandidateCaseID: linked.CaseID,
	})
	if err != nil {
		return err
	}
	for _, item := range evidence {
		if item.Source != casework.MatchEvidenceSourceReviewContext || item.Excerpt == "" {
			continue
		}
		linked.SourceMaterials = append(linked.SourceMaterials, exportrepo.ExportSourceMaterial{
			ItemType:        "case_matching_evidence",
			ObjectType:      "case",
			ObjectID:        linked.CaseID,
			CaseID:          linked.CaseID,
			Relation:        "linked",
			SourceClass:     "review_context_excerpt",
			Content:         item.Excerpt,
			EvidenceExcerpt: item.Excerpt,
			Caption:         "Case matching evidence",
			CoveragePolicy:  exportrepo.ExportScopeRootCaseDetail,
		})
	}
	return nil
}
