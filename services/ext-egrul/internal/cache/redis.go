package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"aibank/ext-egrul/internal/domain"
)

// DefaultTTL по техническому документу § 4.3 — 24 часа.
const DefaultTTL = 24 * time.Hour

// Cache — интерфейс для подмены в тестах.
type Cache interface {
	GetByINN(ctx context.Context, inn string) (*domain.LegalEntity, error)
	SetByINN(ctx context.Context, rec *domain.LegalEntity) error
	GetByOGRN(ctx context.Context, ogrn string) (*domain.LegalEntity, error)
	SetByOGRN(ctx context.Context, rec *domain.LegalEntity) error
	GetFounders(ctx context.Context, inn string) ([]domain.Founder, error)
	SetFounders(ctx context.Context, inn string, founders []domain.Founder) error
}

// RedisCache — реализация Cache на go-redis.
type RedisCache struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewRedisCache подключается к Redis по адресу `addr` с TTL по умолчанию.
func NewRedisCache(addr string) *RedisCache {
	return NewRedisCacheWithTTL(addr, ttlFromEnv())
}

// NewRedisCacheWithTTL — то же, но с кастомным TTL (для тестов).
func NewRedisCacheWithTTL(addr string, ttl time.Duration) *RedisCache {
	return &RedisCache{
		rdb: redis.NewClient(&redis.Options{Addr: addr}),
		ttl: ttl,
	}
}

// NewRedisCacheFromClient оборачивает существующий redis.Client (для тестов через miniredis).
func NewRedisCacheFromClient(rdb *redis.Client, ttl time.Duration) *RedisCache {
	return &RedisCache{rdb: rdb, ttl: ttl}
}

func ttlFromEnv() time.Duration {
	if v := os.Getenv("CACHE_TTL_HOURS"); v != "" {
		if hours, err := strconv.Atoi(v); err == nil && hours > 0 {
			return time.Duration(hours) * time.Hour
		}
	}
	return DefaultTTL
}

// TTL возвращает текущее значение TTL — полезно для тестов.
func (c *RedisCache) TTL() time.Duration { return c.ttl }

// GetByINN читает запись по ИНН. Возвращает (nil, nil) при cache miss.
func (c *RedisCache) GetByINN(ctx context.Context, inn string) (*domain.LegalEntity, error) {
	return c.getEntity(ctx, "egrul:inn:"+inn)
}

// SetByINN кладёт запись в кэш под двумя ключами (по ИНН и по ОГРН), если ОГРН задан.
func (c *RedisCache) SetByINN(ctx context.Context, rec *domain.LegalEntity) error {
	if err := c.setEntity(ctx, "egrul:inn:"+rec.INN, rec); err != nil {
		return err
	}
	if rec.OGRN != "" {
		_ = c.setEntity(ctx, "egrul:ogrn:"+rec.OGRN, rec)
	}
	return nil
}

// GetByOGRN читает запись по ОГРН.
func (c *RedisCache) GetByOGRN(ctx context.Context, ogrn string) (*domain.LegalEntity, error) {
	return c.getEntity(ctx, "egrul:ogrn:"+ogrn)
}

// SetByOGRN — сохраняет под ОГРН-ключом + дублирует под ИНН для cross-lookup.
func (c *RedisCache) SetByOGRN(ctx context.Context, rec *domain.LegalEntity) error {
	if err := c.setEntity(ctx, "egrul:ogrn:"+rec.OGRN, rec); err != nil {
		return err
	}
	if rec.INN != "" {
		_ = c.setEntity(ctx, "egrul:inn:"+rec.INN, rec)
	}
	return nil
}

// GetFounders читает учредителей по ИНН.
func (c *RedisCache) GetFounders(ctx context.Context, inn string) ([]domain.Founder, error) {
	val, err := c.rdb.Get(ctx, "egrul:founders:"+inn).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get founders: %w", err)
	}
	var fs []domain.Founder
	if err := json.Unmarshal([]byte(val), &fs); err != nil {
		return nil, fmt.Errorf("unmarshal founders: %w", err)
	}
	return fs, nil
}

// SetFounders сохраняет учредителей.
func (c *RedisCache) SetFounders(ctx context.Context, inn string, founders []domain.Founder) error {
	data, err := json.Marshal(founders)
	if err != nil {
		return fmt.Errorf("marshal founders: %w", err)
	}
	return c.rdb.Set(ctx, "egrul:founders:"+inn, data, c.ttl).Err()
}

// --- internals ---

func (c *RedisCache) getEntity(ctx context.Context, key string) (*domain.LegalEntity, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get %s: %w", key, err)
	}
	var rec domain.LegalEntity
	if err := json.Unmarshal([]byte(val), &rec); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", key, err)
	}
	return &rec, nil
}

func (c *RedisCache) setEntity(ctx context.Context, key string, rec *domain.LegalEntity) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", key, err)
	}
	return c.rdb.Set(ctx, key, data, c.ttl).Err()
}
