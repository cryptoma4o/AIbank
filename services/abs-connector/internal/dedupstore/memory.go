package dedupstore

import (
	"context"
	"sync"
	"time"

	"aibank/abs-connector/internal/domain"
)

// InMemoryStore — реализация IdempotencyStore без внешних зависимостей.
// Используется в unit-тестах handler'а и как DEDUP_STORE=memory-режим
// для local-dev без Redis'а.
//
// TTL поддерживается lazy-eviction: при Get() мы проверяем expiry и
// удаляем просроченную запись на месте. Background-goroutine не запускаем,
// чтобы не усложнять lifecycle (в тестах было бы лишним).
// Для long-running prod-процесса этот подход приведёт к умеренному росту
// памяти — InMemoryStore не предназначен для prod, см. RedisStore.
type InMemoryStore struct {
	mu   sync.RWMutex
	data map[string]memoryEntry
}

type memoryEntry struct {
	response domain.CanonicalResponse
	expires  time.Time // zero == «никогда не истекает»
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{data: make(map[string]memoryEntry)}
}

func memoryKey(tenantID, idempotencyKey string) string {
	return tenantID + "|" + idempotencyKey
}

// Get — поиск ответа. Просроченные записи удаляются (write-lock).
func (s *InMemoryStore) Get(_ context.Context, tenantID, idempotencyKey string) (domain.CanonicalResponse, bool, error) {
	key := memoryKey(tenantID, idempotencyKey)
	s.mu.RLock()
	entry, ok := s.data[key]
	s.mu.RUnlock()
	if !ok {
		return domain.CanonicalResponse{}, false, nil
	}
	if !entry.expires.IsZero() && time.Now().After(entry.expires) {
		s.mu.Lock()
		// Re-check под write-lock'ом: между RUnlock и Lock могли перезаписать.
		if cur, stillThere := s.data[key]; stillThere && !cur.expires.IsZero() && time.Now().After(cur.expires) {
			delete(s.data, key)
		}
		s.mu.Unlock()
		return domain.CanonicalResponse{}, false, nil
	}
	return entry.response, true, nil
}

// Put — сохранить ответ. ttl <= 0 → запись без истечения.
func (s *InMemoryStore) Put(_ context.Context, tenantID, idempotencyKey string, response domain.CanonicalResponse, ttl time.Duration) error {
	key := memoryKey(tenantID, idempotencyKey)
	entry := memoryEntry{response: response}
	if ttl > 0 {
		entry.expires = time.Now().Add(ttl)
	}
	s.mu.Lock()
	s.data[key] = entry
	s.mu.Unlock()
	return nil
}

// Len — диагностический хелпер для тестов.
func (s *InMemoryStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}
