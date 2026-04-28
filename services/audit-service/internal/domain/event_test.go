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

// TestSignatureFields_NotInHashChain — добавление signature/signer полей
// после ComputeHash НЕ должно менять hash. Это означает что signature
// можно добавить позднее (через ALTER ROW ... SET signature = ...) и hash
// chain остаётся валидным.
//
// Контракт: ComputeHash смотрит только на immutable-поля; signature вне.
func TestSignatureFields_NotInHashChain(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)
	base := AuditEvent{
		ID:         "evt_sig_001",
		TenantID:   "tnt_demo",
		EntityType: "application",
		EntityID:   "app_1",
		EventType:  "approved",
		ActorID:    "per_admin",
		ActorType:  ActorTypeUser,
		Payload:    json.RawMessage(`{"by":"manual_review"}`),
		CreatedAt:  now,
	}

	hashWithout := base.ComputeHash("prev_hash_zzz")

	// Заполняем signature-поля и пересчитываем — должен совпасть.
	signed := base
	signed.Signature = []byte{1, 2, 3, 4}
	signed.SignatureAlgorithm = "ed25519"
	signed.SignerKeyID = "audit-service-key-001"
	hashWith := signed.ComputeHash("prev_hash_zzz")

	if hashWithout != hashWith {
		t.Errorf("signature fields влияют на hash chain — это ломает обратную совместимость:\n  without: %s\n  with:    %s", hashWithout, hashWith)
	}
}

func TestHasSignature_AllOrNothing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		signature []byte
		algorithm string
		keyID     string
		want      bool
	}{
		{"all empty", nil, "", "", false},
		{"only sig", []byte{1, 2}, "", "", false},
		{"only alg", nil, "ed25519", "", false},
		{"only keyid", nil, "", "k1", false},
		{"sig + alg, no keyid", []byte{1}, "ed25519", "", false},
		{"all set", []byte{1, 2}, "ed25519", "key-001", true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := AuditEvent{
				Signature:          c.signature,
				SignatureAlgorithm: c.algorithm,
				SignerKeyID:        c.keyID,
			}
			if got := e.HasSignature(); got != c.want {
				t.Errorf("HasSignature() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSignedDigest_DecodesHashHex(t *testing.T) {
	t.Parallel()

	e := AuditEvent{Hash: "deadbeef"}
	digest, err := e.SignedDigest()
	if err != nil {
		t.Fatalf("SignedDigest: %v", err)
	}
	if len(digest) != 4 {
		t.Errorf("expected 4 bytes from 'deadbeef', got %d", len(digest))
	}
	if digest[0] != 0xde || digest[3] != 0xef {
		t.Errorf("unexpected bytes: %v", digest)
	}

	// Невалидный hex → ошибка.
	bad := AuditEvent{Hash: "zzz"}
	if _, err := bad.SignedDigest(); err == nil {
		t.Errorf("expected error on non-hex hash")
	}
}
