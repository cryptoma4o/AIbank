package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestComputeHash_DeterministicAndChained(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	base := AuditEvent{
		ID:         "evt_001",
		TenantID:   "tnt_demo",
		EntityType: "application",
		EntityID:   "app_42",
		EventType:  "submitted",
		ActorID:    "per_99",
		ActorType:  ActorTypeUser,
		Payload:    json.RawMessage(`{"channel":"web"}`),
		CreatedAt:  now,
	}

	// Тот же контент → тот же hash.
	a := base
	b := base
	hashA := a.ComputeHash("")
	hashB := b.ComputeHash("")
	if hashA != hashB {
		t.Fatalf("hash for identical events differs: %s vs %s", hashA, hashB)
	}
	if len(hashA) != 64 {
		t.Fatalf("expected 64-char hex SHA-256, got %d chars", len(hashA))
	}

	// Разный previousHash → разный hash (доказательство связи в цепочку).
	hashWithPrev := base.ComputeHash("deadbeef")
	if hashWithPrev == hashA {
		t.Fatalf("hash unchanged when previous_hash changes — chain is broken")
	}

	// Изменение payload → разный hash (tamper detection).
	tampered := base
	tampered.Payload = json.RawMessage(`{"channel":"branch"}`)
	if tampered.ComputeHash("") == hashA {
		t.Fatalf("hash unchanged when payload changes — tampering not detected")
	}
}
