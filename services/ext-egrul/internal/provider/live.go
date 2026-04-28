package provider

import (
	"context"
	"net/http"
	"time"

	"aibank/ext-egrul/internal/domain"
)

// DefaultLiveEndpoint — placeholder реальной точки входа ФНС ЕГРЮЛ/ЕГРИП.
//
// ВНИМАНИЕ: это не публичный URL — реальный endpoint выдаётся ФНС после заключения
// договора через Минцифры/СМЭВ. Используется только как seed для конфигурации.
const DefaultLiveEndpoint = "https://api.nalog.ru/egrul/v1"

// LiveConfig — параметры подключения к реальному API ФНС.
type LiveConfig struct {
	// Endpoint — базовый URL upstream (env EGRUL_LIVE_ENDPOINT).
	Endpoint string
	// Timeout — общий timeout HTTP-клиента.
	Timeout time.Duration
	// CertPath — путь к УКЭП-сертификату для mTLS (PEM).
	CertPath string
	// KeyPath — путь к приватному ключу.
	KeyPath string
	// APIKey — резервная схема (если ФНС выдаёт API-ключ вместо mTLS).
	APIKey string
}

// LiveProvider — заглушка реального клиента ФНС.
//
// TODO(integration): реализация требует:
//   - реальный endpoint ФНС (DefaultLiveEndpoint — placeholder; конкретный URL
//     получается через СМЭВ-3/Минцифры после заключения договора);
//   - mTLS с УКЭП-сертификатом, выпущенным аккредитованным УЦ ФНС
//     (КриптоПро CSP / Vault Transit);
//   - соблюдение rate limit (по договору обычно 100 RPS на тенанта,
//     burst до 500, дневной лимит 1М запросов);
//   - circuit breaker + ретраи с экспоненциальной паузой (см. § 4.2 docs/security-architecture.md
//     — outbound whitelist, через который только и можно ходить);
//   - mapping ответа ФНС (XML/JSON) → domain.LegalEntity;
//   - audit log каждого запроса (kind=ext-fns-egrul) с подписью (см. § 2.2 STRIDE/Repudiation);
//   - PII-фильтр для исходящих логов (ИНН/ОГРН не маскируем — это идентификаторы запроса).
//
// Документация:
//   - https://www.nalog.gov.ru/rn77/related_activities/statistics_and_analytics/forms/ (открытые данные)
//   - СМЭВ-3 методические рекомендации
//   - 152-ФЗ § 18.1 (защита ПДн при обработке)
type LiveProvider struct {
	cfg    LiveConfig
	client *http.Client
}

// NewLiveProvider собирает заглушку клиента с заданным timeout.
//
// HTTP-клиент сконфигурирован, но реально не используется до завершения интеграции.
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

// GetByINN — TODO: вызвать ФНС API (метод getCompanyByInn / getEntrepreneurByInn).
//
// Ожидаемые шаги после флипа флага:
//  1. Сформировать SOAP/REST-запрос на p.cfg.Endpoint + "/by-inn/{inn}".
//  2. Подписать УКЭП (КриптоПро) и установить mTLS из p.cfg.Cert*.
//  3. Распарсить XML-ответ ФНС и смапить в domain.LegalEntity.
//  4. Распознать коды ошибок ФНС (ЕНТ-1003 «не найдено», 4031 «нет доступа» и т.п.).
func (p *LiveProvider) GetByINN(_ context.Context, _ string) (domain.LegalEntity, error) {
	// TODO(fns): real call here
	return domain.LegalEntity{}, ErrNotImplemented
}

// GetByOGRN — TODO: вызвать ФНС API (метод getCompanyByOgrn / getEntrepreneurByOgrnip).
func (p *LiveProvider) GetByOGRN(_ context.Context, _ string) (domain.LegalEntity, error) {
	// TODO(fns): real call here
	return domain.LegalEntity{}, ErrNotImplemented
}

// GetFounders — TODO: ФНС отдаёт учредителей в составе getCompanyByInn,
// нужно либо переиспользовать GetByINN-ответ, либо звать отдельный getFounders, если он есть.
func (p *LiveProvider) GetFounders(_ context.Context, _ string) ([]domain.Founder, error) {
	// TODO(fns): real call here
	return nil, ErrNotImplemented
}
