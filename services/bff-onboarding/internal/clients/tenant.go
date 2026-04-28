package clients

import (
	"context"
	"fmt"
	"net/http"

	"aibank/bff-onboarding/internal/model"
)

// TenantClient — клиент к tenant-service (ADR-0002 § «tenant registry»).
type TenantClient struct {
	baseURL string
	tr      *transport
}

func NewTenantClient(baseURL string) *TenantClient {
	return &TenantClient{baseURL: baseURL, tr: newTransport()}
}

// tenantPayload — то, что отдаёт tenant-service (services/tenant-service/
// internal/domain.Tenant в JSON).
type tenantPayload struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	BIK            string `json:"bik"`
	INN            string `json:"inn"`
	Status         string `json:"status"`
	DeploymentMode string `json:"deployment_mode"`
}

// GetByID — GET /v1/tenants/{id}.
func (c *TenantClient) GetByID(ctx context.Context, id string) (*model.Tenant, error) {
	url := fmt.Sprintf("%s/v1/tenants/%s", c.baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	var p tenantPayload
	if err := c.tr.doJSON(req, &p); err != nil {
		return nil, err
	}
	return &model.Tenant{
		ID:             p.ID,
		Name:           p.Name,
		BIK:            p.BIK,
		INN:            p.INN,
		Status:         p.Status,
		DeploymentMode: p.DeploymentMode,
	}, nil
}
