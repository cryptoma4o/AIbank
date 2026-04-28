package clients

import (
	"context"
	"fmt"
	"net/http"

	"aibank/bff-onboarding/internal/auth"
	"aibank/bff-onboarding/internal/model"
)

// IdentityClient — клиент к identity-service.
//
// JWT-валидация выполняется локально через auth.Verifier (общий секрет
// JWT_SECRET).  Этот клиент ходит в identity-service только для /v1/me,
// если фронтенду нужны данные пользователя сверх claims (email, флаги).
type IdentityClient struct {
	baseURL string
	tr      *transport
}

func NewIdentityClient(baseURL string) *IdentityClient {
	return &IdentityClient{baseURL: baseURL, tr: newTransport()}
}

// userPayload — то, что отдаёт identity-service GET /v1/me.
type userPayload struct {
	ID       string  `json:"id"`
	TenantID *string `json:"tenant_id,omitempty"`
	Email    string  `json:"email"`
	Role     string  `json:"role"`
	IsActive bool    `json:"is_active"`
}

// GetMe — GET /v1/me с проксированием Authorization из исходного запроса.
//
// Возвращает обогащённую информацию о пользователе (email, IsActive),
// которой нет в JWT claims.
func (c *IdentityClient) GetMe(ctx context.Context, bearerToken string) (*model.Me, error) {
	url := fmt.Sprintf("%s/v1/me", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)

	var p userPayload
	if err := c.tr.doJSON(req, &p); err != nil {
		return nil, err
	}
	tenant := ""
	if p.TenantID != nil {
		tenant = *p.TenantID
	}
	return &model.Me{
		UserID:   p.ID,
		TenantID: tenant,
		Role:     p.Role,
		Email:    p.Email,
	}, nil
}

// MeFromAuth — конструирует Me исключительно из claims JWT, без сетевого
// вызова identity-service.  Используется когда фронтенду достаточно
// (userId, tenantId, role); для applicant-токенов это единственный путь
// (у них нет записи в platform.users — см. identity-service.Me).
func MeFromAuth(ac *auth.AuthContext) *model.Me {
	return &model.Me{
		UserID:   ac.UserID,
		TenantID: ac.TenantID,
		Role:     ac.Role,
	}
}
