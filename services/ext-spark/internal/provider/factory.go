package provider

import (
	"os"
	"strings"
	"time"
)

// EnvLiveFlag — имя переменной окружения, переключающей провайдер в live-режим.
const EnvLiveFlag = "SPARK_LIVE"

// BuildProvider читает ENV и возвращает подходящий Provider.
//
// По умолчанию — SyntheticProvider. Если SPARK_LIVE=true — LiveProvider.
//
// Дополнительные ENV для live-режима:
//
//	SPARK_LIVE_ENDPOINT   — базовый URL (default DefaultLiveEndpoint)
//	SPARK_LIVE_TIMEOUT_MS — timeout, мс (default 10000)
//	SPARK_LIVE_API_KEY    — API-ключ провайдера
//	SPARK_LIVE_CERT_PATH  — опц. mTLS-сертификат
//	SPARK_LIVE_KEY_PATH   — опц. mTLS-ключ
func BuildProvider() Provider {
	if !envBool(EnvLiveFlag) {
		return NewSyntheticProvider()
	}
	cfg := LiveConfig{
		Endpoint: os.Getenv("SPARK_LIVE_ENDPOINT"),
		Timeout:  envDurationMS("SPARK_LIVE_TIMEOUT_MS", 10*time.Second),
		APIKey:   os.Getenv("SPARK_LIVE_API_KEY"),
		CertPath: os.Getenv("SPARK_LIVE_CERT_PATH"),
		KeyPath:  os.Getenv("SPARK_LIVE_KEY_PATH"),
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
