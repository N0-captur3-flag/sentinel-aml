package authz

type OwnerScopeContract struct {
	Resource        string
	Operations      []string
	CrossOwnerError string
	Notes           string
}

func CurrentOwnerScopeContracts() []OwnerScopeContract {
	return []OwnerScopeContract{
		{
			Resource:        "customer",
			Operations:      []string{"GET /api/customers/{customer_id}"},
			CrossOwnerError: "not_found",
			Notes:           "customer ownership is direct through customers.owner_user_id",
		},
		{
			Resource:        "account",
			Operations:      []string{"GET /api/accounts/{account_id}", "GET /api/customers/{customer_id}/accounts", "POST /api/customers/{customer_id}/accounts"},
			CrossOwnerError: "not_found",
			Notes:           "account ownership resolves through customer ownership",
		},
		{
			Resource:        "transaction",
			Operations:      []string{"GET /api/transactions/{transaction_id}", "GET /api/customers/{customer_id}/transactions", "POST /api/transactions"},
			CrossOwnerError: "not_found",
			Notes:           "transaction ownership resolves through account and customer ownership",
		},
		{
			Resource:        "alert",
			Operations:      []string{"GET /api/alerts/queue", "GET /api/alerts/{alert_id}", "POST /api/alerts/{alert_id}/claim", "POST /api/alerts/{alert_id}/triage"},
			CrossOwnerError: "not_found",
			Notes:           "object-scoped alert operations fail closed; queue results are owner-filtered",
		},
		{
			Resource:        "case",
			Operations:      []string{"GET /api/cases/{case_id}", "PATCH /api/cases/{case_id}/status"},
			CrossOwnerError: "not_found",
			Notes:           "case ownership is direct through cases.owner_user_id",
		},
		{
			Resource:        "case_note",
			Operations:      []string{"GET /api/cases/{case_id}/notes", "POST /api/cases/{case_id}/notes"},
			CrossOwnerError: "not_found",
			Notes:           "case-note access resolves through owned cases",
		},
		{
			Resource:        "edd_record",
			Operations:      []string{"GET /api/customers/{customer_id}/edd", "POST /api/customers/{customer_id}/edd", "PATCH /api/customers/{customer_id}/edd"},
			CrossOwnerError: "not_found",
			Notes:           "EDD access resolves through customer ownership",
		},
	}
}
