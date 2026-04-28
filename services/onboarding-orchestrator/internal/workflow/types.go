// Package workflow содержит Temporal-воркфлоу и интерфейсы активностей
// онбординг-оркестратора. Все типы здесь сериализуются Temporal'ом в JSON,
// поэтому используем JSON-теги, без map с не-string ключами и без чанков
// >2 МБ (Temporal payload limit — см. ADR-0001).
package workflow

import (
	"time"

	"aibank/onboarding-orchestrator/internal/domain"
)

// ApplicationInput — параметры запуска OnboardingWorkflow.
//
// Передаётся из HTTP handler'а в Temporal через ExecuteWorkflow.
// ВАЖНО: workflow ОБЯЗАН быть детерминированным — никаких time.Now,
// никаких uuid.New внутри workflow-кода. ApplicationID и WorkflowID
// формируются handler'ом ДО запуска.
type ApplicationInput struct {
	ApplicationID   string                 `json:"application_id"`
	TenantID        string                 `json:"tenant_id"`
	ApplicantID     string                 `json:"applicant_id"`
	LegalEntityID   string                 `json:"legal_entity_id,omitempty"`
	LegalEntityType domain.LegalEntityType `json:"legal_entity_type"`
	Channel         domain.Channel         `json:"channel"`
	ProductCodes    []string               `json:"product_codes"`

	// RiskThresholds — пороги решения после risk_assessing.
	// score < AutoApproveBelow → auto_approved
	// score >= DeclineAbove   → declined (вместе с blocking-rule)
	// иначе                   → manual_review
	RiskThresholds RiskThresholds `json:"risk_thresholds"`

	// MaxDocumentWaitDays — лимит ожидания загрузки документов клиентом.
	// 0 = без лимита (но обычно 14 дней).
	MaxDocumentWaitDays int `json:"max_document_wait_days,omitempty"`
}

// RiskThresholds — конфигурация пороговых значений риск-движка.
//
// Тенант передаёт свои значения; workflow остаётся detеrministic.
type RiskThresholds struct {
	AutoApproveBelow float64 `json:"auto_approve_below"` // напр. 0.30
	DeclineAbove     float64 `json:"decline_above"`      // напр. 0.85
}

// ApplicationOutput — возвращаемое значение OnboardingWorkflow.
type ApplicationOutput struct {
	ApplicationID string                  `json:"application_id"`
	FinalState    domain.ApplicationState `json:"final_state"`
	DecisionID    string                  `json:"decision_id,omitempty"`
	AccountIDs    []string                `json:"account_ids,omitempty"`
	Reason        string                  `json:"reason,omitempty"`
}

// IdentityVerificationResult — результат IdentityActivity.Verify.
type IdentityVerificationResult struct {
	Verified  bool      `json:"verified"`
	Method    string    `json:"method"` // ESIA | UKEP | MANUAL
	CheckedAt time.Time `json:"checked_at"`
	Reason    string    `json:"reason,omitempty"` // при !Verified
}

// DocumentReference — лёгкая ссылка на документ для активностей workflow.
type DocumentReference struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	StorageURI string `json:"storage_uri"`
}

// DocumentExtractionResult — результат DocumentActivity.Extract.
type DocumentExtractionResult struct {
	DocumentID  string  `json:"document_id"`
	Fields      map[string]string `json:"fields"`
	Confidence  float64 `json:"confidence"`
	IsValid     bool    `json:"is_valid"`
	ExtractedAt time.Time `json:"extracted_at"`
}

// ReconciliationResult — результат сверки данных между документами / ЕГРЮЛ.
type ReconciliationResult struct {
	Matched         bool     `json:"matched"`
	Discrepancies   []string `json:"discrepancies,omitempty"`
	RequiresReview  bool     `json:"requires_review"`
	ReconciledAt    time.Time `json:"reconciled_at"`
}

// RiskAssessmentResult — результат RiskActivity.Assess.
//
// Соответствует подмножеству docs/domain-model.md § 2.7 RiskAssessment.
// Score — float [0..1]; пороги задаются клиентом тенанта.
type RiskAssessmentResult struct {
	AssessmentID    string    `json:"assessment_id"`
	Score           float64   `json:"score"`
	Category        string    `json:"category"` // LOW | MEDIUM | HIGH
	HasBlockingRule bool      `json:"has_blocking_rule"`
	BlockingReason  string    `json:"blocking_reason,omitempty"`
	Recommendation  string    `json:"recommendation"` // AUTO_APPROVE | MANUAL_REVIEW | DECLINE_RECOMMENDED
	ComputedAt      time.Time `json:"computed_at"`
}

// AccountOpenResult — результат ABSActivity.OpenAccount.
type AccountOpenResult struct {
	AccountIDs []string  `json:"account_ids"`
	OpenedAt   time.Time `json:"opened_at"`
	ABSRunID   string    `json:"abs_run_id,omitempty"`
}

// HumanDecisionSignal — payload сигнала "human_decision" из manual_review.
type HumanDecisionSignal struct {
	Decision  string    `json:"decision"` // approved | declined | approved_with_edd
	Reason    string    `json:"reason,omitempty"`
	Reviewer  string    `json:"reviewer"`
	DecidedAt time.Time `json:"decided_at"`
}

// DocumentsUploadedSignal — payload сигнала "documents_uploaded" из collecting_documents.
type DocumentsUploadedSignal struct {
	Documents  []DocumentReference `json:"documents"`
	UploadedAt time.Time           `json:"uploaded_at"`
}
