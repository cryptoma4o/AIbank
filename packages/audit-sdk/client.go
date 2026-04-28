package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ClientOptions configures NewClient. BaseURL is the only required field.
type ClientOptions struct {
	// BaseURL of the audit-service, e.g. "http://audit-service:8081".
	BaseURL string
	// Timeout is the upper bound for a single Append/List call (including
	// retries). Defaults to 5s.
	Timeout time.Duration
	// APIKey, if set, is sent as Authorization: Bearer <APIKey>.
	APIKey string
	// HTTPClient lets callers inject a custom *http.Client (mostly for
	// tests). When nil a default client with Timeout is used. The Timeout
	// from Options still bounds individual operations via context.
	HTTPClient *http.Client
	// MaxRetries controls retry attempts on 5xx/429. Defaults to 3.
	MaxRetries int
}

// Client wraps the audit-service HTTP API.
type Client struct {
	baseURL    string
	timeout    time.Duration
	apiKey     string
	httpClient *http.Client
	maxRetries int
}

// NewClient constructs a Client. opts.BaseURL must be non-empty.
func NewClient(opts ClientOptions) (*Client, error) {
	if opts.BaseURL == "" {
		return nil, fmt.Errorf("audit: BaseURL is required")
	}
	if _, err := url.Parse(opts.BaseURL); err != nil {
		return nil, fmt.Errorf("audit: invalid BaseURL: %w", err)
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &Client{
		baseURL:    trimSlash(opts.BaseURL),
		timeout:    timeout,
		apiKey:     opts.APIKey,
		httpClient: httpClient,
		maxRetries: maxRetries,
	}, nil
}

// Append records a new audit event via POST /v1/events.
//
// On 5xx / 429 the call is retried with exponential backoff until either
// the call succeeds or the context-derived budget (defaulting to the
// client Timeout) is exhausted. Non-retryable 4xx errors return an
// *APIError immediately.
func (c *Client) Append(ctx context.Context, ev RecordEventRequest) (*AuditEvent, error) {
	if err := ev.Validate(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("audit: marshal request: %w", err)
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	var out AuditEvent
	if err := c.doWithRetry(ctx, http.MethodPost, "/v1/events", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List fetches events via GET /v1/events with the supplied filters.
// QueryOptions.TenantID is required.
func (c *Client) List(ctx context.Context, q QueryOptions) ([]*AuditEvent, error) {
	if q.TenantID == "" {
		return nil, fmt.Errorf("audit: TenantID is required")
	}

	v := url.Values{}
	v.Set("tenant_id", q.TenantID)
	if q.EntityType != "" {
		v.Set("entity_type", q.EntityType)
	}
	if q.EntityID != "" {
		v.Set("entity_id", q.EntityID)
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	path := "/v1/events?" + v.Encode()

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	var out listResponse
	if err := c.doWithRetry(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// withTimeout shrinks ctx to at most c.timeout from now.
func (c *Client) withTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if dl, ok := parent.Deadline(); ok && time.Until(dl) <= c.timeout {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, c.timeout)
}

// doWithRetry performs an HTTP call with exponential backoff on retryable
// errors. Total budget = remaining context deadline. JSON-decodes 2xx
// bodies into out (which may be nil to discard).
func (c *Client) doWithRetry(ctx context.Context, method, path string, body []byte, out any) error {
	delay := 50 * time.Millisecond
	var lastErr error
	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}

		err := c.do(ctx, method, path, body, out)
		if err == nil {
			return nil
		}
		lastErr = err

		var apiErr *APIError
		if asAPIErr(err, &apiErr) {
			if !apiErr.IsRetryable() {
				return err
			}
		} else if !isTransport(err) {
			return err
		}

		if attempt == c.maxRetries {
			break
		}
		// Honour ctx deadline: don't sleep past it.
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(delay):
		}
		delay *= 2
	}
	return lastErr
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("audit: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &transportError{err: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &transportError{err: err}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || len(respBody) == 0 {
			return nil
		}
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("audit: decode response: %w", err)
		}
		return nil
	}

	apiErr := &APIError{StatusCode: resp.StatusCode}
	var env errorEnvelope
	if jerr := json.Unmarshal(respBody, &env); jerr == nil {
		apiErr.Code = env.Error.Code
		apiErr.Message = env.Error.Message
	}
	return apiErr
}

// transportError marks network-level failures so the retry loop can spot
// them without unwrapping every error type.
type transportError struct{ err error }

func (e *transportError) Error() string { return "audit: transport: " + e.err.Error() }
func (e *transportError) Unwrap() error { return e.err }

func isTransport(err error) bool {
	_, ok := err.(*transportError)
	return ok
}

// asAPIErr is a small helper around errors.As to keep the retry path
// readable.
func asAPIErr(err error, target **APIError) bool {
	for cur := err; cur != nil; {
		if e, ok := cur.(*APIError); ok {
			*target = e
			return true
		}
		// Unwrap manually to avoid pulling errors package nuances into
		// the retry critical path.
		type unwrapper interface{ Unwrap() error }
		u, ok := cur.(unwrapper)
		if !ok {
			return false
		}
		cur = u.Unwrap()
	}
	return false
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
