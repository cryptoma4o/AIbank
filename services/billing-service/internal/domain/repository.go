package domain

import (
	"context"
	"time"
)

// Aggregate — результат AggregateByTenant: сумма за период + разбивка по типу события.
type Aggregate struct {
	TenantID     string           `json:"tenant_id"`
	From         time.Time        `json:"from"`
	To           time.Time        `json:"to"`
	TotalKopecks int64            `json:"total_kopecks"`
	ByEventType  map[string]int64 `json:"by_event_type"`
	EventCount   int              `json:"event_count"`
}

// BillingEventRepository — store для platform.billing_events.
//
// ADR-0010 § 5: Append идемпотентен по PK (id) — повторная запись игнорируется
// без ошибки. Вызывающий код может определить факт дубликата через флаг inserted.
type BillingEventRepository interface {
	// Append вставляет билинг-событие. Если запись с тем же id уже есть,
	// возвращает существующую запись и inserted=false (идемпотентность).
	// При успешной вставке возвращает сохранённую запись и inserted=true.
	Append(ctx context.Context, evt *BillingEvent) (stored *BillingEvent, inserted bool, err error)

	// GetByID возвращает событие по id, либо ErrNotFound.
	GetByID(ctx context.Context, id string) (*BillingEvent, error)

	// ListByTenant возвращает события тенанта за полуоткрытый интервал [from, to).
	// limit=0 трактуется как «без лимита» (с safety cap на стороне реализации).
	ListByTenant(ctx context.Context, tenantID string, from, to time.Time, limit int) ([]*BillingEvent, error)

	// AggregateByTenant возвращает суммы и счёт за период [from, to).
	AggregateByTenant(ctx context.Context, tenantID string, from, to time.Time) (*Aggregate, error)
}
