// Package extclients — HTTP-клиенты к внешним сервисам ЕГРЮЛ / Росфинмониторинг
// / ФССП для предварительного скоринга (этап 1 формы онбординга,
// см. docs/onboarding-form-spec.md §1).
//
// Каждый клиент возвращает структурированный результат + bool «доступен ли
// сервис», чтобы handler мог собрать сводный PrequalificationCheck даже при
// частичной недоступности downstream'ов.
package extclients

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

const defaultTimeout = 5 * time.Second

// EGRULData — подмножество ответа ext-egrul/v1/egrul/by-inn/{inn}, которое
// нужно для PrequalificationCheck. Полный shape — в services/ext-egrul/
// internal/domain/legal_entity.go.
type EGRULData struct {
	INN          string `json:"inn"`
	OGRN         string `json:"ogrn"`
	FullName     string `json:"full_name"`
	ShortName    string `json:"short_name"`
	Address      string `json:"address"`
	CEO          string `json:"ceo"`
	Status       string `json:"status"`
	RegisteredAt string `json:"registered_at"`
}

// RosfinmonResult — результат проверки по перечню 115-ФЗ. Соответствует
// services/ext-rosfinmon/internal/domain.ScreeningResult.
type RosfinmonResult struct {
	Matched         bool    `json:"matched"`
	ListName        string  `json:"list_name,omitempty"`
	MatchConfidence float64 `json:"match_confidence"`
}

// FSSPResult — сводка по исполнительным производствам ФССП.
type FSSPResult struct {
	Count            int   `json:"count"`
	TotalDebtKopecks int64 `json:"total_debt_kopecks"`
}

// Clients агрегирует ссылки на ext-сервисы. Используется handler'ом для
// параллельных вызовов; пустые URL — feature-флаг, источник просто
// помечается как недоступный.
type Clients struct {
	EGRULURL     string
	RosfinmonURL string
	FSSPURL      string
	HTTP         *http.Client
}

func New(egrulURL, rosfinmonURL, fsspURL string) *Clients {
	return &Clients{
		EGRULURL:     egrulURL,
		RosfinmonURL: rosfinmonURL,
		FSSPURL:      fsspURL,
		HTTP:         &http.Client{Timeout: defaultTimeout},
	}
}

// EGRULByINN — GET /v1/egrul/by-inn/{inn}.
func (c *Clients) EGRULByINN(ctx context.Context, inn string) (*EGRULData, error) {
	if c.EGRULURL == "" {
		return nil, errors.New("egrul_url not configured")
	}
	url := fmt.Sprintf("%s/v1/egrul/by-inn/%s", c.EGRULURL, inn)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("egrul status %d", resp.StatusCode)
	}
	var env struct {
		Data EGRULData `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env); err != nil {
		return nil, fmt.Errorf("egrul decode: %w", err)
	}
	return &env.Data, nil
}

// RosfinmonScreen — POST /v1/rosfinmon/screen для юрлица по ИНН.
func (c *Clients) RosfinmonScreen(ctx context.Context, inn string) (*RosfinmonResult, error) {
	if c.RosfinmonURL == "" {
		return nil, errors.New("rosfinmon_url not configured")
	}
	body, _ := json.Marshal(map[string]any{
		"subject_type": "legal_entity",
		"identifiers":  map[string]string{"inn": inn},
	})
	url := fmt.Sprintf("%s/v1/rosfinmon/screen", c.RosfinmonURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rosfinmon status %d", resp.StatusCode)
	}
	var env struct {
		Data RosfinmonResult `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env); err != nil {
		return nil, fmt.Errorf("rosfinmon decode: %w", err)
	}
	return &env.Data, nil
}

// FSSPByINN — GET /v1/fssp/by-inn/{inn}.
func (c *Clients) FSSPByINN(ctx context.Context, inn string) (*FSSPResult, error) {
	if c.FSSPURL == "" {
		return nil, errors.New("fssp_url not configured")
	}
	url := fmt.Sprintf("%s/v1/fssp/by-inn/%s", c.FSSPURL, inn)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return &FSSPResult{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fssp status %d", resp.StatusCode)
	}
	var env struct {
		Data FSSPResult `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env); err != nil {
		return nil, fmt.Errorf("fssp decode: %w", err)
	}
	return &env.Data, nil
}
