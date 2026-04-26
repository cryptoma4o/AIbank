package domain

import "time"

type NotificationChannel string

const (
	ChannelEmail NotificationChannel = "email"
	ChannelSMS   NotificationChannel = "sms"
	ChannelPush  NotificationChannel = "push"
)

type NotificationStatus string

const (
	StatusPending NotificationStatus = "pending"
	StatusSent    NotificationStatus = "sent"
	StatusFailed  NotificationStatus = "failed"
)

type Notification struct {
	ID         string              `json:"id"`
	TenantID   string              `json:"tenant_id"`
	Recipient  string              `json:"recipient"`   // email or phone
	Channel    NotificationChannel `json:"channel"`
	TemplateID string              `json:"template_id"`
	Payload    map[string]string   `json:"payload"`     // template variables
	Status     NotificationStatus  `json:"status"`
	SentAt     *time.Time          `json:"sent_at,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
}
