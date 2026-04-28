package provider

import (
	"context"
	"net/http"
	"time"

	"aibank/ext-spark/internal/domain"
)

// DefaultLiveEndpoint — placeholder API СПАРК-Интерфакс.
//
// Рассматриваем два source-of-truth:
//   - https://api.sparkinterfax.ru/v1 — СПАРК-Интерфакс (исторически основной)
//   - https://api.kontur.ru/focus/v1   — Контур.Фокус (аналог, чаще выбирается
//     банками среднего сегмента из-за ценообразования)
//
// Конкретный provider выбирается per-tenant в configs/tenants/<tenant>/integrations/.
const DefaultLiveEndpoint = "https://api.sparkinterfax.ru/v1"

// LiveConfig — параметры подключения к СПАРК / Контур.Фокус.
type LiveConfig struct {
	// Endpoint — базовый URL (env SPARK_LIVE_ENDPOINT).
	Endpoint string
	// Timeout — общий timeout HTTP.
	Timeout time.Duration
	// APIKey — основная схема аутентификации (header X-API-Key или Bearer).
	APIKey string
	// CertPath/KeyPath — опц. mTLS, если требуется тенантом.
	CertPath string
	KeyPath  string
}

// LiveProvider — заглушка реального клиента СПАРК / Контур.Фокус.
//
// TODO(integration): реализация требует:
//   - выбор провайдера: СПАРК-Интерфакс vs Контур.Фокус (per-tenant);
//   - API-ключ от провайдера (получается по договору) — хранится в Vault,
//     прокидывается в env SPARK_LIVE_API_KEY;
//   - per-tenant контракт в configs/tenants/<tenant>/integrations/spark.yaml
//     с указанием endpoint/key-secret-path/quota;
//   - дневной лимит запросов по тарифу (типично 1000-10000/день);
//   - TTL кэша согласован с обновлением источника (СПАРК обновляет данные раз
//     в сутки → 24h; Контур.Фокус — варьируется);
//   - mapping ответа провайдера → domain.SparkIntel (struct-level normalization,
//     т.к. форматы СПАРК и Фокус существенно отличаются);
//   - PII-фильтр в логах: news_summary может содержать упоминания персон —
//     перед записью в Loki фильтруем по 152-ФЗ (см. § 2.2 STRIDE);
//   - graceful degradation: если внешний API недоступен, возвращаем
//     закэшированный результат + флаг degraded в metadata, не блокируем KYC.
//
// Документация:
//   - https://www.spark-interfax.ru/api (СПАРК-Интерфакс API docs)
//   - https://kontur.ru/focus/api      (Контур.Фокус API docs)
type LiveProvider struct {
	cfg    LiveConfig
	client *http.Client
}

// NewLiveProvider собирает заглушку клиента.
func NewLiveProvider(cfg LiveConfig) *LiveProvider {
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultLiveEndpoint
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &LiveProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

// Name — имя реализации.
func (p *LiveProvider) Name() string { return "live" }

// GetIntel — TODO: вызвать API провайдера.
//
// Шаги после флипа SPARK_LIVE=true:
//  1. Сформировать GET p.cfg.Endpoint + "/companies/{inn}/intel".
//  2. Установить header "X-API-Key" из p.cfg.APIKey (или Authorization: Bearer).
//  3. Распарсить JSON-ответ и смапить → domain.SparkIntel.
//  4. Обработать коды ошибок: 404 (нет данных), 429 (rate limit), 5xx (degrade).
func (p *LiveProvider) GetIntel(_ context.Context, _ string) (domain.SparkIntel, error) {
	// TODO(spark): real call here
	return domain.SparkIntel{}, ErrNotImplemented
}
