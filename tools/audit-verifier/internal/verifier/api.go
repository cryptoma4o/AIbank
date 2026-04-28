package verifier

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"time"
)

// APIStore читает события через HTTP API audit-service'а
// (GET /v1/events?tenant_id=...). Используется в production-runbook'е,
// где прямой доступ к БД у инцидент-респондера часто закрыт IAM,
// но порт audit-service'а доступен внутри кластера.
//
// Контракт ответа: см. handler.ListEvents — {items: [...], count: N}.
type APIStore struct {
	BaseURL    string
	HTTPClient *http.Client
	// Limit — chunk size при пагинации; audit-service ограничивает 1..1000.
	Limit int
}

// NewAPIStore конструирует store с разумными дефолтами.
func NewAPIStore(baseURL string) *APIStore {
	return &APIStore{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Limit:      1000,
	}
}

type listResponse struct {
	Items []Event `json:"items"`
	Count int     `json:"count"`
}

// ListEvents — стримит события по чанкам.
//
// audit-service возвращает события ORDER BY created_at DESC; мы
// разворачиваем порядок (от старого к новому) — этого требует
// инвариант VerifyChain'а. При больших объёмах это вынуждает
// загрузку в память: при росте >1M событий перейти на DBStore
// (PostgreSQL умеет ASC дёшево).
//
// Текущий API не поддерживает from/to — фильтрация делается
// клиентом после получения. Заведено как TODO ниже.
func (s *APIStore) ListEvents(ctx context.Context, tenantID string, from, to *time.Time) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		u, err := url.Parse(s.BaseURL)
		if err != nil {
			yield(Event{}, fmt.Errorf("parse base url: %w", err))
			return
		}
		u.Path = "/v1/events"
		q := u.Query()
		q.Set("tenant_id", tenantID)
		if s.Limit > 0 {
			q.Set("limit", fmt.Sprintf("%d", s.Limit))
		}
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			yield(Event{}, fmt.Errorf("build request: %w", err))
			return
		}
		resp, err := s.HTTPClient.Do(req)
		if err != nil {
			yield(Event{}, fmt.Errorf("http call: %w", err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			yield(Event{}, fmt.Errorf("audit-service returned %d", resp.StatusCode))
			return
		}
		var body listResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			yield(Event{}, fmt.Errorf("decode response: %w", err))
			return
		}
		// audit-service отдаёт DESC; цепочечная верификация требует ASC.
		for i := len(body.Items) - 1; i >= 0; i-- {
			ev := body.Items[i]
			if from != nil && ev.CreatedAt.Before(*from) {
				continue
			}
			if to != nil && ev.CreatedAt.After(*to) {
				continue
			}
			if !yield(ev, nil) {
				return
			}
		}
		// TODO(audit-verifier): API audit-service пока без курсорной
		// пагинации; для тенантов >1000 событий — использовать --source=db
		// или дождаться добавления ?cursor= параметра в handler.ListEvents.
	}
}
