// Package audit — тонкая обёртка вокруг packages/audit-sdk для ubo-service.
// Lazy singleton: клиент инициализируется один раз через sync.Once и
// переиспользуется во всех handler-middleware.
//
// Интеграция следует ADR-0010: создание новой версии UBO-графа фиксируется
// в append-only audit log (event_type=ubo_graph.created) для compliance
// (115-ФЗ § 7.1) и для биллинга (per-event тарификация УБО-проверки).
package audit

import (
	"log/slog"
	"os"
	"sync"
	"time"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
)

// EnvBaseURL — переменная окружения с базовым URL audit-service.
const EnvBaseURL = "AUDIT_SERVICE_URL"

// DefaultBaseURL — путь до audit-service в k8s-кластере по умолчанию.
const DefaultBaseURL = "http://audit-service:8081"

var (
	once     sync.Once
	cached   *auditsdk.Client
	cacheErr error
)

// Client возвращает singleton audit-клиента. Если AUDIT_SERVICE_URL не
// задан, используется DefaultBaseURL. Возвращает nil-клиент с error,
// когда URL некорректен.
func Client() (*auditsdk.Client, error) {
	once.Do(func() {
		baseURL := os.Getenv(EnvBaseURL)
		if baseURL == "" {
			baseURL = DefaultBaseURL
		}
		cached, cacheErr = auditsdk.NewClient(auditsdk.ClientOptions{
			BaseURL: baseURL,
			Timeout: 5 * time.Second,
			APIKey:  os.Getenv("AUDIT_API_KEY"),
		})
	})
	return cached, cacheErr
}

// MustClient возвращает singleton client; log используется для
// предупреждений, если переменная окружения отсутствует. Никогда не
// падает: при ошибке возвращает nil, чтобы handler-middleware мог
// безопасно сгладить отсутствие audit-эмиттера в DEV-окружении.
func MustClient(log *slog.Logger) *auditsdk.Client {
	if log == nil {
		log = slog.Default()
	}
	if os.Getenv(EnvBaseURL) == "" {
		log.Warn("audit: AUDIT_SERVICE_URL не задан, используется значение по умолчанию",
			"default", DefaultBaseURL)
	}
	c, err := Client()
	if err != nil {
		log.Warn("audit: не удалось инициализировать клиент, события не будут отправлены",
			"err", err)
		return nil
	}
	return c
}

// ResetForTest сбрасывает singleton — нужен только для unit-тестов,
// которые подменяют URL audit-service. В проде не вызывать.
func ResetForTest() {
	once = sync.Once{}
	cached = nil
	cacheErr = nil
}
