# db-migrator

CLI и K8s Job для применения SQL-миграций согласно [ADR-0005](../../docs/adr/0005-db-migrations-strategy.md).

Не привязан к одному сервису — работает с любым каталогом миграций в формате goose
(`-- +goose Up` / `-- +goose Down`).

## Команды

### platform

Применяет миграции к platform-схемам (`platform`, `audit` и т.п.). Версии — в `platform_meta.schema_migrations`.

```bash
db-migrator platform \
  --service tenant-service \
  --source services/tenant-service/migrations \
  --dsn "postgres://aibank:aibank@localhost:5432/aibank?sslmode=disable"
```

Подкаталоги (`migrations/tenant/`) игнорируются — обрабатываются командой `tenant`.

### tenant

Применяет миграции к схеме одного тенанта (`tnt_<id>`). Версии — в `platform.tenant_migrations`.

```bash
db-migrator tenant \
  --tenant-id alfa \
  --service tenant-service \
  --source services/tenant-service/migrations/tenant \
  --run-id "wf_$(uuidgen)"
```

Перед запуском схема `tnt_alfa` должна быть создана через `tenant-service`-API
(`SchemaProvisioner.Provision`).

### tenant-all

Применяет tenant-миграции к каждому активному тенанту (`status` ∈ {trial, active}).

```bash
db-migrator tenant-all \
  --service tenant-service \
  --source services/tenant-service/migrations/tenant
```

Поведение по умолчанию — продолжать при ошибке на отдельном тенанте, отчёт в конце.
Флаг `--stop-on-error` останавливает на первом сбое.

## Безопасность

- **Защита от drift.** При несовпадении SHA-256 уже применённой миграции — fail.
  Это означает, что файл миграции изменили после применения. Запрещено политикой ADR-0005.
- **search_path через `set_config`.** Имя схемы передаётся параметром, не интерполируется в текст SQL.
- **Whitelist tenant ID** (`^[a-z][a-z0-9_]{1,31}$`) — тот же, что в tenant-service.

## Down-migrations

В MVP **forward-only**. Down-блок в файле сохраняется (для тестового окружения и
будущей реализации), но runner его не выполняет. Откат в production = новая
миграция, исправляющая поломку (политика ADR-0005).

## Temporal-orchestration

Этот CLI = тот самый бинарь, который вызывается активити Temporal Workflow
`MigrateTenant` (см. ADR-0005). На уровне Workflow:

- `MigrateAllTenants` (parent) → читает список tenants, спавнит child workflows
- `MigrateTenant(tenantID)` (child) → последовательно вызывает activity
- `ApplySingleMigration(tenantID, version)` (activity) → запускает `db-migrator tenant ...`

Реализация Workflow не входит в этот пакет (вынесена в отдельный сервис в Фазе 1+).

## Локальный dev

```bash
# 1. Запустить стек
docker compose up -d postgres

# 2. Применить platform-миграции tenant-service
DATABASE_URL="postgres://aibank:aibank@localhost:5432/aibank?sslmode=disable" \
  go run ./cmd/db-migrator platform \
  --service tenant-service \
  --source ../../services/tenant-service/migrations

# 3. Создать тенанта (через tenant-service API)
curl -X POST http://localhost:8080/v1/tenants -d '{
  "id": "alfa",
  "name": "Альфа-Банк",
  "bik": "044525593",
  "inn": "7728168971",
  "deployment_mode": "saas"
}'

# 4. Применить tenant-миграции
go run ./cmd/db-migrator tenant \
  --tenant-id alfa \
  --service tenant-service \
  --source ../../services/tenant-service/migrations/tenant
```

## Тесты

```bash
go test ./...
```

Юнит-тесты покрывают парсер goose-формата, дискавери файлов, валидацию дубликатов
и сортировку по версии. Интеграционные тесты с реальной БД пока вне scope MVP
(нужен testcontainers — заведено как future work).
