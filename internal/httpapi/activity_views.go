package httpapi

import (
	"context"
	"encoding/json"

	auditrepo "sentinel-aml/internal/audit"
)

func auditActivityViewsAvailable() bool {
	return true
}

func (s *Server) caseAuditViewsForActivityStream(_ context.Context, _ int64, _ int64, events []auditrepo.Event) ([]CaseAuditView, error) {
	return caseAuditViews(events), nil
}

func (s *Server) entityActivityViewsForActivityStream(ctx context.Context, _ int64, _ string, events []auditrepo.EntityActivityEvent) ([]EntityActivityView, error) {
	return s.entityActivityViewsWithDetail(ctx, events, auditrepo.ActivityAudienceRelatedEntity, func(event auditrepo.EntityActivityEvent) auditrepo.ActivityDetailLevel {
		if event.ObjectType == "case_note" && hasControlledActivityChange(event.DiffJSON) {
			return auditrepo.ActivityDetailCollaborator
		}
		return auditrepo.ActivityDetailRelated
	})
}

func hasControlledActivityChange(diffJSON string) bool {
	var changes []auditrepo.RedactedFieldChange
	if err := json.Unmarshal([]byte(diffJSON), &changes); err != nil {
		return false
	}
	for _, change := range changes {
		if change.Redacted && auditrepo.IsRestrictedField(change.Field) {
			return true
		}
	}
	return false
}
