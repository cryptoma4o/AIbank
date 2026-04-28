package provider

import (
	"context"
	"net/http"
	"time"

	"aibank/ext-rosfinmon/internal/domain"
)

// DefaultLiveEndpoint — placeholder URL feed'а Росфинмониторинга.
//
// Перечень 115-ФЗ публикуется ежедневно как XML-документ на www.fedsfm.ru.
// Точный URL после авторизации — TBD (ФСФМ требует подписанное соглашение).
const DefaultLiveEndpoint = "https://www.fedsfm.ru/documents/terr-list"

// LiveConfig — параметры подключения к feed'у Росфинмониторинга.
type LiveConfig struct {
	// Endpoint — URL XML-feed (env RFM_LIVE_ENDPOINT).
	Endpoint string
	// Timeout — общий timeout HTTP-клиента.
	Timeout time.Duration
	// CertPath/KeyPath — mTLS сертификат для доступа к закрытому каналу ФСФМ.
	CertPath string
	KeyPath  string
	// APIKey — резервная схема (если используется bearer-токен).
	APIKey string
}

// LiveProvider — заглушка реального клиента ФСФМ.
//
// TODO(integration): реализация требует:
//   - реальный URL feed'а (DefaultLiveEndpoint указывает на публичную страницу,
//     актуальный закрытый XML отдаётся только участникам ПОД/ФТ-периметра);
//   - mTLS / API-ключ от Росфинмониторинга по соглашению о взаимодействии;
//   - daily cron-задача (06:00 МСК) — выгружать XML, парсить, индексировать
//     в local store (PostgreSQL + GIN-индекс по ФИО/ИНН/birth_date);
//   - дельта-сравнение со вчерашним снапшотом → нотификация SOC при появлении
//     ИНН действующих клиентов в перечне (см. § 4.2 docs/security-architecture.md);
//   - PII-фильтр в логах, чтобы FullName не уходили в Loki в открытом виде;
//   - audit log каждого скрининга с подписью (115-ФЗ § 7 + 152-ФЗ);
//   - circuit breaker на endpoint и offline-fallback (старый снапшот) при
//     недоступности feed'а — закрытие двери стоит дороже, чем устаревший на
//     24 часа результат.
//
// Документация:
//   - http://www.fedsfm.ru/documents/terr-list (публичная страница)
//   - 115-ФЗ «О противодействии легализации (отмыванию) доходов»
//   - Положение Банка России 375-П (правила внутреннего контроля)
type LiveProvider struct {
	cfg    LiveConfig
	client *http.Client
}

// NewLiveProvider собирает заглушку клиента ФСФМ.
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

// Screen — TODO: проверять субъект по локально-индексированному перечню.
//
// Шаги после флипа RFM_LIVE=true:
//  1. Поднять daily-job, скачивающую XML-feed с p.cfg.Endpoint.
//  2. Распарсить (xml.Decoder), нормализовать ФИО (NFKD + lowercase + удаление пунктуации).
//  3. Загрузить в локальный индекс (PostgreSQL pg_trgm / Elasticsearch).
//  4. В Screen() — точный матч по ИНН + фуззи по ФИО+дате рождения с порогом 0.85.
//  5. Сохранять SourceRecord для аудита (origin: 115-FZ:terrorist|extremist|...).
func (p *LiveProvider) Screen(_ context.Context, _ domain.ScreeningRequest) (domain.ScreeningResult, error) {
	// TODO(rosfinmon): real lookup over indexed snapshot
	return domain.ScreeningResult{}, ErrNotImplemented
}

// GetSnapshot — TODO: вернуть метаданные последнего успешно загруженного XML-снапшота.
func (p *LiveProvider) GetSnapshot(_ context.Context) (domain.ListSnapshot, error) {
	// TODO(rosfinmon): read snapshot metadata from local store
	return domain.ListSnapshot{}, ErrNotImplemented
}
