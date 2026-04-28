# DB Migration Runbook

## Когда использовать

- Релиз содержит DDL-изменения (любой PR с файлами в `services/*/migrations/`)
- `db-migrator` Job упал на staging или production
- Партиальный сбой: миграции применились на N тенантах из M
- Нужно проверить, в какой версии находится конкретный тенант
- Подготовка к on-prem обновлению (банк применяет сам)

Ссылки: [ADR-0005](../adr/0005-db-migrations-strategy.md), [`tools/db-migrator/README.md`](../../tools/db-migrator/README.md), [ADR-0009 § «State-handling»](../adr/0009-on-prem-update-strategy.md).

## Базовые принципы (ADR-0005)

- Каждый PR с DDL обязан содержать файлы `NNN_<name>.up.sql` и `NNN_<name>.down.sql`
- В production — **forward-only**; rollback = новая миграция, исправляющая проблему
- Down-миграции применяются автоматически только в pre-prod
- Все миграции должны быть **forward-compatible** (старая версия сервиса работает с новой схемой)
- `db-migrator` управляется через Temporal Workflow (parent `MigrateAllTenants` + child `MigrateTenant`)

## Pre-migration checklist

Перед запуском `db-migrator` в production:

- [ ] PR прошёл review (включая DBA для горячих таблиц)
- [ ] Миграции применены на staging без ошибок
- [ ] Down-миграции протестированы на pre-prod (если применимы)
- [ ] Backup PostgreSQL подтверждён за последние 24 часа (`backup.last_success`)
- [ ] Окно соответствует категории миграции (см. таблицу ниже)
- [ ] Банк-клиенты уведомлены за 7 дней (для деструктивных)
- [ ] Audit-event `platform.migration_planned` записан

### SLA-таблица окон (ADR-0005 § «SLA на миграции»)

| Категория | Окно | Approver |
|---|---|---|
| Backwards-compatible (add column nullable, add index, add table) | в любое время | Tech lead |
| Изменения горячих таблиц (`applications`, `audit_events`) | maintenance window 02:00–04:00 МСК | Tech lead + DBA |
| Удаление данных (drop column, drop table) | quarterly + банк-клиент уведомлён за 7 дней | Архитектор + DPO банка |
| Type changes (full-table rewrite) | quarterly + ZDT-стратегия (shadow column → backfill → rename) | Архитектор |

## Команды

### Локальная разработка

```bash
# Запустить стек и применить platform-миграции tenant-service
make up
make migrate-platform

# Создать тенанта и применить tenant-миграции
make migrate-tenant TENANT=demo SERVICE=tenant-service

# Сухой прогон — посмотреть SQL без применения (если поддержано)
docker compose --profile migrate run --rm db-migrator-platform \
  --service tenant-service --dry-run
```

### Staging — автоматически в CI

GitLab CI запускает `db-migrator` Job на merge в ветку `staging`. См. `.gitlab-ci.yml` (TBD).

```bash
# Если нужно перезапустить вручную
kubectl create job --from=cronjob/db-migrator-platform db-migrator-staging-$(date +%s) \
  -n aibank-staging
kubectl logs -n aibank-staging job/db-migrator-staging-... -f
```

### Production (SaaS) — manual approval gate

```bash
# 1. Через CI — нажать «Run» на этапе deploy:db-migrator-prod (manual approval)
# 2. Или вручную (только в incident-режиме):
kubectl apply -f infrastructure/k8s/jobs/db-migrator-prod-<release>.yaml -n aibank-prod
kubectl wait --for=condition=complete --timeout=30m job/db-migrator-prod-<release> -n aibank-prod
kubectl logs -n aibank-prod job/db-migrator-prod-<release> -f
```

### On-prem — банк сам запускает

```bash
# Из развёрнутого bundle (см. docs/deployment/airgapped-bundle.md)
kubectl apply -f bundle/manifests/db-migrator-job.yaml -n platform-data
kubectl wait --for=condition=complete --timeout=30m job/db-migrator-1.5.0 -n platform-data

# Альтернатива — прямой запуск CLI (debug)
kubectl run db-migrator --rm -it --restart=Never \
  --image=bank-registry.local/aibank/db-migrator:1.5.0 \
  --env="DATABASE_URL=postgres://..." -- \
  tenant-all --service tenant-service --source /migrations/tenant-service/tenant
```

### Команды CLI (`db-migrator`)

| Команда | Назначение |
|---|---|
| `db-migrator platform --service <svc> --source <dir> --dsn <pg>` | Применить platform-миграции одного сервиса |
| `db-migrator tenant --tenant-id <id> --service <svc> --source <dir>` | Применить tenant-миграции к одному тенанту |
| `db-migrator tenant-all --service <svc> --source <dir>` | Прогнать tenant-миграции по всем активным тенантам |
| `db-migrator tenant-all --stop-on-error` | Остановиться на первом сбое (default — продолжать) |

## Verification

```sql
-- В какой версии каждый тенант для конкретного сервиса
SELECT tenant_id, service_name, MAX(version) AS applied_version, MAX(applied_at) AS last_applied
FROM platform.tenant_migrations
WHERE service_name = '<service>'
GROUP BY tenant_id, service_name
ORDER BY tenant_id;

-- Версия одного тенанта
SELECT version, applied_at, applied_by, checksum
FROM platform.tenant_migrations
WHERE tenant_id = '<tenant>' AND service_name = '<service>'
ORDER BY version;

-- Платформенные миграции
SELECT service_name, version, applied_at, checksum
FROM platform_meta.schema_migrations
ORDER BY service_name, version;

-- Расхождения: какие тенанты отстают от target_version
WITH target AS (SELECT 25 AS v, 'tenant-service' AS svc)
SELECT t.tenant_id
FROM platform.tenants t
LEFT JOIN platform.tenant_migrations m
       ON m.tenant_id = t.id AND m.service_name = (SELECT svc FROM target)
WHERE t.status IN ('trial','active')
GROUP BY t.tenant_id
HAVING COALESCE(MAX(m.version),0) < (SELECT v FROM target);
```

## Что делать при partial failure

Сценарий: `db-migrator tenant-all` применил миграцию `005_add_status` на 28 тенантах из 30; на двух — упал.

```bash
# 1. Найти failed тенанты — Temporal UI или прямой SQL
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "SELECT tenant_id, MAX(version)
   FROM platform.tenant_migrations
   WHERE service_name='<service>'
   GROUP BY tenant_id
   HAVING MAX(version) < 5"

# 2. На failed тенанте — посмотреть, что упало
kubectl logs -n aibank-prod job/db-migrator-prod-<release> | grep -A 5 "tenant=<failed>"

# 3. Если ошибка — например, конфликт constraint из-за грязных данных:
#    Принять решение: чистить данные → ретраить, или forward-fix-миграция
#    Чистка данных — отдельная подготовительная activity:
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "SELECT count(*) FROM tnt_<failed>.applications WHERE status IS NULL"

# 4. После исправления — точечный retry на одном тенанте
db-migrator tenant \
  --tenant-id <failed> \
  --service <service> \
  --source /migrations/<service>/tenant \
  --run-id "manual-retry-$(uuidgen)"

# 5. Проверка
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "SELECT version FROM platform.tenant_migrations
   WHERE tenant_id='<failed>' AND service_name='<service>'
   ORDER BY version DESC LIMIT 1"
```

**Важно:** ADR-0005 указывает на риск «один сервис опередил другой по версии схемы». Если в одном релизе мигрируется N сервисов — все `db-migrator` jobs должны успешно завершиться **до** rollout новых версий сервисов. Pipeline это обеспечивает через явный `needs:` step.

## Rollback policy

- В **production** — только forward-fix через новую миграцию (политика ADR-0005)
- В **pre-prod / staging** — допустим `db-migrator down --service <svc> --steps 1`, но только в ручном режиме DBA
- В **локальной разработке** — `make migrate-down` (для Nx-таргета)

Forward-fix паттерн:

```sql
-- 005_add_status.up.sql (уже применён, и поломал что-то)
ALTER TABLE applications ADD COLUMN status TEXT NOT NULL DEFAULT 'pending';

-- 006_fix_status_default.up.sql (новая, исправляющая)
UPDATE applications SET status = 'unknown' WHERE status = '';
ALTER TABLE applications ALTER COLUMN status DROP DEFAULT;
```

## Drift detection

`db-migrator` проверяет SHA-256 каждого уже применённого файла; несовпадение = миграция изменена после применения (запрещено).

```bash
# Если drift detected — fail в логе:
# "ERROR: checksum mismatch for migration 005_add_status: applied=abc123 file=def456"

# Что делать:
# 1. НЕ продолжать миграцию
# 2. Откатить файл миграции к версии, что в БД (git log на файл)
# 3. Если изменение нужно — новая миграция NNN+1
# 4. Audit-event для security: возможна попытка tampering
```

## Что НЕ покрывает этот runbook

- **Data migrations** — массовые изменения данных (пересчёт risk-score у 1М заявок). Это отдельные Temporal-workflow с batching, idempotency, прогрессом
- **Schema changes Temporal cluster** — Temporal сам мигрирует свою `temporal_visibility` схему, мы не вмешиваемся
- **Stateful upgrade** PostgreSQL major version — отдельный runbook (`../operations/backup-restore.md`)

## Связанные документы

- [ADR-0005](../adr/0005-db-migrations-strategy.md) — стратегия миграций
- [ADR-0009](../adr/0009-on-prem-update-strategy.md) — миграции в blue-green-флоу
- [`tools/db-migrator/README.md`](../../tools/db-migrator/README.md) — CLI reference
- [`temporal-recovery.md`](temporal-recovery.md) — если migration workflow завис
- [`incident-response.md`](incident-response.md) — миграция, поломавшая prod = инцидент
