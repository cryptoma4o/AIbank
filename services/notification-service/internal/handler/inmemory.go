package handler

import (
	"context"
	"sync"
	"time"

	"github.com/aibank/platform/services/notification-service/internal/domain"
)

// InMemoryRepo — простой repo для unit-тестов handler-а и для dev-режима
// без БД.
type InMemoryRepo struct {
	mu    sync.Mutex
	items map[string]*domain.Notification
}

// NewInMemoryRepo — конструктор для тестов.
func NewInMemoryRepo() *InMemoryRepo {
	return &InMemoryRepo{items: make(map[string]*domain.Notification)}
}

func (r *InMemoryRepo) Create(_ context.Context, n *domain.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *n
	r.items[n.ID] = &cp
	return nil
}

func (r *InMemoryRepo) MarkSent(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.items[id]
	if !ok {
		return domain.ErrNotFound
	}
	t := time.Now().UTC()
	n.Status = domain.StatusSent
	n.SentAt = &t
	n.AttemptCount++
	return nil
}

func (r *InMemoryRepo) MarkFailed(_ context.Context, id, msg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.items[id]
	if !ok {
		return domain.ErrNotFound
	}
	n.Status = domain.StatusFailed
	n.Error = msg
	n.AttemptCount++
	return nil
}

func (r *InMemoryRepo) ListByTenant(_ context.Context, tenantID string, limit int) ([]domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Notification, 0)
	for _, n := range r.items {
		if n.TenantID == tenantID {
			out = append(out, *n)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryRepo) GetByID(_ context.Context, id string) (*domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *n
	return &cp, nil
}

