package provider

import (
	"os"
	"strings"
	"time"
)

// EnvLiveFlag — имя переменной окружения, переключающей провайдер в live-режим.
const EnvLiveFlag = "EGRUL_LIVE"

// BuildProvider читает ENV и возвращает подходящий Provider.
//
// По умолчанию — SyntheticProvider. Если EGRUL_LIVE=true (или 1, yes, on) —
// возвращает LiveProvider, читая endpoint/timeout/cert из ENV.
//
// Дополнительные переменные окружения для live-режима:
//
//	EGRUL_LIVE_ENDPOINT   — базовый URL ФНС (default DefaultLiveEndpoint)
//	EGRUL_LIVE_TIMEOUT_MS — timeout HTTP-клиента, мс (default 10000)
//	EGRUL_LIVE_CERT_PATH  — путь к УКЭП-сертификату (PEM)
//	EGRUL_LIVE_KEY_PATH   — путь к приватному ключу (PEM)
//	EGRUL_LIVE_API_KEY    — резервная схема (API-ключ)
func BuildProvider() Provider {
	if !envBool(EnvLiveFlag) {
		return NewSyntheticProvider()
	}
	cfg := LiveConfig{
		Endpoint: os.Getenv("EGRUL_LIVE_ENDPOINT"),
		Timeout:  envDurationMS("EGRUL_LIVE_TIMEOUT_MS", 10*time.Second),
		CertPath: os.Getenv("EGRUL_LIVE_CERT_PATH"),
		KeyPath:  os.Getenv("EGRUL_LIVE_KEY_PATH"),
		APIKey:   os.Getenv("EGRUL_LIVE_API_KEY"),
	}
	return NewLiveProvider(cfg)
}

// envBool — true для значений 1/true/yes/on (case-insensitive).
func envBool(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// envDurationMS читает переменную как миллисекунды; при отсутствии/ошибке возвращает def.
func envDurationMS(name string, def time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	ms, err := time.ParseDuration(v + "ms")
	if err != nil || ms <= 0 {
		return def
	}
	return ms
}
