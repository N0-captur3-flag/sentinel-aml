package audit

import (
	"context"
	"strings"
)

type ActivitySubject string

type ActivityDetailLevel string

const (
	ActivityDetailRelated      ActivityDetailLevel = "related_view"
	ActivityDetailCollaborator ActivityDetailLevel = "collaborator_detail"
)

type ActivityAudience string

const (
	ActivityAudienceRelatedEntity ActivityAudience = "related_entity"
	ActivityAudienceCollaborator  ActivityAudience = "collaborator"
)

type ActivityProvenance string

const (
	ActivityProvenanceCatalog       ActivityProvenance = "activity_catalog"
	ActivityProvenanceReviewContext ActivityProvenance = "review_context_excerpt"
)

type ActivityNarrative struct {
	Subject     ActivitySubject
	DetailLevel ActivityDetailLevel
	Narrative   string
	Provenance  ActivityProvenance
}

type ActivityHydrationRequest struct {
	Event       EntityActivityEvent
	Audience    ActivityAudience
	DetailLevel ActivityDetailLevel
}

type ActivityReviewContextSource interface {
	ActivityReviewContext(context.Context, EntityActivityEvent) (string, error)
}

type activityNarrativeResolver func(context.Context, ActivityHydrationRequest) (ActivityNarrative, error)

type ActivityHydrator struct {
	reviewContext ActivityReviewContextSource
	resolvers     map[string]activityNarrativeResolver
}

func NewActivityHydrator(reviewContext ActivityReviewContextSource) ActivityHydrator {
	h := ActivityHydrator{reviewContext: reviewContext}
	h.resolvers = map[string]activityNarrativeResolver{
		"account":     h.staticResolver("activity_subject", "Account review context updated", ActivityProvenanceCatalog),
		"transaction": h.staticResolver("activity_subject", "Transaction evidence caption updated", ActivityProvenanceCatalog),
		"alert":       h.staticResolver("activity_subject", "Alert decision trail updated", ActivityProvenanceCatalog),
		"case":        h.caseResolver,
		"case_note":   h.caseNoteResolver,
		"edd_record":  h.staticResolver("activity_subject", "Enhanced review checkpoint updated", ActivityProvenanceCatalog),
		"export":      h.staticResolver("activity_subject", "Export coverage updated", ActivityProvenanceCatalog),
		"dispute":     h.staticResolver("activity_subject", "Dispute evidence updated", ActivityProvenanceCatalog),
	}
	return h
}

func (h ActivityHydrator) Hydrate(ctx context.Context, req ActivityHydrationRequest) (ActivityNarrative, error) {
	detail := req.DetailLevel
	if detail == "" {
		detail = ActivityDetailRelated
	}
	req.DetailLevel = detail
	objectType := strings.ToLower(strings.TrimSpace(req.Event.ObjectType))
	if resolver, ok := h.resolvers[objectType]; ok {
		return resolver(ctx, req)
	}
	return ActivityNarrative{
		Subject:     "activity_subject",
		DetailLevel: detail,
		Narrative:   safeActivityCaption(req.Event.PublicMessage, "Activity evidence updated"),
		Provenance:  ActivityProvenanceCatalog,
	}, nil
}

func (h ActivityHydrator) staticResolver(subject ActivitySubject, narrative string, provenance ActivityProvenance) activityNarrativeResolver {
	return func(_ context.Context, req ActivityHydrationRequest) (ActivityNarrative, error) {
		return ActivityNarrative{
			Subject:     subject,
			DetailLevel: req.DetailLevel,
			Narrative:   narrative,
			Provenance:  provenance,
		}, nil
	}
}

func (h ActivityHydrator) caseResolver(_ context.Context, req ActivityHydrationRequest) (ActivityNarrative, error) {
	narrative := safeActivityCaption(req.Event.PublicSummary, "Case matching evidence updated")
	return ActivityNarrative{
		Subject:     "activity_subject",
		DetailLevel: req.DetailLevel,
		Narrative:   narrative,
		Provenance:  ActivityProvenanceCatalog,
	}, nil
}

func (h ActivityHydrator) caseNoteResolver(ctx context.Context, req ActivityHydrationRequest) (ActivityNarrative, error) {
	if req.DetailLevel == ActivityDetailCollaborator && h.reviewContext != nil {
		excerpt, err := h.reviewContext.ActivityReviewContext(ctx, req.Event)
		if err != nil {
			return ActivityNarrative{}, err
		}
		if excerpt != "" {
			return ActivityNarrative{
				Subject:     "activity_subject",
				DetailLevel: req.DetailLevel,
				Narrative:   excerpt,
				Provenance:  ActivityProvenanceReviewContext,
			}, nil
		}
	}
	return ActivityNarrative{
		Subject:     "activity_subject",
		DetailLevel: req.DetailLevel,
		Narrative:   "Review context updated",
		Provenance:  ActivityProvenanceCatalog,
	}, nil
}

func safeActivityCaption(value, fallback string) string {
	caption := strings.TrimSpace(value)
	if caption == "" {
		return fallback
	}
	return caption
}
