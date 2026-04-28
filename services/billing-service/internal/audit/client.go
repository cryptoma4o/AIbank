// Package audit — тонкая обёртка вокруг packages/audit-sdk для
// billing-service.  Lazy singleton.
//
// Интеграция следует ADR-0010: каждый billing event эмитит audit-event с
// тем же correlation_id, чтобы при сверке с банком по строке биллинга
// можно было найти связанные доменные события.
package audit

import (
	"log/slog"
	"os"
	"sync"
	"time"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
)

const (
	EnvBaseURL     = "AUDIT_SERVICE_URL"
	DefaultBaseURL = "http://audit-service:8081"
)

var (
	once     sync.Once
	cached   *auditsdk.Client
	cacheErr error
)

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

func ResetForTest() {
	once = sync.Once{}
	cached = nil
	cacheErr = nil
}
