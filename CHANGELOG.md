# Changelog

Все значимые изменения в проекте фиксируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
проект использует [Semantic Versioning](https://semver.org/lang/ru/).

## [Unreleased]

### Added — Pre-integration foundations (2026-04-28)

Ниже перечислено всё, что подготовлено к пилоту до получения внешних ресурсов
(ФНС-договор, GPU, КриптоПро/VipNet лицензии, партнёр-банк). Реальный production
rollout — после внешних ресурсов и DevOps review (см. [docs/devops-review-package.md](docs/devops-review-package.md)).

#### Reliability
- **Outbox DLQ для poison messages** (`packages/outbox`): после `MaxAttempts=5` (configurable) строка перемещается в `*_outbox_dead_letter`. Колонки `attempts/last_error/last_attempt_at`. Миграция 003 для billing-service (add-only). Конфигурация через `outbox.NewWithOptions(...)`.
- **Outbox cleanup published rows**: `Outbox.CleanupPublished(ctx, retention, batchSize)` — iterative DELETE с `SKIP LOCKED`, не блокирует relay. CLI `tools/outbox-cleanup` + Helm `CronJob` template (default OFF).
- **Runbook** [`docs/runbooks/outbox-dlq-recovery.md`](docs/runbooks/outbox-dlq-recovery.md) — inventory, классификация (transient / schema mismatch / poisoned / topic-removed), bulk replay через INSERT-back, selective discard.

#### AI / RAG
- **`ai/llm-gateway` env-overrides**: `VLLM_GEMMA_URL` / `VLLM_QWEN_URL` / `VLLM_TPRO_URL` + generic `VLLM_BACKEND_URL_<NAME>` — переключение на live vLLM без правки YAML. Mock-backend защищён.
- **`ai/eval-harness/baselines/`**: формат и placeholder `mock-2026-04-28.json` для регрессии-метрик (per ADR-0011, threshold 5 п.п.).
- **`ai/rag-service` pluggable Reranker**: `LexicalReranker` (default), `BGEReranker` (HTTP к TEI/vLLM с `BAAI/bge-reranker-v2-m3`), `NoOpReranker`. Конфигурация через `RAG_RERANKER` ENV, fallback на lexical при отсутствии URL. 12 unit-тестов.

#### Security / Audit
- **Audit криптоподпись (Ed25519 production-grade)**: `signature/signature_algorithm/signer_key_id` колонки в `audit.events` (миграция 002), `EventSigner` interface в handler, Ed25519 signer с auto-renewal-friendly keypair management. `audit-verifier --pubkey` / `--pubkey-file` для verification. End-to-end e2e test через testcontainers (build-tag `e2e`) — записывает 10 событий с подписью, проверяет верификатором, имитирует tampering и ловит mismatch.
- **`packages/signature/ed25519`**: 64-byte signature, KeyID = hex SHA-256(PublicKey), `FromSeed/FromBase64Seed/FromEnv/Generate`. 11 unit-тестов.
- **`packages/signature/gost2012`** STUB: подготовка к реальному КриптоПро. Algorithm `gost-2012-256-stub` явно отличается от целевого. KeyID с префиксом `STUB-` для grep'а в SOC. SHA-256 inside (не криптостойко). 16 тестов.
- **`packages/signature` provider abstraction**: `SignatureProvider` interface (Sign/Verify/ListCertificates), `SignedPayload` (DER-encoded signature + Streebog hash + cert chain + timestamp), `MockSignatureProvider` для тестов document-service / identity-service.
- **`packages/pii-encryption`** (новый): field-level encryption через Vault Transit для PII (паспорт, СНИЛС, ИНН физлица, bank-account, phone). `EncryptHash` для search-friendly equals lookups (HMAC-SHA256). `MockPIIEncryptor` для тестов. 20 unit + integration тестов через httptest mock Vault.

#### KYC интеграции
- **`services/ext-egrul` LiveProvider**: реальный HTTP-клиент к СМЭВ-3 endpoint, retry 100/200/400 ms, circuit breaker (5 fails → 30s open + half-open), audit через `slog`. Sentinel ошибки `ErrNotFound`, `ErrCircuitOpen`, `ErrUpstream`.
- **`services/ext-egrul` XML→LegalEntity mapper** (`internal/provider/xml_mapper.go`): нормализация ОПФ-кодов, статусов («Действующее»→`active`), дат (DD.MM.YYYY → ISO), рублей в копейки. 6 test groups (юрлицо/ИП/status/kopecks/errors/founder type inference).
- **`tools/mock-smev`** (новый Go-модуль): mock SMEV3-server для contract-tests, маршруты `/by-inn/{inn}` / `/by-ogrn/{ogrn}`, INN `9999999999` → 404 для теста `ErrNotFound`. 8 тестов.

#### ABS интеграции
- **`services/abs-adapter-cft` contract tests** под build-tag `contract`: 12 golden fixtures (canonical ↔ ЦФТ-формат) для 4 команд (open_account_llc, get_balance, close_account, reject_unknown). 11 тестов: roundtrip, deterministic ID, request/response translation. Translation-helpers временно в тест-пакете — переедут в `internal/handler/cft_translator.go` при ADR-0006 § 5 Phase 2.

#### Frontend
- **`apps/web-onboarding` document upload UX**: drag-and-drop модальное окно (`DocumentUploadModal.tsx`), HTML5 drag events без `react-dropzone` (без новых deps), per-file статус (queued/uploading%/done/error), retry на error, MIME/size валидация client-side. `lib/document-upload.ts` через XHR (для upload-progress callback). 2 e2e теста (Playwright).

#### Infrastructure
- **`packages/secrets/approle.go`**: AppRole login flow с auto-renewal (renewer goroutine, default 70% lease lifetime, fallback re-login на просрочке). 3 теста через fake Vault HTTP.
- **`infrastructure/helm/charts/vault/`** (новый skeleton): 3-узловой Raft HA, опциональный auto-unseal через cloud KMS (Yandex/AWS/GCP) или Shamir, encrypted PVC, PodDisruptionBudget, NetworkPolicy. README с bootstrap procedure. См. ADR-0013.
- **`infrastructure/helm/charts/istio-mesh/`** (новый skeleton): AIbank-policies для установленного Istio control plane — `PeerAuthentication STRICT`, `AuthorizationPolicy` default-deny + per-service allow, NetworkPolicy как defence-in-depth. README с install procedure. См. ADR-0014.

#### Documentation
- **`docs/adr/0013-vault-ha-topology.md`** (Proposed): Vault Raft HA + auto-unseal + AppRole, 4 open questions для DevOps decisions.
- **`docs/adr/0014-mtls-istio.md`** (Proposed): Istio mTLS + AuthorizationPolicy + workload identity (SPIFFE), 5 open questions.
- **`docs/devops-review-package.md`** — единый artefact для DevOps review (ссылки на ADR, self-validation команды, 7 решений с чек-боксами, structure встречи, action items template на 10 issues).
- **`docs/audit-bundle.md`** — для отправки CISO банка-партнёра. One-pager, deep-dive doc list, ADR-таблица для ИБ, pilot-readiness gap, ИБ-чек-листы.
- **Runbook `outbox-dlq-recovery.md`** (см. выше).
- **Optimization sessии (Цикл 0)**: CLAUDE.md сжат с 244 → 73 строк (-60% токенов), вынесена harness-документация в `.claude/AGENTS.md`. Создан `docs/agents-rules.md` (правила работы агентов), 3 skill в `.claude/skills/aibank-{add-agent,add-service,domain-change}`. Makefile-таргеты `context-light`, `agent-pr-check`, `changed-services` + скрипты в `scripts/`. 5 memory pattern-файлов для эволюции системы.

### Added — Documentation (initial)
- Начальная архитектурная документация: product vision, technical structure, AI agents, domain model, compliance map, security architecture, tenant configuration
- Шаблон ADR (`docs/adr/0000-template.md`)
- Структура монорепозитория

### Added — Architecture Decision Records (12 принятых)
- ADR-0001: Temporal для оркестрации онбординг-воркфлоу
- ADR-0002: schema-per-tenant в PostgreSQL для изоляции данных банков
- ADR-0003: GraphQL для BFF (single endpoint per BFF, gqlgen+codegen, без Federation в MVP)
- ADR-0004: Nx как инструмент управления монорепо
- ADR-0005: стратегия миграций БД для multi-tenant PostgreSQL (golang-migrate + Temporal orchestration)
- ADR-0006: версионирование ABS-адаптеров (отдельный gRPC-сервис на адаптер, semver, поддержка N-2 поколений canonical proto)
- ADR-0007: prompt-management — YAML в Git + `prompt_version` в audit log для регуляторной трассируемости
- ADR-0008: vector DB — Qdrant для RAG-корпуса, pgvector для embedding-сэмплов в платформенном Postgres
- ADR-0009: стратегия on-prem обновлений — blue-green namespaces с переключением через Istio VirtualService, подписанный air-gapped tarball-bundle
- ADR-0010: биллинг-модель и audit log — отдельный billing-service с `platform.billing_events` (append-only), Kafka outbox-pattern, idempotency через UUID v5, correlation_id с audit log
- ADR-0011: LLM routing strategy — multi-model role-based pool с per-tenant overrides, формальная процедура promotion candidate→production по eval-метрикам
- ADR-0012: eval-corpus governance — per-corpus owner (banking expert + ML co-owner), semver в manifest.yaml, monthly drift-detection через KL-divergence на embeddings

### Added — Backend services (Go 1.22)
- **tenant-service** (port 8080): HTTP API (Get/List/Create), provisioning схем тенантов через `PostgresSchemaProvisioner`, шаблон tenant migrations
- **audit-service** (port 8081): HTTP API (Append/List) с tamper-evident hash chain (SHA-256), append-only triggers
- **identity-service** (port 8082): HS256 JWT (access 15m + refresh 7d), bcrypt cost 12, login/refresh/me/applicants endpoints, миграции `platform.users` + tenant-template `applicants`; 11 unit-тестов
- **document-service** (port 8083): multipart upload до 25MB с whitelist MIME (pdf/png/jpeg/tiff), streaming SHA-256, Storage abstraction (in-memory + MinIO-stub с явными TODO), search_path-изоляция per tenant; 6 тестов
- **onboarding-orchestrator** (port 8085): Temporal workflows (15-state application state machine со строгими переходами), signal-driven `documents_uploaded` и `human_decision`, ABS SAGA compensation, активити-стабы для будущих интеграций; 4 workflow-теста + 36 sub-cases state machine
- **risk-engine** (port 8086): CatBoost-stub с детерминированной hash-функцией + 5 SHAP-style факторов, объяснимый scoring через `TextExplainer`, recommendation logic (auto-approve/manual-review/decline), интеграция с `packages/rule-engine` через replace-директиву; 25 тестов (scorer 9 + pipeline 6 + handler 6 + остальное)
- **billing-service** (port 8087, per ADR-0010): `platform.billing_events` append-only (cross-tenant) с UUID v5 идемпотентностью, базовая ценовая модель ИП/ООО/АО + UBO check + manual review, агрегация per tenant, fallback на 'basic' tier; 17 тестов
- **abs-connector** (port 8088, per ADR-0006): canonical-command facade с Redis dedup-store (24h TTL) + InMemory для тестов, registry адаптеров через YAML+env override, версионирование через `adapter_version` метаданные; 17 тестов (30 sub-tests)
- **client-service** (port 8089): КУС с PATCH /status и append-only `client_history` (UPDATE/DELETE forbidden), pagination guards; 7 тестов
- **ubo-service** (port 8090): хранение и версионирование UBO-графов (MAX(version)+1), JSONB nodes/edges/ubos, shape совпадает с Pydantic-схемой agent-ubo-tracing; 7 тестов
- **bff-onboarding** (port 8091, GraphQL per ADR-0003): single GraphQL endpoint, 5 HTTP-клиентов к upstream-сервисам с retry на 5xx, JWT middleware HS256, schema 201 строк (Tenant, Application, Document, Person, RiskAssessment, Decision); 22 теста
- **bff-admin** (port 8092, GraphQL): RBAC (требует bank.admin или platform.admin), Mutation `updateApplicationDecision` для manual review, `suspendTenant` (только platform.admin); 10 тестов
- **api-gateway** (port 8000): tenant routing по subdomain (`<tenant>.platform.ru`), Redis token-bucket rate-limit per `(tenant,ip)` 60rpm/burst10, JWT pre-validation, reverse proxy с longest-match-wins, JSON access log; 24 теста
- **notification-service** (port 8110): email/SMS/push с шаблонами text/template, MockSender для CI + SMTP/SMS-stub с явным `ErrNotImplemented`, 5 встроенных шаблонов на русском (welcome/approved/declined/document_request/otp); 15 тестов
- **abs-adapter-cft, -diasoft, -rs-bank** (порты 9001/9002/9003 per ADR-0006): canonical `/v1/execute`, /version endpoint, VERSION-файлы, детерминированные SHA-256-stub-ответы (cft → `cft_`+чистые цифры, diasoft → `diasoft_`+DIA, rs-bank → `rsbank_`+RSB); 27 тестов (9 на адаптер)
- **ext-egrul** (port 8201): GET by-inn/by-ogrn/founders с Redis cache 24h, синтетические LegalEntity-stubs детерминированные по hash(INN); 8 тестов
- **ext-rosfinmon** (port 8202): screening против синтетического 115-ФЗ перечня (50 записей + ~1% fuzzy match), GET list-info; 8 тестов
- **ext-fssp** (port 8203): по ИНН (юрлица) и по ФИО+дата_рождения (физлица), ~5% юрлиц с производствами; 6 тестов
- **ext-spark** (port 8204): financial_health (60% green / 30% yellow / 10% red), 3% sanctions; 6 тестов
- **tools/db-migrator**: CLI и K8s Job для применения миграций (platform / tenant / tenant-all) с защитой от drift через SHA-256 checksum (ADR-0005); 6 тестов парсера

### Added — Shared packages
- **packages/audit-sdk** (Go + Python): client с retry (exp backoff на 5xx/429, no-retry на 4xx), chi-middleware `EmitOnSuccess` (detached background ctx чтобы request shutdown не убивал audit write), correlation IDs; Go 18 тестов + Python 9 тестов
- **packages/openapi**: 7 полных OpenAPI 3.0.3 specs для backend-сервисов (tenant, audit, identity, document, onboarding-orchestrator, risk-engine + bff-onboarding HTTP-envelope) — 35 paths, 57 schemas в сумме

### Added — AI services (Python 3.12)
- **llm-gateway** v0.2 (port 8100): YAML-роутинг с ролями, fallback per role, mock-backend для CI/eval, per-tenant учёт стоимости в копейках, обязательный X-Tenant-Id, X-Agent-Role override, эндпоинт /v1/usage; 23 теста
- **agent-reconciliation** (port 8102, role: reconciliation): сверка извлечённых данных с ЕГРЮЛ через llm-gateway, детерминированные эвристики как fallback при сбое LLM; 9 тестов с FakeGateway monkeypatch
- **agent-ubo-tracing** (port 8103, role: ubo-tracing): построение графа владения и определение UBO ≥25%, поддержка nested ownership и unresolved foreign branches; 9 тестов
- **agent-conversational** (port 8104, role: ru-chat): русскоязычный чат с hard-guarded escalation (жалобы, причины отказа, мошенничество), system prompt запрещает обещание одобрения и раскрытие причин отказа; 10 тестов
- **agent-document-intake** (port 8101, role: document-vision): wired к llm-gateway, JSON-schema-in-prompt + tolerant parser с регексным fallback при сбое LLM, OCR-pipeline сохранён; 11 тестов
- **agent-risk-scoring** (role: text-reasoning): CatBoost-stub детерминированный + добавлен `explain_with_llm` через role:text-reasoning поверх программного объяснения; 12 тестов
- **agent-compliance-assistant** (port 8106, role: rag): RAG-pipeline через rag-service → top-5 snippets → llm-gateway → структурированный ответ с цитатами; 14 тестов
- **rag-service** (port 8105, per ADR-0008): MockEmbedder (384-dim deterministic) + MemoryStore для тестов / Qdrant для prod, hybrid search (dense + Jaccard placeholder для BM25), tenant-isolated через collection-per-tenant `rag_<tenant>`; 14 тестов

### Added — Eval-harness корпуса (165 синтетических кейсов в 6 категориях)
- docs-parsing: 50 кейсов (паспорта 25 + уставы 25, 10% adversarial)
- reconciliation: 30 кейсов (no/minor/major discrepancies)
- ubo-graphs: 20 кейсов (trivial/split/nested/edge cases)
- ru-banking-chat: 30 кейсов (typical/decline-questions/off-topic/jailbreak)
- rag-quality: 20 кейсов (115-ФЗ, 375-П, 499-П, 590-П)
- adversarial: 15 кейсов (prompt-injection в полях, нечитаемые документы, спорные легальные ситуации)

### Added — Infrastructure
- docker-compose.yml: 10 сервисов с healthchecks (`wget /health` или `/healthz`), `depends_on healthy postgres+llm-gateway`, db-migrator-platform как one-shot job под profile `migrate`
- Makefile: `make migrate-platform`, `make smoke`, `make smoke-clean`
- scripts/smoke.sh: end-to-end happy-path из 9 шагов (поднятие → миграции → создание тенанта → audit event → llm-gateway mock chat → agent-conversational → отчёт по usage), color output, idempotent (HTTP 409 = OK при повторных прогонах)
- **Helm charts**: универсальный `charts/aibank-service` (10 templates: Deployment, Service, ConfigMap, ServiceAccount, HPA, NetworkPolicy, ServiceMonitor, PDB, NOTES, helpers) + специфичный `charts/llm-gateway` с PVC + 5 wrapper-чартов под Bitnami (postgres, redis, kafka, minio, temporal); umbrella-чарты `platform-saas` и `platform-onprem` каждый с 18 зависимостями

### Added — Frontend
- **apps/web-onboarding** (Next.js 14 App Router): Apollo Client 3.11 + zod + react-hook-form, JWT в localStorage, 4 страницы (login, applications list, applications/new, applications/[id]), 9 переиспользуемых компонентов (AuthGuard, AppHeader, ApplicationStateTimeline, DocumentList, StateBadge, RiskBadge, ErrorBanner, LoadingSpinner, ApolloProviderClient), все UI-тексты на русском
- **apps/web-admin** (Next.js 14): операторский UI с RBAC (bank.admin/bank.compliance_officer/platform.admin), 5 страниц (login + applications с фильтрами sidebar + applications/[id] с DecisionModal + audit timeline + tenant config viewer), 11 компонентов; Apollo Client к bff-admin GraphQL
- **e2e (Playwright + msw)**: web-onboarding 4 spec файла (login/applications/new-application/application-detail) + web-admin 3 spec файла (login/applications/audit) с моками upstream backend через msw

### Added — Quality / Codegen / Schemas
- **packages/domain-model**: codegen из schema.json (10 канонических типов + value objects) → детерминированный (SHA-1 идентичен между прогонами) Go (378 строк) + TS (374) + Python Pydantic v2 (348); 8 codegen-тестов
- **packages/tenant-config-schema**: 7 JSON Schema (Draft-07) для tenant.yaml/branding/workflow/risk-policy/integrations/ai/sla, CLI `validate.py` с path-based routing, 7 тестов; `_template` валидируется end-to-end
- **packages/proto**: gRPC codegen pipeline через buf (buf.yaml + buf.gen.yaml + Makefile) для 6 .proto-файлов (TenantService, IdentityService, DocumentService, OnboardingService, RiskService, AuditService); generated/ scaffold для Go/TS/Python (реальная генерация в CI через bufbuild/buf-action); 7 codegen-тестов
- **Integration tests с testcontainers-go v0.34.0**: 27 интеграционных кейсов в 4 critical-сервисах (tenant 9 + audit 5 + identity 6 + document 7), build tag `//go:build integration` для изоляции от unit-тестов; реальный Postgres 16 + миграции + поведенческая проверка append-only triggers и schema-isolation

### Added — CI/CD
- **.github/workflows/** (8 workflows): go-services (matrix 31 modules), python-ai (11 projects), frontend (web-onboarding+web-admin), openapi-lint (Spectral), helm-lint (9 charts), codegen-drift (domain-model + buf), eval-harness, release (Docker images → GHCR + syft SBOM)
- **.github/dependabot.yml**: gomod/npm/pip/github-actions weekly; **CODEOWNERS** с 6 командами; **PULL_REQUEST_TEMPLATE.md** с разделами Security/Eval/Migration impact
- **.gitlab-ci.yml** расширен: новые stages `codegen` + `e2e` (codegen:verify, helm:lint, openapi:lint, e2e:smoke против dind-compose)
- Все pinned action versions (без @main/@master)

### Added — Performance benchmarks
- **tools/benchmarks/**: 4 k6-сценария (tenant-service 100 VUs, audit-service 500 VUs, llm-gateway 50 VUs mock, bff-onboarding 100 VUs GraphQL) с thresholds; seed.sh идемпотентный; run-all.sh с color-diff против baselines (>25% p95 regression = fail)
- baselines/ — целевые числа из product-vision.md § 6 (capacity 5-50K заявок/год)
- 6 Makefile targets: bench / bench-seed / bench-tenant / bench-audit / bench-llm / bench-bff

### Added — Operational documentation (15 файлов, 168 КБ)
- **docs/runbooks/** (7): incident-response (top-5 incident playbooks: ПДн leak, pod CrashLoop, Temporal hang, Postgres replication, ext-API down), db-migration (per ADR-0005 SLA), temporal-recovery, llm-gateway-troubleshooting, break-glass-procedure (per security-architecture § 3.3), audit-log-integrity (hash-chain verification, 5-year retention)
- **docs/deployment/** (4): saas-deploy (Yandex/VK Cloud + Helm), on-prem-deploy (blue-green per ADR-0009), airgapped-bundle (signed tarball spec)
- **docs/operations/** (4): backup-restore (RPO 15min/RTO 4h), monitoring-alerts (LGTM stack + SLI/SLO + P0/P1/P2 alert rules), onboarding-engineer (first-day guide)

### Changed
- llm-gateway: переход с comma-string env-конфига на YAML; добавлен `LLM_GATEWAY_FORCE_MOCK` для безопасного запуска без upstream-моделей
- docker-compose: notification-service host port 8083→8110 для разрешения конфликта с document-service

### Removed
- N/A

---

## История версий

Версии будут добавляться по мере релизов:

- `0.1.0` — первый внутренний MVP (ожидается через ~5 месяцев)
- `0.5.0` — готовность к первому пилоту
- `1.0.0` — первый production-релиз с пилотным банком
