package casework

import (
	"context"
	"strings"
)

type MatchEvidenceSource string

const (
	MatchEvidenceSourcePublicTransaction MatchEvidenceSource = "public_transaction_evidence"
	MatchEvidenceSourceCounterparty      MatchEvidenceSource = "counterparty_overlap"
	MatchEvidenceSourceRiskTags          MatchEvidenceSource = "risk_tags"
	MatchEvidenceSourceReviewContext     MatchEvidenceSource = "review_context_excerpt"
)

type MatchEvidenceRequest struct {
	OwnerUserID     int64
	SourceCaseID    int64
	CandidateCaseID int64
}

type CaseSimilarityEvidence struct {
	Source  MatchEvidenceSource
	Excerpt string
	Weight  int
}

type MatchEvidenceResolver interface {
	ResolveMatchEvidence(context.Context, MatchEvidenceRequest) ([]CaseSimilarityEvidence, error)
}

type MatchExplanationBuilder struct {
	preferredSources []MatchEvidenceSource
}

func NewMatchExplanationBuilder(preferredSources ...MatchEvidenceSource) MatchExplanationBuilder {
	return MatchExplanationBuilder{preferredSources: append([]MatchEvidenceSource(nil), preferredSources...)}
}

func PublicMatchEvidenceSources() []MatchEvidenceSource {
	return []MatchEvidenceSource{
		MatchEvidenceSourceCounterparty,
		MatchEvidenceSourceRiskTags,
		MatchEvidenceSourcePublicTransaction,
	}
}

func (b MatchExplanationBuilder) Build(evidence []CaseSimilarityEvidence, fallback string) string {
	for _, preferred := range b.preferredSources {
		if excerpt := firstEvidenceExcerpt(evidence, preferred); excerpt != "" {
			return excerpt
		}
	}
	for _, item := range evidence {
		if excerpt := strings.TrimSpace(item.Excerpt); excerpt != "" {
			return excerpt
		}
	}
	return strings.TrimSpace(fallback)
}

func PublicCaseSimilarityEvidence(candidate DuplicateCandidate) []CaseSimilarityEvidence {
	evidence := []CaseSimilarityEvidence{
		{
			Source:  MatchEvidenceSourceCounterparty,
			Excerpt: strings.TrimSpace(candidate.DisplayName),
			Weight:  80,
		},
		{
			Source:  MatchEvidenceSourcePublicTransaction,
			Excerpt: strings.TrimSpace(candidate.DedupHint),
			Weight:  70,
		},
	}
	if risk := publicRiskEvidence(candidate); risk != "" {
		evidence = append(evidence, CaseSimilarityEvidence{
			Source:  MatchEvidenceSourceRiskTags,
			Excerpt: risk,
			Weight:  60,
		})
	}
	return evidence
}

func firstEvidenceExcerpt(evidence []CaseSimilarityEvidence, source MatchEvidenceSource) string {
	for _, item := range evidence {
		if item.Source == source {
			if excerpt := strings.TrimSpace(item.Excerpt); excerpt != "" {
				return excerpt
			}
		}
	}
	return ""
}

func publicRiskEvidence(candidate DuplicateCandidate) string {
	parts := make([]string, 0, 2)
	if candidate.RiskLevel != "" {
		parts = append(parts, "risk_level:"+candidate.RiskLevel)
	}
	if candidate.TypologyHint != "" {
		parts = append(parts, "typology:"+candidate.TypologyHint)
	}
	return strings.Join(parts, " ")
}
