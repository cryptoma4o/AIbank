package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"aibank/ext-egrul/internal/domain"
)

// DefaultLiveEndpoint — placeholder реальной точки входа ФНС ЕГРЮЛ/ЕГРИП.
//
// ВНИМАНИЕ: это не публичный URL — реальный endpoint выдаётся ФНС после заключения
// договора через Минцифры/СМЭВ. Используется только как seed для конфигурации.
// На pre-integration стенде сюда подставляется адрес mock-smev (tools/mock-smev),
// например http://mock-smev:8500.
const DefaultLiveEndpoint = "https://api.nalog.ru/egrul/v1"

// liveRetry* — параметры retry-стратегии. Экспоненциальный backoff:
// 100ms → 200ms → 400ms (max 3 попытки на запрос).
const (
	liveRetryMaxAttempts  = 3
	liveRetryInitialDelay = 100 * time.Millisecond

	// circuitFailureThreshold — после скольких подряд идущих 5xx/transport-ошибок
	// breaker переходит в open.
	circuitFailureThreshold = 5
	// circuitOpenDuration — на сколько open-состояние блокирует все запросы.
	circuitOpenDuration = 30 * time.Second
)

// LiveConfig — параметры подключения к реальному API ФНС / mock-серверу SMEV.
type LiveConfig struct {
	// Endpoint — базовый URL upstream (env EGRUL_LIVE_ENDPOINT).
	Endpoint string
	// Timeout — общий timeout HTTP-клиента (per-request).
	Timeout time.Duration
	// CertPath — путь к УКЭП-сертификату для mTLS (PEM). На mock-стенде не используется.
	CertPath string
	// KeyPath — путь к приватному ключу.
	KeyPath string
	// APIKey — резервная схема (если ФНС выдаёт API-ключ вместо mTLS).
	APIKey string
	// Logger — slog для аудита запросов; при nil используется slog.Default().
	Logger *slog.Logger
}

// LiveProvider — реальный клиент SMEV3-ФНС ЕГРЮЛ.
//
// Текущее состояние (pre-integration):
//   - HTTP-клиент работает против mock-smev (tools/mock-smev) или реального ФНС.
//   - XML-ответ парсится через xml_mapper.MapXMLToLegalEntity.
//   - Реализованы retry с экспоненциальным backoff и простой circuit breaker.
//   - Аудит идёт через slog (TODO: подключить packages/audit-sdk через replace
//     директиву отдельным PR — добавить kind=ext-fns-egrul с подписью).
//
// TODO(integration) после получения ФНС-договора:
//   - mTLS с УКЭП-сертификатом (КриптоПро CSP / Vault Transit) из CertPath/KeyPath;
//   - Соблюдение rate limit (по договору обычно 100 RPS на тенанта,
//     burst до 500, дневной лимит 1М запросов);
//   - Outbound whitelist (см. § 4.2 docs/security-architecture.md);
//   - Распознавание кодов ошибок ФНС (ЕНТ-1003 «не найдено» → ErrNotFound,
//     4031 «нет доступа» и т.п.);
//   - PII-фильтр для исходящих логов (ИНН/ОГРН не маскируем — это идентификаторы
//     запроса).
//
// Документация:
//   - https://www.nalog.gov.ru/rn77/related_activities/statistics_and_analytics/forms/ (открытые данные)
//   - СМЭВ-3 методические рекомендации
//   - 152-ФЗ § 18.1 (защита ПДн при обработке)
type LiveProvider struct {
	cfg    LiveConfig
	client *http.Client
	logger *slog.Logger

	// breaker — простой circuit breaker (см. circuitFailureThreshold/Duration).
	breaker circuitBreaker
}

// NewLiveProvider собирает HTTP-клиент с заданным timeout.
func NewLiveProvider(cfg LiveConfig) *LiveProvider {
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultLiveEndpoint
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &LiveProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
		logger: logger,
	}
}

// Name — имя реализации.
func (p *LiveProvider) Name() string { return "live" }

// GetByINN — карточка ЮЛ/ИП по ИНН.
//
// Шаги:
//  1. POST/GET на p.cfg.Endpoint + "/by-inn/{inn}" (на mock-стенде — GET; на ФНС
//     по договору будет POST с SOAP-envelope, см. TODO выше).
//  2. Распарсить XML через xml_mapper.MapXMLToLegalEntity.
//  3. 404 от upstream → ErrNotFound; 5xx — retry до 3 раз; circuit breaker на 5
//     подряд ошибок.
func (p *LiveProvider) GetByINN(ctx context.Context, inn string) (domain.LegalEntity, error) {
	body, err := p.fetchXML(ctx, "/by-inn/"+inn, "by-inn", inn)
	if err != nil {
		return domain.LegalEntity{}, err
	}
	le, err := MapXMLToLegalEntity(body)
	if err != nil {
		return domain.LegalEntity{}, fmt.Errorf("%w: parse xml: %v", ErrUpstream, err)
	}
	return le, nil
}

// GetByOGRN — карточка по ОГРН/ОГРНИП. Использует тот же endpoint /by-ogrn/{ogrn}.
func (p *LiveProvider) GetByOGRN(ctx context.Context, ogrn string) (domain.LegalEntity, error) {
	body, err := p.fetchXML(ctx, "/by-ogrn/"+ogrn, "by-ogrn", ogrn)
	if err != nil {
		return domain.LegalEntity{}, err
	}
	le, err := MapXMLToLegalEntity(body)
	if err != nil {
		return domain.LegalEntity{}, fmt.Errorf("%w: parse xml: %v", ErrUpstream, err)
	}
	return le, nil
}

// GetFounders — учредители ЮЛ. ФНС отдаёт их в составе getCompanyByInn,
// поэтому переиспользуем /by-inn/{inn} и берём founders из распарсенной карточки.
func (p *LiveProvider) GetFounders(ctx context.Context, inn string) ([]domain.Founder, error) {
	le, err := p.GetByINN(ctx, inn)
	if err != nil {
		return nil, err
	}
	if le.Founders == nil {
		return []domain.Founder{}, nil
	}
	return le.Founders, nil
}

// fetchXML выполняет HTTP-запрос с retry и circuit breaker, возвращает тело ответа.
//
// kind/identifier используются только для аудит-логов (audit emit).
func (p *LiveProvider) fetchXML(ctx context.Context, path, kind, identifier string) ([]byte, error) {
	if p.breaker.isOpen(time.Now()) {
		p.audit(kind, identifier, "circuit_open", 0, 0, ErrCircuitOpen)
		return nil, ErrCircuitOpen
	}

	url := strings.TrimRight(p.cfg.Endpoint, "/") + path
	var lastErr error
	delay := liveRetryInitialDelay
	start := time.Now()

	for attempt := 1; attempt <= liveRetryMaxAttempts; attempt++ {
		body, status, err := p.doOnce(ctx, url)
		latency := time.Since(start)

		// 200 — успех.
		if err == nil && status == http.StatusOK {
			p.breaker.onSuccess()
			p.audit(kind, identifier, "ok", status, latency, nil)
			return body, nil
		}

		// 404 — финальная ошибка, без retry, без счётчика breaker'а.
		if err == nil && status == http.StatusNotFound {
			p.audit(kind, identifier, "not_found", status, latency, nil)
			return nil, ErrNotFound
		}

		// Не-5xx (4xx кроме 404) — финальная upstream-ошибка без retry,
		// но и без увеличения breaker (это не «нездоровый» upstream).
		if err == nil && status >= 400 && status < 500 {
			p.audit(kind, identifier, "client_error", status, latency, nil)
			return nil, fmt.Errorf("%w: статус %d", ErrUpstream, status)
		}

		// 5xx или transport error — кандидат на retry и инкремент breaker'а.
		p.breaker.onFailure(time.Now())
		if err != nil {
			lastErr = fmt.Errorf("%w: %v", ErrUpstream, err)
		} else {
			lastErr = fmt.Errorf("%w: статус %d", ErrUpstream, status)
		}
		p.audit(kind, identifier, "retry", status, latency, lastErr)

		// Если breaker только что открылся — прекращаем попытки.
		if p.breaker.isOpen(time.Now()) {
			return nil, ErrCircuitOpen
		}

		if attempt < liveRetryMaxAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
	}

	p.audit(kind, identifier, "exhausted", 0, time.Since(start), lastErr)
	return nil, lastErr
}

// doOnce — один HTTP-запрос без retry. Возвращает тело, статус и transport error.
func (p *LiveProvider) doOnce(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/xml")
	if p.cfg.APIKey != "" {
		req.Header.Set("X-API-Key", p.cfg.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// audit — единая точка emit аудита запроса.
//
// TODO(audit-sdk): после подключения packages/audit-sdk (отдельный PR) тут
// должна формироваться запись kind=ext-fns-egrul с подписью HMAC и persistent
// log; сейчас используется slog (см. STRIDE/Repudiation в docs/security-architecture.md).
func (p *LiveProvider) audit(kind, identifier, outcome string, status int, latency time.Duration, err error) {
	args := []any{
		"kind", "ext-fns-egrul",
		"op", kind,
		"identifier", identifier,
		"outcome", outcome,
		"http_status", status,
		"latency_ms", latency.Milliseconds(),
		"endpoint", p.cfg.Endpoint,
	}
	if err != nil {
		args = append(args, "err", err.Error())
	}
	if errors.Is(err, ErrCircuitOpen) || outcome == "exhausted" {
		p.logger.Warn("ext-egrul live audit", args...)
		return
	}
	p.logger.Info("ext-egrul live audit", args...)
}

// circuitBreaker — простой counter-based breaker.
//
// Логика:
//   - failures < threshold              → closed, запросы пропускаются;
//   - failures ≥ threshold              → open до openedAt + circuitOpenDuration;
//   - после истечения окна              → half-open: первый запрос проходит, на
//     успехе — closed, на ошибке — снова open.
//
// Простой и thread-safe; sufficient для pre-integration. Прод-уровневый
// hystrix-style state machine можно подключить позже (gobreaker и т.п.).
type circuitBreaker struct {
	mu       sync.Mutex
	failures int
	openedAt time.Time
}

func (b *circuitBreaker) isOpen(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures < circuitFailureThreshold {
		return false
	}
	if now.Sub(b.openedAt) >= circuitOpenDuration {
		// half-open: пропускаем один запрос, не сбрасывая счётчик; финальный
		// onSuccess/onFailure определит дальнейшее состояние.
		return false
	}
	return true
}

func (b *circuitBreaker) onSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openedAt = time.Time{}
}

func (b *circuitBreaker) onFailure(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= circuitFailureThreshold {
		b.openedAt = now
	}
}
