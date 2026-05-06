package domain

import (
	"context"
	"time"
)

// ApplicationActivity — AML-сведения о деятельности (этап 3 формы
// онбординга). Соответствует packages/domain-model/schema.json
// $defs.ApplicationActivity (v1.1.0). Один-к-одному с Application.
type ApplicationActivity struct {
	ID                  string            `json:"id"`
	TenantID            string            `json:"tenant_id"`
	ApplicationID       string            `json:"application_id"`
	BusinessDescription string            `json:"business_description"`
	BusinessCategory    string            `json:"business_category"`
	TopSuppliers        []Counterparty    `json:"top_suppliers,omitempty"`
	TopBuyers           []Counterparty    `json:"top_buyers,omitempty"`
	OperationalModel    *OperationalModel `json:"operational_model,omitempty"`
	FundsSource         FundsSource       `json:"funds_source"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

// Counterparty — top-5 поставщик/покупатель.
type Counterparty struct {
	Name             string  `json:"name"`
	INN              string  `json:"inn,omitempty"`
	Country          string  `json:"country"`
	SharePercent     float64 `json:"share_percent"`
	RelationshipType string  `json:"relationship_type"` // "regular" | "one_off"
}

// OperationalModel — планируемая модель операций.
type OperationalModel struct {
	Geography                []string     `json:"geography,omitempty"`
	MonthlyTurnoverPlanned   *MoneyAmount `json:"monthly_turnover_planned,omitempty"`
	AnnualTurnoverPlanned    *MoneyAmount `json:"annual_turnover_planned,omitempty"`
	CashSharePercent         *float64     `json:"cash_share_percent,omitempty"`
	ForeignEconomicActivity  bool         `json:"foreign_economic_activity"`
	ForeignCountries         []string     `json:"foreign_countries,omitempty"`
	CurrencyOperations       []string     `json:"currency_operations,omitempty"`
}

// FundsSource — заявленный источник средств. Description обязателен только
// при category="other".
type FundsSource struct {
	Category    string `json:"category"`
	Description string `json:"description,omitempty"`
}

// ApplicationActivityRepository — порт хранилища AML-анкеты.
type ApplicationActivityRepository interface {
	Upsert(ctx context.Context, a *ApplicationActivity) error
	GetByApplication(ctx context.Context, tenantID, applicationID string) (*ApplicationActivity, error)
}
