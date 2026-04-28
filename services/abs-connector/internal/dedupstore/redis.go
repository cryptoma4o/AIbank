// Package dedupstore — реализации domain.IdempotencyStore.
//
// Redis-вариант — production-default. Для unit-тестов используется
// InMemoryStore из memory.go: подключаться к настоящему Redis'у в тестах
// мы намеренно не хотим (см. CLAUDE.md, principle: tests must be hermetic).
package dedupstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"aibank/abs-connector/internal/domain"
)

// DefaultTTL — по умолчанию ABS-ответ кешируется на 24 часа.
// Соответствует ADR-0006 / technical-structure.md § 6:
// «idempotency_key TTL = 24h». Caller может задать другое значение
// явным аргументом Put().
const DefaultTTL = 24 * time.Hour

// RedisStore — реализация IdempotencyStore поверх go-redis.
// Все ключи имеют префикс `abs:idem:{tenant_id}:{idempotency_key}` —
// tenant_id входит в ключ намеренно: разные тенанты могут переиспользовать
// один и тот же idempotency_key (в каждом своём UUID-пространстве),
// и кросс-тенантного смешения быть не должно.
type RedisStore struct {
	rdb        *redis.Client
	defaultTTL time.Duration
}

// NewRedisStore — конструктор. Если defaultTTL == 0, используется DefaultTTL.
func NewRedisStore(rdb *redis.Client, defaultTTL time.Duration) *RedisStore {
	if defaultTTL <= 0 {
		defaultTTL = DefaultTTL
	}
	return &RedisStore{rdb: rdb, defaultTTL: defaultTTL}
}

func redisKey(tenantID, idempotencyKey string) string {
	return fmt.Sprintf("abs:idem:%s:%s", tenantID, idempotencyKey)
}

// Get — получить кешированный ответ.
// Контракт: при отсутствии ключа возвращаем (zero, false, nil); ошибка означает
// проблему транспорта (Redis недоступен / поломался JSON).
func (s *RedisStore) Get(ctx context.Context, tenantID, idempotencyKey string) (domain.CanonicalResponse, bool, error) {
	key := redisKey(tenantID, idempotencyKey)
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return domain.CanonicalResponse{}, false, nil
	}
	if err != nil {
		return domain.CanonicalResponse{}, false, fmt.Errorf("redis get %s: %w", key, err)
	}
	var resp domain.CanonicalResponse
	if uerr := json.Unmarshal(raw, &resp); uerr != nil {
		// Битая запись — лечим её как miss: возвращаем (zero, false, error)
		// чтобы caller'у было видно, что данные кеша неконсистентны.
		// Хендлер примет решение: пройти мимо кеша и обратиться к адаптеру.
		return domain.CanonicalResponse{}, false, fmt.Errorf("decode cached response %s: %w", key, uerr)
	}
	return resp, true, nil
}

// Put — сохранить ответ. ttl <= 0 → используем defaultTTL.
func (s *RedisStore) Put(ctx context.Context, tenantID, idempotencyKey string, response domain.CanonicalResponse, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	key := redisKey(tenantID, idempotencyKey)
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	if err := s.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %s: %w", key, err)
	}
	return nil
}
