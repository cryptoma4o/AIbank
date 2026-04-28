// Package audit is a tiny client SDK around the platform's audit-service
// HTTP API. It is intentionally dependency-free (stdlib net/http only) so
// every Go service can pull it in without dragging extra deps.
//
// The shapes here mirror services/audit-service/internal/handler and
// internal/domain. If those change, this package must change in lockstep
// (contract is captured in packages/openapi/audit-service.yaml).
package audit

import (
	"encoding/json"
	"errors"
	"time"
)

// ActorType — who triggered the audited event. Mirrors
// services/audit-service/internal/domain.ActorType.
type ActorType string

const (
	ActorTypeUser    ActorType = "user"
	ActorTypeSystem  ActorType = "system"
	ActorTypeAIAgent ActorType = "ai_agent"
)

// IsValid reports whether v is a known actor type.
func (a ActorType) IsValid() bool {
	switch a {
	case ActorTypeUser, ActorTypeSystem, ActorTypeAIAgent:
		return true
	}
	return false
}

// RecordEventRequest is the JSON payload accepted by POST /v1/events.
//
// Payload may be any JSON-encodable value; it is forwarded to the audit
// service as raw JSON. Hash-chain bookkeeping (previous_hash + hash) is
// computed server-side, so callers do not — and must not — populate it.
type RecordEventRequest struct {
	TenantID   string          `json:"tenant_id"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	EventType  string          `json:"event_type"`
	ActorID    string          `json:"actor_id"`
	ActorType  ActorType       `json:"actor_type"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

// Validate returns an error if any required field is missing.
func (r RecordEventRequest) Validate() error {
	switch {
	case r.TenantID == "":
		return errors.New("audit: tenant_id is required")
	case r.EntityType == "":
		return errors.New("audit: entity_type is required")
	case r.EntityID == "":
		return errors.New("audit: entity_id is required")
	case r.EventType == "":
		return errors.New("audit: event_type is required")
	case r.ActorID == "":
		return errors.New("audit: actor_id is required")
	case !r.ActorType.IsValid():
		return errors.New("audit: actor_type must be user|system|ai_agent")
	}
	return nil
}

// AuditEvent is the response shape returned from POST /v1/events and
// items in GET /v1/events. Mirrors domain.AuditEvent.
type AuditEvent struct {
	ID           string          `json:"id"`
	TenantID     string          `json:"tenant_id"`
	EntityType   string          `json:"entity_type"`
	EntityID     string          `json:"entity_id"`
	EventType    string          `json:"event_type"`
	ActorID      string          `json:"actor_id"`
	ActorType    ActorType       `json:"actor_type"`
	Payload      json.RawMessage `json:"payload"`
	PreviousHash string          `json:"previous_hash"`
	Hash         string          `json:"hash"`
	CreatedAt    time.Time       `json:"created_at"`
}

// QueryOptions are the URL-query filters accepted by GET /v1/events.
//
// TenantID is required by the server. Limit must be 1..1000 (server
// enforces; default 100). EntityType/EntityID are optional narrowing
// filters.
type QueryOptions struct {
	TenantID   string
	EntityType string
	EntityID   string
	Limit      int
}

// listResponse mirrors the {items, count} envelope returned by ListEvents.
type listResponse struct {
	Items []*AuditEvent `json:"items"`
	Count int           `json:"count"`
}

// errorEnvelope is what the audit-service writeError helper produces:
//
//	{"error": {"code": "...", "message": "..."}}
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// APIError is returned for non-2xx responses. It exposes the HTTP status
// and the structured error body when present.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

// Error implements error.
func (e *APIError) Error() string {
	if e.Code != "" || e.Message != "" {
		return "audit: " + e.Code + ": " + e.Message
	}
	return "audit: unexpected status"
}

// IsRetryable returns true for 5xx and 429 — see Client.Append for details.
func (e *APIError) IsRetryable() bool {
	return e.StatusCode == 429 || (e.StatusCode >= 500 && e.StatusCode < 600)
}
