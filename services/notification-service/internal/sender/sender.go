// Package sender — абстракция канала отправки уведомлений.
//
// Sender — узкий интерфейс; реализации:
//   - MockSender (mock.go) — для dev/CI, всегда успех
//   - SMTPSender (email_smtp.go) — stub реальной SMTP-доставки
//   - SMSStubSender (sms_stub.go) — stub SMS-провайдера
//
// Выбор реализации — через ENV NOTIFICATION_SENDER в main.go.
package sender

import (
	"context"
	"errors"

	"github.com/aibank/platform/services/notification-service/internal/domain"
)

// ErrNotImplemented — реализация заявлена, но не подключена к реальному
// провайдеру.  Возвращается SMTPSender и SMSStubSender в текущей версии.
var ErrNotImplemented = errors.New("sender: not implemented")

// Rendered — то, что приходит от templates.Registry.Render.
//
// Sender не знает о templates — только готовый текст.
type Rendered struct {
	Subject string
	Body    string
}

// Sender — единый интерфейс канала.
type Sender interface {
	// Send — синхронная отправка; возвращает nil при успехе.
	//
	// Реализации обязаны не зависеть от глобального стейта; идемпотентность
	// (повторная отправка одного и того же n.ID) — ответственность вызывателя.
	Send(ctx context.Context, n *domain.Notification, content Rendered) error

	// Channel — канал, который умеет отправлять (для роутинга).
	Channel() domain.RecipientType
}
