package sender

import (
	"context"
	"sync"

	"github.com/aibank/platform/services/notification-service/internal/domain"
)

// MockSender — всегда успешная отправка; пишет всё в Sent для тестов.
type MockSender struct {
	channel domain.RecipientType

	mu   sync.Mutex
	Sent []SentRecord
}

// SentRecord — запись о вызове Send (для assertions в тестах).
type SentRecord struct {
	NotificationID string
	Recipient      string
	Subject        string
	Body           string
}

// NewMockSender — конструктор; channel определяет, для какого канала
// будет использоваться (можно создать отдельные mocks под email/sms/push).
func NewMockSender(channel domain.RecipientType) *MockSender {
	return &MockSender{channel: channel}
}

// Send — всегда nil.
func (m *MockSender) Send(_ context.Context, n *domain.Notification, c Rendered) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Sent = append(m.Sent, SentRecord{
		NotificationID: n.ID,
		Recipient:      n.Recipient,
		Subject:        c.Subject,
		Body:           c.Body,
	})
	return nil
}

// Channel — канал, под который мок настроен.
func (m *MockSender) Channel() domain.RecipientType { return m.channel }

// Records — read-only снимок отправленного.
func (m *MockSender) Records() []SentRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SentRecord, len(m.Sent))
	copy(out, m.Sent)
	return out
}
