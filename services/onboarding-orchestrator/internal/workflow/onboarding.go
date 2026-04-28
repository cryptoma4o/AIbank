package workflow

import (
	"errors"
	"time"

	"aibank/onboarding-orchestrator/internal/domain"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// TaskQueue — имя очереди для онбординг-воркера.
const TaskQueue = "onboarding"

// Имена сигналов. Должны совпадать в HTTP-handler'е и в тестах.
const (
	SignalDocumentsUploaded = "documents_uploaded"
	SignalHumanDecision     = "human_decision"
)

// ErrCompensated — workflow откатил частично-успешное открытие счёта.
// Возвращается в ApplicationOutput.Reason для аудита.
var ErrCompensated = errors.New("account open compensated")

// OnboardingWorkflow — детерминированный Temporal workflow онбординга.
//
// Реализует state machine из docs/domain-model.md § 2.2:
//
//	draft → identifying → collecting_documents → validating →
//	  risk_assessing → {auto_approved | manual_review | requires_more_info | declined}
//	manual_review → {approved | approved_with_edd | declined}
//	approved → opening_account → account_opened
//
// Детерминизм:
//   - Никаких time.Now (используем workflow.Now)
//   - Никаких goroutine (только workflow.Go)
//   - Никаких uuid.New (ID-шники приходят в input)
//
// Активности — interface'ы (см. activities.go); регистрация и реальная
// реализация — за пределами этого файла.
func OnboardingWorkflow(ctx workflow.Context, input ApplicationInput) (*ApplicationOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("OnboardingWorkflow started",
		"application_id", input.ApplicationID,
		"tenant_id", input.TenantID,
	)

	// Стандартные опции активностей.  Внешние API могут быть медленными,
	// поэтому даём 5 минут на StartToClose с ретраями.
	activityOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOpts)

	out := &ApplicationOutput{
		ApplicationID: input.ApplicationID,
		FinalState:    domain.StateDraft,
	}

	// ── 1. draft → identifying ────────────────────────────────────────
	var idResult IdentityVerificationResult
	if err := workflow.ExecuteActivity(ctx, ActivityVerifyIdentity, input.ApplicantID).
		Get(ctx, &idResult); err != nil {
		out.FinalState = domain.StateDeclined
		out.Reason = "identity verification failed: " + err.Error()
		return out, nil
	}
	if !idResult.Verified {
		out.FinalState = domain.StateDeclined
		out.Reason = "identity not verified: " + idResult.Reason
		return out, nil
	}

	// ── 2. identifying → collecting_documents ─────────────────────────
	docs, err := waitForDocuments(ctx, input)
	if err != nil {
		out.FinalState = domain.StateAbandoned
		out.Reason = "client did not upload documents in time"
		return out, nil
	}

	// ── 3. collecting_documents → validating ──────────────────────────
	extractions := make([]DocumentExtractionResult, 0, len(docs))
	for _, ref := range docs {
		var ex DocumentExtractionResult
		if err := workflow.ExecuteActivity(ctx, ActivityExtractDocument, ref).
			Get(ctx, &ex); err != nil {
			out.FinalState = domain.StateDeclined
			out.Reason = "document extraction failed: " + err.Error()
			return out, nil
		}
		extractions = append(extractions, ex)
	}

	var rec ReconciliationResult
	if err := workflow.ExecuteActivity(ctx, ActivityReconcile, input.ApplicationID, extractions).
		Get(ctx, &rec); err != nil {
		out.FinalState = domain.StateDeclined
		out.Reason = "reconciliation failed: " + err.Error()
		return out, nil
	}
	if rec.RequiresReview {
		// Для MVP: blocking-несоответствие приводит к declined.  В будущем
		// здесь будет цикл waiting_for_client → validating (см. state.go).
		out.FinalState = domain.StateDeclined
		out.Reason = "reconciliation discrepancies require manual fix"
		return out, nil
	}

	// ── 4. validating → risk_assessing ────────────────────────────────
	var risk RiskAssessmentResult
	if err := workflow.ExecuteActivity(ctx, ActivityAssessRisk, input.ApplicationID).
		Get(ctx, &risk); err != nil {
		out.FinalState = domain.StateDeclined
		out.Reason = "risk assessment failed: " + err.Error()
		return out, nil
	}

	// ── 5. risk_assessing → auto_approved | manual_review | declined ──
	approved := false
	switch {
	case risk.HasBlockingRule:
		out.FinalState = domain.StateDeclined
		out.Reason = "blocking rule: " + risk.BlockingReason
		return out, nil

	case risk.Score >= input.RiskThresholds.DeclineAbove:
		out.FinalState = domain.StateDeclined
		out.Reason = "risk score above decline threshold"
		return out, nil

	case risk.Score < input.RiskThresholds.AutoApproveBelow:
		// auto_approved → approved (без оператора).
		approved = true

	default:
		// manual_review — ждём сигнал от оператора.
		decision, derr := waitForHumanDecision(ctx)
		if derr != nil {
			out.FinalState = domain.StateAbandoned
			out.Reason = "manual review timeout"
			return out, nil
		}
		switch decision.Decision {
		case "approved", "approved_with_edd":
			approved = true
		case "declined":
			out.FinalState = domain.StateDeclined
			out.Reason = "operator declined: " + decision.Reason
			return out, nil
		default:
			out.FinalState = domain.StateDeclined
			out.Reason = "invalid operator decision"
			return out, nil
		}
	}

	if !approved {
		// Защитная ветка — не должна срабатывать.
		out.FinalState = domain.StateDeclined
		out.Reason = "approval flow indeterminate"
		return out, nil
	}

	// ── 6. approved → opening_account (с компенсацией) ────────────────
	absOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    2, // ABS — внешняя система; reduces double-open risk
		},
	}
	absCtx := workflow.WithActivityOptions(ctx, absOpts)

	var openResult AccountOpenResult
	openErr := workflow.ExecuteActivity(absCtx, ActivityOpenAccount, input.ApplicationID, input.ProductCodes).
		Get(absCtx, &openResult)
	if openErr != nil {
		// Компенсация: откат любых частично созданных счетов.
		// Состояние НЕ переходит в account_opened; остаётся approved.
		// CancelAccount idempotent — безопасно вызывать с пустым списком.
		_ = workflow.ExecuteActivity(absCtx, ActivityCancelAccount, []string{}).
			Get(absCtx, nil)
		out.FinalState = domain.StateApproved
		out.Reason = "abs open failed: " + openErr.Error()
		return out, nil
	}

	out.FinalState = domain.StateAccountOpened
	out.AccountIDs = openResult.AccountIDs
	logger.Info("OnboardingWorkflow finished",
		"application_id", input.ApplicationID,
		"final_state", out.FinalState,
	)
	return out, nil
}

// waitForDocuments блокирует workflow до получения сигнала
// "documents_uploaded" или истечения таймера.
//
// Использует workflow.NewSelector — детерминированно и корректно работает
// с replay'ем Temporal'а.
func waitForDocuments(ctx workflow.Context, input ApplicationInput) ([]DocumentReference, error) {
	signalCh := workflow.GetSignalChannel(ctx, SignalDocumentsUploaded)

	var payload DocumentsUploadedSignal
	var timedOut bool

	timeoutDays := input.MaxDocumentWaitDays
	if timeoutDays <= 0 {
		timeoutDays = 14
	}
	timer := workflow.NewTimer(ctx, time.Duration(timeoutDays)*24*time.Hour)

	sel := workflow.NewSelector(ctx)
	sel.AddReceive(signalCh, func(c workflow.ReceiveChannel, _ bool) {
		c.Receive(ctx, &payload)
	})
	sel.AddFuture(timer, func(workflow.Future) {
		timedOut = true
	})
	sel.Select(ctx)

	if timedOut {
		return nil, errors.New("document upload timeout")
	}
	return payload.Documents, nil
}

// waitForHumanDecision ждёт сигнал "human_decision".  Жёсткого таймаута
// нет — комплаенс может думать сколько нужно, но workflow всё ещё
// детерминированный (любая активность по перепроверке вызывается извне).
func waitForHumanDecision(ctx workflow.Context) (*HumanDecisionSignal, error) {
	ch := workflow.GetSignalChannel(ctx, SignalHumanDecision)
	var d HumanDecisionSignal
	ch.Receive(ctx, &d)
	if d.Decision == "" {
		return nil, errors.New("empty decision signal")
	}
	return &d, nil
}
