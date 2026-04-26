<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# packages/

## Purpose

Shared-библиотеки, используемые несколькими сервисами. Изменения в этих пакетах — кросс-командные и могут требовать ADR (особенно `domain-model`). Статус: **директория пустая, пакеты создаются с Фазы 0–1**.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Обзор shared-библиотек: список пакетов, принципы versioning, правила изменения domain-model |

## Planned Subdirectories

| Директория | Назначение | Фаза |
|-----------|-----------|------|
| `domain-model/` | Каноническая доменная модель. Единый источник: JSON Schema → генерация в TypeScript, Go, Python. **Создать первым (Ф0)** |
| `proto/` | gRPC proto-файлы для межсервисного взаимодействия. Генерация stub'ов для Go, Python, TypeScript. **Ф0** |
| `openapi/` | OpenAPI 3.x схемы REST API. Для BFF, внешних API. **Ф0** |
| `ui-kit/` | React-компоненты (white-label ready): поддержка брендинга тенанта через CSS-переменные. Storybook. **Ф1** |
| `tenant-config-schema/` | JSON Schema для всех конфигурационных файлов тенанта. Используется для валидации в CI. **Ф0** |
| `audit-sdk/` | SDK для записи событий в `audit-service`. Обязателен в каждом сервисе. Go + Python варианты. **Ф1** |
| `rule-engine/` | Декларативный движок правил (YAML → условия → действия). Используется в `risk-engine`. **Ф1** |
| `crypto-utils/` | Криптографические утилиты: ГОСТ 34.10-2012, AES-256-GCM, Ed25519 JWT, интеграция с Vault Transit. **Ф1** |

## For AI Agents

### Working In This Directory

- `domain-model/` — **самый критичный пакет**. Изменения типов сущностей — только через ADR при breaking changes. Добавление полей с дефолтами — backward-compatible, без ADR.
- Пакеты генерируют код для нескольких языков из единого источника (JSON Schema, proto). Изменяй источник, не генерированный код.
- `tenant-config-schema/` версионируется — поле `schema_version` в каждом конфиге тенанта. При изменении схемы — мигрируй существующие конфиги.
- `audit-sdk/` — append-only API. Никогда не добавляй операции изменения или удаления событий.
- `ui-kit/` — компоненты должны быть брендинг-нейтральными (цвета через CSS-переменные, логотипы через пропсы).

### Testing Requirements

- `domain-model/`: snapshot tests на генерированные файлы (Go, TS, Python) при изменении schema
- `proto/`: breaking change detection через `buf breaking`
- `openapi/`: валидация через `spectral`
- `ui-kit/`: Storybook + Chromatic для визуальной регрессии
- `rule-engine/`: unit тесты каждого правила с golden fixtures
- `crypto-utils/`: NIST test vectors для криптографических примитивов

### Common Patterns

ID-форматы доменной модели:
```
tnt_{ULID}  — Tenant
app_{ULID}  — Application
per_{ULID}  — Person
le_{ULID}   — LegalEntity
doc_{ULID}  — Document
acc_{ULID}  — Account
```

Versioning доменной модели:
- Backward-compatible (новые поля с дефолтами) → minor version, без ADR
- Breaking changes → major version + ADR + миграция данных
- Поддерживаем N-1 версию API минимум 6 месяцев

## Dependencies

### Internal
- Нет зависимостей от других директорий репо (`packages/` — самый нижний слой)

### External
- `buf` — protobuf toolchain
- `openapi-generator` — генерация клиентов из OpenAPI
- JSON Schema Draft 7 (для domain-model и tenant-config-schema)

<!-- MANUAL: -->
