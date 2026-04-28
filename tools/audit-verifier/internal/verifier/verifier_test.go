package verifier

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"sort"
	"testing"
	"time"
)

// memStore — тестовый in-memory EventStore. Игнорирует from/to для
// простоты; цепочечная семантика — единственное, что мы тестируем.
type memStore struct{ events []Event }

func (m *memStore) ListEvents(ctx context.Context, tenantID string, from, to *time.Time) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		// Эмулируем БД-инвариант: события в порядке возрастания CreatedAt.
		ordered := make([]Event, 0, len(m.events))
		for _, e := range m.events {
			if e.TenantID != tenantID {
				continue
			}
			if from != nil && e.CreatedAt.Before(*from) {
				continue
			}
			if to != nil && e.CreatedAt.After(*to) {
				continue
			}
			ordered = append(ordered, e)
		}
		sort.Slice(ordered, func(i, j int) bool {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		})
		for _, e := range ordered {
			if !yield(e, nil) {
				return
			}
		}
	}
}

// buildChain создаёт N валидных событий, связанных hash-цепочкой,
// эмулируя то, что записал бы audit-service handler.RecordEvent.
func buildChain(t *testing.T, tenantID string, n int) []Event {
	t.Helper()
	base := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	events := make([]Event, 0, n)
	prev := ""
	for i := 0; i < n; i++ {
		ev := Event{
			ID:         "evt_" + string(rune('a'+i)),
			TenantID:   tenantID,
			EntityType: "application",
			EntityID:   "app_" + string(rune('1'+i)),
			EventType:  "submitted",
			ActorID:    "per_42",
			ActorType:  ActorTypeUser,
			Payload:    json.RawMessage(`{"i":` + string(rune('0'+i)) + `}`),
			CreatedAt:  base.Add(time.Duration(i) * time.Minute),
		}
		ev.PreviousHash = prev
		ev.Hash = ev.ComputeHash(prev)
		prev = ev.Hash
		events = append(events, ev)
	}
	return events
}

func TestVerifyChain_HappyPath(t *testing.T) {
	t.Parallel()
	store := &memStore{events: buildChain(t, "bank-alpha", 5)}
	v := NewVerifier(store)

	res, err := v.VerifyChain(context.Background(), "bank-alpha", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Valid {
		t.Fatalf("expected valid chain, got mismatch: %+v", res)
	}
	if res.EventCount != 5 {
		t.Fatalf("expected 5 events, got %d", res.EventCount)
	}
	if res.LastHash == "" {
		t.Fatalf("expected non-empty last hash")
	}
}

func TestVerifyChain_TamperedPayload(t *testing.T) {
	t.Parallel()
	chain := buildChain(t, "bank-alpha", 5)
	// Подделка payload в середине — Hash остаётся прежним, но
	// ComputeHash(prev) теперь даёт другое значение.
	chain[2].Payload = json.RawMessage(`{"channel":"branch-malicious"}`)
	store := &memStore{events: chain}

	res, err := NewVerifier(store).VerifyChain(context.Background(), "bank-alpha", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Valid {
		t.Fatalf("expected invalid chain after tamper")
	}
	if res.FirstMismatch == nil || res.FirstMismatch.ID != chain[2].ID {
		t.Fatalf("expected first mismatch at index 2 (id=%s), got %+v",
			chain[2].ID, res.FirstMismatch)
	}
	if res.MismatchReason != "hash_mismatch" {
		t.Fatalf("expected reason=hash_mismatch, got %q", res.MismatchReason)
	}
}

func TestVerifyChain_TruncatedChain(t *testing.T) {
	t.Parallel()
	chain := buildChain(t, "bank-alpha", 5)
	// Удаляем элемент с индексом 2: следующий за ним продолжает
	// ссылаться на старый prev_hash, который больше не существует
	// в потоке → prev_hash mismatch.
	truncated := append([]Event{}, chain[:2]...)
	truncated = append(truncated, chain[3:]...)
	store := &memStore{events: truncated}

	res, err := NewVerifier(store).VerifyChain(context.Background(), "bank-alpha", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Valid {
		t.Fatalf("expected invalid chain after truncation")
	}
	if res.MismatchReason != "prev_hash_mismatch" {
		t.Fatalf("expected reason=prev_hash_mismatch, got %q", res.MismatchReason)
	}
	if res.FirstMismatch == nil || res.FirstMismatch.ID != chain[3].ID {
		t.Fatalf("expected first mismatch to be the event after the gap (id=%s), got %+v",
			chain[3].ID, res.FirstMismatch)
	}
}

func TestVerifyChain_EmptyChain(t *testing.T) {
	t.Parallel()
	store := &memStore{events: nil}
	res, err := NewVerifier(store).VerifyChain(context.Background(), "bank-alpha", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Valid {
		t.Fatalf("empty chain must be vacuously valid")
	}
	if res.EventCount != 0 {
		t.Fatalf("expected 0 events, got %d", res.EventCount)
	}
}

// TestVerifySignatures_AllValid — все события подписаны и signature-callback
// возвращает nil → Valid=true.
func TestVerifySignatures_AllValid(t *testing.T) {
	t.Parallel()
	chain := buildChain(t, "bank-alpha", 3)
	for i := range chain {
		chain[i].Signature = []byte{byte(i + 1)}
		chain[i].SignatureAlgorithm = "ed25519"
		chain[i].SignerKeyID = "audit-key-001"
	}
	v := NewVerifier(&memStore{events: chain})

	res, err := v.VerifySignatures(context.Background(), "bank-alpha",
		func(_ context.Context, _ *Event) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Valid {
		t.Errorf("expected Valid=true, got %+v", res)
	}
	if res.EventCount != 3 || res.SignedCount != 3 || res.UnsignedCount != 0 {
		t.Errorf("counts = (event=%d signed=%d unsigned=%d), want (3,3,0)",
			res.EventCount, res.SignedCount, res.UnsignedCount)
	}
}

// TestVerifySignatures_MixedSignedUnsigned — backwards compat: события без
// подписи пропускаются и не считаются невалидными. Это позволяет внедрять
// signing инкрементально.
func TestVerifySignatures_MixedSignedUnsigned(t *testing.T) {
	t.Parallel()
	chain := buildChain(t, "bank-alpha", 4)
	// Подписаны только 2-й и 4-й события (индекс 1, 3).
	for _, i := range []int{1, 3} {
		chain[i].Signature = []byte{0x01}
		chain[i].SignatureAlgorithm = "ed25519"
		chain[i].SignerKeyID = "audit-key-001"
	}
	v := NewVerifier(&memStore{events: chain})

	res, _ := v.VerifySignatures(context.Background(), "bank-alpha",
		func(_ context.Context, _ *Event) error { return nil })
	if !res.Valid {
		t.Errorf("expected Valid=true (unsigned events не валятся)")
	}
	if res.SignedCount != 2 {
		t.Errorf("SignedCount = %d, want 2", res.SignedCount)
	}
	if res.UnsignedCount != 2 {
		t.Errorf("UnsignedCount = %d, want 2", res.UnsignedCount)
	}
}

// TestVerifySignatures_FirstInvalidStopsScan — verifyFn вернул ошибку →
// результат содержит FirstInvalid и обработка прекращается.
func TestVerifySignatures_FirstInvalidStopsScan(t *testing.T) {
	t.Parallel()
	chain := buildChain(t, "bank-alpha", 3)
	for i := range chain {
		chain[i].Signature = []byte{byte(i)}
		chain[i].SignatureAlgorithm = "ed25519"
		chain[i].SignerKeyID = "audit-key-001"
	}
	v := NewVerifier(&memStore{events: chain})

	// verifyFn падает на втором событии.
	calls := 0
	verifyFn := func(_ context.Context, ev *Event) error {
		calls++
		if ev.ID == chain[1].ID {
			return fmt.Errorf("simulated invalid signature for %s", ev.ID)
		}
		return nil
	}

	res, _ := v.VerifySignatures(context.Background(), "bank-alpha", verifyFn)
	if res.Valid {
		t.Errorf("expected Valid=false after first invalid")
	}
	if res.FirstInvalid == nil || res.FirstInvalid.ID != chain[1].ID {
		t.Errorf("FirstInvalid mismatch: %+v", res.FirstInvalid)
	}
	if res.InvalidReason == "" {
		t.Errorf("InvalidReason empty")
	}
	if calls != 2 {
		t.Errorf("verifyFn called %d times, want 2 (early exit)", calls)
	}
}

// TestVerifySignatures_NoSignedEvents — все события без подписи →
// SignedCount=0, Valid=true. Это нормальный сценарий до внедрения signing.
func TestVerifySignatures_NoSignedEvents(t *testing.T) {
	t.Parallel()
	chain := buildChain(t, "bank-alpha", 3)
	v := NewVerifier(&memStore{events: chain})

	res, _ := v.VerifySignatures(context.Background(), "bank-alpha",
		func(_ context.Context, _ *Event) error {
			t.Fatal("verifyFn should not be called for unsigned events")
			return nil
		})
	if !res.Valid {
		t.Errorf("expected Valid=true for unsigned chain")
	}
	if res.SignedCount != 0 || res.UnsignedCount != 3 {
		t.Errorf("counts wrong: signed=%d unsigned=%d", res.SignedCount, res.UnsignedCount)
	}
}

func TestVerifySignatures_RequiresVerifyFn(t *testing.T) {
	t.Parallel()
	v := NewVerifier(&memStore{})
	_, err := v.VerifySignatures(context.Background(), "bank-alpha", nil)
	if err == nil {
		t.Errorf("expected error for nil verifyFn")
	}
}

func TestValidateTenantID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", "bank-alpha", false},
		{"valid_with_underscore", "tnt_demo", false},
		{"empty", "", true},
		{"uppercase", "Bank-Alpha", true},
		{"sql_injection", "alfa'; DROP TABLE--", true},
		{"too_long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"starts_with_digit", "1bank", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTenantID(tc.id)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateTenantID(%q) err=%v, wantErr=%v", tc.id, err, tc.wantErr)
			}
		})
	}
}
