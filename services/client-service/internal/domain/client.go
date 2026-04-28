// Package domain holds the core entities for client-service (КУС — Клиентское
// Унифицированное Состояние / Client Unified Card).
//
// КУС — единая карточка клиента, в которой по доменной модели (см.
// docs/domain-model.md § 2.4 LegalEntity, § 2.10 AuditEvent) аккумулируются
// связи между Application, LegalEntity и состоянием обслуживания. Сущность
// `Client` — это операционная проекция, а не источник правды о юрлице:
// исходные данные ЕГРЮЛ хранятся в LegalEntity (другой контекст).
package domain

import "time"

// ClientStatus описывает жизненный цикл клиентской карточки.
type ClientStatus string

const (
	// ClientStatusOnboarding — заявка ещё в процессе, счёт не открыт.
	ClientStatusOnboarding ClientStatus = "onboarding"
	// ClientStatusActive — активный клиент, счёт открыт и обслуживается.
	ClientStatusActive ClientStatus = "active"
	// ClientStatusSuspended — обслуживание приостановлено (комплаенс/санкции).
	ClientStatusSuspended ClientStatus = "suspended"
	// ClientStatusArchived — клиент закрыт. Запись хранится 5 лет (152-ФЗ).
	ClientStatusArchived ClientStatus = "archived"
)

// IsValid сообщает, входит ли значение в зафиксированный enum.
func (s ClientStatus) IsValid() bool {
	switch s {
	case ClientStatusOnboarding, ClientStatusActive, ClientStatusSuspended, ClientStatusArchived:
		return true
	}
	return false
}

// RiskCategory — крупнокатегорийная риск-метка из RiskAssessment.
// Числовой score не дублируем — он живёт в risk-engine.
type RiskCategory string

const (
	RiskCategoryLow    RiskCategory = "LOW"
	RiskCategoryMedium RiskCategory = "MEDIUM"
	RiskCategoryHigh   RiskCategory = "HIGH"
)

// IsValid возвращает true для одного из трёх ожидаемых значений.
func (r RiskCategory) IsValid() bool {
	switch r {
	case RiskCategoryLow, RiskCategoryMedium, RiskCategoryHigh:
		return true
	}
	return false
}

// Client — операционная проекция клиента банка (КУС).
//
// Привязка идёт к Application и LegalEntity, чтобы из одной точки была видна
// история обслуживания, риск-категория и текущий статус. Поля денежных сумм
// или подробностей юрлица здесь намеренно отсутствуют — это другие контексты.
type Client struct {
	ID            string       `json:"id"`
	TenantID      string       `json:"tenant_id"`
	ApplicantID   string       `json:"applicant_id"`
	LegalEntityID string       `json:"legal_entity_id"`
	Status        ClientStatus `json:"status"`
	RiskCategory  RiskCategory `json:"risk_category"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// ClientHistory — append-only событие в истории клиентской карточки.
// Создаётся orchestrator'ом при значимых переходах (новый документ, решение,
// смена риск-категории и т.п.). UPDATE и DELETE запрещены триггером БД.
type ClientHistory struct {
	ID        string    `json:"id"`
	ClientID  string    `json:"client_id"`
	EventType string    `json:"event_type"`
	Summary   string    `json:"summary"`
	// Source — ссылка на audit_event_id в audit-service (или иной первичный
	// источник). Хранится как строка, чтобы не создавать жёстких FK
	// между сервисами.
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}
