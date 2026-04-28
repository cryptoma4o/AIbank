package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"aibank/ext-spark/internal/domain"
)

// DefaultTTL — 24 часа на корпоративные сигналы.
const DefaultTTL = 24 * time.Hour

// Cache — интерфейс для тестов.
type Cache interface {
	Get(ctx context.Context, inn string) (*domain.SparkIntel, error)
	Set(ctx context.Context, intel *domain.SparkIntel) error
}

// RedisCache — реализация Cache.
type RedisCache struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewRedisCache подключается к Redis.
func NewRedisCache(addr string) *RedisCache {
	return &RedisCache{
		rdb: redis.NewClient(&redis.Options{Addr: addr}),
		ttl: ttlFromEnv(),
	}
}

// NewRedisCacheFromClient — для тестов.
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

// TTL возвращает текущий TTL.
func (c *RedisCache) TTL() time.Duration { return c.ttl }

// Get читает кэш по ИНН.
func (c *RedisCache) Get(ctx context.Context, inn string) (*domain.SparkIntel, error) {
	val, err := c.rdb.Get(ctx, "spark:intel:"+inn).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var intel domain.SparkIntel
	if err := json.Unmarshal([]byte(val), &intel); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &intel, nil
}

// Set сохраняет данные на TTL.
func (c *RedisCache) Set(ctx context.Context, intel *domain.SparkIntel) error {
	data, err := json.Marshal(intel)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return c.rdb.Set(ctx, "spark:intel:"+intel.INN, data, c.ttl).Err()
}
