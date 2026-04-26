package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aibank/abs-connector/internal/domain"
)

// adapterURLs maps tenant_id to adapter base URL.
// In production, loaded from tenant-service configuration.
var adapterURLs = map[string]string{
	"default": "http://abs-adapter-cft:9001",
}

type Router struct {
	httpClient *http.Client
}

func NewRouter() *Router {
	return &Router{httpClient: &http.Client{Timeout: 30 * time.Second}}
}

func (r *Router) Route(ctx context.Context, cmd *domain.ABSCommand) (*domain.ABSResponse, error) {
	adapterURL, ok := adapterURLs[cmd.TenantID]
	if !ok {
		adapterURL = adapterURLs["default"]
	}

	body, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("abs-connector: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, adapterURL+"/v1/execute", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("abs-connector: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("abs-connector: do: %w", err)
	}
	defer resp.Body.Close()

	var result domain.ABSResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("abs-connector: decode response: %w", err)
	}
	result.AdapterUsed = adapterURL
	return &result, nil
}
