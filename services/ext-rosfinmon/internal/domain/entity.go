package domain

type SanctionedEntity struct {
	INN      string `json:"inn,omitempty"`
	FullName string `json:"full_name"`
	ListType string `json:"list_type"` // "terrorist" | "extremist" | "proliferation"
}

type CheckResult struct {
	INN     string `json:"inn"`
	Blocked bool   `json:"blocked"`
	Reason  string `json:"reason,omitempty"`
}
