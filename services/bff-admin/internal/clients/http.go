// Package clients — HTTP-клиенты к доменным сервисам для bff-admin.
//
// Зеркалит структуру bff-onboarding/internal/clients (минимальный transport
// с одним retry на 5xx, ошибкой ErrNotFound на 404, JSON-сериализацией).
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	DefaultTimeout = 5 * time.Second
	MaxRetries     = 1
)

var (
	ErrUpstream = errors.New("upstream error")
	ErrNotFound = errors.New("not found")
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type transport struct{ http httpDoer }

func newTransport() *transport {
	return &transport{http: &http.Client{Timeout: DefaultTimeout}}
}

func (t *transport) doJSON(req *http.Request, out any) error {
	var lastErr error
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		clone, err := cloneRequest(req)
		if err != nil {
			return fmt.Errorf("clone: %w", err)
		}
		resp, err := t.http.Do(clone)
		if err != nil {
			lastErr = fmt.Errorf("%w: %v", ErrUpstream, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("%w: %s", ErrNotFound, req.URL.Path)
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
			continue
		case resp.StatusCode >= 400:
			return fmt.Errorf("client error: status %d body=%s", resp.StatusCode, truncate(body))
		}
		if out == nil || len(body) == 0 {
			return nil
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		return nil
	}
	return lastErr
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	if req.Body == nil {
		return req.Clone(req.Context()), nil
	}
	buf, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(buf))
	c := req.Clone(req.Context())
	c.Body = io.NopCloser(bytes.NewReader(buf))
	return c, nil
}

func newJSONRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func truncate(b []byte) string {
	const max = 256
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}
