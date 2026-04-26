package audit

import "time"

type EventType string

const (
	EventApplicationCreated  EventType = "application.created"
	EventApplicationUpdated  EventType = "application.updated"
	EventDocumentUploaded    EventType = "document.uploaded"
	EventDecisionMade        EventType = "decision.made"
	EventAccountOpened       EventType = "account.opened"
	EventIdentityVerified    EventType = "identity.verified"
	EventRiskScored          EventType = "risk.scored"
	EventTenantConfigChanged EventType = "tenant_config.changed"
)

type AuditEvent struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	EventType    EventType      `json:"event_type"`
	ActorID      string         `json:"actor_id"`
	ActorRole    string         `json:"actor_role"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Payload      map[string]any `json:"payload,omitempty"`
	OccurredAt   time.Time      `json:"occurred_at"`
}
