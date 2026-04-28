package provider

import (
	"os"
	"strings"
	"time"
)

// EnvLiveFlag — имя переменной окружения, переключающей провайдер в live-режим.
const EnvLiveFlag = "RFM_LIVE"

// BuildProvider читает ENV и возвращает подходящий Provider.
//
// По умолчанию — SyntheticProvider. Если RFM_LIVE=true — LiveProvider.
//
// Дополнительные ENV для live-режима:
//
//	RFM_LIVE_ENDPOINT   — URL XML-feed (default DefaultLiveEndpoint)
//	RFM_LIVE_TIMEOUT_MS — timeout HTTP, мс (default 10000)
//	RFM_LIVE_CERT_PATH  — путь к сертификату (PEM)
//	RFM_LIVE_KEY_PATH   — путь к ключу (PEM)
//	RFM_LIVE_API_KEY    — bearer-токен (если применимо)
func BuildProvider() Provider {
	if !envBool(EnvLiveFlag) {
		return NewSyntheticProvider()
	}
	cfg := LiveConfig{
		Endpoint: os.Getenv("RFM_LIVE_ENDPOINT"),
		Timeout:  envDurationMS("RFM_LIVE_TIMEOUT_MS", 10*time.Second),
		CertPath: os.Getenv("RFM_LIVE_CERT_PATH"),
		KeyPath:  os.Getenv("RFM_LIVE_KEY_PATH"),
		APIKey:   os.Getenv("RFM_LIVE_API_KEY"),
	}
	return NewLiveProvider(cfg)
}

func envBool(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

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
