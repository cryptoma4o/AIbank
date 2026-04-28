package provider

import (
	"context"
	"net/http"
	"time"

	"aibank/ext-fssp/internal/domain"
)

// DefaultLiveEndpoint — placeholder URL OpenAPI ФССП (банк данных исполнительных производств).
//
// ВНИМАНИЕ: точный URL TBD. ФССП предоставляет два контура: публичный
// fssp.gov.ru (с CAPTCHA, не пригоден для production) и закрытый IS-канал
// для участников по соглашению.
const DefaultLiveEndpoint = "https://fssp.gov.ru/iss/ip/v2"

// LiveConfig — параметры подключения к API ФССП.
type LiveConfig struct {
	// Endpoint — базовый URL ФССП (env FSSP_LIVE_ENDPOINT).
	Endpoint string
	// Timeout — общий timeout HTTP.
	Timeout time.Duration
	// CertPath/KeyPath — mTLS-сертификат «агента-получателя» по соглашению с ФССП.
	CertPath string
	KeyPath  string
	// APIKey — токен (если используется в закрытом контуре).
	APIKey string
}

// LiveProvider — заглушка реального клиента ФССП.
//
// TODO(integration): реализация требует:
//   - реальный URL ФССП (DefaultLiveEndpoint — placeholder; публичный fssp.gov.ru
//     требует CAPTCHA и не покрывает batch-сценарии — для production нужен
//     закрытый канал по соглашению об информационном взаимодействии);
//   - регистрация юрлица как «агента-получателя» (agent-of-record) в ФССП;
//   - mTLS-сертификат, выпущенный по соглашению (КриптоПро / аккредитованный УЦ);
//   - решение CAPTCHA, если используется публичный контур (anti-captcha service
//     запрещён по compliance — только закрытый контур);
//   - rate limit: согласно регламенту обычно 60 RPS на агента + дневной лимит
//     500K запросов; circuit breaker + ретраи с экспоненциальной паузой;
//   - mapping XML/JSON ответа ФССП → domain.ProceedingsResult;
//   - PII-фильтр в логах: ФИО/дата рождения должника не должны утекать в Loki
//     в открытом виде (см. § 2.2 STRIDE/Information disclosure);
//   - audit log каждого запроса (kind=ext-fssp) с подписью.
//
// Документация:
//   - https://fssp.gov.ru/iss/ip/ (публичный сервис)
//   - 229-ФЗ «Об исполнительном производстве» § 6.1 (доступ к банку данных)
//   - Регламент ФССП о порядке предоставления сведений
type LiveProvider struct {
	cfg    LiveConfig
	client *http.Client
}

// NewLiveProvider собирает заглушку клиента ФССП.
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

// GetByINN — TODO: вызвать ФССП API getProceedingsByInn.
func (p *LiveProvider) GetByINN(_ context.Context, _ string) (domain.ProceedingsResult, error) {
	// TODO(fssp): real call here
	return domain.ProceedingsResult{}, ErrNotImplemented
}

// GetByPerson — TODO: вызвать ФССП API getProceedingsByPerson.
//
// Поиск идёт по ФИО+дате рождения с возможным регионом (опц.). Результат —
// список активных и закрытых производств.
func (p *LiveProvider) GetByPerson(_ context.Context, _, _ string) (domain.ProceedingsResult, error) {
	// TODO(fssp): real call here
	return domain.ProceedingsResult{}, ErrNotImplemented
}
