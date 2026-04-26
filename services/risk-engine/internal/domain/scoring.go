package domain

type ScoreRequest struct {
	ApplicationID string         `json:"application_id"`
	TenantID      string         `json:"tenant_id"`
	INN           string         `json:"inn"`
	OKVED         string         `json:"okved"`
	OGRN          string         `json:"ogrn"`
	CompanyAge    int            `json:"company_age_years"`
	Facts         map[string]any `json:"facts"` // additional facts for rules
}

type ScoreResult struct {
	ApplicationID string   `json:"application_id"`
	Score         int      `json:"score"`         // 0-100, higher = less risky
	Blocked       bool     `json:"blocked"`
	Flags         []string `json:"flags"`
	RequiredDocs  []string `json:"required_docs"`
	FiredRules    []string `json:"fired_rules"`
	MLScore       *int     `json:"ml_score,omitempty"` // nil when ML not available
}
