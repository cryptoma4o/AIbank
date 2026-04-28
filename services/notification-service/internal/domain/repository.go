package domain

import (
	"context"
	"errors"
)

// ErrNotFound — уведомление не найдено.
var ErrNotFound = errors.New("notification: not found")

// NotificationRepository — порт для хранилища.
//
// Реализации:
//   - repository.PostgresNotificationRepository — production
//   - tests in handler_test использует in-memory заглушку
type NotificationRepository interface {
	Create(ctx context.Context, n *Notification) error
	MarkSent(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, errMsg string) error
	ListByTenant(ctx context.Context, tenantID string, limit int) ([]Notification, error)
	GetByID(ctx context.Context, id string) (*Notification, error)
}
