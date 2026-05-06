package domain

import (
	"context"
	"time"
)

// LegalEntityProfile — расширенная анкета юрлица (этап 2 формы онбординга).
// Соответствует packages/domain-model/schema.json $defs.LegalEntityProfile
// (v1.1.0). Адреса/лицензии/СРО/контакты держим как value objects ниже —
// в БД они хранятся в JSONB колонках.
type LegalEntityProfile struct {
	ID                    string             `json:"id"`
	TenantID              string             `json:"tenant_id"`
	ApplicationID         string             `json:"application_id"`
	LegalEntityID         string             `json:"legal_entity_id"`
	OPFCode               string             `json:"opf_code,omitempty"`
	RegistrationAuthority string             `json:"registration_authority,omitempty"`
	AuthorizedCapital     *MoneyAmount       `json:"authorized_capital,omitempty"`
	LegalAddress          *StructuredAddress `json:"legal_address_struct,omitempty"`
	ActualAddress         *StructuredAddress `json:"actual_address_struct,omitempty"`
	ActualSameAsLegal     bool               `json:"actual_same_as_legal"`
	PostalAddress         *StructuredAddress `json:"postal_address_struct,omitempty"`
	PostalSameAsLegal     bool               `json:"postal_same_as_legal"`
	OKVEDMain             string             `json:"okved_main_v2,omitempty"`
	OKVEDAdditional       []string           `json:"okved_additional_v2,omitempty"`
	Licenses              []LicenseInfo      `json:"licenses,omitempty"`
	SROMembership         []SROMembership    `json:"sro_membership,omitempty"`
	Contacts              *ContactInfo       `json:"contacts,omitempty"`
	EmployeesCount        *int               `json:"employees_count,omitempty"`
	RevenueLastYear       *MoneyAmount       `json:"revenue_last_year,omitempty"`
	TaxRegime             string             `json:"tax_regime,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

// StructuredAddress — структурированный адрес (этап 2/4). Зеркалит
// $defs.StructuredAddress в schema.json.
type StructuredAddress struct {
	CountryCode string `json:"country_code"`
	PostalCode  string `json:"postal_code,omitempty"`
	RegionCode  string `json:"region_code,omitempty"`
	RegionName  string `json:"region_name,omitempty"`
	City        string `json:"city"`
	Street      string `json:"street,omitempty"`
	Building    string `json:"building,omitempty"`
	Office      string `json:"office,omitempty"`
	FIASID      string `json:"fias_id,omitempty"`
}

// MoneyAmount — сумма + валюта (ISO 4217).
type MoneyAmount struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// ContactInfo — контакты юрлица.
type ContactInfo struct {
	Phone   string `json:"phone,omitempty"`
	Email   string `json:"email,omitempty"`
	Website string `json:"website,omitempty"`
}

// LicenseInfo — лицензия / разрешение.
type LicenseInfo struct {
	Number       string `json:"number"`
	IssueDate    string `json:"issue_date"`
	ExpiryDate   string `json:"expiry_date,omitempty"`
	Issuer       string `json:"issuer"`
	ActivityType string `json:"activity_type"`
}

// SROMembership — членство в СРО.
type SROMembership struct {
	Name      string `json:"name"`
	RegNumber string `json:"reg_number"`
	JoinDate  string `json:"join_date"`
}

// LegalEntityProfileRepository — порт персистентного хранилища анкеты
// юрлица. Один профиль на заявку (UNIQUE INDEX в БД).
type LegalEntityProfileRepository interface {
	// Upsert создаёт или обновляет профиль для заявки. Возвращает свежее
	// состояние с актуальными created_at / updated_at.
	Upsert(ctx context.Context, profile *LegalEntityProfile) error

	// GetByApplication возвращает профиль для заявки. ErrNotFound если
	// анкета ещё не была заполнена.
	GetByApplication(ctx context.Context, tenantID, applicationID string) (*LegalEntityProfile, error)
}
