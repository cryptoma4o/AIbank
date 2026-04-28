# healthz

Общий пакет для `/health` (liveness) и `/ready` (readiness) эндпоинтов
во всех Go-сервисах AIbank.  Заменяет inline `WriteHeader(200)` стабы
структурированным JSON, который понимают k8s-пробы и `scripts/smoke.sh`.

## Контракт ответа

```json
{
  "status": "ok",            // ok | degraded | error
  "service": "tenant-service",
  "version": "v1.2.3",
  "timestamp": "2025-04-27T12:34:56Z",
  "checks": {
    "db":  { "status": "healthy",   "duration": "3ms" },
    "kafka": { "status": "unhealthy", "message": "dial timeout", "duration": "2s" }
  }
}
```

| status     | HTTP | Когда |
|------------|------|-------|
| `ok`       | 200  | Все проверки healthy (или их нет — liveness-style) |
| `degraded` | 200  | Soft-проверка fail, обязательные ok |
| `error`    | 503  | Хотя бы одна обязательная проверка fail |

## Использование

```go
import "github.com/aibank/platform/packages/healthz"

hz := healthz.New("tenant-service", os.Getenv("APP_VERSION"))
hz.Register("db", healthz.DBCheck(db))
hz.Register("audit-service", healthz.HTTPCheck(auditURL+"/health", 2*time.Second))
hz.Register("kafka", healthz.TCPCheck(kafkaAddr, 2*time.Second), healthz.Soft())

r := chi.NewRouter()
r.Method("GET", "/health", hz.LivenessHandler()) // всегда 200
r.Method("GET", "/ready",  hz.HTTPHandler())     // запускает checks
```

## Builders (готовые CheckFunc)

| Builder | Семантика |
|---------|-----------|
| `DBCheck(db *sql.DB)` | `db.PingContext` с per-check таймаутом |
| `HTTPCheck(url, timeout)` | GET, ожидает 2xx |
| `TCPCheck(addr, timeout)` | `net.Dial("tcp", addr)` |
| `FileCheck(path)` | `os.Stat`, файл существует и не директория |
| `AlwaysHealthy(reason)` | для memory-backend'ов |
| `FuncCheck(fn func(ctx) error)` | обёртка над любой `Ping`-функцией |

## Опции

- `healthz.Soft()` — soft-проверка: fail → `degraded`, не блокирует readiness.
- `healthz.Timeout(d)` — переопределить дефолтный 2s таймаут на проверку.

## Когда использовать `/health` vs `/ready`

- `/health` (liveness) — k8s решает «рестартовать ли pod».  Всегда 200, если
  процесс отвечает.  Не зависит от внешних систем.
- `/ready` (readiness) — k8s решает «гнать ли трафик».  503, если ключевая
  зависимость недоступна.  Так под при перезапуске Postgres сам себя
  выводит из service endpoints, а не отдаёт 5xx клиенту.

Подробнее: `docs/operations/monitoring-alerts.md` § Service Health,
`infrastructure/helm/charts/aibank-service/templates/deployment.yaml`.

## Принципы дизайна

- Stdlib only — никаких лишних версий chi/log в зависимостях.
- Потокобезопасен — `Register` и `HTTPHandler` можно вызывать конкурентно.
- Per-check timeout — медленный check не валит весь /ready.
- Soft-сheck — для опциональных зависимостей (Kafka в dev, secondary cache).
