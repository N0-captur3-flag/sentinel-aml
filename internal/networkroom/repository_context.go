package networkroom

import (
	"context"
	"fmt"
)

func (r *Repository) ListExtendedDossierCases(ctx context.Context, userID, roomID int64) ([]ExtendedCaseDossierInput, error) {
	cases, err := r.ListDossierCases(ctx, userID, roomID)
	if err != nil {
		return nil, err
	}

	detailCases := make([]ExtendedCaseDossierInput, 0, len(cases))
	byCaseID := make(map[int64]*ExtendedCaseDossierInput, len(cases))
	for _, c := range cases {
		item := ExtendedCaseDossierInput{CaseDossierInput: c}
		detailCases = append(detailCases, item)
		byCaseID[c.CaseID] = &detailCases[len(detailCases)-1]
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT nra.case_id, nra.id, nra.customer_id, nra.artifact_kind, nra.coverage_policy, nra.content
FROM network_room_artifacts nra
JOIN network_room_participants owned ON owned.room_id = nra.room_id AND owned.user_id = $1
WHERE nra.room_id = $2
  AND btrim(nra.content) <> ''
ORDER BY nra.case_id, nra.artifact_kind, nra.id`,
		userID,
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("list network room artifacts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var caseID int64
		var artifactID, customerID int64
		var artifactKind, coveragePolicy, content string
		if err := rows.Scan(&caseID, &artifactID, &customerID, &artifactKind, &coveragePolicy, &content); err != nil {
			return nil, fmt.Errorf("scan network room artifact: %w", err)
		}
		if item, ok := byCaseID[caseID]; ok {
			switch artifactKind {
			case "room_context":
				item.ContextBlocks = append(item.ContextBlocks, DossierContextBlock{
					BlockID:        artifactID,
					CaseID:         caseID,
					CoveragePolicy: coveragePolicy,
					Content:        content,
				})
			case "assessment_summary":
				item.EvaluationBlocks = append(item.EvaluationBlocks, DossierEvaluationBlock{
					BlockID:        artifactID,
					CustomerID:     customerID,
					CoveragePolicy: coveragePolicy,
					Content:        content,
				})
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate network room artifacts: %w", err)
	}

	return detailCases, nil
}
