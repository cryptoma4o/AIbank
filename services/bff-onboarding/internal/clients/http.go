// Package clients — HTTP-клиенты к доменным сервисам.
//
// Каждый клиент:
//   - net/http с таймаутом 5s на запрос (см. ADR-0003 «BFF — тонкий слой»);
//   - один retry на 5xx или сетевые ошибки;
//   - проброс X-Tenant-Id и Authorization из контекста по необходимости;
//   - возвращает доменные типы из internal/model.
//
// gRPC-клиенты к этим же сервисам — TODO (см. ADR-0008): для MVP HTTP+JSON
// достаточно, gRPC будет добавлен, когда появятся отчётливо горячие пути.
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

// DefaultTimeout — таймаут одного HTTP-вызова в downstream-сервис.
const DefaultTimeout = 5 * time.Second

// MaxRetries — single retry; основная защита — circuit-breaker в api-gateway.
const MaxRetries = 1

// ErrUpstream — ошибка downstream-сервиса (5xx после ретраев или сеть).
var ErrUpstream = errors.New("upstream error")

// ErrNotFound — 404 от downstream-сервиса.
var ErrNotFound = errors.New("not found")

// StatusError — структурированная 4xx-ошибка от downstream-сервиса с
// исходным телом, чтобы вызывающий мог распарсить domain-specific детали
// (например, existing_application_id из 409 от orchestrator).
type StatusError struct {
	StatusCode int
	Body       []byte
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("client error: status %d body=%s", e.StatusCode, truncate(e.Body))
}

// httpDoer — узкий интерфейс, чтобы тесты могли заменить http.Client моком.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// transport — общая обвязка для всех клиентов.
type transport struct {
	http httpDoer
}

func newTransport() *transport {
	return &transport{
		http: &http.Client{Timeout: DefaultTimeout},
	}
}

// doJSON отправляет req и декодирует JSON-ответ в out.  Ретраит один раз
// на 5xx и на сетевые ошибки; 4xx возвращает сразу.
//
// Когда out == nil, тело ответа просто отбрасывается; используется для
// эндпоинтов без полезного body (signal-эндпоинты).
func (t *transport) doJSON(req *http.Request, out any) error {
	var lastErr error
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		// Каждая попытка — свежий клон тела, иначе req.Body отдаст EOF.
		clone, err := cloneRequest(req)
		if err != nil {
			return fmt.Errorf("clone request: %w", err)
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
			lastErr = fmt.Errorf("%w: status %d body=%s", ErrUpstream, resp.StatusCode, truncate(body))
			continue
		case resp.StatusCode >= 400:
			// Возвращаем structured StatusError, чтобы вызывающий мог
			// распарсить body (например, 409 c existing_application_id).
			return &StatusError{StatusCode: resp.StatusCode, Body: body}
		}
		if out == nil {
			return nil
		}
		if len(body) == 0 {
			return nil
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode upstream body: %w", err)
		}
		return nil
	}
	return lastErr
}

// cloneRequest нужен, чтобы повтор не упал на закрытом io.Reader.
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
	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(buf))
	return clone, nil
}

// newJSONRequest — helper для POST/PUT с JSON body.
func newJSONRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
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

// truncate — обрезает длинные тела ошибок для логов.
func truncate(b []byte) string {
	const max = 256
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}
