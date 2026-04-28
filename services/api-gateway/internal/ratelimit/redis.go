// Package ratelimit — per-tenant token-bucket rate limiter.
//
// Использует Redis в качестве distributed-стора, чтобы лимит работал
// корректно за несколькими репликами api-gateway.
//
// Ключи: `ratelimit:{tenant_id}:{ip}`.  Значение — два поля (tokens, ts)
// в hash; обновление неатомарно через GET/SET (race-condition допустима
// в пределах ±1 токена; критичные сценарии — TODO Lua-скрипт).
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"aibank/api-gateway/internal/router"

	"github.com/redis/go-redis/v9"
)

// Config — параметры лимитера.
type Config struct {
	// RatePerMinute — устойчивая скорость, токены/минуту (default 60).
	RatePerMinute int
	// Burst — мгновенный запас сверх устойчивой скорости (default 10).
	Burst int
	// KeyTTL — TTL ключа в Redis после последнего обращения.
	KeyTTL time.Duration
}

func (c Config) withDefaults() Config {
	if c.RatePerMinute <= 0 {
		c.RatePerMinute = 60
	}
	if c.Burst <= 0 {
		c.Burst = 10
	}
	if c.KeyTTL <= 0 {
		c.KeyTTL = 10 * time.Minute
	}
	return c
}

// Store — узкий интерфейс над Redis.  Тесты подменяют InMemoryStore.
type Store interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

// ErrStoreMiss — ключ не найден.  Локальный аналог redis.Nil для интерфейса.
var ErrStoreMiss = errors.New("ratelimit: key miss")

// RedisStore — реальный backend.
type RedisStore struct{ Client *redis.Client }

func (s *RedisStore) Get(ctx context.Context, key string) (string, error) {
	v, err := s.Client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", ErrStoreMiss
	}
	return v, err
}
func (s *RedisStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.Client.Set(ctx, key, value, ttl).Err()
}

// InMemoryStore — для тестов и для запуска в dev без Redis.
type InMemoryStore struct {
	mu   sync.Mutex
	data map[string]string
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{data: make(map[string]string)}
}
func (s *InMemoryStore) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	if !ok {
		return "", ErrStoreMiss
	}
	return v, nil
}
func (s *InMemoryStore) Set(_ context.Context, key, value string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

// Limiter — основная сущность пакета.
type Limiter struct {
	store Store
	cfg   Config
	now   func() time.Time
}

// New — конструктор.
func New(store Store, cfg Config) *Limiter {
	return &Limiter{store: store, cfg: cfg.withDefaults(), now: time.Now}
}

// Allow проверяет, разрешён ли запрос для (tenantID, ip).
//
// Алгоритм token-bucket:
//   - bucket: max=Burst+RatePerMinute, refill=RatePerMinute/60 токенов/сек
//   - первая попытка → bucket полный, allow=true
//   - каждый запрос отнимает 1 токен; если <1 — 429
//
// Гонки между репликами игнорируются: при равном distributed lock'е лимит
// был бы детерминирован, но требовал бы Lua-скрипт.  Для MVP допускаем
// небольшую погрешность в пиках; см. TODO в шапке пакета.
func (l *Limiter) Allow(ctx context.Context, tenantID, ip string) (bool, error) {
	if tenantID == "" {
		return false, fmt.Errorf("ratelimit: empty tenant id")
	}
	key := fmt.Sprintf("ratelimit:%s:%s", tenantID, ip)
	now := l.now()
	maxTokens := float64(l.cfg.Burst + l.cfg.RatePerMinute)
	refillPerSec := float64(l.cfg.RatePerMinute) / 60.0

	tokens := maxTokens

	raw, err := l.store.Get(ctx, key)
	switch {
	case errors.Is(err, ErrStoreMiss):
		// первый запрос — bucket полный
	case err != nil:
		return false, fmt.Errorf("ratelimit: get: %w", err)
	default:
		if t, ts, ok := parseBucket(raw); ok {
			elapsed := now.Sub(ts).Seconds()
			tokens = t + elapsed*refillPerSec
			if tokens > maxTokens {
				tokens = maxTokens
			}
		}
	}
	if tokens < 1 {
		_ = l.persist(ctx, key, tokens, now)
		return false, nil
	}
	tokens--
	if err := l.persist(ctx, key, tokens, now); err != nil {
		return true, fmt.Errorf("ratelimit: set: %w", err)
	}
	return true, nil
}

func (l *Limiter) persist(ctx context.Context, key string, tokens float64, ts time.Time) error {
	v := fmt.Sprintf("%.4f|%d", tokens, ts.UnixNano())
	return l.store.Set(ctx, key, v, l.cfg.KeyTTL)
}

func parseBucket(s string) (float64, time.Time, bool) {
	parts := strings.SplitN(s, "|", 2)
	if len(parts) != 2 {
		return 0, time.Time{}, false
	}
	t, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, time.Time{}, false
	}
	ns, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, time.Time{}, false
	}
	return t, time.Unix(0, ns), true
}

// Middleware — chi-совместимый middleware; ожидает router.Middleware
// выше по цепочке (для tenant_id из контекста).
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := router.FromContext(r.Context())
		if tenantID == "" {
			tenantID = r.Header.Get(router.HeaderTenantID)
		}
		if tenantID == "" {
			writeJSONError(w, http.StatusBadRequest, "tenant_unresolved",
				"tenant_id required for rate limiting")
			return
		}
		ip := clientIP(r)
		ok, err := l.Allow(r.Context(), tenantID, ip)
		if err != nil {
			// fail-open в случае проблем со стором — лучше не блокировать,
			// чем отказать всем при сбое Redis.
			next.ServeHTTP(w, r)
			return
		}
		if !ok {
			w.Header().Set("Retry-After", "60")
			writeJSONError(w, http.StatusTooManyRequests, "rate_limited",
				fmt.Sprintf("rate limit exceeded for tenant %s", tenantID))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.Index(v, ","); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if i := strings.LastIndex(r.RemoteAddr, ":"); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := `{"error":{"code":"` + code + `","message":"` + msg + `"}}`
	_, _ = w.Write([]byte(body))
}
