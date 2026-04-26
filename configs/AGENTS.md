<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# configs/

## Purpose

Конфигурации тенантов (банков-клиентов). Каждый банк = отдельная поддиректория с YAML/JSON-файлами, описывающими его бизнес-правила, брендинг, воркфлоу, AI-модели и интеграции. Это главный артефакт «Configuration over Code» — ключевого архитектурного принципа платформы.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Обзор системы конфигурирования тенантов: структура, принципы, жизненный цикл изменений |

## Subdirectories

| Директория | Назначение |
|-----------|-----------|
| `tenants/` | Конфигурации конкретных банков-тенантов (см. `tenants/AGENTS.md`) |

## Planned Additions

| Директория | Назначение | Фаза |
|-----------|-----------|------|
| `schemas/` | JSON Schema для всех конфигурационных файлов (источник — `../packages/tenant-config-schema/`). Используется для валидации в CI | Ф0 |

## For AI Agents

### Working In This Directory

- Конфигурации — **декларативны**: описывают «что должно быть», не «как сделать».
- Все конфиги версионируются в Git. Изменения — через PR с автоматической валидацией.
- **Доставка через GitOps (ArgoCD)**: применение без ручного `kubectl` или прямых API-вызовов.
- Секреты (endpoint'ы АБС, API-ключи, сертификаты) — **только ссылки на Vault** (`vault://kv/tenants/{id}/...`), никогда не сами значения.
- Каждый конфиг-файл содержит `schema_version` — обязательно проверяй при изменении схемы.
- Default-конфигурация — самая строгая. Банк может только ослаблять в рамках `mutability: "policy_bounded"` полей.

### Жизненный цикл изменения конфигурации

```
Предложение изменения
    ↓
JSON Schema validation (автоматически, CI)
    ↓
Бизнес-валидация (нельзя убрать обязательную проверку Росфинмониторинга)
    ↓
Dry-run на staging
    ↓
PR + approval
    ↓
ArgoCD apply на staging → smoke tests
    ↓
Manual promote на production
    ↓
Audit-запись о применении
```

### Права редактирования

| Кто | Что редактирует | Уровень mutability |
|----|-----------------|-------------------|
| Бизнес-аналитик банка | Тексты, SLA, риск-пороги | `policy_bounded` |
| Tech-lead банка | Интеграции, AI-модели | `policy_bounded` + `free` |
| Комплаенс банка | Risk rules, blocked OKVEDS, retention | `policy_bounded` (аппрув второго лица) |
| Наш CS-менеджер | Помощь в миграции, troubleshooting | Читает, не правит сам |

### Testing Requirements

- CI проверяет: JSON Schema validation → бизнес-валидация → reachability check (ссылки на ABS/модели доступны) → dry-run
- Smoke tests после ArgoCD apply на staging

## Dependencies

### Internal
- `../packages/tenant-config-schema/` — JSON Schema для валидации
- `../services/tenant-service/` — читает и применяет конфигурации
- `../infrastructure/helm/` — ArgoCD использует конфиги как Helm values

<!-- MANUAL: -->
