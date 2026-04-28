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

	// Криптографическая подпись (опционально, добавляется ПОСЛЕ ComputeHash).
	// Подпись считается над hex-decoded value поля Hash. Это обеспечивает:
	//   * non-repudiation поверх существующего hash chain;
	//   * backwards compat — existing rows и legacy producers могут оставлять
	//     эти поля пустыми, hash chain продолжает работать.
	// Алгоритмы: "ed25519" (Pre-MVP), "gost-2012-256", "gost-2012-512".
	Signature           []byte `json:"signature,omitempty"`
	SignatureAlgorithm  string `json:"signature_algorithm,omitempty"`
	SignerKeyID         string `json:"signer_key_id,omitempty"`
}

// ComputeHash computes SHA-256 over the immutable fields of the event.
// previousHash links events into a tamper-evident chain.
//
// IMPORTANT: signature/signer fields исключены из payload хеширования.
// Они добавляются ПОСЛЕ ComputeHash и не влияют на hash chain — это
// аналог TSA-таймстампа в RFC 3161 (signature применяется к финальному
// digest'у, а не входит в него).
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

// HasSignature возвращает true, если событие подписано (все три поля заполнены).
// Используется в verifier для решения — проверять подпись или нет.
func (e *AuditEvent) HasSignature() bool {
	return len(e.Signature) > 0 && e.SignatureAlgorithm != "" && e.SignerKeyID != ""
}

// SignedDigest возвращает hex-decoded bytes поля Hash — это то значение,
// над которым делается криптоподпись. Используется и при подписании
// (внешним signer'ом, например packages/signature.SignatureProvider), и
// при верификации.
func (e *AuditEvent) SignedDigest() ([]byte, error) {
	return hex.DecodeString(e.Hash)
}
