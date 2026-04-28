package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"aibank/ext-fssp/internal/domain"
)

// DefaultTTL — 24 часа на исполнительные производства (изменения нечастые).
const DefaultTTL = 24 * time.Hour

// Cache — интерфейс для тестов.
type Cache interface {
	GetByINN(ctx context.Context, inn string) (*domain.ProceedingsResult, error)
	SetByINN(ctx context.Context, inn string, res *domain.ProceedingsResult) error
	GetByPerson(ctx context.Context, fullName, birthDate string) (*domain.ProceedingsResult, error)
	SetByPerson(ctx context.Context, fullName, birthDate string, res *domain.ProceedingsResult) error
}

// RedisCache — реализация Cache на go-redis.
type RedisCache struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewRedisCache подключается к Redis по адресу `addr`.
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

// GetByINN читает результат по ИНН.
func (c *RedisCache) GetByINN(ctx context.Context, inn string) (*domain.ProceedingsResult, error) {
	return c.getKey(ctx, "fssp:inn:"+inn)
}

// SetByINN сохраняет результат.
func (c *RedisCache) SetByINN(ctx context.Context, inn string, res *domain.ProceedingsResult) error {
	return c.setKey(ctx, "fssp:inn:"+inn, res)
}

// GetByPerson читает результат по ФЛ.
func (c *RedisCache) GetByPerson(ctx context.Context, fullName, birthDate string) (*domain.ProceedingsResult, error) {
	return c.getKey(ctx, personKey(fullName, birthDate))
}

// SetByPerson сохраняет результат по ФЛ.
func (c *RedisCache) SetByPerson(ctx context.Context, fullName, birthDate string, res *domain.ProceedingsResult) error {
	return c.setKey(ctx, personKey(fullName, birthDate), res)
}

// --- internals ---

func personKey(fullName, birthDate string) string {
	payload := strings.ToLower(strings.TrimSpace(fullName)) + "|" + birthDate
	sum := sha256.Sum256([]byte(payload))
	return "fssp:person:" + hex.EncodeToString(sum[:16])
}

func (c *RedisCache) getKey(ctx context.Context, key string) (*domain.ProceedingsResult, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get %s: %w", key, err)
	}
	var res domain.ProceedingsResult
	if err := json.Unmarshal([]byte(val), &res); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", key, err)
	}
	return &res, nil
}

func (c *RedisCache) setKey(ctx context.Context, key string, res *domain.ProceedingsResult) error {
	data, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", key, err)
	}
	return c.rdb.Set(ctx, key, data, c.ttl).Err()
}
