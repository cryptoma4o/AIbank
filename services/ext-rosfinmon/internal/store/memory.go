package store

import (
	"sync"

	"aibank/ext-rosfinmon/internal/domain"
)

type MemoryStore struct {
	mu    sync.RWMutex
	byINN map[string]domain.SanctionedEntity
}

func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{byINN: make(map[string]domain.SanctionedEntity)}
	// Pre-populate with a few test INNs
	s.byINN["0000000000"] = domain.SanctionedEntity{INN: "0000000000", FullName: "Тестовый террорист", ListType: "terrorist"}
	return s
}

func (s *MemoryStore) Check(inn string) domain.CheckResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entity, ok := s.byINN[inn]
	if !ok {
		return domain.CheckResult{INN: inn, Blocked: false}
	}
	return domain.CheckResult{INN: inn, Blocked: true, Reason: entity.ListType + ": " + entity.FullName}
}

func (s *MemoryStore) Load(entities []domain.SanctionedEntity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	newMap := make(map[string]domain.SanctionedEntity, len(entities))
	for _, e := range entities {
		if e.INN != "" {
			newMap[e.INN] = e
		}
	}
	s.byINN = newMap
}
