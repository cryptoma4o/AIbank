<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# configs/tenants/

## Purpose

Конфигурации конкретных банков-клиентов. Каждый банк — отдельная поддиректория `{tenant-id}/` с полным набором YAML/JSON-файлов. Сейчас есть только шаблон (`_template/`), реальные конфиги появятся при onboarding первых банков.

## Key Files

| Файл | Описание |
|------|----------|
| `_template/README.md` | Описание структуры шаблона конфигурации тенанта и инструкция по onboarding |

## Subdirectories

| Директория | Назначение |
|-----------|-----------|
| `_template/` | Шаблон конфигурации для нового банка — копировать при onboarding (см. `_template/AGENTS.md`) |

## Planned Additions

| Директория | Назначение | Когда появится |
|-----------|-----------|------|
| `demo-bank/` | Демо-тенант для продаж и тестирования | Ф2 (Фаза 5 Sandbox) |
| `{real-bank-id}/` | Конфигурация первого пилотного банка | Ф7 (первый пилот) |

## For AI Agents

### Working In This Directory

- **Никогда не коммить реальные банковские данные** без явного согласования с командой безопасности.
- Tenant ID — строковый, без спецсимволов, формат: `{bank-short-name}` (например, `alfa-bank`, `sovком-bank`). Используется как prefix для K8s namespaces, PostgreSQL schema, S3 buckets — должен быть DNS-safe.
- Структура каждого тенанта — строго по шаблону `_template/`. Дополнительные файлы вне схемы будут проигнорированы ArgoCD.
- При onboarding нового банка: скопируй `_template/` → заполни → открой PR → пройди автоматическую валидацию.

### Isolation per Tenant

Каждый тенант `{id}` соответствует:
- PostgreSQL schema: `tenant_{id}`
- S3 bucket: `platform-{id}-documents`
- Kafka topic prefix: `{id}.`
- K8s namespace: `tenant-{id}`
- Vault path: `kv/tenants/{id}/`
- Redis key prefix: `tenant:{id}:`

### Testing Requirements

При добавлении/изменении конфигурации тенанта CI прогоняет:
1. `validate-tenant-config` — JSON Schema по всем файлам
2. `business-validate` — проверка бизнес-правил (не убраны обязательные проверки)
3. `dry-run-staging` — конфиг применяется на staging-кластер без реального эффекта
4. Smoke tests на staging после apply

## Dependencies

### Internal
- `_template/` — шаблон для новых тенантов
- `../../packages/tenant-config-schema/` — JSON Schema для валидации
- `../../services/tenant-service/` — источник правды по структуре конфига

<!-- MANUAL: -->
