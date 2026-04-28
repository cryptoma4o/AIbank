package domain

import "github.com/golang-jwt/jwt/v5"

// TokenType различает access и refresh токены.
type TokenType string

const (
	TokenAccess  TokenType = "access"
	TokenRefresh TokenType = "refresh"
)

// JWTClaims — payload подписываемых JWT.
//
// TenantID опционален: для платформенных админов он пуст; для банковских ролей
// и для applicant'ов — обязателен.
type JWTClaims struct {
	jwt.RegisteredClaims
	TenantID string    `json:"tenant_id,omitempty"`
	Role     string    `json:"role,omitempty"`
	Type     TokenType `json:"type"`
}

// TokenPair — результат успешной аутентификации.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds until access token expiration
}
