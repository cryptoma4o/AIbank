package workflow

import "context"

// Имена активностей, регистрируемых в Temporal worker'е.
//
// Используем явные строковые имена через RegisterActivityWithOptions,
// чтобы тесты могли подменять реализации без зависимости от типа структуры.
const (
	ActivityVerifyIdentity = "VerifyIdentity"
	ActivityExtractDocument = "ExtractDocument"
	ActivityReconcile       = "Reconcile"
	ActivityAssessRisk      = "AssessRisk"
	ActivityOpenAccount     = "OpenAccount"
	ActivityCancelAccount   = "CancelAccount"
)

// IdentityActivity — порт к identity-service (ЕСИА, УКЭП, ручная верификация).
//
// Вызывается из workflow на переходе draft→identifying.
//
// Workflow invokes the activity by string name (ActivityVerifyIdentity) with
// the argument tuple (tenantID, applicantID); see onboarding.go §1.
type IdentityActivity interface {
	// Verify — синхронный вызов identity-service. Должен быть idempotent
	// по (tenantID, applicantID) (повтор при retry активности — ок).
	Verify(ctx context.Context, tenantID, applicantID string) (*IdentityVerificationResult, error)
}

// DocumentActivity — порт к document-service (загрузка/распознавание).
type DocumentActivity interface {
	// Extract — извлечение полей из документа (OCR + LLM).
	// Должен быть idempotent по documentID.
	Extract(ctx context.Context, ref DocumentReference) (*DocumentExtractionResult, error)
}

// ReconciliationActivity — порт к reconciliation-service.
//
// Сверяет данные клиента, документов и регистров (ЕГРЮЛ, ФССП).
type ReconciliationActivity interface {
	// Reconcile принимает список извлечённых документов + ID юрлица и
	// возвращает результат сверки. RequiresReview=true означает
	// state→waiting_for_client (нужны уточнения от клиента).
	Reconcile(ctx context.Context, applicationID string, extractions []DocumentExtractionResult) (*ReconciliationResult, error)
}

// RiskActivity — порт к risk-engine.
//
// Workflow invokes the activity by string name (ActivityAssessRisk) with the
// argument tuple (tenantID, applicationID); see onboarding.go §4.
type RiskActivity interface {
	// Assess вычисляет риск-скор и возвращает рекомендацию.
	// Применяет blocking-rules — например, OKVED_BLOCKED, ROSFINMON_HIT.
	Assess(ctx context.Context, tenantID, applicationID string) (*RiskAssessmentResult, error)
}

// ABSActivity — порт к abs-connector (АБС банка).
type ABSActivity interface {
	// OpenAccount создаёт счета в АБС по списку productCodes.
	// При сбое — workflow ОБЯЗАН вызвать CancelAccount (компенсация SAGA).
	OpenAccount(ctx context.Context, applicationID string, productCodes []string) (*AccountOpenResult, error)

	// CancelAccount — компенсирующая активность. Вызывается, если
	// open-цепочка частично выполнилась и нужно откатить состояние АБС.
	// Должна быть idempotent.
	CancelAccount(ctx context.Context, accountIDs []string) error
}
