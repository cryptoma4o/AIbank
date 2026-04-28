// Package clients — HTTP-клиент connector'а к адаптерам ABS.
//
// Согласно ADR-0006 целевой контракт между connector и адаптером — gRPC,
// но адаптеры в текущем состоянии (services/abs-adapter-cft) экспонируют
// HTTP/JSON. Этот клиент — переходный «HTTP-layer», совместимый с тем,
// что уже работает; миграция на gRPC — отдельная задача (см. TODO ниже).
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"aibank/abs-connector/internal/domain"
)

// Default-настройки. ConnectTimeout / RequestTimeout сознательно отдельные:
// первое — короткое, чтобы быстро понять «адаптер не поднят»; второе —
// длинное, потому что банковские операции (open-account через SOAP CFT)
// могут идти десяток секунд.
const (
	DefaultRequestTimeout = 30 * time.Second
	DefaultConnectTimeout = 5 * time.Second
)

// AdapterClient — клиент с одним публичным методом Execute.
// На стороне connector'а мы не реализуем retry на 4xx/5xx — за ретраи
// отвечает Temporal-activity с бэкоффом (ADR-0001). На connector'е
// делаем единственный «retry on connection error», чтобы переловить
// краткие сетевые блипы (kube-proxy hiccup).
type AdapterClient struct {
	httpClient *http.Client
}

// NewAdapterClient — фабрика. timeout <= 0 → DefaultRequestTimeout.
func NewAdapterClient(timeout time.Duration) *AdapterClient {
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   DefaultConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	return &AdapterClient{
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// ExecuteParams — параметры запроса. Передаём AdapterEntry целиком,
// чтобы клиент мог вписать adapter_used / adapter_version в результат.
type ExecuteParams struct {
	Entry   domain.AdapterEntry
	Command domain.CanonicalCommand
}

// Execute синхронно вызывает adapter.POST /v1/execute.
//
// Семантика ошибок:
//   - context cancellation / timeout: вернётся ctx.Err() обёрнутый в fmt.Errorf
//   - dial error / temporary network: один retry, потом возврат ошибки
//   - HTTP 4xx/5xx: ошибки НЕ возвращаются как Go-error; они приходят как
//     CanonicalResponse{Success:false, Error:…}, чтобы caller (handler)
//     мог сохранить failure-ответ в idempotency-store и не дёргать
//     адаптер повторно при retries.
func (c *AdapterClient) Execute(ctx context.Context, p ExecuteParams) (domain.CanonicalResponse, error) {
	// Особый псевдо-URL для тестов / demo: возвращаем deterministic-ответ,
	// фактического HTTP-вызова не делаем. Это упрощает интеграционные
	// сценарии без поднятия mock-сервера.
	if p.Entry.URL == domain.MockAdapterURL {
		return c.mockResponse(p), nil
	}

	endpoint, err := buildExecuteURL(p.Entry.URL)
	if err != nil {
		return domain.CanonicalResponse{}, err
	}

	adapterReq := domain.AdapterRequest{
		IdempotencyKey: p.Command.IdempotencyKey,
		TenantID:       p.Command.TenantID,
		Command:        p.Command.Command,
		Payload:        p.Command.Payload,
	}
	body, err := json.Marshal(adapterReq)
	if err != nil {
		return domain.CanonicalResponse{}, fmt.Errorf("marshal adapter request: %w", err)
	}

	resp, doErr := c.doWithSingleRetry(ctx, endpoint, body)
	if doErr != nil {
		return domain.CanonicalResponse{}, doErr
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if readErr != nil {
		return domain.CanonicalResponse{}, fmt.Errorf("read adapter body: %w", readErr)
	}

	var adapterResp domain.AdapterResponse
	if err := json.Unmarshal(raw, &adapterResp); err != nil {
		// Adapter вернул не-JSON / невалидный JSON. Считаем это failure-ответом,
		// чтобы зафиксировать в dedup-store и не дёргать снова.
		return domain.CanonicalResponse{
			IdempotencyKey: p.Command.IdempotencyKey,
			Success:        false,
			Error:          fmt.Sprintf("adapter %s returned invalid json (status=%d): %v", p.Entry.AdapterName, resp.StatusCode, err),
			AdapterUsed:    p.Entry.AdapterName,
			AdapterVersion: p.Entry.Version,
			CompletedAt:    time.Now().UTC(),
		}, nil
	}

	canonical := domain.CanonicalResponse{
		IdempotencyKey: adapterResp.IdempotencyKey,
		Success:        adapterResp.Success,
		Error:          adapterResp.Error,
		Data:           adapterResp.Data,
		AdapterUsed:    p.Entry.AdapterName,
		AdapterVersion: p.Entry.Version,
		CompletedAt:    time.Now().UTC(),
	}
	if canonical.IdempotencyKey == "" {
		canonical.IdempotencyKey = p.Command.IdempotencyKey
	}
	// Если адаптер сам отдал adapter_used (сейчас отдаёт «abs-adapter-cft»),
	// connector всё равно нормализует на logical-name из registry —
	// это нужно для billing/audit consistent-naming.

	if !canonical.Success && resp.StatusCode >= 500 {
		// 5xx без полезного payload — приклеиваем status в Error, иначе caller
		// потеряет сигнал.
		if canonical.Error == "" {
			canonical.Error = fmt.Sprintf("adapter %s returned status %d", p.Entry.AdapterName, resp.StatusCode)
		}
	}
	return canonical, nil
}

func buildExecuteURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid adapter url %q: %w", base, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid adapter url scheme %q (expected http/https)", u.Scheme)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/execute"
	} else {
		// Если кто-то вписал URL уже с path'ом — оставляем как есть.
	}
	return u.String(), nil
}

func (c *AdapterClient) doWithSingleRetry(ctx context.Context, endpoint string, body []byte) (*http.Response, error) {
	const attempts = 2
	var lastErr error
	for i := 0; i < attempts; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isRetryable(err) || ctx.Err() != nil {
			break
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("call adapter %s: %w", endpoint, lastErr)
	}
	return nil, errors.New("call adapter: unreachable")
}

// isRetryable — только connection-errors и временные net-ошибки.
// HTTP-status'ы НЕ ретраим: за это отвечает оркестратор уровня выше.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || isTemporary(netErr)
	}
	return false
}

func isTemporary(err error) bool {
	type temporary interface{ Temporary() bool }
	var t temporary
	if errors.As(err, &t) {
		return t.Temporary()
	}
	return false
}

// mockResponse — deterministic-ответ для MockAdapterURL. Используется
// в demo-tenant и в части unit-тестов handler'а, где не хочется поднимать
// httptest.Server. Семантика согласована с adapter-cft stub'ом.
func (c *AdapterClient) mockResponse(p ExecuteParams) domain.CanonicalResponse {
	resp := domain.CanonicalResponse{
		IdempotencyKey: p.Command.IdempotencyKey,
		AdapterUsed:    p.Entry.AdapterName,
		AdapterVersion: p.Entry.Version,
		CompletedAt:    time.Now().UTC(),
		Success:        true,
	}
	switch p.Command.Command {
	case domain.CmdOpenAccount:
		resp.Data = json.RawMessage(`{"account_number":"40702810000000000000","bik":"044525000"}`)
	case domain.CmdCreateClient:
		resp.Data = json.RawMessage(`{"client_id":"mock-client-001"}`)
	case domain.CmdGetAccountInfo:
		resp.Data = json.RawMessage(`{"balance_kopecks":0,"status":"active"}`)
	case domain.CmdCloseAccount:
		resp.Data = json.RawMessage(`{"status":"closed"}`)
	default:
		resp.Success = false
		resp.Error = fmt.Sprintf("mock adapter: unknown command %q", p.Command.Command)
	}
	return resp
}
