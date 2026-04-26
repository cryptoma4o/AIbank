package domain

import "time"

type ClientType string

const (
	ClientTypeIP  ClientType = "ip"  // Individual entrepreneur (ИП)
	ClientTypeOOO ClientType = "ooo" // LLC (ООО)
	ClientTypeAO  ClientType = "ao"  // JSC (АО)
)

type ClientStatus string

const (
	ClientStatusActive    ClientStatus = "active"
	ClientStatusSuspended ClientStatus = "suspended"
	ClientStatusClosed    ClientStatus = "closed"
)

type Client struct {
	ID         string       `json:"id"`
	TenantID   string       `json:"tenant_id"`
	INN        string       `json:"inn"`
	OGRN       string       `json:"ogrn"`
	FullName   string       `json:"full_name"`
	Type       ClientType   `json:"type"`
	Status     ClientStatus `json:"status"`
	AccountIDs []string     `json:"account_ids"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

type EventCategory string

const (
	EventCategoryApplication EventCategory = "application"
	EventCategoryDocument    EventCategory = "document"
	EventCategoryDecision    EventCategory = "decision"
	EventCategoryAccount     EventCategory = "account"
	EventCategoryRisk        EventCategory = "risk"
)

type ClientEvent struct {
	ID         string        `json:"id"`
	ClientID   string        `json:"client_id"`
	TenantID   string        `json:"tenant_id"`
	Category   EventCategory `json:"category"`
	EventType  string        `json:"event_type"`
	ResourceID string        `json:"resource_id"`
	Payload    []byte        `json:"payload"` // JSON blob
	OccurredAt time.Time     `json:"occurred_at"`
}
