package graph

import "strings"

type ReviewNarrativePolicy string

const (
	ReviewNarrativePolicyGraph      ReviewNarrativePolicy = "graph_view"
	ReviewNarrativePolicyExtended   ReviewNarrativePolicy = "extended_review"
	ReviewNarrativePolicyRestricted ReviewNarrativePolicy = "restricted_review"
)

type AssessmentSummaryPolicy struct {
	NarrativePolicy ReviewNarrativePolicy
}

type ReviewMaterial struct {
	EDDSummary
	RiskSummary      string
	AnalystNarrative string
	ReviewCaption    string
	ReviewScope      string
}

type ReviewEDDSummary struct {
	EDDSummary
	AssessmentSummary string
}

func GraphAssessmentSummaryPolicy() AssessmentSummaryPolicy {
	return AssessmentSummaryPolicy{NarrativePolicy: ReviewNarrativePolicyGraph}
}

func ExtendedAssessmentSummaryPolicy() AssessmentSummaryPolicy {
	return AssessmentSummaryPolicy{NarrativePolicy: ReviewNarrativePolicyExtended}
}

func RestrictedAssessmentSummaryPolicy() AssessmentSummaryPolicy {
	return AssessmentSummaryPolicy{NarrativePolicy: ReviewNarrativePolicyRestricted}
}

func (p AssessmentSummaryPolicy) Build(material ReviewMaterial) string {
	switch p.NarrativePolicy {
	case ReviewNarrativePolicyExtended:
		if isDetailedReviewStatus(material.ReviewStatus) {
			if narrative := cleanSummary(material.AnalystNarrative); narrative != "" {
				return narrative
			}
		}
		if summary := cleanSummary(material.RiskSummary); summary != "" {
			return summary
		}
	case ReviewNarrativePolicyRestricted:
		if caption := cleanSummary(material.ReviewCaption); caption != "" {
			return caption
		}
		if scope := cleanSummary(material.ReviewScope); scope != "" {
			return scope
		}
	default:
		if summary := cleanSummary(material.RiskSummary); summary != "" {
			return summary
		}
		if caption := cleanSummary(material.ReviewCaption); caption != "" {
			return caption
		}
	}
	return defaultAssessmentSummary(material)
}

func (m ReviewMaterial) WithAssessmentSummary(policy AssessmentSummaryPolicy) ReviewEDDSummary {
	return ReviewEDDSummary{
		EDDSummary:        m.EDDSummary,
		AssessmentSummary: policy.Build(m),
	}
}

func isDetailedReviewStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "DRAFT", "IN_REVIEW", "APPROVED":
		return true
	default:
		return false
	}
}

func cleanSummary(value string) string {
	return strings.TrimSpace(value)
}

func defaultAssessmentSummary(material ReviewMaterial) string {
	status := strings.ToLower(strings.TrimSpace(material.ReviewStatus))
	risk := strings.TrimSpace(material.RiskLevel)
	if status == "" {
		status = "pending"
	}
	if risk == "" {
		return "EDD review " + status
	}
	return "EDD review " + status + " for " + risk + " customer"
}
