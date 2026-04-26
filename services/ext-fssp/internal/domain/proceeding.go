package domain

type EnforcementProceeding struct {
	Number     string `json:"number"`
	INN        string `json:"inn"`
	DebtorName string `json:"debtor_name"`
	Amount     int64  `json:"amount_kopecks"`
	Status     string `json:"status"` // "active" | "closed"
	OpenedAt   string `json:"opened_at"`
}

type FSSPResult struct {
	INN         string                  `json:"inn"`
	HasActive   bool                    `json:"has_active"`
	TotalDebt   int64                   `json:"total_debt_kopecks"`
	Proceedings []EnforcementProceeding `json:"proceedings"`
}
