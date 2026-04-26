package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const ClaimsKey contextKey = "jwt_claims"

// JWTAuth validates Bearer tokens. publicKeyPEM is the RS256 public key from Keycloak.
// Pass nil publicKeyPEM to skip validation (dev mode).
func JWTAuth(publicKeyPEM []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if publicKeyPEM == nil {
				next.ServeHTTP(w, r)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			pubKey, err := jwt.ParseRSAPublicKeyFromPEM(publicKeyPEM)
			if err != nil {
				http.Error(w, `{"error":"invalid server key config"}`, http.StatusInternalServerError)
				return
			}
			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
				return pubKey, nil
			}, jwt.WithValidMethods([]string{"RS256"}))
			if err != nil || !token.Valid {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, token.Claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
