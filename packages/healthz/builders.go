// Часто используемые CheckFunc-билдеры: БД, HTTP, TCP-dial, чтение файла.
//
// Все билдеры — чистые функции без скрытого state'а.  Если в будущем
// потребуется метрика «сколько раз падал check X», можно обернуть
// CheckFunc в декоратор, не меняя API HealthChecker.
package healthz

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// DBCheck возвращает CheckFunc, которая пингует *sql.DB.  Если db == nil —
// проверка всегда unhealthy (а не паника).  Это удобно: сервис может
// зарегистрировать DB-проверку до того, как реально открыл соединение.
func DBCheck(db *sql.DB) CheckFunc {
	return func(ctx context.Context) (Status, string) {
		if db == nil {
			return StatusUnhealthy, "db handle is nil"
		}
		if err := db.PingContext(ctx); err != nil {
			return StatusUnhealthy, "ping failed: " + err.Error()
		}
		return StatusHealthy, ""
	}
}

// HTTPCheck возвращает CheckFunc, которая делает GET на url и ожидает
// 2xx.  Используется для проверки upstream-сервисов (например,
// billing-service → tenant-service /health).  timeout применяется к
// HTTP-клиенту; per-check таймаут HealthChecker'а — отдельный.
//
// Замечание: проверять только своих критичных upstream'ов.  Если
// сервис формально может работать с N зависимостями, не нужно ставить
// /ready=503 при падении любого из N — это инвертирует роль readiness.
func HTTPCheck(url string, timeout time.Duration) CheckFunc {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	return func(ctx context.Context) (Status, string) {
		if url == "" {
			return StatusUnhealthy, "url is empty"
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return StatusUnhealthy, "build request: " + err.Error()
		}
		resp, err := client.Do(req)
		if err != nil {
			return StatusUnhealthy, "http get: " + err.Error()
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return StatusUnhealthy, fmt.Sprintf("unexpected status %d", resp.StatusCode)
		}
		return StatusHealthy, ""
	}
}

// TCPCheck возвращает CheckFunc, которая делает net.Dial("tcp", addr).
// Используется для системы без HTTP API (Temporal frontend, Kafka
// broker, SMTP).  Семантика — «порт открыт, listener отвечает».
func TCPCheck(addr string, timeout time.Duration) CheckFunc {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return func(ctx context.Context) (Status, string) {
		if addr == "" {
			return StatusUnhealthy, "addr is empty"
		}
		dl := net.Dialer{Timeout: timeout}
		conn, err := dl.DialContext(ctx, "tcp", addr)
		if err != nil {
			return StatusUnhealthy, "tcp dial: " + err.Error()
		}
		_ = conn.Close()
		return StatusHealthy, ""
	}
}

// FileCheck возвращает CheckFunc, которая проверяет наличие/доступность
// файла.  Используется, например, в risk-engine для rules pack:
// если файла нет, движок работает в degraded-режиме (без правил),
// но это не блокер.
func FileCheck(path string) CheckFunc {
	return func(_ context.Context) (Status, string) {
		if path == "" {
			return StatusUnhealthy, "path is empty"
		}
		info, err := os.Stat(path)
		if err != nil {
			return StatusUnhealthy, "stat: " + err.Error()
		}
		if info.IsDir() {
			return StatusUnhealthy, "expected file, got directory"
		}
		return StatusHealthy, ""
	}
}

// AlwaysHealthy — тривиальная проверка, полезная для memory-backend'ов
// и тестов.  Возвращает healthy в любом контексте.
func AlwaysHealthy(reason string) CheckFunc {
	return func(_ context.Context) (Status, string) {
		return StatusHealthy, reason
	}
}

// FuncCheck оборачивает простую функцию error в CheckFunc.  Удобно для
// случаев, когда у клиента есть готовый Ping(ctx) error метод.
func FuncCheck(fn func(ctx context.Context) error) CheckFunc {
	return func(ctx context.Context) (Status, string) {
		if fn == nil {
			return StatusUnhealthy, "check fn is nil"
		}
		if err := fn(ctx); err != nil {
			return StatusUnhealthy, err.Error()
		}
		return StatusHealthy, ""
	}
}
