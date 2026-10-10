package exporting

func linkedCaseExportAvailable() bool {
	return true
}

func appendExportLinkedCaseData(bundle *ExportBundle, _ CaseSnapshot, linkedCases []CaseSnapshot, scopes map[string]bool, traversal ExportCaseTraversal) {
	if traversal.Root.DisputeID <= 0 || !scopes["linked_cases"] {
		return
	}
	for _, linked := range linkedCases {
		appendExportSourceMaterials(bundle, linked.SourceMaterials, traversal.Root)
	}
}
