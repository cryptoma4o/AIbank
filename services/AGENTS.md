<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# services/

## Purpose

Бэкенд-микросервисы платформы. Каждый сервис — отдельная кодовая база, деплоится независимо. Основной язык — **Go 1.22+**. Исключение: ABS-адаптеры с legacy SOAP/MQ протоколами могут быть на Java 21. Статус: **директория пустая, сервисы создаются с Фазы 1**.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Обзор слоя бэкенд-сервисов: список сервисов, технологии, принципы изоляции тенантов |

## Planned Subdirectories

### Edge Layer
| Директория | Назначение |
|-----------|-----------|
| `api-gateway/` | Единая точка входа: tenant routing по subdomain, rate limiting per-tenant, JWT-проверка, audit log всех запросов |
| `bff-onboarding/` | GraphQL BFF для `apps/web-onboarding/` — агрегирует данные из core services |
| `bff-admin/` | GraphQL BFF для `apps/web-admin/` |

### Core Services
| Директория | Назначение |
|-----------|-----------|
| `tenant-service/` | CRUD тенантов и их конфигураций, версионирование, JSON Schema валидация. Источник правды для всех сервисов |
| `identity-service/` | Аутентификация клиентов (ЕСИА, УКЭП, логин+пароль+SMS), сотрудников банка (Keycloak SSO + MFA), ГОСТ-криптография |
| `onboarding-orchestrator/` | **Temporal-воркфлоу** — сердце платформы. Каждая заявка = один воркфлоу. Activities вызывают AI-агентов, ABS-коннектор, ext-сервисы |
| `risk-engine/` | CatBoost-скоринг + декларативные правила из `packages/rule-engine/`. Объяснимость через SHAP-values |
| `document-service/` | Приём, хранение, версионирование, подписание документов. УКЭП через КриптоПро/VipNet. OCR вызывает `ai/agent-document-intake/` |
| `ubo-service/` | Хранение и версионирование графа владения (recursive CTE в PostgreSQL). Вычисления — в `ai/agent-ubo-tracing/` |
| `client-service/` | Единая карточка клиента (КУС) — history всех событий, документов, решений |

### Integration Layer — ABS
| Директория | Назначение |
|-----------|-----------|
| `abs-connector/` | Фасад над адаптерами. Канонические команды → нужный адаптер по тенанту. Идемпотентность через Redis dedup-store |
| `abs-adapter-cft/` | Адаптер ЦФТ (Центр Финансовых Технологий) |
| `abs-adapter-diasoft/` | Адаптер Diasoft FA# |
| `abs-adapter-rs-bank/` | Адаптер RS-Bank/ЦАБС |

### Integration Layer — External APIs
| Директория | Назначение |
|-----------|-----------|
| `ext-egrul/` | Интеграция с ФНС ЕГРЮЛ/ЕГРИП. Кеш ответов 24 ч в Redis |
| `ext-esia/` | Интеграция с ЕСИА (Госуслуги для бизнеса) |
| `ext-rosfinmon/` | Перечни 115-ФЗ (Росфинмониторинг), обновление 1р/сутки |
| `ext-fssp/` | Проверка по базе ФССП (исполнительные производства). Используется Risk Scoring Agent в параллельном скрининге |
| `ext-spark/` | СПАРК или Контур.Фокус (опционально, по запросу тенанта) |

### Supporting Services
| Директория | Назначение |
|-----------|-----------|
| `notification-service/` | Email/SMS/Push уведомления. Шаблоны — из конфигурации тенанта |
| `audit-service/` | Write-only API для audit log (append-only, immutable, с цифровой подписью каждой записи) |
| `billing-service/` | Биллинг по тенантам — учёт per-event (открытый счёт, проверка УБО и т.д.) |

## For AI Agents

### Working In This Directory

- Каждый сервис: **Go 1.22+** (chi для HTTP, gRPC, sqlc для SQL, temporal-go-sdk).
- Изоляция тенантов: PostgreSQL schema-per-tenant. Никаких `tenant_id` в общих таблицах.
- Межсервисное взаимодействие: Kafka (async events) + gRPC (sync RPC только при необходимости UX).
- Все API описаны в `../packages/proto/` (gRPC) или `../packages/openapi/` (REST) **до реализации**.
- Секреты — только через Vault sidecar/CSI, никогда ENV vars.
- Каждый сервис обязан подключать `../packages/audit-sdk/` и писать audit events.
- mTLS между сервисами обеспечивается Istio — не реализовывать вручную.

### Testing Requirements

- Unit тесты: Go testing + testify
- Integration тесты: реальная БД (Docker Compose), не мок
- Contract тесты для ABS-адаптеров: golden tests с mock-ABS из `../tools/`
- Temporal workflows: Temporal Test Framework (`testsuite.WorkflowTestSuite`)

### Common Patterns

Структура Go-сервиса:
```
service-name/
├── cmd/server/main.go      # точка входа
├── internal/
│   ├── domain/             # доменная логика (без зависимостей на инфраструктуру)
│   ├── handler/            # HTTP/gRPC handlers
│   ├── repository/         # sqlc-generated + репозитории
│   └── workflow/           # Temporal workflow definitions (для orchestrator)
├── migrations/             # SQL миграции (goose)
└── Dockerfile
```

Канонические команды ABS (пример):
```yaml
command: OpenAccount
idempotency_key: "uuid"
tenant: "bank-alpha"
payload: { client_id, account_type, signature_card_id }
```

## Dependencies

### Internal
- `../packages/domain-model/` — канонические типы (Go-генерация)
- `../packages/proto/` — gRPC proto-файлы
- `../packages/audit-sdk/` — SDK записи audit log
- `../packages/rule-engine/` — движок правил для `risk-engine`
- `../packages/crypto-utils/` — ГОСТ-криптография
- `../ai/` — вызовы AI-агентов из `document-service`, `ubo-service`, `onboarding-orchestrator`

### External
- Temporal Server (self-hosted, PostgreSQL backend)
- PostgreSQL 16 (schema-per-tenant)
- Kafka (KRaft mode, без ZooKeeper)
- Redis 7 (кеш, dedup-store, сессии)
- MinIO (S3-совместимое хранилище документов)
- Vault (секреты, Transit encryption)
- Keycloak (SSO, RBAC)
- OpenSearch (поиск и аналитика — замена Elastic с учётом лицензий)
- КриптоПро CSP / VipNet CSP (УКЭП)

<!-- MANUAL: -->
