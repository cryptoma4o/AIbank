package workflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	"aibank/onboarding-orchestrator/internal/domain"
	"aibank/onboarding-orchestrator/internal/workflow"
)

// activityMocks — централизованный mock-набор для всех 6 активностей.
//
// Регистрируем функции с теми же именами через RegisterActivityWithOptions —
// workflow вызывает их по строковому имени (см. activities.go).
type activityMocks struct {
	mock.Mock
}

func (m *activityMocks) VerifyIdentity(_ context.Context, applicantID string) (*workflow.IdentityVerificationResult, error) {
	args := m.Called(applicantID)
	if v := args.Get(0); v != nil {
		return v.(*workflow.IdentityVerificationResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *activityMocks) ExtractDocument(_ context.Context, ref workflow.DocumentReference) (*workflow.DocumentExtractionResult, error) {
	args := m.Called(ref)
	if v := args.Get(0); v != nil {
		return v.(*workflow.DocumentExtractionResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *activityMocks) Reconcile(_ context.Context, applicationID string, extractions []workflow.DocumentExtractionResult) (*workflow.ReconciliationResult, error) {
	args := m.Called(applicationID, extractions)
	if v := args.Get(0); v != nil {
		return v.(*workflow.ReconciliationResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *activityMocks) AssessRisk(_ context.Context, applicationID string) (*workflow.RiskAssessmentResult, error) {
	args := m.Called(applicationID)
	if v := args.Get(0); v != nil {
		return v.(*workflow.RiskAssessmentResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *activityMocks) OpenAccount(_ context.Context, applicationID string, productCodes []string) (*workflow.AccountOpenResult, error) {
	args := m.Called(applicationID, productCodes)
	if v := args.Get(0); v != nil {
		return v.(*workflow.AccountOpenResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *activityMocks) CancelAccount(_ context.Context, accountIDs []string) error {
	args := m.Called(accountIDs)
	return args.Error(0)
}

// register регистрирует все методы под именами, которые ожидает workflow.
func (m *activityMocks) register(env *testsuite.TestWorkflowEnvironment) {
	env.RegisterActivityWithOptions(m.VerifyIdentity, activity.RegisterOptions{Name: workflow.ActivityVerifyIdentity})
	env.RegisterActivityWithOptions(m.ExtractDocument, activity.RegisterOptions{Name: workflow.ActivityExtractDocument})
	env.RegisterActivityWithOptions(m.Reconcile, activity.RegisterOptions{Name: workflow.ActivityReconcile})
	env.RegisterActivityWithOptions(m.AssessRisk, activity.RegisterOptions{Name: workflow.ActivityAssessRisk})
	env.RegisterActivityWithOptions(m.OpenAccount, activity.RegisterOptions{Name: workflow.ActivityOpenAccount})
	env.RegisterActivityWithOptions(m.CancelAccount, activity.RegisterOptions{Name: workflow.ActivityCancelAccount})
}

func baseInput() workflow.ApplicationInput {
	return workflow.ApplicationInput{
		ApplicationID:   "app_test_1",
		TenantID:        "alfa",
		ApplicantID:     "per_alice",
		LegalEntityType: domain.LegalEntityLLC,
		Channel:         domain.ChannelWeb,
		ProductCodes:    []string{"current_rub"},
		RiskThresholds: workflow.RiskThresholds{
			AutoApproveBelow: 0.30,
			DeclineAbove:     0.85,
		},
		MaxDocumentWaitDays: 14,
	}
}

func sampleDocs() []workflow.DocumentReference {
	return []workflow.DocumentReference{
		{ID: "doc_1", Type: "PASSPORT", StorageURI: "s3://x/1"},
		{ID: "doc_2", Type: "CHARTER", StorageURI: "s3://x/2"},
	}
}

// ── Test 1: happy path — auto-approved → account_opened ──────────────
func TestOnboardingWorkflow_HappyPath_AutoApproved(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mocks := &activityMocks{}
	mocks.register(env)

	input := baseInput()
	docs := sampleDocs()

	mocks.On("VerifyIdentity", input.ApplicantID).
		Return(&workflow.IdentityVerificationResult{Verified: true, Method: "ESIA"}, nil)

	mocks.On("ExtractDocument", docs[0]).
		Return(&workflow.DocumentExtractionResult{DocumentID: "doc_1", IsValid: true, Confidence: 0.95}, nil)
	mocks.On("ExtractDocument", docs[1]).
		Return(&workflow.DocumentExtractionResult{DocumentID: "doc_2", IsValid: true, Confidence: 0.92}, nil)

	mocks.On("Reconcile", input.ApplicationID, mock.AnythingOfType("[]workflow.DocumentExtractionResult")).
		Return(&workflow.ReconciliationResult{Matched: true, RequiresReview: false}, nil)

	mocks.On("AssessRisk", input.ApplicationID).
		Return(&workflow.RiskAssessmentResult{
			AssessmentID:   "ra_1",
			Score:          0.10, // < 0.30 → auto-approve
			Category:       "LOW",
			Recommendation: "AUTO_APPROVE",
		}, nil)

	mocks.On("OpenAccount", input.ApplicationID, input.ProductCodes).
		Return(&workflow.AccountOpenResult{AccountIDs: []string{"acc_1"}}, nil)

	// После старта workflow отправим сигнал documents_uploaded.
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(workflow.SignalDocumentsUploaded,
			workflow.DocumentsUploadedSignal{Documents: docs, UploadedAt: time.Now()})
	}, time.Millisecond)

	env.ExecuteWorkflow(workflow.OnboardingWorkflow, input)

	require.True(t, env.IsWorkflowCompleted(), "workflow should complete")
	require.NoError(t, env.GetWorkflowError())

	var out workflow.ApplicationOutput
	require.NoError(t, env.GetWorkflowResult(&out))
	require.Equal(t, domain.StateAccountOpened, out.FinalState)
	require.Equal(t, []string{"acc_1"}, out.AccountIDs)

	// Подтверждаем последовательность активностей.
	mocks.AssertCalled(t, "VerifyIdentity", input.ApplicantID)
	mocks.AssertCalled(t, "AssessRisk", input.ApplicationID)
	mocks.AssertCalled(t, "OpenAccount", input.ApplicationID, input.ProductCodes)
	mocks.AssertNotCalled(t, "CancelAccount", mock.Anything)
}

// ── Test 2: manual review approved via signal ────────────────────────
func TestOnboardingWorkflow_ManualReviewApproved(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mocks := &activityMocks{}
	mocks.register(env)

	input := baseInput()
	docs := sampleDocs()

	mocks.On("VerifyIdentity", input.ApplicantID).
		Return(&workflow.IdentityVerificationResult{Verified: true, Method: "ESIA"}, nil)
	mocks.On("ExtractDocument", mock.Anything).
		Return(&workflow.DocumentExtractionResult{IsValid: true, Confidence: 0.9}, nil)
	mocks.On("Reconcile", input.ApplicationID, mock.Anything).
		Return(&workflow.ReconciliationResult{Matched: true}, nil)
	// Score между порогами — manual_review.
	mocks.On("AssessRisk", input.ApplicationID).
		Return(&workflow.RiskAssessmentResult{
			Score: 0.55, Category: "MEDIUM", Recommendation: "MANUAL_REVIEW",
		}, nil)
	mocks.On("OpenAccount", input.ApplicationID, input.ProductCodes).
		Return(&workflow.AccountOpenResult{AccountIDs: []string{"acc_42"}}, nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(workflow.SignalDocumentsUploaded,
			workflow.DocumentsUploadedSignal{Documents: docs, UploadedAt: time.Now()})
	}, time.Millisecond)

	// human_decision приходит после AssessRisk; даём workflow время дойти.
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(workflow.SignalHumanDecision,
			workflow.HumanDecisionSignal{
				Decision:  "approved",
				Reviewer:  "compliance@bank",
				DecidedAt: time.Now(),
			})
	}, time.Hour)

	env.ExecuteWorkflow(workflow.OnboardingWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var out workflow.ApplicationOutput
	require.NoError(t, env.GetWorkflowResult(&out))
	require.Equal(t, domain.StateAccountOpened, out.FinalState)
	require.Equal(t, []string{"acc_42"}, out.AccountIDs)
}

// ── Test 3: declined at risk_assessing (high score + blocking rule) ──
func TestOnboardingWorkflow_DeclinedAtRiskAssessing(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mocks := &activityMocks{}
	mocks.register(env)

	input := baseInput()
	docs := sampleDocs()

	mocks.On("VerifyIdentity", input.ApplicantID).
		Return(&workflow.IdentityVerificationResult{Verified: true, Method: "ESIA"}, nil)
	mocks.On("ExtractDocument", mock.Anything).
		Return(&workflow.DocumentExtractionResult{IsValid: true, Confidence: 0.9}, nil)
	mocks.On("Reconcile", input.ApplicationID, mock.Anything).
		Return(&workflow.ReconciliationResult{Matched: true}, nil)
	mocks.On("AssessRisk", input.ApplicationID).
		Return(&workflow.RiskAssessmentResult{
			Score:           0.95,
			Category:        "HIGH",
			HasBlockingRule: true,
			BlockingReason:  "OKVED_BLOCKED",
			Recommendation:  "DECLINE_RECOMMENDED",
		}, nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(workflow.SignalDocumentsUploaded,
			workflow.DocumentsUploadedSignal{Documents: docs, UploadedAt: time.Now()})
	}, time.Millisecond)

	env.ExecuteWorkflow(workflow.OnboardingWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var out workflow.ApplicationOutput
	require.NoError(t, env.GetWorkflowResult(&out))
	require.Equal(t, domain.StateDeclined, out.FinalState)
	require.Contains(t, out.Reason, "OKVED_BLOCKED")

	// OpenAccount/CancelAccount не должны были вызваться.
	mocks.AssertNotCalled(t, "OpenAccount", mock.Anything, mock.Anything)
	mocks.AssertNotCalled(t, "CancelAccount", mock.Anything)
}

// ── Test 4: ABS open fails → compensation, no double-open ────────────
func TestOnboardingWorkflow_ABSOpenFails_Compensation(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mocks := &activityMocks{}
	mocks.register(env)

	input := baseInput()
	docs := sampleDocs()

	mocks.On("VerifyIdentity", input.ApplicantID).
		Return(&workflow.IdentityVerificationResult{Verified: true, Method: "ESIA"}, nil)
	mocks.On("ExtractDocument", mock.Anything).
		Return(&workflow.DocumentExtractionResult{IsValid: true, Confidence: 0.9}, nil)
	mocks.On("Reconcile", input.ApplicationID, mock.Anything).
		Return(&workflow.ReconciliationResult{Matched: true}, nil)
	mocks.On("AssessRisk", input.ApplicationID).
		Return(&workflow.RiskAssessmentResult{
			Score: 0.10, Category: "LOW", Recommendation: "AUTO_APPROVE",
		}, nil)

	// ABS падает на всех попытках.
	mocks.On("OpenAccount", input.ApplicationID, input.ProductCodes).
		Return(nil, errors.New("ABS unreachable"))
	// Компенсация ожидается ровно один раз (idempotent).
	mocks.On("CancelAccount", []string{}).Return(nil).Once()

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(workflow.SignalDocumentsUploaded,
			workflow.DocumentsUploadedSignal{Documents: docs, UploadedAt: time.Now()})
	}, time.Millisecond)

	env.ExecuteWorkflow(workflow.OnboardingWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var out workflow.ApplicationOutput
	require.NoError(t, env.GetWorkflowResult(&out))
	// Компенсация — финальное состояние НЕ account_opened, остаётся approved.
	require.Equal(t, domain.StateApproved, out.FinalState)
	require.Contains(t, out.Reason, "abs open failed")
	require.Empty(t, out.AccountIDs)

	mocks.AssertCalled(t, "OpenAccount", input.ApplicationID, input.ProductCodes)
	mocks.AssertCalled(t, "CancelAccount", []string{})
}
