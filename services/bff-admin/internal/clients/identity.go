package clients

import (
	"context"
	"fmt"
	"net/http"
)

// User — расширенные данные пользователя из identity-service.
type User struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id,omitempty"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	IsActive bool   `json:"is_active"`
}

// IdentityClient — клиент к identity-service.
type IdentityClient struct {
	baseURL string
	tr      *transport
}

func NewIdentityClient(baseURL string) *IdentityClient {
	return &IdentityClient{baseURL: baseURL, tr: newTransport()}
}

// GetMe — GET /v1/me с пробросом Authorization.
func (c *IdentityClient) GetMe(ctx context.Context, bearerToken string) (*User, error) {
	url := fmt.Sprintf("%s/v1/me", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	var u User
	if err := c.tr.doJSON(req, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
