package domain

import (
	"context"
	"time"
)

// Account — orchestrator-side представление счёта (что банк решил открыть
// для applicant'а). Реальное создание в АБС делает abs-connector;
// abs_reference обновляется после успешной интеграции.
// Соответствует packages/domain-model/schema.json $defs.Account (v1.1.0).
type Account struct {
	ID                  string             `json:"id"`
	TenantID            string             `json:"tenant_id"`
	ApplicationID       string             `json:"application_id"`
	LegalEntityID       string             `json:"legal_entity_id"`
	AccountNumber       string             `json:"account_number,omitempty"`
	BIK                 string             `json:"bik,omitempty"`
	BankName            string             `json:"bank_name,omitempty"`
	Currency            string             `json:"currency"`
	AccountType         string             `json:"account_type"` // см. enum в DB CHECK
	CorrespondentAccount string            `json:"correspondent_account,omitempty"`
	TariffPlan          string             `json:"tariff_plan,omitempty"`
	Agreements          AccountAgreements  `json:"agreements"`
	MonitoringProfileID string             `json:"monitoring_profile_id,omitempty"`
	ABSReference        string             `json:"abs_reference,omitempty"`
	OpenedAt            *time.Time         `json:"opened_at,omitempty"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

// AccountAgreements — пакет согласий и подписи при открытии счёта.
// Соответствует $defs.AccountAgreements в schema.json.
type AccountAgreements struct {
	AgreementAcceptance   bool       `json:"agreement_acceptance"`
	AgreementAcceptedAt   time.Time  `json:"agreement_accepted_at"`
	DBOAgreement          bool       `json:"dbo_agreement"`
	DBOChannels           []string   `json:"dbo_channels,omitempty"` // web | mobile | api
	EDOAgreement          bool       `json:"edo_agreement"`
	PersonalDataConsent   bool       `json:"personal_data_consent"`
	SigningMethod         string     `json:"signing_method"` // ukep | sms_code | handwritten
	UKEPCertificateSerial string     `json:"ukep_certificate_serial,omitempty"`
}

// AccountRepository — порт.
type AccountRepository interface {
	Upsert(ctx context.Context, a *Account) error
	GetByApplication(ctx context.Context, tenantID, applicationID string) (*Account, error)
}
