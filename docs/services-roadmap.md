<!-- Parent: AGENTS.md -->
<!-- Generated: 2026-05-08 -->
<!-- Status: Draft v1 — service inventory + 3-milestone roadmap -->
<!-- Owner: Архитектор + продакт. Пересматривается ежеквартально. -->

# План по продуктам и сервисам AIbank

Карта всех сервисов платформы: что **уже есть**, что **доводится до
пилота** (M1), что добавляется на **production** (M2), что в **scale**
(M3, post-pilot, year 2).

Связано с:
- [docs/cost-estimate.md](cost-estimate.md) — финансовая сторона roadmap
- [docs/pilot-readiness.md](pilot-readiness.md) — детальные блокеры
- [docs/system-overview.md](system-overview.md) — архитектурный обзор
- [docs/onboarding-form-spec.md](onboarding-form-spec.md) — функциональные требования формы

Условные обозначения:
- ✅ — готов и работает в production-quality
- 🟡 — есть skeleton/stub, требует доработки
- ❌ — не начат
- 🆕 — новый сервис (ещё не существует в репо)

---

## Текущий inventory (на 2026-05-08)

### Core domain services (services/)

| Сервис | Состояние | Что делает |
|--------|-----------|------------|
| **tenant-service** | ✅ | Хранит банков-тенантов и их config'и (тарифы, риск-пороги) |
| **identity-service** | ✅ | Login (email+password), JWT, регистрация applicant'а, согласия 152-ФЗ |
| **onboarding-orchestrator** | 🟡 | Заявки + state machine + Temporal workflow. Activities — stubs |
| **document-service** | 🟡 | Upload/storage (MinIO), типы из ТЗ — частично |
| **audit-service** | ✅ | Append-only audit log с SHA-256 hash chain |
| **billing-service** | 🟡 | Outbox publisher готов, тарификация per-tenant TODO |
| **risk-engine** | 🟡 | Правила-движок (CatBoost — stub) |
| **client-service** | 🟡 | CRUD клиентов после открытия счёта |
| **ubo-service** | 🟡 | Граф владения; FATCA/CRS — TODO |
| **notification-service** | 🟡 | Email/SMS/Telegram dispatch — skeleton |

### API Layer

| Сервис | Состояние | Что делает |
|--------|-----------|------------|
| **api-gateway** | ✅ | JWT-валидация, rate-limit per tenant, CORS, healthcheck |
| **bff-onboarding** | ✅ | GraphQL для applicant-фронта |
| **bff-admin** | ✅ | GraphQL для bank-операторов |

### External integrations (services/ext-*)

| Сервис | Состояние | Что делает |
|--------|-----------|------------|
| **ext-egrul** | 🟡 stub provider | ФНС ЕГРЮЛ (live = договор через Минцифры) |
| **ext-rosfinmon** | 🟡 stub provider | 115-ФЗ перечни (live = соглашение с ФСФМ) |
| **ext-fssp** | 🟡 stub provider | Исполнительные производства |
| **ext-spark** | 🟡 stub provider | СПАРК / Контур.Фокус (per-tenant) |

### ABS layer

| Сервис | Состояние | Что делает |
|--------|-----------|------------|
| **abs-connector** | 🟡 | Маршрутизация в нужный adapter по типу АБС банка |
| **abs-adapter-cft** | 🟡 stub | ЦФТ SOAP-клиент |
| **abs-adapter-diasoft** | ❌ skeleton | Diasoft FA# |
| **abs-adapter-rs-bank** | ❌ skeleton | RS-Bank/ЦАБС |

### AI / ML layer (ai/)

| Компонент | Состояние | Что делает |
|-----------|-----------|------------|
| **llm-gateway** | 🟡 mock backend | Прокси к vLLM (live = развернуть Gemma 4 / Qwen) |
| **rag-service** | 🟡 stub | RAG для compliance-вопросов (115-ФЗ / 375-П / 639-П) |
| **eval-harness** | 🟡 framework | Регрессионные тесты AI-агентов |
| **agent-document-intake** | 🟡 mock | OCR + LLM extract из паспорта/устава |
| **agent-reconciliation** | 🟡 mock | Сверка ЕГРЮЛ vs анкета клиента |
| **agent-ubo-tracing** | 🟡 mock | Распутывание цепочки владения |
| **agent-risk-scoring** | 🟡 stub | Объяснение риск-скора (SHAP-values) |
| **agent-conversational** | 🟡 mock | Чат-бот для applicant'а |
| **agent-compliance-assistant** | 🟡 mock | Помощник для bank-комплаенс-офицера |

### Frontend (apps/)

| Приложение | Состояние |
|------------|-----------|
| **web-onboarding** (port 3000) | ✅ login, register, /applications wizard минимум, /applications/{id} details |
| **web-admin** (port 3001) | ✅ login, applications, audit, tenant, decision modal |

### Tools (tools/)

| Tool | Состояние |
|------|-----------|
| **db-migrator** | ✅ goose-style миграции для platform + tenant |
| **audit-verifier** | ✅ верификация SHA-256 chain |
| **tenant-cli** | ✅ создание тенанта + applying config'а |
| **data-generator** | ✅ синтетика для тестов |
| **mock-abs** | ✅ mock для contract-тестов |
| **mock-smev** | ✅ mock СМЭВ для интеграционных тестов |
| **benchmarks** | 🟡 framework |
| **outbox-cleanup** | ✅ cron-cleanup poison messages |

---

## M1 — До первого пилота (Standard сценарий, 7 месяцев)

Цель: одна банк-тенант в проде с реальным потоком заявок, прохождением KYC, открытием счёта.

### M1.1 — Достроить core (этапы 5-10 формы онбординга)

| Сервис | Что добавить |
|--------|--------------|
| onboarding-orchestrator | UBO endpoint, ScreeningResultSet, MonitoringProfile, Account opening flow |
| onboarding-orchestrator | Реальные Temporal activities (5 шт): Verify, Extract, Reconcile, Assess, OpenAccount |
| document-service | Типизация документов (charter, EGRUL extract, license, ...). Drag-and-drop UX. |
| risk-engine | CatBoost ONNX вместо StubScorer + SHAP explainer |
| ubo-service | FATCA/CRS-декларации, ownership_chain поля |

### M1.2 — Включить реальные KYC/AML

| Сервис | Live-mode что нужно |
|--------|---------------------|
| ext-egrul | Договор Минцифры + СМЭВ-3 endpoint + флаг `EGRUL_LIVE=true` |
| ext-rosfinmon | Соглашение ФСФМ + cron-парсер XML фида + pg_trgm индекс |
| ext-fssp | mTLS-сертификат + agent-of-record регистрация |
| 🆕 **ext-zsk-tsfb** | Новый сервис для ЗСК ЦБ (светофор) — нет в репо, добавить |
| 🆕 **ext-esia-business** | OIDC relying party для входа клиента через Госуслуги |

### M1.3 — ABS-интеграция для первого банка

| Сервис | Что |
|--------|-----|
| abs-adapter-cft | Реальный SOAP-клиент к ЦФТ. Контракт-тесты на mock-abs. |
| abs-connector | Per-tenant маршрутизация (если первый банк на ЦФТ) |

(Diasoft / RS-Bank — отложены до второго банка-партнёра)

### M1.4 — Production hardening (DevOps + Security)

| Что | Где |
|-----|-----|
| Vault HA cluster + AppRole flow | infra/helm/charts/vault |
| mTLS Istio sidecar injection | новый Helm chart |
| Kafka KRaft cluster prod | infra/helm/charts/kafka |
| TLS 1.3 + cert-manager | new chart |
| WAF (Cloudflare для SaaS) | DevOps config |
| Pen-test + аудит-bundle | внешний контракт |

### M1.5 — Real LLM stack

| Что | Стек |
|-----|------|
| vLLM на GPU | A100 80GB на vast.ai/Selectel/Yandex |
| Gemma 4 + Qwen 3.5 + T-pro модели | airgapped-bundle для on-prem |
| BGE-M3 embedder | TEI sidecar |
| Reranker bge-reranker-v2-m3 | замена Jaccard placeholder |

### M1.6 — Frontend wizard этапов 5-10

| Страница | Что |
|----------|-----|
| `/applications/{id}/profile` | Этап 2 — анкета юрлица (ОПФ/ОКВЭД/адреса/...) |
| `/applications/{id}/activity` | Этап 3 — AML-сведения (контрагенты/обороты/...) |
| `/applications/{id}/representatives` | Этап 4 — ЕИО + представители |
| `/applications/{id}/ubo` | Этап 5 — УБО + FATCA |
| `/applications/{id}/documents` | Этап 6 — drag-drop загрузка |
| `/applications/{id}/account-opening` | Этап 9 — выбор продукта + тариф + согласия |
| Web-admin: кнопки `transitionApplicationState` | На странице деталей заявки |

### M1.7 — Mobile / branding

| Что | Состояние |
|-----|-----------|
| PWA support (web-onboarding) | TODO в Y1 (mobile нативные нет) |
| Per-tenant branding | applying `configs/tenants/<bank>/branding/theme.json` к UI |

---

## M2 — Production stability (после первого пилота, +3-6 месяцев)

Цель: SLA 99.5%, 5K заявок/мес/банк, 3-5 банков на стенде.

### M2.1 — Observability

| Сервис | Что |
|--------|-----|
| 🆕 **VictoriaMetrics + vmagent** | Метрики prom-style |
| 🆕 **Loki + promtail** | Логи |
| 🆕 **Tempo** | Distributed tracing (OTel SDK уже подключён в 18/21 сервисов) |
| 🆕 **Grafana дашборды** | Per-сервис + per-tenant overview |
| 🆕 **Alertmanager + Telegram/Mattermost** | SLO-based alerts |

### M2.2 — Безопасность глубже

| Что |
|-----|
| Vault Transit для PII полей (паспорт серия/номер, СНИЛС, ИНН физлица) |
| Audit log с Ed25519 / ГОСТ-2012 подписью каждой записи |
| 🆕 **e2ee-document-vault** или расширение document-service: end-to-end encryption файлов клиента |
| Quarterly DR drills (runbook + recovery test) |

### M2.3 — Backup & DR

| Что |
|-----|
| Postgres WAL archive в offsite S3-compatible bucket |
| MinIO replication между регионами (для multi-region SaaS) |
| Восстановление RTO ≤ 4ч / RPO ≤ 15 мин |
| Регулярные DR-drill'ы |

### M2.4 — Multi-tenant DLQ + observability

| Что |
|-----|
| outbox publisher: retry counter + dead-letter table + DLQ topic |
| audit-service: consumer-side dedup (idempotent ON CONFLICT) |
| Tenant-aware bottleneck dashboards (Grafana per tenant) |

### M2.5 — Регуляторика year 2

| Что |
|-----|
| 152-ФЗ право на удаление (real delete вместо archive_at field) |
| 375-П periodic review каждые 3/6/12 мес по риск-категории |
| 187-ФЗ КИИ если банк-субъект |
| ISO 27001 (cert процесс) |

---

## M3 — Scale (post-2-pilot, year 2)

Цель: 10+ банков, 50K+ заявок/мес, multi-region failover, on-prem банки с air-gapped средами.

### M3.1 — On-prem поддержка

| Что |
|-----|
| `tools/airgapped-bundler` — генератор offline-bundle с моделями + образами |
| Поддержка Ascend GPU (Huawei) для on-prem банков |
| Поддержка ЦСРС-каналов (закрытые СВТ-сети) |

### M3.2 — Дополнительные ABS-адаптеры

| Сервис | Когда |
|--------|-------|
| abs-adapter-diasoft | при втором банке на Diasoft |
| abs-adapter-rs-bank | при банке на RS-Bank |
| 🆕 **abs-adapter-temenos** | если европейский банк-партнёр |

### M3.3 — Расширенные AI capabilities

| Сервис | Что |
|--------|-----|
| 🆕 **agent-fraud-detection** | Anti-fraud при подаче (device fingerprint + IP + behavior) |
| 🆕 **agent-explanation** | Explainable AI для отказов (для регулятора) |
| 🆕 **agent-translator** | Multi-language onboarding (для иностранных компаний) |
| eval-harness | Continuous benchmarking + drift detection |

### M3.4 — Расширенные KYC-источники

| 🆕 Сервис | Зачем |
|-----------|-------|
| **ext-ofac-sanctions** | OFAC SDN / EU CFSP / UK HMT санкции — для международных компаний |
| **ext-fatca-irs** | FATCA/CRS reporting в IRS |
| **ext-pep-databases** | PEP databases (WorldCheck, Dow Jones) |

### M3.5 — Mobile app (если нужно)

| Что | Когда |
|-----|-------|
| iOS/Android native | Если PWA usage показывает реальный спрос |

### M3.6 — Marketplace

| Что | Зачем |
|-----|-------|
| Plugin SDK для third-party интеграций | Партнёры могут добавлять свои KYC-источники |
| Whitelisted plugin store | Сертифицированные расширения |

---

## Сводная таблица: что появится новым в каждой Milestone

| Milestone | Новые сервисы (🆕) | Существующие → production-ready |
|-----------|-------------------|--------------------------------|
| **M1** (pilot) | ext-zsk-tsfb, ext-esia-business | onboarding-orchestrator, document-service, risk-engine, ubo-service, abs-adapter-cft, llm-gateway, rag-service, frontend wizard |
| **M2** (production) | victoriametrics, loki, tempo, e2ee-vault | observability, vault transit, DR/backup, audit signing |
| **M3** (scale) | abs-adapter-{diasoft,rs-bank,temenos}, ext-{ofac,fatca,pep}, agent-{fraud,explanation,translator}, mobile app, plugin marketplace | airgapped-bundler, multi-region failover |

---

## Риски и зависимости

| Риск | Влияние | Митигация |
|------|---------|-----------|
| УЗ-2 аттестат не выдан в срок (6-9 мес) | Не выходим в пилот | Стартовать аттестацию в M0, не ждать LOI |
| Банк-партнёр на Diasoft/RS-Bank | M1 расширяется на ABS-adapter | Договариваться с ЦФТ-банком первым |
| GPU цены вырастают (vast.ai → managed) | OpEx ×3 | Фолбэк на on-prem H100 (CapEx ₽4M) |
| Регуляторика 187-ФЗ КИИ | M2 расширяется на 9-12 мес | Переключиться на банки вне топ-200 КИИ |

---

## Связанные документы

- [docs/system-overview.md](system-overview.md) — архитектурный обзор
- [docs/technical-structure.md](technical-structure.md) — структура репо
- [docs/adr/](adr/) — архитектурные решения
- [docs/pilot-readiness.md](pilot-readiness.md) — детальные блокеры
- [docs/cost-estimate.md](cost-estimate.md) — финансы
- [docs/onboarding-form-spec.md](onboarding-form-spec.md) — UX-требования
