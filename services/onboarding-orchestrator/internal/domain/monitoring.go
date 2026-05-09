package domain

import (
	"context"
	"time"
)

// MonitoringProfile — параметры постоянного мониторинга открытого счёта
// (этапы 8 и 10 формы онбординга). Соответствует
// packages/domain-model/schema.json $defs.MonitoringProfile (v1.1.0).
type MonitoringProfile struct {
	ID                    string             `json:"id"`
	TenantID              string             `json:"tenant_id"`
	ApplicationID         string             `json:"application_id"`
	AccountID             string             `json:"account_id,omitempty"`
	ReviewFrequencyMonths int                `json:"review_frequency_months"` // 3 | 6 | 12
	NextReviewDate        string             `json:"next_review_date"`        // YYYY-MM-DD
	MonitoringRules       []MonitoringRule   `json:"monitoring_rules,omitempty"`
	KYCRefreshTriggers    []string           `json:"kyc_refresh_triggers,omitempty"`
	TransactionLimits     *TransactionLimits `json:"transaction_limits,omitempty"`
	NotificationChannels  []string           `json:"notification_channels,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

// MonitoringRule — одно правило 375-П / per-tenant.
type MonitoringRule struct {
	Code        string         `json:"code"`
	Description string         `json:"description"`
	Params      map[string]any `json:"params,omitempty"`
}

// TransactionLimits — лимиты по типам операций (этап 8).
type TransactionLimits struct {
	DailyOutgoing        *MoneyAmount `json:"daily_outgoing,omitempty"`
	DailyCashWithdrawal  *MoneyAmount `json:"daily_cash_withdrawal,omitempty"`
	MonthlyOutgoing      *MoneyAmount `json:"monthly_outgoing,omitempty"`
	SingleTransactionMax *MoneyAmount `json:"single_transaction_max,omitempty"`
}

// MonitoringProfileRepository — порт.
type MonitoringProfileRepository interface {
	Upsert(ctx context.Context, p *MonitoringProfile) error
	GetByApplication(ctx context.Context, tenantID, applicationID string) (*MonitoringProfile, error)
}
