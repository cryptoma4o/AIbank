package esia

import (
	"os"
	"strings"
)

// Env-ключи фабрики провайдеров.
const (
	EnvLive               = "ESIA_LIVE"
	EnvClientID           = "ESIA_CLIENT_ID"
	EnvClientSecretPEMPath = "ESIA_CLIENT_SECRET_PEM_PATH"
	EnvScope              = "ESIA_SCOPE"
)

// BuildProvider читает окружение и возвращает StubProvider или LiveProvider.
//
// Контракт:
//   - ESIA_LIVE != "true" (любое значение, кроме "true"/"1"/"yes")  → Stub.
//   - ESIA_LIVE == "true" → LiveProvider; client_id может быть пустым на
//     этапе разработки — Exchange всё равно вернёт ErrNotImplemented.
//
// Дефолт — Stub: безопасно для всех non-prod окружений (dev/test/staging).
// Включение LiveProvider в проде должно сопровождаться commit'ом ADR.
func BuildProvider() Provider {
	if !isLiveEnabled(os.Getenv(EnvLive)) {
		return NewStubProvider()
	}
	return NewLiveProvider(LiveProviderConfig{
		ClientID:            os.Getenv(EnvClientID),
		ClientSecretPEMPath: os.Getenv(EnvClientSecretPEMPath),
		Scope:               os.Getenv(EnvScope),
	})
}

func isLiveEnabled(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}
