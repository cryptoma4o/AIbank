package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"aibank/ext-egrul/internal/domain"
)

const TTL = 24 * time.Hour

type RedisCache struct {
	rdb *redis.Client
}

func NewRedisCache(addr string) *RedisCache {
	return &RedisCache{rdb: redis.NewClient(&redis.Options{Addr: addr})}
}

func (c *RedisCache) Get(ctx context.Context, inn string) (*domain.EGRULRecord, error) {
	val, err := c.rdb.Get(ctx, "egrul:"+inn).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var rec domain.EGRULRecord
	if err := json.Unmarshal([]byte(val), &rec); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &rec, nil
}

func (c *RedisCache) Set(ctx context.Context, rec *domain.EGRULRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return c.rdb.Set(ctx, "egrul:"+rec.INN, data, TTL).Err()
}
