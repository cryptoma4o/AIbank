// Package domain содержит доменные типы и бизнес-логику онбординг-оркестратора.
//
// Каноническая модель — docs/domain-model.md § 2.2 (Application).
// Все переходы состояний инкапсулированы в state.go (ApplicationState +
// ValidTransitions).  Здесь — только pure-data структуры, без зависимостей
// от Temporal или БД.
package domain

import (
	"time"
)

// LegalEntityType — организационно-правовая форма заявителя.
//
// IP — индивидуальный предприниматель, LLC — ООО, JSC — АО, NPF — НКО.
// См. docs/domain-model.md § 2.4 (LegalEntity.type) — здесь подмножество,
// поддерживаемое онбордингом на старте.
type LegalEntityType string

const (
	LegalEntityIP  LegalEntityType = "IP"
	LegalEntityLLC LegalEntityType = "LLC"
	LegalEntityJSC LegalEntityType = "JSC"
	LegalEntityNPF LegalEntityType = "NPF"
)

// IsValid возвращает true, если значение допустимое.
func (t LegalEntityType) IsValid() bool {
	switch t {
	case LegalEntityIP, LegalEntityLLC, LegalEntityJSC, LegalEntityNPF:
		return true
	}
	return false
}

// Channel — канал поступления заявки (web|mobile|courier|branch).
type Channel string

const (
	ChannelWeb     Channel = "web"
	ChannelMobile  Channel = "mobile"
	ChannelCourier Channel = "courier"
	ChannelBranch  Channel = "branch"
)

// IsValid возвращает true для разрешённых каналов.
func (c Channel) IsValid() bool {
	switch c {
	case ChannelWeb, ChannelMobile, ChannelCourier, ChannelBranch:
		return true
	}
	return false
}

// Application — главная workflow-сущность.
//
// Один Application = одна заявка от draft до account_opened|declined|abandoned.
// Поля соответствуют docs/domain-model.md § 2.2.
type Application struct {
	ID              string           `json:"id"`
	TenantID        string           `json:"tenant_id"`
	ApplicantID     string           `json:"applicant_id"`
	LegalEntityID   string           `json:"legal_entity_id,omitempty"`
	LegalEntityType LegalEntityType  `json:"legal_entity_type"`
	Channel         Channel          `json:"channel"`
	State           ApplicationState `json:"state"`
	ProductCodes    []string         `json:"product_codes"`
	WorkflowID      string           `json:"workflow_id"`

	RiskAssessmentID string   `json:"risk_assessment_id,omitempty"`
	DecisionID       string   `json:"decision_id,omitempty"`
	AccountIDs       []string `json:"account_ids,omitempty"`

	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
}
