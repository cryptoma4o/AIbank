# ADR-0005: Стратегия миграций БД для multi-tenant PostgreSQL

**Status:** Accepted  
**Date:** 2026-04-26  
**Authors:** Главный архитектор  
**Reviewers:** Тех-лид backend, DBA, Security-инженер

## Context

ADR-0002 зафиксировал schema-per-tenant: одна БД, схема `platform` для метаданных, схемы `tnt_<id>` для каждого банка. Это создаёт нетривиальную задачу применения DDL-миграций:

- **N схем тенантов**, каждая со своей версией миграций. На момент применения релиза в продакшене один тенант может быть на v23, другой — на v25 (если задержалось обновление по операционным причинам).
- **Разные схемы — один кластер.** Параллельное применение N миграций в одной БД конкурирует за блокировки и connection pool.
- **Append-only audit и активные приложения.** Нельзя останавливать сервис на время миграции; long-running ALTER блокирует записи.
- **On-prem-цикл квартальный** (см. `docs/technical-structure.md` § 11). За квартал может накопиться 10–20 миграций. Они должны применяться в строго определённом порядке без сбоев — иначе у банка-клиента сломанная БД, и нам нужен on-call.
- **Регуляторика 152-ФЗ.** Любое изменение структуры данных, затрагивающее ПДн, должно быть отражено в audit log и согласовано с DPO банка.
- **Несколько типов «миграций» с разной семантикой:**
  1. Миграции схемы `platform` (применяются один раз, не зависят от тенантов).
  2. **Шаблон** миграций для тенантов (применяется при создании нового тенанта и при апгрейде существующих).
  3. Data migrations — переносы данных, обычно идемпотентные batch-задачи (≠ DDL).
  4. Backfills — однократные инициализации новых полей.
- **Откат.** Часть миграций обратимы, часть — нет (drop column → потеря данных). Политика отката должна быть зафиксирована.

Если каждый сервис принимает решения сам (один пишет миграции в `migrations/*.sql`, другой генерирует через ORM, третий хранит в Helm-чарте) — синхронизация версий между схемами тенантов и сервисами становится практически невозможной.

## Decision

Используем **golang-migrate** как единый инструмент применения миграций, с разделением на **platform** и **tenant** namespace, и оркестрацией через специализированный job, не на уровне сервисов.

### Архитектура

```
services/
└── tenant-service/
    ├── migrations/
    │   ├── platform/                  -- применяется один раз к схеме `platform`
    │   │   ├── 001_create_tenants.sql
    │   │   ├── 002_create_tenant_keys.sql
    │   │   └── ...
    │   └── tenant/                    -- ШАБЛОН, применяется к каждой схеме `tnt_<id>`
    │       ├── 001_create_audit_chain.sql
    │       ├── 002_create_applications.sql
    │       └── ...
    └── ...

services/<other-service>/
└── migrations/
    └── tenant/                         -- DDL для конкретного bounded context
        ├── 001_create_<entity>.sql
        └── ...
```

### Tooling

- **[`golang-migrate/migrate`](https://github.com/golang-migrate/migrate)** v4.x — single binary, поддержка Postgres, обратимые миграции, версионирование через таблицу `schema_migrations`.
- Миграции пишем на чистом SQL. Никаких ORM-генерируемых миграций (`gorm.AutoMigrate` и подобных) в production.
- Файлы: `NNN_short_name.up.sql` + `NNN_short_name.down.sql`.

### Кто применяет миграции

Создаём отдельный Go-сервис **`tools/db-migrator`** (CLI + Kubernetes Job), который:

1. Подключается к платформенной БД с ролью `platform_owner`.
2. Применяет миграции к схеме `platform` (один раз).
3. Читает `platform.tenants` для списка активных тенантов.
4. Для каждого тенанта читает `platform.tenant_migrations` (где какая версия применена).
5. Применяет недостающие миграции из `migrations/tenant/` к схеме `tnt_<id>`.
6. Записывает применённые версии обратно в `platform.tenant_migrations`.
7. Публикует событие `audit.schema_migration_applied` в Kafka для audit-сервиса.

Сервисы (`tenant-service`, `audit-service`, доменные сервисы) **не применяют миграции при старте** в SaaS-режиме. Это ответственность `db-migrator` job, запускаемого CI/CD pipeline до выкатывания новых версий сервисов.

### Оркестрация per-tenant

`db-migrator` использует **Temporal Workflow** для оркестрации:

- Один parent workflow `MigrateAllTenants` запускается на релиз.
- На каждый тенант — child workflow `MigrateTenant(tenantID, targetVersion)`.
- Child запускает activity `ApplySingleMigration(tenantID, version)` для каждой версии последовательно.
- Activity-уровень: `golang-migrate` применяет один файл в одной транзакции.
- Retry на сетевые ошибки и deadlock-и (Temporal-стандарт: exponential backoff, max 5 attempts).
- При фатальной ошибке (некорректный SQL, нарушение constraint) — child workflow помечает тенанта как `migration_failed`, parent продолжает с другими тенантами. Failed тенанты обрабатываются вручную.

### Для on-prem

Бандл миграций встроен в `db-migrator`-бинарь (через `embed.FS`). На стороне банка `db-migrator` запускается как обычный K8s Job — никаких сетевых обращений к нашим серверам. Журнал применения сохраняется в локальный `platform.tenant_migrations` и экспортируется в наш билинг при следующем синхронизации (см. ADR-0010).

### Политика отката

| Тип изменения | Down-миграция | Политика |
|---|---|---|
| Add column nullable | Drop column | Допустима автоматически |
| Add table | Drop table (если пустая) | Допустима автоматически |
| Add index | Drop index | Допустима автоматически |
| Rename column | Reverse rename | Допустима, но требует app-совместимости (N-1) |
| Drop column | NOT REVERSIBLE | Запрещено в одной миграции; разделяется на 2 релиза: «перестать читать → удалить» |
| Add NOT NULL constraint | Drop constraint | Разрешена, но требует backfill в предыдущей миграции |
| Сложное изменение типа | Зависит | Каждый случай — review архитектора |

Down-миграция применяется **только в pre-production средах** автоматически. В production откат — это всегда **forward fix** (новая миграция, исправляющая поломку), а не `migrate down`. Это политика, не техническое ограничение.

### Что НЕ покрывает это решение

- **Data migrations** (массовые изменения данных, например пересчёт risk-score у 1М заявок) — отдельный класс задач, выполняется через Temporal Workflows со своей семантикой (batching, idempotency, прогресс). Это не DDL, инструменты другие.
- **Schema-changes for Temporal itself** — Temporal управляет своими таблицами в схеме `temporal_visibility` через свои миграции, мы не вмешиваемся.

## Alternatives Considered

### Альтернатива 1: Liquibase

**Плюсы:**
- Зрелое промышленное решение (15+ лет).
- Декларативные changelogs (XML/YAML/JSON).
- Поддержка conditional migrations (по диалекту, по среде).
- Rollback из коробки.

**Минусы:**
- **Java-runtime в нашем Go-стеке.** Дополнительный слой, чужеродный экосистеме.
- XML-стиль changelogs тяжелее SQL-файлов. YAML-альтернатива есть, но всё равно больше абстракции.
- Commercial-фичи (например, Liquibase Pro для drift detection) — vendor lock-in.
- Setup-сложность для multi-schema-сценариев нетривиальна.

**Причина отклонения:** избыточен; Go-команда не хочет тащить Java-runtime ради миграций.

### Альтернатива 2: Flyway

**Плюсы:**
- Простой и популярный (особенно в Java-мире).
- SQL-first.
- Хорошая документация.

**Минусы:**
- Java-runtime (как у Liquibase).
- Многие нужные нам фичи в Flyway Teams/Enterprise (commercial).
- Per-schema baseline и conditional migrations требуют commercial-фич.

**Причина отклонения:** те же причины, что у Liquibase.

### Альтернатива 3: Atlas (HCL-based, от Ariga)

**Плюсы:**
- Современная, declarative-first (опционально). HCL-описание схемы → Atlas вычисляет diff и генерит миграцию.
- Pure Go, single binary.
- Хорошая поддержка Postgres.

**Минусы:**
- **Молодая экосистема.** Версия v0.x на момент решения; breaking changes регулярны.
- Declarative-режим интересен, но требует full-schema-определения, которое у нас распределено по сервисам (плохо ложится).
- Меньше материалов, runbook-ов, StackOverflow-ответов.

**Причина отклонения:** перспективен, но рискованно строить продакшен-критичный путь на v0.x. Возвращаемся к рассмотрению через 12 месяцев.

### Альтернатива 4: ORM-управляемые миграции (Ent, GORM, sqlx-migrations)

**Плюсы:**
- Часть стека приложения; не нужен отдельный инструмент.
- Автогенерация миграций из изменений модели.

**Минусы:**
- Автогенерируемые миграции часто **не учитывают данные**, делают опасные операции.
- **Конфликт с sqlc**, который мы уже выбрали для Go (типизированные запросы).
- В multi-schema сценарии все эти ORM рассчитаны на «one schema per app», требуют heavy customization.

**Причина отклонения:** философия не совпадает с нашим подходом «миграции — критичный артефакт, его пишет человек, ревьюит DBA».

### Альтернатива 5: Самописный orchestrator + чистые psql-вызовы

**Плюсы:**
- Полный контроль.

**Минусы:**
- Заново изобретаем `schema_migrations` таблицу, partial application, idempotency, file ordering.
- Через 2 года — самописная Liquibase, которую никто не хочет поддерживать.

**Причина отклонения:** не наш value-prop. Те же аргументы, что и в ADR-0001 для Temporal vs самописный SAGA-orchestrator.

## Consequences

### Positive

- **Один инструмент на весь монорепо.** Onboarding нового разработчика — час, а не день.
- **Чистый SQL.** Любой DBA читает миграции без знания специфичного DSL.
- **Версионирование per-tenant** в `platform.tenant_migrations` даёт точную картину состояния каждого банка. Это входит в банковский ИБ-аудит как «доказательство контроля изменений».
- **Temporal-orchestration** даёт встроенную visibility (через Temporal UI), retry-механику, и recovery после сбоя без потери прогресса.
- **On-prem self-contained.** Бандл миграций в бинарь — один artifact, никаких сетевых вызовов.
- **CI-friendly.** На pre-merge можно запустить миграции на тестовом тенанте и проверить, что они применяются + down-миграции работают.

### Negative

- **Lag между релизом и применением миграций.** Отдельный `db-migrator` job — это лишние 5–30 минут к pipeline до того, как новые сервисы запустятся. Mitigation: pipeline ждёт миграции явным `needs:` step.
- **`db-migrator` как single-source-of-truth для версий БД.** Если он сам сломан, обновлять схемы нечем. Mitigation: бинарь подписывается, есть процедура hot-fix через ручной запуск.
- **Сложность отладки multi-tenant миграций.** Если миграция падает на 5-м тенанте из 30 — нужно понимать, чем он отличается. Помогает Temporal UI.
- **Per-service tenant-migrations означают, что один сервис может «опередить» другой по версии схемы.** Тенант, у которого migrations применил один сервис до того, как до него дошёл другой, — в неконсистентном состоянии. Mitigation: orchestration на уровне релиза («все мигратор-job-ы в этом релизе должны успешно завершиться до того, как сервисы пересобираются»).

### Neutral

- **Файлов миграций станет много.** За год — 50–100. Категоризация по сервисам и нумерация не дают им смешаться, но регулярный housekeeping (squash старых пред-MVP миграций перед первым прод-релизом) — нормальная практика.
- **Изменения политики отката требуют дисциплины.** «Только forward fix в проде» — это процесс, который надо заложить в SDLC документ и проверять в code review.
- **Шаблон tenant-миграций. Их структура одинакова для всех тенантов.** Это и плюс (предсказуемость), и потенциальная проблема при необходимости per-tenant DDL-вариаций. Нашу архитектуру (banking — с одинаковой регуляторикой) такие вариации не должны требовать; если потребуются — это сигнал нарушения принципа multi-tenant.

## Implementation Notes

**Структура `tools/db-migrator`** (выносится в отдельный сервис):

```
tools/db-migrator/
├── cmd/
│   ├── platform/         # CLI: применить platform.* миграции
│   └── tenant/           # CLI: применить tenant.* миграции к одному тенанту
├── internal/
│   ├── workflow/         # Temporal workflows
│   ├── activity/         # Activities (single migration apply)
│   └── store/            # platform.tenant_migrations CRUD
├── migrations/           # symlink/embed на migrations всех сервисов
└── Dockerfile
```

**Таблица `platform.tenant_migrations`** (создаётся в первой platform-миграции):

```sql
CREATE TABLE platform.tenant_migrations (
    tenant_id    TEXT NOT NULL REFERENCES platform.tenants(id),
    service_name TEXT NOT NULL,                 -- 'tenant-service', 'audit-service', и т.д.
    version      INT  NOT NULL,
    applied_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_by   TEXT NOT NULL,                 -- run_id Temporal workflow
    checksum     TEXT NOT NULL,                 -- SHA-256 файла миграции
    PRIMARY KEY (tenant_id, service_name, version)
);
```

`checksum` нужен, чтобы детектировать ситуацию: миграция была применена, но потом файл изменён в репо (запрещено). Несовпадение → алерт.

**Команды разработчика:**

```bash
# Создать новую миграцию
nx generate db-migrator:migration --service=tenant-service --target=tenant --name=add_status_column

# Локальный прогон на dev-тенанте
make migrate-dev TENANT=demo

# Сухой прогон (только показать SQL, не применять)
make migrate-dev-dry-run

# Применение в staging (через CI)
# инициируется автоматически на merge в `staging` ветку

# Production применение
# отдельный manual approval gate в pipeline
```

**SLA на миграции в production:**

| Категория | Окно | Approver |
|---|---|---|
| Backwards-compatible (add column nullable, add index, add table) | в любое время | Tech lead |
| Изменения горячих таблиц (ALTER на applications, audit_events) | maintenance window 02:00–04:00 МСК | Tech lead + DBA |
| Удаление данных (drop column, drop table) | quarterly release window + банк-клиент уведомлён за 7 дней | Архитектор + DPO банка |
| Type changes (требующие full-table rewrite) | quarterly + ZDT-стратегия (shadow column → backfill → rename) | Архитектор |

## References

- ADR-0001 (Temporal): мы переиспользуем его для migration orchestration.
- ADR-0002 (Schema-per-tenant): этот ADR — прямое следствие, без 0002 решение было бы другим.
- ADR-0009 (Стратегия on-prem обновлений): описывает квартальный цикл, в который вписываются миграции; пока не принят.
- ADR-0010 (Биллинг и audit log): билинг получает события миграций как часть аудиторского следа.
- `docs/technical-structure.md` § 11 (CI/CD).
- [`golang-migrate/migrate`](https://github.com/golang-migrate/migrate) — основной инструмент.
- [PostgreSQL pessimistic vs optimistic DDL](https://www.postgresql.org/docs/current/sql-altertable.html) — обоснование, почему long-running ALTER требует maintenance window.
