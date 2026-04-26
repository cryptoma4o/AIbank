package domain

import "context"

type AuditEventRepository interface {
	// Append adds a new event. NEVER updates or deletes.
	Append(ctx context.Context, event *AuditEvent) error
	// LatestHash returns the hash of the last event for a tenant (for chain linking).
	LatestHash(ctx context.Context, tenantID string) (string, error)
	// List returns events filtered by entity or event type.
	List(ctx context.Context, tenantID, entityType, entityID string, limit int) ([]*AuditEvent, error)
}
