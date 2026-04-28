package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aibank/api-gateway/internal/router"
)

func TestLimiter_AllowsUnderBudget(t *testing.T) {
	l := New(NewInMemoryStore(), Config{RatePerMinute: 60, Burst: 10})
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		ok, err := l.Allow(ctx, "alpha", "1.1.1.1")
		if err != nil {
			t.Fatalf("Allow err: %v", err)
		}
		if !ok {
			t.Fatalf("expected allow at iter %d", i)
		}
	}
}

func TestLimiter_BlocksOverBudget(t *testing.T) {
	l := New(NewInMemoryStore(), Config{RatePerMinute: 6, Burst: 2})
	l.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	ctx := context.Background()
	allowed := 0
	for i := 0; i < 20; i++ {
		ok, err := l.Allow(ctx, "alpha", "1.1.1.1")
		if err != nil {
			t.Fatalf("Allow err: %v", err)
		}
		if ok {
			allowed++
		}
	}
	if allowed > 8 {
		t.Fatalf("expected at most 8 allowed (Burst+Rate), got %d", allowed)
	}
	if allowed < 1 {
		t.Fatalf("expected at least 1 allowed, got %d", allowed)
	}
}

func TestLimiter_IsolationPerTenant(t *testing.T) {
	l := New(NewInMemoryStore(), Config{RatePerMinute: 6, Burst: 1})
	l.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	ctx := context.Background()
	// исчерпываем тенант A
	for i := 0; i < 20; i++ {
		_, _ = l.Allow(ctx, "alpha", "ip")
	}
	ok, err := l.Allow(ctx, "alpha", "ip")
	if err != nil || ok {
		t.Fatalf("alpha must be exhausted: ok=%v err=%v", ok, err)
	}
	// тенант B должен быть свежим
	ok, err = l.Allow(ctx, "beta", "ip")
	if err != nil {
		t.Fatalf("beta err: %v", err)
	}
	if !ok {
		t.Fatalf("beta must be allowed (separate bucket)")
	}
}

func TestLimiter_IsolationPerIP(t *testing.T) {
	l := New(NewInMemoryStore(), Config{RatePerMinute: 6, Burst: 1})
	l.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		_, _ = l.Allow(ctx, "alpha", "1.1.1.1")
	}
	ok, _ := l.Allow(ctx, "alpha", "1.1.1.1")
	if ok {
		t.Fatalf("first ip must be exhausted")
	}
	ok, _ = l.Allow(ctx, "alpha", "2.2.2.2")
	if !ok {
		t.Fatalf("second ip must be allowed (separate bucket)")
	}
}

func TestLimiter_Refill(t *testing.T) {
	store := NewInMemoryStore()
	l := New(store, Config{RatePerMinute: 60, Burst: 0})
	clock := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return clock }
	ctx := context.Background()

	// исчерпываем сразу
	for i := 0; i < 100; i++ {
		_, _ = l.Allow(ctx, "alpha", "ip")
	}
	ok, _ := l.Allow(ctx, "alpha", "ip")
	if ok {
		t.Fatalf("must be blocked after exhaust")
	}
	// 60 секунд позже — должны восполниться 60 токенов
	clock = clock.Add(60 * time.Second)
	ok, _ = l.Allow(ctx, "alpha", "ip")
	if !ok {
		t.Fatalf("must be allowed after refill")
	}
}

func TestMiddleware_429(t *testing.T) {
	l := New(NewInMemoryStore(), Config{RatePerMinute: 6, Burst: 1})
	// Замораживаем время, чтобы refill не маскировал ограничение.
	l.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	mw := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// 7 токенов начально (Rate+Burst). Делаем 30 запросов — точно
	// исчерпаем за счёт фиксированных часов.
	got429 := false
	for i := 0; i < 30; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req = req.WithContext(router.WithContext(req.Context(), "alpha"))
		req.RemoteAddr = "1.1.1.1:1234"
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("expected at least one 429 response")
	}
}

func TestMiddleware_400IfTenantMissing(t *testing.T) {
	l := New(NewInMemoryStore(), Config{})
	mw := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when tenant absent, got %d", rec.Code)
	}
}
