package workflow

import "context"

// EGRULData holds data fetched from the EGRUL registry.
type EGRULData struct {
	FullName string `json:"full_name"`
	OKVED    string `json:"okved"`
	CEO      string `json:"ceo"`
}

// Activities groups all Temporal activity implementations.
type Activities struct{}

// VerifyINN validates the INN number via an external service (stub).
func (a *Activities) VerifyINN(_ context.Context, _ string) error { return nil }

// FetchEGRUL fetches company data from the EGRUL registry (stub).
func (a *Activities) FetchEGRUL(_ context.Context, _, _ string) (*EGRULData, error) {
	return &EGRULData{}, nil
}

// ScreenRosfinmon checks the applicant against the Rosfinmonitoring list (stub).
// Returns true if the applicant is blocked.
func (a *Activities) ScreenRosfinmon(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// RunRiskScoring calculates a risk score for the application (stub).
func (a *Activities) RunRiskScoring(_ context.Context, _ string) (int, error) {
	return 75, nil
}

// OpenAccount creates a bank account for an approved application (stub).
func (a *Activities) OpenAccount(_ context.Context, _, _ string) (string, error) {
	return "acc_placeholder", nil
}

// NotifyClient sends a status notification to the applicant (stub).
func (a *Activities) NotifyClient(_ context.Context, _, _ string) error { return nil }
