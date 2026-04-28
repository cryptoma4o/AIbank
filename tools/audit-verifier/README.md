# audit-verifier

CLI для **read-only** проверки целостности hash-цепочки `audit-service`.
Назначение и обязательные сценарии — в [`docs/runbooks/audit-log-integrity.md`](../../docs/runbooks/audit-log-integrity.md).

Tool пересчитывает `ComputeHash(prev_hash)` для каждого события тенанта
и сравнивает с записанным `Hash`. Любое расхождение = разрыв цепочки =
P0-инцидент (tampering suspected). Источник истины для логики хэша —
[`services/audit-service/internal/domain/event.go`](../../services/audit-service/internal/domain/event.go).

## Когда использовать

- Алерт `audit.hash_chain_broken` (Critical).
- Подозрение на ручной UPDATE/DELETE в `audit.events`.
- Регулярная еженедельная проверка (CronJob `audit-verifier-weekly`).
- После каждой break-glass-сессии (см. `break-glass-procedure.md`).
- Регуляторный аудит банка («чем доказывается неизменность audit log»).

## Команды

### `verify` — пройти всю цепочку и вернуть статус

```bash
# API-режим (через audit-service внутри кластера)
audit-verifier verify --tenant-id bank-alpha

# DB-режим (для DBA / forensic; обходит audit-service-pod)
audit-verifier verify \
  --tenant-id bank-alpha \
  --source db \
  --dsn "postgres://dba:***@pg-host:5432/aibank?sslmode=require"

# Диапазон по времени
audit-verifier verify \
  --tenant-id bank-alpha \
  --from 2026-04-01T00:00:00Z \
  --to   2026-04-26T23:59:59Z

# Машинный вывод для CronJob → Prometheus
audit-verifier verify --tenant-id bank-alpha --format json
```

### `latest` — хэш последнего события

```bash
audit-verifier latest --tenant-id bank-alpha
# 9d3f...  (SHA-256 hex или "(нет событий)")
```

Полезно как quick-check при подозрении: сравнить хэш до и после
заявленного инцидент-окна.

## Флаги

| Флаг          | Команды         | Описание                                                   |
|---------------|-----------------|------------------------------------------------------------|
| `--tenant-id` | verify, latest  | Обязательный. Whitelist `^[a-z][a-z0-9_-]{1,63}$`.         |
| `--source`    | verify, latest  | `api` (default) или `db`.                                  |
| `--audit-url` | verify, latest  | URL audit-service. Default `http://audit-service:8081`.    |
| `--dsn`       | verify, latest  | Postgres DSN или `$DATABASE_URL` (для `--source=db`).      |
| `--from`      | verify          | RFC3339, нижняя граница `created_at`.                      |
| `--to`        | verify          | RFC3339, верхняя граница `created_at`.                     |
| `--format`    | verify          | `text` (default) или `json`.                               |

## Коды выхода

| Код | Значение                                                                |
|-----|-------------------------------------------------------------------------|
| 0   | Цепочка целая. Для `latest` — хэш напечатан в stdout.                   |
| 1   | Разрыв обнаружен. JSON/text-вывод содержит `first_mismatch`. **P0.**    |
| 2   | Ошибка флагов / валидации `tenant-id`.                                  |
| 3   | Внутренняя ошибка (БД недоступна, audit-service не отвечает, и т. п.). |

CronJob/Alert правила:
- `exit_code == 1` на любом тенанте → `audit.hash_chain_broken` Critical.
- `exit_code == 3` подряд 3 раза → `audit_verifier.unhealthy` Warning.

## Архитектура

```
cmd/audit-verifier/main.go        — CLI: verify / latest
internal/verifier/event.go        — Event + ComputeHash (зеркало audit-service)
internal/verifier/verifier.go     — VerifyChain, LatestHash, ValidateTenantID
internal/verifier/api.go          — APIStore (HTTP к audit-service)
internal/verifier/db.go           — DBStore (прямой SELECT FROM audit.events)
internal/verifier/verifier_test.go — happy/tampered/truncated/empty/validation
```

## Sync с audit-service

Логика `ComputeHash` буквально скопирована из
`services/audit-service/internal/domain/event.go`. Это сознательное
решение, **не** циклический импорт:

1. Тот пакет — `internal`, его публичный импорт за пределы `services/`
   запрещён правилами Go.
2. Tool обязан работать независимо от состояния `audit-service`-pod'а
   (forensic): даже если у него другая версия, audit-verifier
   продолжает воспроизводить хэш «как было записано».
3. Любое изменение `AuditEvent` или `ComputeHash` — обновить
   `internal/verifier/event.go` и тесты в **одном PR**. CI ловит
   расхождение через `internal/verifier/verifier_test.go` (тест
   `TestVerifyChain_HappyPath` падает при любом drift'е сериализации).

## Связанные документы

- [`docs/runbooks/audit-log-integrity.md`](../../docs/runbooks/audit-log-integrity.md) — операционный сценарий, что делать при разрыве.
- [`docs/adr/0010-billing-and-audit-log.md`](../../docs/adr/0010-billing-and-audit-log.md) — связь audit ↔ billing через `correlation_id`; реконсиляция использует `audit-verifier` для подтверждения целостности перед сверкой.
- [`docs/security-architecture.md`](../../docs/security-architecture.md) §8.2–8.3 — что логируется, требования по 5-летнему хранению.

## TODO / Future work

- Криптографическая верификация подписи (Ed25519 / ГОСТ) — отложено
  до момента, когда `audit-service` начнёт писать `signature` в
  каждое событие (см. `security-architecture.md` §8.3, поле
  `signature` пока в схеме как опциональный TBD).
- Курсорная пагинация: текущий `APIStore` тянет всё одной страницей
  (audit-service пока без `?cursor=`). Для тенантов >1000 событий —
  использовать `--source=db` или дождаться расширения handler.
- `verify-all` (как в runbook) — обход всех активных тенантов;
  ждёт реализации `tenants`-listing endpoint в `tenant-service`.
- Экспорт метрик в формате Prometheus (`audit_chain_broken_count{tenant}`)
  — оставлено CronJob-обёртке, не самому tool'у.
