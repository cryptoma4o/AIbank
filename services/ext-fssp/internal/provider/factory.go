package provider

import (
	"os"
	"strings"
	"time"
)

// EnvLiveFlag — имя переменной окружения, переключающей провайдер в live-режим.
const EnvLiveFlag = "FSSP_LIVE"

// BuildProvider читает ENV и возвращает подходящий Provider.
//
// По умолчанию — SyntheticProvider. Если FSSP_LIVE=true — LiveProvider.
//
// Дополнительные ENV для live-режима:
//
//	FSSP_LIVE_ENDPOINT   — базовый URL (default DefaultLiveEndpoint)
//	FSSP_LIVE_TIMEOUT_MS — timeout, мс (default 10000)
//	FSSP_LIVE_CERT_PATH  — путь к сертификату (PEM)
//	FSSP_LIVE_KEY_PATH   — путь к ключу (PEM)
//	FSSP_LIVE_API_KEY    — токен (опц.)
func BuildProvider() Provider {
	if !envBool(EnvLiveFlag) {
		return NewSyntheticProvider()
	}
	cfg := LiveConfig{
		Endpoint: os.Getenv("FSSP_LIVE_ENDPOINT"),
		Timeout:  envDurationMS("FSSP_LIVE_TIMEOUT_MS", 10*time.Second),
		CertPath: os.Getenv("FSSP_LIVE_CERT_PATH"),
		KeyPath:  os.Getenv("FSSP_LIVE_KEY_PATH"),
		APIKey:   os.Getenv("FSSP_LIVE_API_KEY"),
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
