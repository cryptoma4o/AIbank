package clients

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"aibank/bff-onboarding/internal/model"
)

// DocumentClient — клиент к document-service.
type DocumentClient struct {
	baseURL string
	tr      *transport
}

func NewDocumentClient(baseURL string) *DocumentClient {
	return &DocumentClient{baseURL: baseURL, tr: newTransport()}
}

type documentPayload struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	ApplicationID string    `json:"application_id"`
	Filename      string    `json:"filename"`
	State         string    `json:"state"`
	UploadedAt    time.Time `json:"uploaded_at"`
}

// ListByApplication — GET /v1/documents?application_id=...
func (c *DocumentClient) ListByApplication(ctx context.Context, tenantID, applicationID string) ([]model.Document, error) {
	q := url.Values{}
	q.Set("application_id", applicationID)
	u := fmt.Sprintf("%s/v1/documents?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Tenant-Id", tenantID)

	var resp struct {
		Items []documentPayload `json:"items"`
	}
	if err := c.tr.doJSON(req, &resp); err != nil {
		return nil, err
	}
	out := make([]model.Document, 0, len(resp.Items))
	for _, p := range resp.Items {
		out = append(out, model.Document{
			ID:            p.ID,
			Type:          model.DocumentType(p.Type),
			ApplicationID: p.ApplicationID,
			Filename:      p.Filename,
			State:         p.State,
			UploadedAt:    p.UploadedAt,
		})
	}
	return out, nil
}

// UploadInput — тело POST /v1/documents.
type UploadInput struct {
	ApplicationID string `json:"application_id"`
	Type          string `json:"type"`
	Filename      string `json:"filename"`
	ContentBase64 string `json:"content_base64"`
}

// Upload — POST /v1/documents.
func (c *DocumentClient) Upload(ctx context.Context, tenantID string, in UploadInput) (*model.Document, error) {
	u := fmt.Sprintf("%s/v1/documents", c.baseURL)
	req, err := newJSONRequest(ctx, http.MethodPost, u, in)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Tenant-Id", tenantID)

	var p documentPayload
	if err := c.tr.doJSON(req, &p); err != nil {
		return nil, err
	}
	return &model.Document{
		ID:            p.ID,
		Type:          model.DocumentType(p.Type),
		ApplicationID: p.ApplicationID,
		Filename:      p.Filename,
		State:         p.State,
		UploadedAt:    p.UploadedAt,
	}, nil
}
