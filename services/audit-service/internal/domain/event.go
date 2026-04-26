package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type ActorType string

const (
	ActorTypeUser    ActorType = "user"
	ActorTypeSystem  ActorType = "system"
	ActorTypeAIAgent ActorType = "ai_agent"
)

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

// ComputeHash computes SHA-256 over the immutable fields of the event.
// previousHash links events into a tamper-evident chain.
func (e *AuditEvent) ComputeHash(previousHash string) string {
	e.PreviousHash = previousHash
	data, _ := json.Marshal(struct {
		ID           string          `json:"id"`
		TenantID     string          `json:"tenant_id"`
		EntityType   string          `json:"entity_type"`
		EntityID     string          `json:"entity_id"`
		EventType    string          `json:"event_type"`
		ActorID      string          `json:"actor_id"`
		ActorType    ActorType       `json:"actor_type"`
		Payload      json.RawMessage `json:"payload"`
		PreviousHash string          `json:"previous_hash"`
		CreatedAt    time.Time       `json:"created_at"`
	}{
		ID: e.ID, TenantID: e.TenantID, EntityType: e.EntityType,
		EntityID: e.EntityID, EventType: e.EventType, ActorID: e.ActorID,
		ActorType: e.ActorType, Payload: e.Payload,
		PreviousHash: previousHash, CreatedAt: e.CreatedAt,
	})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
