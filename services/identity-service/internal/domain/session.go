package domain

import "time"

type PrincipalType string

const (
	PrincipalClient   PrincipalType = "client"
	PrincipalEmployee PrincipalType = "employee"
)

type AuthMethod string

const (
	AuthMethodESIA     AuthMethod = "esia"
	AuthMethodOTP      AuthMethod = "otp"
	AuthMethodPassword AuthMethod = "password"
	AuthMethodSSO      AuthMethod = "sso"
)

type Principal struct {
	ID       string        `json:"id"`
	TenantID string        `json:"tenant_id"`
	Type     PrincipalType `json:"type"`
	INN      string        `json:"inn,omitempty"`
	Email    string        `json:"email,omitempty"`
	Phone    string        `json:"phone,omitempty"`
	Roles    []string      `json:"roles"`
}

type Session struct {
	ID          string     `json:"id"`
	PrincipalID string     `json:"principal_id"`
	TenantID    string     `json:"tenant_id"`
	AuthMethod  AuthMethod `json:"auth_method"`
	AccessToken string     `json:"access_token"`
	ExpiresAt   time.Time  `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
}
