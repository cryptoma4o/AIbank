// Package domain — доменные типы notification-service.
//
// Структура совпадает с миграцией migrations/001_create_notifications.sql;
// поля в JSON — snake_case (для совместимости с остальным платформенным API).
package domain

import "time"

// RecipientType — канал доставки.
type RecipientType string

const (
	RecipientEmail RecipientType = "email"
	RecipientSMS   RecipientType = "sms"
	RecipientPush  RecipientType = "push"
)

// IsValid проверяет, известен ли канал.
func (r RecipientType) IsValid() bool {
	switch r {
	case RecipientEmail, RecipientSMS, RecipientPush:
		return true
	}
	return false
}

// Status — состояние уведомления.
type Status string

const (
	StatusQueued Status = "queued"
	StatusSent   Status = "sent"
	StatusFailed Status = "failed"
)

// Notification — основная сущность.
type Notification struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id"`
	RecipientType RecipientType     `json:"recipient_type"`
	Recipient     string            `json:"recipient"`
	TemplateID    string            `json:"template_id"`
	Vars          map[string]string `json:"vars"`
	Status        Status            `json:"status"`
	SentAt        *time.Time        `json:"sent_at,omitempty"`
	AttemptCount  int               `json:"attempt_count"`
	Error         string            `json:"error,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
}
