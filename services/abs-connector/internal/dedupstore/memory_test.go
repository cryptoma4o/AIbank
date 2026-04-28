package dedupstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aibank/abs-connector/internal/domain"
)

func TestInMemoryStore_PutGetRoundTrip(t *testing.T) {
	t.Parallel()

	s := NewInMemoryStore()
	ctx := context.Background()

	resp := domain.CanonicalResponse{
		IdempotencyKey: "key-1",
		Success:        true,
		Data:           json.RawMessage(`{"account_number":"40702810000000000000"}`),
		AdapterUsed:    "cft",
		AdapterVersion: "1.4.2",
		CompletedAt:    time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC),
	}

	if err := s.Put(ctx, "bank-alpha", "key-1", resp, 0); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, ok, err := s.Get(ctx, "bank-alpha", "key-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("expected hit, got miss")
	}
	if got.IdempotencyKey != resp.IdempotencyKey ||
		got.AdapterUsed != resp.AdapterUsed ||
		got.AdapterVersion != resp.AdapterVersion ||
		!got.Success {
		t.Fatalf("unexpected response: %+v", got)
	}
	if string(got.Data) != string(resp.Data) {
		t.Fatalf("data mismatch: got %s want %s", got.Data, resp.Data)
	}
}

func TestInMemoryStore_MissReturnsFalse(t *testing.T) {
	t.Parallel()

	s := NewInMemoryStore()
	ctx := context.Background()

	got, ok, err := s.Get(ctx, "bank-alpha", "no-such-key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Fatalf("expected miss, got hit: %+v", got)
	}
}

func TestInMemoryStore_TenantIsolation(t *testing.T) {
	t.Parallel()

	s := NewInMemoryStore()
	ctx := context.Background()

	if err := s.Put(ctx, "alpha", "shared-key", domain.CanonicalResponse{
		IdempotencyKey: "shared-key", AdapterUsed: "cft", Success: true,
	}, 0); err != nil {
		t.Fatalf("Put alpha: %v", err)
	}
	if err := s.Put(ctx, "beta", "shared-key", domain.CanonicalResponse{
		IdempotencyKey: "shared-key", AdapterUsed: "diasoft", Success: true,
	}, 0); err != nil {
		t.Fatalf("Put beta: %v", err)
	}

	a, ok, _ := s.Get(ctx, "alpha", "shared-key")
	if !ok || a.AdapterUsed != "cft" {
		t.Fatalf("alpha: got %+v ok=%v", a, ok)
	}
	b, ok, _ := s.Get(ctx, "beta", "shared-key")
	if !ok || b.AdapterUsed != "diasoft" {
		t.Fatalf("beta: got %+v ok=%v", b, ok)
	}
}

func TestInMemoryStore_TTLExpiry(t *testing.T) {
	t.Parallel()

	s := NewInMemoryStore()
	ctx := context.Background()

	if err := s.Put(ctx, "alpha", "k", domain.CanonicalResponse{
		IdempotencyKey: "k", Success: true,
	}, 10*time.Millisecond); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok, _ := s.Get(ctx, "alpha", "k"); !ok {
		t.Fatal("expected hit before expiry")
	}
	time.Sleep(25 * time.Millisecond)
	if _, ok, _ := s.Get(ctx, "alpha", "k"); ok {
		t.Fatal("expected miss after expiry")
	}
	if got := s.Len(); got != 0 {
		t.Fatalf("expected lazy eviction (len=0), got len=%d", got)
	}
}

func TestInMemoryStore_PutOverwrites(t *testing.T) {
	t.Parallel()

	s := NewInMemoryStore()
	ctx := context.Background()

	_ = s.Put(ctx, "alpha", "k", domain.CanonicalResponse{Success: false, Error: "first"}, 0)
	_ = s.Put(ctx, "alpha", "k", domain.CanonicalResponse{Success: true}, 0)

	got, ok, _ := s.Get(ctx, "alpha", "k")
	if !ok {
		t.Fatal("expected hit after overwrite")
	}
	if !got.Success || got.Error != "" {
		t.Fatalf("expected overwrite to win, got %+v", got)
	}
}
