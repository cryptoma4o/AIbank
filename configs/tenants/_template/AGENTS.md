<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# configs/tenants/_template/

## Purpose

Шаблон конфигурации нового банка-клиента. При onboarding нового тенанта — копировать эту директорию в `configs/tenants/{tenant-id}/` и заполнять по документации. Содержимое файлов будет добавлено в Фазе 1.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Инструкция по использованию шаблона: как скопировать, заполнить и применить конфигурацию нового банка |

## Planned File Structure

```
_template/
├── tenant.yaml                  # Основные параметры банка (ID, BIK, INN, контакты, features)
├── branding/
│   ├── theme.json               # Цвета (primary, secondary), шрифты, логотипы (vault-ссылки), домен
│   └── content.yaml             # Тексты писем, уведомлений, UI-строки, ответы чат-агента
├── workflows/
│   ├── ip.yaml                  # Воркфлоу для ИП: шаги, required docs, timeout'ы, error handling
│   ├── llc.yaml                 # Воркфлоу для ООО: аналогично + UBO tracing
│   └── jsc.yaml                 # Воркфлоу для АО: сложнее, реестродержатели
├── risk-policy/
│   ├── rules.yaml               # Жёсткие правила: ROSFINMON_MATCH, BANKRUPTCY, BLOCKED_OKVED и др.
│   ├── thresholds.yaml          # Пороги скоринга: low_max, medium_max, auto_approve_criteria
│   └── blocked-okveds.yaml      # ОКВЭДы, по которым банк не открывает счета
├── integrations/
│   ├── abs.yaml                 # АБС: type (cft|diasoft|rs_bank), endpoint (vault-ссылка), маппинг статусов
│   ├── external.yaml            # ЕГРЮЛ, СПАРК, БКИ: включены/выключены, endpoint'ы
│   └── notifications.yaml       # Email/SMS/Push провайдеры и их настройки
├── ai/
│   ├── models.yaml              # Маппинг ролей агентов на модели, rate limits, cost alerts
│   └── prompts.yaml             # Кастомные промпты поверх дефолтных (тон, специфика банка)
├── sla.yaml                     # SLA-цели по типам заявок (ИП: 5 мин, ООО low risk: 15 мин и т.д.)
├── compliance/
│   ├── consents.yaml            # Тексты форм согласий на обработку ПДн
│   ├── retention.yaml           # Сроки хранения (если отличаются от дефолтных 5 лет)
│   └── document-templates/      # Шаблоны договоров банка (HTML/DOCX)
└── security/
    └── access-policies.yaml     # RBAC-политики: какие роли видят какие данные
```

## For AI Agents

### Working In This Directory

- Не редактируй `_template/` напрямую — это шаблон, не конфигурация реального банка.
- При изменении структуры шаблона — синхронизируй JSON Schema в `../../../packages/tenant-config-schema/` и документацию в `../../../docs/tenant-configuration.md`.
- Все значения с `vault://kv/...` — ссылки на Vault. В шаблоне это плейсхолдеры; при заполнении конфига тенанта — должны указывать на реальные секреты.
- Поле `schema_version: "1.0"` в каждом файле — обязательно. Инкрементировать при изменении схемы.

### Ключевые поля tenant.yaml

```yaml
tenant:
  id: "{tenant-id}"           # DNS-safe, без спецсимволов
  status: "trial"             # trial при создании → active после проверки
  deployment:
    mode: "saas|on_prem|hybrid"
  features:
    enabled: []               # явный список включённых фич
    disabled: []              # явный список выключенных
```

### Ограничения (mutability)

Некоторые поля — read-only для тенанта (нельзя переопределить через конфиг):
- Обязательные регуляторные проверки (Росфинмониторинг, скрининг по 115-ФЗ)
- Минимальные SLA (нельзя установить меньше технологического минимума)
- Гардрейлы AI (`pii_filter: always_on`, `audit_logging: always_on`)

## Dependencies

### Internal
- `../../../packages/tenant-config-schema/` — JSON Schema для валидации каждого файла
- `../../../docs/tenant-configuration.md` — полная документация всех параметров с примерами

<!-- MANUAL: -->
