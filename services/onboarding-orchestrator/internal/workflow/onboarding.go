package workflow

import (
	"time"

	"aibank/onboarding-orchestrator/internal/domain"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// OnboardingWorkflow is the main Temporal workflow that drives a client application
// through every stage of the onboarding state machine.
func OnboardingWorkflow(ctx workflow.Context, input domain.OnboardingInput) (*domain.OnboardingResult, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var a *Activities

	// Step 1: identifying — verify INN
	if err := workflow.ExecuteActivity(ctx, a.VerifyINN, input.INN).Get(ctx, nil); err != nil {
		return &domain.OnboardingResult{
			ApplicationID: input.ApplicationID,
			FinalStatus:   domain.StatusRejected,
		}, err
	}

	// Step 2: fetch EGRUL data
	var egrulData EGRULData
	if err := workflow.ExecuteActivity(ctx, a.FetchEGRUL, input.INN, input.OGRN).Get(ctx, &egrulData); err != nil {
		return &domain.OnboardingResult{
			ApplicationID: input.ApplicationID,
			FinalStatus:   domain.StatusRejected,
		}, err
	}

	// Step 3: collecting — Rosfinmon screen
	var blocked bool
	if err := workflow.ExecuteActivity(ctx, a.ScreenRosfinmon, input.INN).Get(ctx, &blocked); err != nil {
		return &domain.OnboardingResult{
			ApplicationID: input.ApplicationID,
			FinalStatus:   domain.StatusRejected,
		}, err
	}
	if blocked {
		return &domain.OnboardingResult{
			ApplicationID: input.ApplicationID,
			FinalStatus:   domain.StatusRejected,
		}, nil
	}

	// Step 4: validating — risk scoring
	var score int
	if err := workflow.ExecuteActivity(ctx, a.RunRiskScoring, input.ApplicationID).Get(ctx, &score); err != nil {
		return &domain.OnboardingResult{
			ApplicationID: input.ApplicationID,
			FinalStatus:   domain.StatusManualReview,
		}, nil
	}

	if score < 60 {
		finalStatus := domain.StatusManualReview
		_ = workflow.ExecuteActivity(ctx, a.NotifyClient, input.ApplicationID, string(finalStatus)).Get(ctx, nil)
		return &domain.OnboardingResult{
			ApplicationID: input.ApplicationID,
			FinalStatus:   finalStatus,
		}, nil
	}

	// Step 5: account opening
	var accountID string
	if err := workflow.ExecuteActivity(ctx, a.OpenAccount, input.ApplicationID, input.TenantID).Get(ctx, &accountID); err != nil {
		return &domain.OnboardingResult{
			ApplicationID: input.ApplicationID,
			FinalStatus:   domain.StatusRejected,
		}, err
	}

	_ = workflow.ExecuteActivity(ctx, a.NotifyClient, input.ApplicationID, string(domain.StatusCompleted)).Get(ctx, nil)

	return &domain.OnboardingResult{
		ApplicationID: input.ApplicationID,
		FinalStatus:   domain.StatusCompleted,
		AccountID:     accountID,
	}, nil
}
