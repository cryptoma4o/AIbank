package auth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type JWTValidator struct {
	publicKeyPEM []byte
}

func NewJWTValidator(publicKeyPEM []byte) (*JWTValidator, error) {
	// validate key parses on construction
	_, err := jwt.ParseRSAPublicKeyFromPEM(publicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("jwt: invalid public key: %w", err)
	}
	return &JWTValidator{publicKeyPEM: publicKeyPEM}, nil
}

type Claims struct {
	jwt.RegisteredClaims
	TenantID string   `json:"tenant_id"`
	Roles    []string `json:"roles"`
	Type     string   `json:"type"`
}

func (v *JWTValidator) Validate(tokenString string) (*Claims, error) {
	key, err := jwt.ParseRSAPublicKeyFromPEM(v.publicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("jwt: parse key: %w", err)
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("jwt: unexpected signing method: %v", t.Header["alg"])
		}
		return key, nil
	})
	if err != nil {
		return nil, fmt.Errorf("jwt: parse: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("jwt: invalid token")
	}
	return claims, nil
}
