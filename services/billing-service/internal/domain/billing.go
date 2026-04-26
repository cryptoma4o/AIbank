package domain

import "time"

type EventKind string

const (
	EventKindAccountOpened    EventKind = "account_opened"
	EventKindUBOCheck         EventKind = "ubo_check"
	EventKindDocumentProcessed EventKind = "document_processed"
	EventKindRiskScored       EventKind = "risk_scored"
	EventKindEGRULLookup      EventKind = "egrul_lookup"
)

// Default prices in kopecks (integer, never float)
var DefaultPriceKopecks = map[EventKind]int64{
	EventKindAccountOpened:    50000, // 500 ₽
	EventKindUBOCheck:         20000, // 200 ₽
	EventKindDocumentProcessed: 5000, // 50 ₽
	EventKindRiskScored:        3000, // 30 ₽
	EventKindEGRULLookup:       1000, // 10 ₽
}

type BillableEvent struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Kind         EventKind `json:"kind"`
	ResourceID   string    `json:"resource_id"`
	PriceKopecks int64     `json:"price_kopecks"`
	OccurredAt   time.Time `json:"occurred_at"`
}

type UsageReport struct {
	TenantID     string              `json:"tenant_id"`
	PeriodStart  time.Time           `json:"period_start"`
	PeriodEnd    time.Time           `json:"period_end"`
	TotalKopecks int64               `json:"total_kopecks"`
	ByKind       map[EventKind]int64 `json:"by_kind"`
	EventCount   int                 `json:"event_count"`
}
