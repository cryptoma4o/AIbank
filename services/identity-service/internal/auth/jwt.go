package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"aibank/identity-service/internal/domain"
)

// Длительности токенов согласно спецификации.
const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 7 * 24 * time.Hour
	tokenIssuer     = "aibank-identity"
)

// ErrInvalidToken — токен не прошёл валидацию (подпись, срок, формат).
var ErrInvalidToken = errors.New("invalid token")

// Issuer выпускает HS256 access/refresh токены, подписанные общим secret'ом.
//
// Секрет берётся из JWT_SECRET и НЕ должен попадать в логи или ответы API.
type Issuer struct {
	secret []byte
	now    func() time.Time
}

// NewIssuer возвращает Issuer; secret должен быть >= 32 байт (см. RFC 7518 §3.2).
func NewIssuer(secret []byte) (*Issuer, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("jwt: secret must be at least 32 bytes, got %d", len(secret))
	}
	return &Issuer{secret: secret, now: time.Now}, nil
}

// IssueParams — параметры выпуска пары токенов.
type IssueParams struct {
	Subject  string // user.id или applicant.id
	TenantID string // пусто для платформенных ролей
	Role     string // domain.Role или пусто (например, applicant)
}

// Issue выпускает access+refresh с одинаковым subject/tenant/role и общим jti для пары.
func (i *Issuer) Issue(p IssueParams) (*domain.TokenPair, error) {
	now := i.now().UTC()
	access, err := i.sign(p, domain.TokenAccess, now, AccessTokenTTL)
	if err != nil {
		return nil, err
	}
	refresh, err := i.sign(p, domain.TokenRefresh, now, RefreshTokenTTL)
	if err != nil {
		return nil, err
	}
	return &domain.TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(AccessTokenTTL.Seconds()),
	}, nil
}

// Sign подписывает заранее заполненные claims.
func (i *Issuer) Sign(claims *domain.JWTClaims) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(i.secret)
}

func (i *Issuer) sign(p IssueParams, kind domain.TokenType, now time.Time, ttl time.Duration) (string, error) {
	claims := &domain.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   p.Subject,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		TenantID: p.TenantID,
		Role:     p.Role,
		Type:     kind,
	}
	return i.Sign(claims)
}

// Verifier разбирает и проверяет подпись/срок токенов.
type Verifier struct {
	secret []byte
}

func NewVerifier(secret []byte) (*Verifier, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("jwt: secret must be at least 32 bytes, got %d", len(secret))
	}
	return &Verifier{secret: secret}, nil
}

// Parse валидирует подпись и срок, возвращает claims.
func (v *Verifier) Parse(tokenStr string) (*domain.JWTClaims, error) {
	claims := &domain.JWTClaims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return v.secret, nil
	}, jwt.WithIssuer(tokenIssuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !tok.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
