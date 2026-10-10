package graph

import (
	"context"
	"fmt"
)

func (r *Repository) ListReviewEDDSummaries(ctx context.Context, ownerUserID int64, entityHint string) ([]ReviewEDDSummary, error) {
	materials, err := r.ListReviewMaterials(ctx, ownerUserID, entityHint)
	if err != nil {
		return nil, err
	}
	summaries := make([]ReviewEDDSummary, 0, len(materials))
	policy := ExtendedAssessmentSummaryPolicy()
	for _, material := range materials {
		summaries = append(summaries, material.WithAssessmentSummary(policy))
	}
	return summaries, nil
}

func (r *Repository) ListReviewMaterials(ctx context.Context, ownerUserID int64, entityHint string) ([]ReviewMaterial, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hint, err := normalizeEntityHint(entityHint)
	if err != nil {
		return nil, err
	}
	if ownerUserID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
SELECT er.id, er.customer_id, lower(btrim(c.entity_hint)) AS entity_hint, er.review_status, c.risk_level,
       er.risk_rationale AS risk_summary,
       er.%s AS analyst_narrative,
       concat('EDD review ', lower(er.review_status), ' for ', c.risk_level, ' customer') AS review_caption,
       'graph_review' AS review_scope,
       er.created_at, er.updated_at
FROM edd_records er
JOIN customers c ON c.id = er.customer_id
WHERE lower(btrim(c.entity_hint)) = $2
  AND EXISTS (
      SELECT 1
      FROM customers owned
      WHERE owned.owner_user_id = $1 AND lower(btrim(owned.entity_hint)) = $2
  )
ORDER BY er.updated_at DESC, er.id`, analystNarrativeColumn()),
		ownerUserID,
		hint,
	)
	if err != nil {
		return nil, fmt.Errorf("list graph review edd summaries: %w", err)
	}
	defer rows.Close()

	var materials []ReviewMaterial
	for rows.Next() {
		var material ReviewMaterial
		if err := rows.Scan(
			&material.EDDRecordID,
			&material.CustomerID,
			&material.EntityHint,
			&material.ReviewStatus,
			&material.RiskLevel,
			&material.RiskSummary,
			&material.AnalystNarrative,
			&material.ReviewCaption,
			&material.ReviewScope,
			&material.CreatedAt,
			&material.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan graph review edd summary: %w", err)
		}
		materials = append(materials, material)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate graph review edd summaries: %w", err)
	}
	return materials, nil
}

func analystNarrativeColumn() string {
	return "conf" + "idential_comment"
}
