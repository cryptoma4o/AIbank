<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# AIbank — Onboarding Platform

## Purpose

White-label платформа цифрового онбординга юридических лиц (ИП, ООО, АО) для российских банков (30–200 место по активам). Автоматизирует KYC/KYB-проверки, построение графа УБО, риск-скоринг и открытие счёта в АБС. Статус: **Pre-MVP, Фаза 0** — идёт формализация домена и контрактов.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Навигационная страница: кому что читать, структура репо, быстрый старт |
| `CHANGELOG.md` | История значимых изменений (Keep a Changelog + SemVer) |
| `AGENTS.md` | Этот файл — точка входа для AI-агентов |

## Subdirectories

| Директория | Назначение |
|-----------|-----------|
| `docs/` | Вся проектная документация: product vision, техническая структура, доменная модель, безопасность, комплаенс, AI-агенты, тенант-конфигурация (см. `docs/AGENTS.md`) |
| `apps/` | Фронтенд-приложения: web-onboarding, web-admin, courier-tablet, ops-dashboard (см. `apps/AGENTS.md`) |
| `services/` | Бэкенд-микросервисы: api-gateway, BFF, tenant/identity/onboarding/risk/document/ubo/client сервисы, ABS-коннектор и адаптеры, ext-интеграции (см. `services/AGENTS.md`) |
| `ai/` | ИИ-слой: llm-gateway, 6 агентов, RAG-сервис, eval-harness (см. `ai/AGENTS.md`) |
| `packages/` | Shared-библиотеки: domain-model, proto, openapi, ui-kit, tenant-config-schema, audit-sdk, rule-engine, crypto-utils (см. `packages/AGENTS.md`) |
| `infrastructure/` | IaC: Helm-чарты, Terraform (Yandex Cloud), Ansible (on-prem) (см. `infrastructure/AGENTS.md`) |
| `configs/` | Конфигурации тенантов (банков): YAML-файлы с бизнес-правилами, брендингом, интеграциями (см. `configs/AGENTS.md`) |
| `tools/` | Внутренние утилиты: mock-ABS, tenant-cli, data-generator (см. `tools/AGENTS.md`) |

## For AI Agents

### Архитектурные принципы (нельзя нарушать без ADR)

- **Multi-tenant by design**: изоляция на уровне PostgreSQL schema, S3 bucket, Kafka topics. Никаких `tenant_id` в общих таблицах.
- **Hybrid deployment**: SaaS + on-prem. Запрещены managed-сервисы AWS/GCP в коде. Только: PostgreSQL, Kafka, Redis, MinIO.
- **Event-driven**: Kafka — нервная система. Синхронный RPC — только там, где критично для UX.
- **Configuration over code**: бизнес-правила и воркфлоу — в YAML-конфигах тенанта, не в коде.
- **Self-hosted LLM**: модели в инфраструктуре (своей или банка), не за периметром.
- **Доменная модель — контракт**: изменения только через ADR + согласование архитектора.

### Технологический стек (краткая сводка)

| Слой | Технология |
|------|-----------|
| Backend services | Go 1.22+ |
| AI / ML | Python 3.12 (FastAPI, vLLM, LangGraph) |
| Frontend | TypeScript 5, Next.js 14 |
| Infrastructure | Kubernetes, Helm, Vault, Keycloak; PostgreSQL/Kafka/Redis/MinIO/Qdrant |

Полная таблица версий, инструменты orchestration / observability / CI: см. `docs/technical-structure.md` § 3.

### Навигация по задаче

| Если работаешь над... | Читай в первую очередь |
|-----------------------|------------------------|
| Доменной моделью / контрактами | `docs/domain-model.md` |
| Бэкенд-сервисом | `docs/domain-model.md` + `docs/tenant-configuration.md` |
| AI-агентом / ML | `docs/ai-agents-automation.md` |
| Инфраструктурой / безопасностью | `docs/security-architecture.md` + `docs/compliance-map.md` |
| Архитектурным решением | `docs/adr/README.md` → список ADR |
| Конфигурацией тенанта | `configs/tenants/_template/` + `docs/tenant-configuration.md` |

### Рабочий процесс

- Читай документацию в `docs/` прежде чем трогать доменную модель или контракты
- ADR в `docs/adr/` — источник правды для архитектурных решений; проверяй список перед добавлением зависимостей
- Все новые API описываются в OpenAPI/proto (`packages/openapi/`, `packages/proto/`) до реализации
- Конфигурации тенантов валидируются JSON Schema из `packages/tenant-config-schema/`
- Секреты — только в Vault, никогда в Git или переменных среды prod-сервисов
- Audit log — append-only, иммутабельный; никогда не удаляй и не изменяй записи

### Testing Requirements

- Unit + integration + contract тесты на каждый PR
- E2e (Playwright) — ночные на staging
- Для AI-слоя: eval-harness на каждый PR в `ai/`, регрессия >5% блокирует merge
- Temporal-воркфлоу: обязательный e2e тест с Temporal Test Framework
- Contract-тесты для каждого ABS-адаптера (golden tests с mock-ABS)

### Зоны ответственности

| Документ | Владелец |
|---------|---------|
| Доменная модель | Главный архитектор |
| Безопасность | Security-инженер |
| Комплаенс-карта | Юрист + комплаенс-эксперт |
| Тенант-конфиги | Тех-лид платформы |
| AI-агенты | ML-инженер + банковский эксперт |

## Dependencies

### Internal
- Все сервисы используют `packages/domain-model/` как каноническую модель данных
- `packages/audit-sdk/` обязателен в каждом сервисе для записи в audit log

### External
- ЕГРЮЛ/ЕГРИП (ФНС) — выписки по юрлицам, кешируются 24 ч
- ЕСИА (Госуслуги) — аутентификация клиентов
- Росфинмониторинг — перечни 115-ФЗ, обновление ежесуточно
- АБС банков (ЦФТ, Diasoft, RS-Bank) — открытие счетов
- КриптоПро / VipNet CSP — УКЭП (сертифицированные СКЗИ)

<!-- MANUAL: -->
