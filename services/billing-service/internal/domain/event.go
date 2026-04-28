package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AIbankBillingNamespace — DNS-style UUID namespace для детерминированной
// генерации billing event_id (UUID v5) per ADR-0010 § 5 (idempotency).
// Один и тот же {tenant_id, event_type, source_event_id} всегда даёт
// один и тот же UUID v5, что гарантирует идемпотентность при retry источника.
//
// Значение фиксированное: его нельзя менять без миграции существующих ID.
var AIbankBillingNamespace = uuid.MustParse("6f9a1b2c-3d4e-5f60-8a90-a1ba0010beef")

// Известные event_type — каноничный список билинг-событий платформы.
// Источники (onboarding-orchestrator, agent-ubo-tracing, ...) используют
// эти константы при публикации.
const (
	EventTypeAccountOpenedIP    = "account_opened.ip"
	EventTypeAccountOpenedLLC   = "account_opened.llc"
	EventTypeAccountOpenedJSC   = "account_opened.jsc"
	EventTypeUBOCheckExecuted   = "ubo_check.executed"
	EventTypeManualReviewDone   = "manual_review.completed"
	EventTypeDocumentParsed     = "document.parsed"
	EventTypeRiskScored         = "risk.scored"
)

// BillingEvent — запись об одном billable событии в платформенной таблице
// platform.billing_events (cross-tenant, ADR-0010 § 3).
type BillingEvent struct {
	ID               string          `json:"id"`
	TenantID         string          `json:"tenant_id"`
	EventType        string          `json:"event_type"`
	SourceService    string          `json:"source_service"`
	SourceEventID    string          `json:"source_event_id"`
	Quantity         int             `json:"quantity"`
	UnitPriceKopecks int64           `json:"unit_price_kopecks"`
	TotalKopecks     int64           `json:"total_kopecks"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
	AuditEventID     string          `json:"audit_event_id,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	BilledAt         *time.Time      `json:"billed_at,omitempty"`
}

// DeriveID вычисляет UUID v5 от {tenant_id, event_type, source_event_id}.
// Согласно ADR-0010 § 5 retry источника с теми же тремя полями даёт один
// и тот же id, что обеспечивает идемпотентность INSERT'а в platform.billing_events.
func DeriveID(tenantID, eventType, sourceEventID string) string {
	key := tenantID + "|" + eventType + "|" + sourceEventID
	return uuid.NewSHA1(AIbankBillingNamespace, []byte(key)).String()
}

// ComputeTotal возвращает quantity * unit_price_kopecks без переполнения
// для всех допустимых quantity (платформа не выпускает события с quantity > 10^6).
func ComputeTotal(quantity int, unitPriceKopecks int64) int64 {
	if quantity <= 0 {
		quantity = 1
	}
	return int64(quantity) * unitPriceKopecks
}
