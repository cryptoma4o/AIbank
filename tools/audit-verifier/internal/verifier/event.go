package verifier

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// ActorType зеркалит services/audit-service/internal/domain/event.go
// (audit-service ActorType). Любое расхождение в наборе значений → mismatch
// hash → ложный chain-broken-алерт.
type ActorType string

const (
	ActorTypeUser    ActorType = "user"
	ActorTypeSystem  ActorType = "system"
	ActorTypeAIAgent ActorType = "ai_agent"
)

// Event — read-only копия audit-service domain.AuditEvent.
//
// Поля (имена + JSON-теги) ОБЯЗАНЫ совпадать с
// services/audit-service/internal/domain/event.go.AuditEvent. ComputeHash
// использует эти теги при сериализации, поэтому даже невинное
// переименование "tenant_id" → "tenantId" сломает воспроизведение хэша
// и приведёт к ложному chain-broken-алерту.
//
// Этот tool принципиально read-only и не импортирует пакет audit-service:
// (1) тот пакет — internal, его публичный импорт за пределы services/
//     невозможен по правилам Go;
// (2) tool должен быть способен пересчитать хэш по любым событиям, даже
//     если бинарник audit-service временно недоступен или его версия
//     отличается от tool'а (incident-response runbook требует независимости).
//
// Контракт совместимости: при любом изменении audit-service'овой
// AuditEvent / ComputeHash — обновлять этот файл и verifier_test.go.
// CI-проверка drift выполняется тестом event_compat_test.go.
type Event struct {
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

// ComputeHash повторяет логику audit-service domain.AuditEvent.ComputeHash
// БУКВАЛЬНО — те же поля, тот же порядок JSON-сериализации, тот же
// SHA-256 → hex. Любая модификация ломает совместимость; читать вместе
// с docs/runbooks/audit-log-integrity.md.
func (e *Event) ComputeHash(previousHash string) string {
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
