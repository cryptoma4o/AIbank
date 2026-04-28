package sender

import (
	"context"
	"fmt"

	"github.com/aibank/platform/services/notification-service/internal/domain"
)

// SMSStubSender — заглушка SMS-провайдера.
//
// TODO(notification-service): подключить реального РФ-провайдера
// (SMSC, SMS Aero, MTS Communicator, etc.).  Per-tenant выбор провайдера
// — конфигурацией; pricing и delivery-rate подкладываются в audit log.
type SMSStubSender struct {
	provider string
}

// NewSMSStubSender — конструктор; provider логируется в ошибку для
// диагностики (например, "smsc.ru", "smsaero.ru").
func NewSMSStubSender(provider string) *SMSStubSender {
	if provider == "" {
		provider = "stub"
	}
	return &SMSStubSender{provider: provider}
}

// Send — STUB: возвращает ErrNotImplemented.
func (s *SMSStubSender) Send(_ context.Context, n *domain.Notification, _ Rendered) error {
	return fmt.Errorf("%w: sms provider=%s recipient=%s notification=%s",
		ErrNotImplemented, s.provider, n.Recipient, n.ID)
}

// Channel — sms.
func (s *SMSStubSender) Channel() domain.RecipientType { return domain.RecipientSMS }
