<!-- Parent: ./AGENTS.md -->
<!-- Generated: 2026-04-28 -->

# AIbank — обзор системы

Высокоуровневое описание платформы для founder'ов, инвесторов,
банков-партнёров и новых членов команды. Без углубления в код.

---

## 1. Что такое AIbank (one-liner)

**White-label платформа цифрового онбординга юридических лиц для российских
банков**, автоматизирующая до 70-80% процесса от заявки клиента до
открытого расчётного счёта в АБС банка через AI-агентов и canonical
интеграции.

**Целевая аудитория**: банки 30-200 места по активам, для которых ручной
KYC/KYB — drag на time-to-market и unit-economics.

---

## 2. Что система делает

### 2.1 Бизнес-flow клиента

```
клиент → форма / документы → AI обработка → решение → счёт открыт
                                                       (90 секунд)
```

Конкретно:

1. **Заявка**: клиент (физлицо или представитель юрлица) заходит на
   white-label-сайт банка, вводит ОПФ (ООО / ИП / АО), отгружает паспорт
   и учредительные документы.
2. **Document Intake AI**: vision-LLM извлекает поля (паспортные данные,
   реквизиты юрлица, ИНН/ОГРН/КПП, адрес, ОКВЭД) с проверкой контрольных
   сумм по алгоритмам ФНС.
3. **Reconciliation AI**: сверяет извлечённые данные с ЕГРЮЛ через
   federated API ФНС.
4. **UBO Tracing AI**: строит граф владения юрлица, обнаруживает
   офшорные/foreign chains, определяет конечных бенефициаров (≥25% доли
   per 115-ФЗ § 7.3).
5. **Risk Scoring AI**: explainable scoring с SHAP-style факторами;
   результат — категория `low / medium / high` + численный score.
6. **Compliance проверки**: Росфинмониторинг 115-ФЗ список,
   ФССП исп. производства, СПАРК financial health.
7. **Decision Engine**: per-tenant пороги (auto_approve / manual_review /
   auto_decline) → автоматическое решение или эскалация в operator UI.
8. **Operator UI** (для manual review): compliance-офицер банка видит
   все собранные данные + reasoning AI-агентов + risk factors, принимает
   решение с обязательным комментарием.
9. **ABS Integration**: открытие счёта в АБС банка через canonical-команды
   (адаптеры для ЦФТ / Diasoft / RS-Bank).
10. **Conversational AI**: чат-бот для клиентских вопросов на русском с
    hard-guarded escalation на сложных кейсах.

---

## 3. Архитектурные слои

### 3.1 Frontend (`apps/`)

| Приложение | Кому | Что |
|------------|------|-----|
| `web-onboarding` | Клиент банка | Login, заявки, upload документов, статус |
| `web-admin` | Compliance-офицер банка | Список заявок, decision modal, audit-trail, tenant management |

**Стек**: TypeScript 5, Next.js 14 (App Router), Tailwind, shadcn/ui,
Apollo Client (GraphQL).

### 3.2 BFF (`services/bff-*`)

| Сервис | Endpoint | Кому |
|--------|----------|------|
| `bff-onboarding` | GraphQL :8091 | web-onboarding → backend |
| `bff-admin` | GraphQL :8092 | web-admin → backend (RBAC) |

**Зачем**: per-portal aggregation вместо чтения 5+ микросервисов из
браузера. Single GraphQL endpoint, JWT middleware, retry на 5xx.

### 3.3 Backend микросервисы (`services/`, 21 шт.)

| Категория | Сервисы |
|-----------|---------|
| **Core** | api-gateway (8000), tenant-service (8080), identity-service (8082) |
| **Onboarding** | onboarding-orchestrator (8085, Temporal), client-service (8089), document-service (8083) |
| **Audit** | audit-service (8081, append-only hash chain) |
| **Risk & UBO** | risk-engine (8086), ubo-service (8090) |
| **Billing** | billing-service (8087, ADR-0010) |
| **ABS bridge** | abs-connector (8088), abs-adapter-cft/-diasoft/-rs-bank (9001-9003) |
| **External KYC** | ext-egrul (8201), ext-rosfinmon (8202), ext-fssp (8203), ext-spark (8204) |
| **Notification** | notification-service (8110) |

**Стек**: Go 1.22, chi router, gRPC, sqlc, Temporal SDK, OpenTelemetry,
JWT (HS256), bcrypt, PostgreSQL.

### 3.4 AI слой (`ai/`)

| Сервис | Порт | Назначение |
|--------|------|------------|
| `llm-gateway` | 8100 | OpenAI-compatible proxy с role-based routing, per-tenant биллингом, mock-backend |
| `agent-document-intake` | 8101 | Извлечение данных из PDF (vision-LLM) |
| `agent-reconciliation` | 8102 | Сверка с ЕГРЮЛ |
| `agent-ubo-tracing` | 8103 | Граф владения + UBO ≥25% |
| `agent-conversational` | 8104 | Русскоязычный чат с escalation |
| `agent-risk-scoring` | 8105 | Explainable scoring |
| `agent-compliance-assistant` | 8106 | 115-ФЗ / 375-П проверки |
| `rag-service` | — | Semantic search по регуляторике |
| `eval-harness` | — | Регрессионные метрики качества AI |

**Стек**: Python 3.12, FastAPI, Pydantic 2, OpenTelemetry, vLLM/SGLang
(запланировано), Qdrant (vector DB), BGE-M3 embedder, BGE-reranker-v2-m3.

### 3.5 Shared библиотеки (`packages/`)

| Пакет | Назначение |
|-------|-----------|
| `domain-model` | Канонические сущности (Tenant, Application, Person, LegalEntity, UBOGraph, Document, RiskAssessment, Decision, Account, AuditEvent) — JSON Schema |
| `proto` | Protobuf definitions для gRPC |
| `openapi` | 7 OpenAPI 3.0.3 specs |
| `tenant-config-schema` | JSON Schema для YAML-конфигов тенантов |
| `audit-sdk` | Go + Python клиенты для emit audit events |
| `outbox` | Transactional outbox pattern с DLQ для poison messages |
| `secrets` | Vault KV v2 + AppRole login + auto-renewal |
| `signature` | Provider-абстракция УКЭП |
| `signature/ed25519` | Production-grade Ed25519 signer |
| `signature/gost2012` | STUB (preparation для КриптоПро) |
| `pii-encryption` | Field-level encryption через Vault Transit |
| `crypto-utils` | Hashing/signing helpers |
| `rule-engine` | Декларативные business rules |
| `observability` | OpenTelemetry SDK wrapper |
| `healthz` | /healthz + /readyz контракт |
| `ui-kit` | React-компоненты для white-label UI |

### 3.6 Infrastructure (`infrastructure/`)

| Компонент | Технология |
|-----------|------------|
| Container orchestration | Kubernetes 1.30+ (Helm charts) |
| Service mesh | Istio (mTLS + AuthorizationPolicy) — pre-integration |
| Secrets management | HashiCorp Vault HA (Raft + auto-unseal) — pre-integration |
| Database | PostgreSQL 16 (schema-per-tenant per ADR-0002) |
| Message broker | Kafka 3.7 (KRaft) |
| Cache | Redis 7 |
| Object storage | MinIO (S3-compatible) |
| Vector DB | Qdrant v1.9 + pgvector |
| Workflow engine | Temporal 1.24 (self-hosted, PostgreSQL backend) |
| Observability | VictoriaMetrics + Grafana + Loki + Tempo |
| LLM inference | vLLM, SGLang (после получения GPU) |

**IaC**: Helm-чарты (включая custom для Vault HA, istio-mesh, aibank-service),
Terraform (Yandex Cloud), Ansible (on-prem provisioning).

### 3.7 Tools (`tools/`)

| Утилита | Назначение |
|---------|-----------|
| `tenant-cli` | CLI управления тенантами (создание, обновление YAML) |
| `db-migrator` | Apply/rollback миграций PostgreSQL |
| `mock-abs` | Mock ABS API для contract-tests |
| `mock-smev` | Mock SMEV3-server для ext-egrul live integration |
| `audit-verifier` | Валидация append-only audit-log + signature verification |
| `data-generator` | Синтетические кейсы для eval-harness (паспорта, ИНН/ОГРН с checksum) |
| `benchmarks` | k6 нагрузочные тесты с baselines |
| `outbox-cleanup` | CronJob для удаления published rows из outbox |

---

## 4. Architectural Decision Records (ADR)

14 принятых решений в `docs/adr/`:

| # | Решение | Статус |
|---|---------|--------|
| 0001 | Temporal для оркестрации онбординг-воркфлоу | Accepted |
| 0002 | Schema-per-tenant в PostgreSQL | Accepted |
| 0003 | GraphQL для BFF | Accepted |
| 0004 | Nx как инструмент управления монорепо | Accepted |
| 0005 | Стратегия миграций БД для multi-tenant | Accepted |
| 0006 | Версионирование ABS-адаптеров (semver) | Accepted |
| 0007 | Prompt-management через YAML в Git | Accepted |
| 0008 | Vector DB: Qdrant (RAG) + pgvector (embedding sample) | Accepted |
| 0009 | On-prem update strategy (blue-green через Istio) | Accepted |
| 0010 | Биллинг + audit log (Kafka outbox + UUID v5 idempotency) | Accepted |
| 0011 | LLM routing strategy (multi-model role-based pool) | Accepted |
| 0012 | Eval-corpus governance (per-corpus owner + KL-divergence drift) | Accepted |
| 0013 | Vault HA topology + AppRole | **Proposed** (ждём DevOps review) |
| 0014 | mTLS Istio + AuthorizationPolicy | **Proposed** (ждём DevOps review) |

---

## 5. Что было сделано за сессию (2026-04-28, 1 день)

### 5.1 Foundation (Цикл 0)

- Реструктуризация документации для агентов: `CLAUDE.md` сжат с
  244 → 73 строк (-60% токенов на каждой сессии)
- Создан `docs/agents-rules.md` (правила работы агентов)
- 3 skill в `.claude/skills/`: aibank-add-agent, aibank-add-service,
  aibank-domain-change
- Makefile-таргеты: `context-light`, `agent-pr-check`, `changed-services`

### 5.2 Reliability (Циклы 2 + 6 раунд 2)

- **Outbox DLQ для poison messages**: после `MaxAttempts=5` строки
  перемещаются в `*_dead_letter`
- **Outbox cleanup published rows**: iterative DELETE с SKIP LOCKED + CLI
  `tools/outbox-cleanup` + Helm CronJob (default OFF)
- **Runbook outbox-dlq-recovery.md**: inventory, классификация причин,
  bulk replay, selective discard

### 5.3 Security / Audit (Циклы 4-5 + 6 раунд 2)

- **Audit криптоподпись**: 3 nullable колонки в `audit.events`
  (signature/algorithm/signer_key_id), миграция 002
- **`packages/signature/ed25519`**: production-grade signer (Generate /
  FromSeed / FromBase64Seed / FromEnv), 11 тестов
- **`packages/signature/gost2012`** STUB: подготовка к КриптоПро,
  Algorithm "gost-2012-256-stub", 16 тестов
- **`packages/signature` provider abstraction**: SignatureProvider interface,
  SignedPayload, MockSignatureProvider
- **`packages/pii-encryption`**: field-level encryption через Vault Transit
  для паспорт/СНИЛС/ИНН-физлица, 20 тестов
- **`audit-verifier` CLI**: `--pubkey` / `--pubkey-file` для
  ed25519-verification поверх hash chain
- **End-to-end e2e test** через testcontainers: записывает 10 signed
  events → audit-verifier → tampering detection (build-tag `e2e`)

### 5.4 AI / RAG (Циклы 3 + 6)

- **`ai/llm-gateway` env-overrides**: `VLLM_GEMMA_URL` / `VLLM_QWEN_URL` /
  `VLLM_TPRO_URL` + generic `VLLM_BACKEND_URL_<NAME>`
- **`ai/eval-harness/baselines/`**: формат для регрессии-метрик
- **`ai/rag-service` pluggable Reranker**: `LexicalReranker` (default),
  `BGEReranker` (HTTP к TEI/vLLM), `NoOpReranker`. Switch через
  `RAG_RERANKER` ENV. 12 тестов

### 5.5 KYC интеграции (Цикл 3)

- **`services/ext-egrul` LiveProvider**: реальный HTTP-клиент к СМЭВ-3,
  retry (100/200/400 ms), circuit breaker (5 fails → 30s open)
- **`services/ext-egrul` XML→LegalEntity mapper**: нормализация ОПФ,
  статусов, дат, рублей в копейки
- **`tools/mock-smev`** (новый Go-модуль): SMEV3 mock-server для
  contract-tests

### 5.6 ABS интеграции (Цикл 3)

- **`services/abs-adapter-cft` contract tests** под build-tag `contract`:
  12 golden fixtures (canonical ↔ ЦФТ XML) для 4 команд (open_account_llc,
  get_balance, close_account, reject_unknown). 11 тестов

### 5.7 Frontend (Цикл 7)

- **Document upload UX**: drag-and-drop модалка `DocumentUploadModal.tsx`,
  HTML5 drag events, multi-file селектор, per-file статус
  (queued/uploading%/done/error), retry на error, MIME/size валидация
  client-side. XHR для upload-progress callback. 2 e2e теста (Playwright).

### 5.8 Infrastructure (Цикл 8 + 9)

- **`packages/secrets/approle.go`**: AppRole login flow с auto-renewal
- **`infrastructure/helm/charts/vault/`**: 3-узловой Raft HA, опциональный
  auto-unseal через cloud KMS (Yandex/AWS/GCP) или Shamir
- **`infrastructure/helm/charts/istio-mesh/`**: AIbank-policies — STRICT
  PeerAuthentication, AuthorizationPolicy default-deny + allow rules,
  NetworkPolicy как defence-in-depth

### 5.9 Documentation

- **ADR-0013** Vault HA topology (Proposed)
- **ADR-0014** mTLS Istio (Proposed)
- **`docs/devops-review-package.md`**: единый artefact для DevOps review
  (7 решений с чек-боксами, structure встречи, action items template)
- **`docs/audit-bundle.md`**: для отправки CISO банка-партнёра
  (one-pager, deep-dive, ИБ-чек-листы)
- **`docs/demo-script.md`**: пошаговый walkthrough встречи 15-20 мин с
  talking points
- **`docs/demo-slide-deck-outline.md`**: структура 12 слайдов
- **`demo/`** комплект: 3 бизнес-сценария (LLC happy / IP manual review /
  AO rejected), seed JSON, bash-orchestrator, teardown
- **`.devcontainer/`**: GitHub Codespaces config (Go 1.22, Node 20,
  Python 3.12, Docker-in-Docker)
- **`docs/runbooks/`**: 7 runbook'ов (incident-response, audit-log-integrity,
  break-glass, db-migration, llm-gateway-troubleshooting, temporal-recovery,
  outbox-dlq-recovery)

---

## 6. Метрики проекта

| Метрика | Значение |
|---------|----------|
| **Backend сервисов** | 21 (Go 1.22) |
| **AI агентов** | 6 (Python 3.12) |
| **Frontend приложений** | 2 (Next.js 14) |
| **Shared packages** | 16 |
| **Tools / utilities** | 8 |
| **ADR** | 14 (12 Accepted + 2 Proposed) |
| **OpenAPI spec'ов** | 7 (35 paths, 57 schemas) |
| **Helm charts** | 11 (включая Vault, Istio, aibank-service, Postgres, Kafka, Redis, MinIO, Temporal, llm-gateway, postgres) |
| **Runbook'ов** | 7 |
| **Demo сценариев** | 3 (LLC / IP / AO) |
| **Тестов** | 600+ unit + integration + contract + e2e |
| **LOC** (~) | 890K (включая apps/web-* — большинство) |

---

## 7. Pilot-readiness статус (2026-04-28)

### 🟢 Готово

- 21 Go-сервис + 6 AI-агентов: тесты зелёные, build clean
- 14 ADR, threat model, compliance map (115-ФЗ, 375-П, 152-ФЗ, 187-ФЗ, 63-ФЗ)
- Audit-bundle для CISO банка
- Demo для встреч с банками
- DevOps review package для ИТ-команд
- Frontend dev-server поднимается, оба портала рендерятся

### 🟡 Pre-integration (готово к флипу при получении ресурса)

- ext-egrul live SOAP (mapper + retry + circuit breaker — ждём ФНС-договор)
- packages/signature (mock + ed25519 production + gost2012 stub —
  ждём КриптоПро лицензии)
- llm-gateway live mode (env-overrides готовы — ждём GPU + vLLM weights)
- Vault HA chart (skeleton — ждём DevOps выбор KMS)
- Istio mesh chart (skeleton — ждём DevOps выбор version + CA)

### 🔴 Требует внешних ресурсов (lead-time 2-12 недель)

- ФНС/ЕГРЮЛ договор через Минцифры (4-8 нед)
- Росфинмониторинг договор (4-6 нед)
- УКЭП КриптоПро/VipNet лицензии (2-4 нед)
- GPU для vLLM (2-3 нед)
- Партнёр-банк для пилота (2-3 мес)
- Pen-test внешний (2-3 нед)

---

## 8. Что отсутствует / out of scope

- **Mobile apps**: PWA достаточно в Y1 (per product-vision); native
  iOS/Android — Y2+
- **Multi-region failover**: поддержка single-region в MVP, multi-region
  в Y2 при росте >5 банков
- **Realtime metrics для клиента**: client-side polling вместо WebSocket
  в MVP
- **Full GDPR-compliance**: 152-ФЗ покрыт, GDPR — для CIS банков не
  актуально
- **Auto-scaling AI workers**: scaling в Y2, MVP — fixed pool

---

## 9. Где почитать дальше

| Тема | Файл |
|------|------|
| Архитектура | [docs/technical-structure.md](./technical-structure.md) |
| Доменная модель | [docs/domain-model.md](./domain-model.md) |
| Безопасность | [docs/security-architecture.md](./security-architecture.md) |
| Compliance | [docs/compliance-map.md](./compliance-map.md) |
| AI агенты | [docs/ai-agents-automation.md](./ai-agents-automation.md) |
| Pilot-readiness | [docs/pilot-readiness.md](./pilot-readiness.md) |
| Demo для банков | [docs/demo-script.md](./demo-script.md) |
| DevOps review | [docs/devops-review-package.md](./devops-review-package.md) |
| Audit-bundle для CISO | [docs/audit-bundle.md](./audit-bundle.md) |
| ADR | [docs/adr/](./adr/) |
| Runbook'ы | [docs/runbooks/](./runbooks/) |
