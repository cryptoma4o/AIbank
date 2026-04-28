package domain

import "time"

// PrincipalType различает тип субъекта аутентификации.
type PrincipalType string

const (
	PrincipalClient   PrincipalType = "client"   // applicant
	PrincipalEmployee PrincipalType = "employee" // user (bank operator etc.)
)

// AuthMethod — способ, которым субъект подтвердил себя.
type AuthMethod string

const (
	AuthMethodPassword AuthMethod = "password"
	AuthMethodESIA     AuthMethod = "esia"
	AuthMethodOTP      AuthMethod = "otp"
	AuthMethodSSO      AuthMethod = "sso"
)

// Session описывает активную сессию для отчётов/ревокации (в этой версии
// сервиса не хранится в БД — заложено на будущее).
type Session struct {
	ID          string        `json:"id"`
	PrincipalID string        `json:"principal_id"`
	TenantID    string        `json:"tenant_id"`
	Type        PrincipalType `json:"type"`
	AuthMethod  AuthMethod    `json:"auth_method"`
	ExpiresAt   time.Time     `json:"expires_at"`
	CreatedAt   time.Time     `json:"created_at"`
}
