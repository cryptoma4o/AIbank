package sender

import (
	"context"
	"fmt"

	"github.com/aibank/platform/services/notification-service/internal/domain"
)

// SMTPConfig — конфигурация SMTP-сервера.
//
// Заполняется из ENV в main.go (SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS,
// SMTP_FROM).  Пароль приходит из Vault, не хардкодится.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// SMTPSender — STUB реализации SMTP-отправки.
//
// TODO(notification-service): подключить реальную доставку через
// net/smtp.SendMail или go-mail/mail.  В текущей итерации возвращает
// ErrNotImplemented, чтобы избежать тяжёлых зависимостей и flaky-тестов
// (real SMTP требует test-контейнер с MailHog/MailCrab).
type SMTPSender struct {
	cfg SMTPConfig
}

// NewSMTPSender — создаёт sender; конструктор не валидирует connectivity
// (Send проверит при первом вызове).
func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	return &SMTPSender{cfg: cfg}
}

// Send — STUB: возвращает ErrNotImplemented.
func (s *SMTPSender) Send(_ context.Context, n *domain.Notification, _ Rendered) error {
	return fmt.Errorf("%w: smtp host=%s recipient=%s notification=%s",
		ErrNotImplemented, s.cfg.Host, n.Recipient, n.ID)
}

// Channel — email.
func (s *SMTPSender) Channel() domain.RecipientType { return domain.RecipientEmail }
