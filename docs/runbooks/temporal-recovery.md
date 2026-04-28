# Temporal Recovery

## Когда использовать

- Алерт `workflow.stuck` (workflow в статусе Running дольше ожидаемого)
- Алерт `temporal.cluster.unhealthy`, `temporal.task_queue.backlog > 1000`
- Onboarding не завершается, заявки висят в state `processing` > 30 мин
- Сбой Temporal-кластера, нужен restore из backup
- Перед on-prem обновлением — проверить, что workflow versioning не сломан

Ссылки: [ADR-0001](../adr/0001-use-temporal-for-orchestration.md), [ADR-0009 § «Temporal workflow»](../adr/0009-on-prem-update-strategy.md).

## Архитектура

```
SaaS:                                    On-prem:
┌─────────────────────────────────┐     ┌─────────────────────────────────┐
│ Один Temporal Cluster           │     │ Temporal Cluster per банк       │
│ (multi-tenant через namespaces) │     │ (в namespace platform-data)     │
└─────────────────────────────────┘     └─────────────────────────────────┘
   ↑              ↑                            ↑
   │              │                            │
   tenant=alfa  tenant=beta                  default namespace
```

Все workflows используют PostgreSQL backend (тот же кластер, схема `temporal_visibility`).
Worker'ы (`onboarding-orchestrator`) подключены к task queues:

| Task Queue | Что обрабатывает | Сервис-worker |
|---|---|---|
| `onboarding-default` | Главные онбординг-воркфлоу | onboarding-orchestrator |
| `risk-scoring` | Скоринг и правила | risk-engine |
| `migrate-tenant` | Миграции (ADR-0005) | db-migrator-worker |
| `monthly-invoicing` | Биллинг (ADR-0010) | billing-service |
| `weekly-reconciliation` | Реконсиляция audit↔billing | billing-service |
| `ext-retry` | Retry внешних API после downtime | onboarding-orchestrator |

## Чтение Temporal UI

URL по умолчанию: `http://localhost:8089` (docker-compose) / `https://temporal-ui.<env>.aibank.internal` (prod).

| Раздел | Что смотреть |
|---|---|
| **Workflows** | Список с фильтрами: `Status=Running` + `WorkflowType=OnboardingV2` + время |
| **Workflow detail** | Input, current state, pending activities, history events |
| **History** | Полная цепочка событий — что заставило workflow застрять |
| **Pending Activities** | Если есть pending > N минут — повод для investigate |
| **Task Queues** | Backlog: сколько worker'ов подписано, lag по обработке |
| **Schedules** | Cron-workflows (например, `MonthlyInvoicing` 1-го числа) |

## Команды temporal CLI (`tctl`)

```bash
# 0. Подключение
kubectl port-forward -n platform-data svc/temporal-frontend 7233:7233 &
export TEMPORAL_ADDRESS=localhost:7233
export TEMPORAL_NAMESPACE=default        # или per-tenant в SaaS

# 1. Описание workflow
tctl workflow describe --workflow_id <wf_id>

# 2. История событий
tctl workflow showid <wf_id> --output_filename /tmp/wf-<id>.history

# 3. Список Running workflow по типу
tctl workflow list --query "WorkflowType='OnboardingV2' AND ExecutionStatus='Running'"

# 4. Stuck workflows (Running > 1 час)
tctl workflow list --query "ExecutionStatus='Running' AND StartTime < '$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ)'"

# 5. Описание task queue
tctl taskqueue describe --taskqueue onboarding-default
tctl taskqueue list-partitions --taskqueue onboarding-default

# 6. Termination — крайняя мера, workflow не сможет завершиться сам
tctl workflow terminate --workflow_id <wf_id> --reason "stuck: <root-cause>"

# 7. Cancel — corrective, workflow получает сигнал и сам пишет compensation
tctl workflow cancel --workflow_id <wf_id> --reason "<reason>"

# 8. Reset workflow на конкретное событие (advanced)
tctl workflow reset --workflow_id <wf_id> --event_id <event_id> --reason "<reason>"

# 9. Signal — отправить событие активному workflow
tctl workflow signal --workflow_id <wf_id> --name resume_after_manual_check --input '{"approved":true}'

# 10. Namespace operations
tctl --ns default namespace describe
tctl namespace list
```

## Типичные проблемы

### Проблема 1: Workflow stuck

**Признаки:** Running > expected, в `Pending Activities` есть activity с `Attempt > 5`.

```bash
# A. Логи orchestrator-сервиса (worker, который должен был забрать activity)
kubectl logs -n platform deploy/onboarding-orchestrator --since=1h | grep <wf_id>

# B. Проверить, что worker подписан на task queue
tctl taskqueue describe --taskqueue onboarding-default
# Должен показывать активных pollers

# C. Если pollers есть, но activity не обрабатывается — проверить activity logic
tctl workflow describe --workflow_id <wf_id> | jq '.pendingActivities'

# D. Решение:
# 1. Если activity timeout-ы и сама retry-policy здоровая → дождаться очередного retry
# 2. Если внешний сервис недоступен → перевести в degraded mode (см. incident-response § 5)
# 3. Если активность зависла на bug в коде → terminate + создать compensation workflow
```

### Проблема 2: Activity timeout

**Признаки:** в pending activities `LastFailure: ScheduleToCloseTimeout`.

Retry policy (стандарт ADR-0001):
- Initial interval: 1s
- Max interval: 100s
- Max attempts: 5
- Backoff coefficient: 2.0

```bash
# Если 5 attempts уже исчерпаны — escalation
tctl workflow describe --workflow_id <wf_id> | jq '.pendingActivities[].lastFailure'

# Решение:
# - Внешняя проблема (ЕГРЮЛ down) → подождать восстановления, после — `tctl workflow signal --name retry_external`
# - Внутренний bug → fix + redeploy worker; workflow автоматически продолжит на следующем poll
# - Невосстановимая ошибка (например, документ не валиден) → cancel + клиенту запрос на доработку
```

### Проблема 3: Workflow versioning conflict

**Признаки:** worker логирует `non-deterministic workflow detected`, история событий не совпадает с кодом.

Это случается, когда workflow code изменён без `workflow.GetVersion`-патча. ADR-0001 фиксирует: breaking changes — через явный `Patch`.

```go
// Правильный паттерн
v := workflow.GetVersion(ctx, "add-fssp-check", workflow.DefaultVersion, 1)
if v == workflow.DefaultVersion {
    // старый путь
} else {
    // новый путь
}
```

```bash
# Если уже воркфлоу запустились на старом коде, и код мерджнут несовместимо:
# 1. Вернуть старую версию worker (rollback deploy)
kubectl rollout undo deploy/onboarding-orchestrator -n platform

# 2. Дождаться завершения старых workflow (через `--query StartTime < <release_time>`)
tctl workflow list --query "ExecutionStatus='Running' AND StartTime < '<release_time>'"

# 3. Только потом раскатить новый код с `GetVersion`-патчем
```

## Recovery после сбоя cluster

Temporal-cluster — stateful, persistence в PostgreSQL (схема `temporal_visibility`). RPO 15 минут / RTO 4 часа per `security-architecture.md` § 9.3.

### Сценарий A: PostgreSQL упал, но не потерян

```bash
# 1. Восстановить PG (см. operations/backup-restore.md § «Postgres»)
# 2. Temporal-frontend pod рестартнётся сам, как только Postgres healthy
kubectl rollout status deploy/temporal-frontend -n platform-data

# 3. Worker'ы переподключатся автоматически (exponential backoff)
# 4. Все long-running workflows продолжатся из последнего event

# 5. Verify
tctl cluster health
tctl workflow list --query "ExecutionStatus='Running'" | head -20
```

### Сценарий B: Catastrophic loss — restore из backup

```bash
# 1. Restore PostgreSQL из последнего snapshot (RPO ≤ 15 мин)
# Команды — см. ../operations/backup-restore.md

# 2. После restore Temporal автоматически перечитает event history
# Workflow'ы восстановятся в state, который был на момент snapshot

# 3. Workflow events после snapshot потеряны — re-emit вручную:
# - Найти в audit log все events за gap-период
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "SELECT subject_id, action FROM tnt_<tenant>.audit_events
   WHERE recorded_at BETWEEN '<snapshot_time>' AND '<restore_time>'
     AND action LIKE 'application.%'"
# - Для каждого application_id — signal в активный workflow или создать новый

# 4. Communicate банку: 15-минутный gap, какие заявки нужно перезапустить
```

### Сценарий C: Event replay для отладки

Temporal позволяет «прокрутить» workflow заново из истории, чтобы найти баг:

```bash
tctl workflow showid <wf_id> --output_filename /tmp/replay.history
# В тестах:
go test -run TestWorkflowReplay -history=/tmp/replay.history
```

## Backup Temporal

Per ADR-0001 § «Implementation Notes»: «Backup схемы Temporal — каждый час, retention 30 дней».

```bash
# Backup происходит как часть PostgreSQL backup; отдельно гнать не нужно
# Verify backup
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "SELECT pg_size_pretty(pg_database_size('temporal'))"
```

## Health checks

```bash
# 1. Cluster health
tctl cluster health

# 2. Все namespaces
tctl namespace list

# 3. Метрики из VictoriaMetrics (Grafana dashboard «Temporal Cluster»)
# Ключевые SLI:
#  - temporal_request_latency_p99 < 500ms
#  - temporal_workflow_endtoend_latency_seconds (p95 для OnboardingV2 < 1 час)
#  - temporal_task_queue_pending_tasks (по очередям < 100)
#  - temporal_worker_task_slots_available (> 10% от max)

# 4. Если worker scale-out требуется — HPA по CPU + custom metric
kubectl get hpa -n platform onboarding-orchestrator
```

## Что НЕ покрывает этот runbook

- Прикладная логика workflow (что значит «openAccount активность зависла») — это уровень кода сервиса
- Temporal upgrade — отдельная процедура (`../operations/backup-restore.md` § «Stateful upgrades»)

## Связанные документы

- [ADR-0001](../adr/0001-use-temporal-for-orchestration.md) — выбор Temporal
- [ADR-0009 § «Temporal»](../adr/0009-on-prem-update-strategy.md) — versioning при blue-green
- [`incident-response.md`](incident-response.md) § «Инцидент 3» — workflow hang escalation
- [`db-migration.md`](db-migration.md) — `MigrateAllTenants` workflow
- [`../operations/backup-restore.md`](../operations/backup-restore.md) — PostgreSQL backup, который содержит и Temporal-схему
- [Temporal docs](https://docs.temporal.io/) — общая справка
