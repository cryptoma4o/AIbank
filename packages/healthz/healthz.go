// Package healthz реализует структурированные /health и /ready эндпоинты
// для Go-сервисов AIbank.  Использование одного пакета во всех сервисах
// обеспечивает единый JSON-формат для:
//   - Kubernetes liveness/readiness-проб (см. infrastructure/helm/charts/aibank-service);
//   - smoke-теста (scripts/smoke.sh) — он парсит status="ok";
//   - оркестрации между сервисами (например, billing → tenant-service /health).
//
// Дизайн-принципы:
//   - Liveness (HTTPHandler без зарегистрированных проверок) — всегда 200,
//     если процесс жив.  Используется k8s для решения о рестарте.
//   - Readiness (HTTPHandler со зарегистрированными CheckFunc) — 503,
//     если хотя бы одна обязательная проверка fail.  Используется k8s для
//     решения о трафике.
//   - Soft-режим: проверка может быть помечена как «soft» — её провал
//     понижает общий статус до "degraded", но возвращает 200.  Полезно
//     для опциональных зависимостей (Kafka, secondary cache).
//
// Зависимости: только stdlib.  Это сознательное ограничение, чтобы пакет
// можно было импортировать в любой сервис без расхождений по версиям
// chi / log libs.
package healthz

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Status — итоговый статус проверки или сервиса.
type Status string

const (
	// StatusHealthy — проверка прошла успешно.
	StatusHealthy Status = "healthy"
	// StatusUnhealthy — проверка упала; для readiness даёт 503.
	StatusUnhealthy Status = "unhealthy"
	// StatusDegraded — soft-проверка упала, ядро живо; для readiness даёт 200.
	StatusDegraded Status = "degraded"
)

// CheckFunc — единичная проверка.  Возвращает статус и человекочитаемое
// сообщение (для диагностики; в healthy-кейсе можно оставить "").
type CheckFunc func(ctx context.Context) (Status, string)

// CheckResult — результат одной проверки в JSON-ответе.
type CheckResult struct {
	Status   Status `json:"status"`
	Message  string `json:"message,omitempty"`
	Duration string `json:"duration,omitempty"`
}

// Response — формат тела ответа /health и /ready.
//
// Контракт:
//   - status="ok"        — все обязательные checks healthy;
//   - status="degraded"  — все ядерные checks healthy, но soft-проверки fail;
//   - status="error"     — хотя бы одна обязательная проверка fail (HTTP 503).
type Response struct {
	Status    string                 `json:"status"`
	Service   string                 `json:"service"`
	Version   string                 `json:"version,omitempty"`
	Timestamp string                 `json:"timestamp"`
	Checks    map[string]CheckResult `json:"checks,omitempty"`
}

// registeredCheck — внутреннее представление зарегистрированной проверки.
type registeredCheck struct {
	name    string
	fn      CheckFunc
	soft    bool
	timeout time.Duration
}

// Option — опция Register для тонкой настройки поведения проверки.
type Option func(*registeredCheck)

// Soft помечает проверку как «не блокирующая readiness».  Если такая
// проверка упадёт, общий статус будет "degraded", HTTP 200.
func Soft() Option {
	return func(c *registeredCheck) { c.soft = true }
}

// Timeout задаёт таймаут на конкретную проверку.  По умолчанию 2s.
func Timeout(d time.Duration) Option {
	return func(c *registeredCheck) { c.timeout = d }
}

// HealthChecker аккумулирует список проверок и отдаёт http.Handler.
//
// Концептуально это «реестр CheckFunc + JSON-renderer».  Конкретные
// проверки (БД, upstream HTTP, Kafka) живут в builders.go и в коде
// сервиса.  HealthChecker потокобезопасен: Register/HTTPHandler можно
// дёргать из разных горутин.
type HealthChecker struct {
	service string
	version string

	mu     sync.RWMutex
	checks []registeredCheck
}

// New создаёт HealthChecker.  service попадает в JSON как "service",
// version — как "version" (можно оставить пустым).  service пустым
// быть не должен, но проверять не будем — это не критично для рантайма.
func New(service, version string) *HealthChecker {
	return &HealthChecker{service: service, version: version}
}

// Register добавляет проверку.  Имена должны быть уникальны в рамках
// одного HealthChecker; повторная регистрация перетирает предыдущую
// (это удобно при рестарте без полной пересборки HealthChecker).
func (h *HealthChecker) Register(name string, fn CheckFunc, opts ...Option) {
	if fn == nil {
		return
	}
	rc := registeredCheck{name: name, fn: fn, timeout: 2 * time.Second}
	for _, opt := range opts {
		opt(&rc)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.checks {
		if h.checks[i].name == name {
			h.checks[i] = rc
			return
		}
	}
	h.checks = append(h.checks, rc)
}

// HTTPHandler возвращает http.Handler для /ready.  Поведение:
//   - 200 + status=ok        — все обязательные checks healthy;
//   - 200 + status=degraded  — обязательные ok, soft-проверки fail;
//   - 503 + status=error     — есть хотя бы одна обязательная unhealthy.
//
// Без зарегистрированных проверок всегда 200 + status=ok — это
// liveness-style поведение, удобное для /health.
func (h *HealthChecker) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := h.Run(r.Context())
		code := http.StatusOK
		if resp.Status == "error" {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		// Игнорируем ошибку записи — клиент уже отвалился, делать нечего.
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// LivenessHandler — отдельный хендлер для /health: всегда 200, если процесс
// жив.  Не запускает зарегистрированные проверки.  Это сознательное
// разделение: liveness нужен k8s, чтобы решать «рестартовать ли pod»;
// readiness — «гнать ли трафик».  Подробнее: docs/operations/monitoring-alerts.md.
func (h *HealthChecker) LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := Response{
			Status:    "ok",
			Service:   h.service,
			Version:   h.version,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// Run выполняет все проверки последовательно (с per-check таймаутом)
// и возвращает агрегированный Response.  Вынесен публично для тестов
// и для случаев, когда сервис хочет залогировать результат, не отдавая
// его клиенту.
func (h *HealthChecker) Run(ctx context.Context) Response {
	h.mu.RLock()
	checks := make([]registeredCheck, len(h.checks))
	copy(checks, h.checks)
	h.mu.RUnlock()

	results := make(map[string]CheckResult, len(checks))
	hasHardFail := false
	hasSoftFail := false

	for _, c := range checks {
		cctx, cancel := context.WithTimeout(ctx, c.timeout)
		start := time.Now()
		status, msg := c.fn(cctx)
		dur := time.Since(start)
		cancel()

		results[c.name] = CheckResult{
			Status:   status,
			Message:  msg,
			Duration: dur.Round(time.Millisecond).String(),
		}
		if status != StatusHealthy {
			if c.soft {
				hasSoftFail = true
			} else {
				hasHardFail = true
			}
		}
	}

	overall := "ok"
	switch {
	case hasHardFail:
		overall = "error"
	case hasSoftFail:
		overall = "degraded"
	}

	resp := Response{
		Status:    overall,
		Service:   h.service,
		Version:   h.version,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if len(results) > 0 {
		// Сортируем ключи неявно через map → JSON: encoding/json уже
		// сортирует ключи по алфавиту, но если изменится — пусть тест
		// ловит.  sort здесь только для детерминистичности при
		// возможной миграции на []CheckResult в будущем.
		_ = sortedKeys(results)
		resp.Checks = results
	}
	return resp
}

// sortedKeys возвращает отсортированный список ключей.  Не используется
// сейчас (json.Marshal уже сортирует), но оставлен как hook на случай
// миграции на slice-формат.
func sortedKeys(m map[string]CheckResult) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
