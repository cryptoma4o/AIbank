package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"aibank/ext-rosfinmon/internal/domain"
)

// TTL по умолчанию: результаты скрининга — 7 суток, снапшот списка — 24 часа.
const (
	DefaultScreeningTTL = 7 * 24 * time.Hour
	DefaultSnapshotTTL  = 24 * time.Hour
)

// Cache — интерфейс для подмены в тестах.
type Cache interface {
	GetScreening(ctx context.Context, req domain.ScreeningRequest) (*domain.ScreeningResult, error)
	SetScreening(ctx context.Context, req domain.ScreeningRequest, res *domain.ScreeningResult) error
	GetSnapshot(ctx context.Context) (*domain.ListSnapshot, error)
	SetSnapshot(ctx context.Context, snap *domain.ListSnapshot) error
}

// RedisCache — реализация Cache на go-redis.
type RedisCache struct {
	rdb         *redis.Client
	screeningTTL time.Duration
	snapshotTTL  time.Duration
}

// NewRedisCache подключается к Redis по адресу `addr` с TTL по умолчанию.
func NewRedisCache(addr string) *RedisCache {
	return &RedisCache{
		rdb:          redis.NewClient(&redis.Options{Addr: addr}),
		screeningTTL: ttlFromEnv("SCREENING_TTL_HOURS", DefaultScreeningTTL),
		snapshotTTL:  ttlFromEnv("SNAPSHOT_TTL_HOURS", DefaultSnapshotTTL),
	}
}

// NewRedisCacheFromClient — для тестов с miniredis/моками.
func NewRedisCacheFromClient(rdb *redis.Client, screeningTTL, snapshotTTL time.Duration) *RedisCache {
	return &RedisCache{rdb: rdb, screeningTTL: screeningTTL, snapshotTTL: snapshotTTL}
}

func ttlFromEnv(name string, def time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if hours, err := strconv.Atoi(v); err == nil && hours > 0 {
			return time.Duration(hours) * time.Hour
		}
	}
	return def
}

// ScreeningTTL возвращает текущий TTL результатов скрининга.
func (c *RedisCache) ScreeningTTL() time.Duration { return c.screeningTTL }

// SnapshotTTL возвращает текущий TTL снапшота списка.
func (c *RedisCache) SnapshotTTL() time.Duration { return c.snapshotTTL }

// GetScreening читает результат скрининга по запросу. Возвращает (nil, nil) при cache miss.
func (c *RedisCache) GetScreening(ctx context.Context, req domain.ScreeningRequest) (*domain.ScreeningResult, error) {
	key := screeningKey(req)
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get screening: %w", err)
	}
	var res domain.ScreeningResult
	if err := json.Unmarshal([]byte(val), &res); err != nil {
		return nil, fmt.Errorf("unmarshal screening: %w", err)
	}
	return &res, nil
}

// SetScreening сохраняет результат скрининга на screeningTTL.
func (c *RedisCache) SetScreening(ctx context.Context, req domain.ScreeningRequest, res *domain.ScreeningResult) error {
	data, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("marshal screening: %w", err)
	}
	return c.rdb.Set(ctx, screeningKey(req), data, c.screeningTTL).Err()
}

// GetSnapshot читает метаданные перечня.
func (c *RedisCache) GetSnapshot(ctx context.Context) (*domain.ListSnapshot, error) {
	val, err := c.rdb.Get(ctx, "rfm:snapshot").Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get snapshot: %w", err)
	}
	var snap domain.ListSnapshot
	if err := json.Unmarshal([]byte(val), &snap); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return &snap, nil
}

// SetSnapshot сохраняет снапшот.
func (c *RedisCache) SetSnapshot(ctx context.Context, snap *domain.ListSnapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	return c.rdb.Set(ctx, "rfm:snapshot", data, c.snapshotTTL).Err()
}

// screeningKey строит детерминированный ключ из запроса.
//
// Внутри хэшируем нормализованную форму (strip + lowercase для имени), чтобы
// похожие, но не идентичные запросы попадали в один кэш-слот.
func screeningKey(req domain.ScreeningRequest) string {
	payload := fmt.Sprintf("%s|%s|%s|%s",
		req.SubjectType,
		req.Identifiers.INN,
		req.Identifiers.FullName,
		req.Identifiers.BirthDate,
	)
	sum := sha256.Sum256([]byte(payload))
	return "rfm:screen:" + hex.EncodeToString(sum[:16])
}
