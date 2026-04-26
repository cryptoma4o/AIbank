package domain

import "time"

type ApplicationStatus string

const (
	StatusDraft          ApplicationStatus = "draft"
	StatusIdentifying    ApplicationStatus = "identifying"
	StatusCollecting     ApplicationStatus = "collecting"
	StatusValidating     ApplicationStatus = "validating"
	StatusAutoApproved   ApplicationStatus = "auto_approved"
	StatusManualReview   ApplicationStatus = "manual_review"
	StatusRejected       ApplicationStatus = "rejected"
	StatusAccountOpening ApplicationStatus = "account_opening"
	StatusCompleted      ApplicationStatus = "completed"
)

type Application struct {
	ID        string            `json:"id"`
	TenantID  string            `json:"tenant_id"`
	Status    ApplicationStatus `json:"status"`
	INN       string            `json:"inn"`
	OGRN      string            `json:"ogrn"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type OnboardingInput struct {
	ApplicationID string `json:"application_id"`
	TenantID      string `json:"tenant_id"`
	INN           string `json:"inn"`
	OGRN          string `json:"ogrn"`
}

type OnboardingResult struct {
	ApplicationID string            `json:"application_id"`
	FinalStatus   ApplicationStatus `json:"final_status"`
	AccountID     string            `json:"account_id,omitempty"`
}
