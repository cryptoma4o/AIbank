<!-- Parent: ./AGENTS.md -->
<!-- Generated: 2026-04-28 -->

# Audit-bundle для банковского ИБ-аудита

Сборка артефактов для презентации платформы AIbank банковской ИБ-команде во время технического due diligence перед пилотом. Собрано из существующей документации — единой точкой для отправки.

**Аудитория**: CISO банка, ИБ-команда, compliance officer, DPO. **Срок презентации**: на одной встрече 60-90 мин + материалы для глубокого изучения.

---

## 1. Один-страничник (executive summary)

**Что**: White-label платформа цифрового онбординга юрлиц для банков (KYC/KYB, граф УБО, риск-скоринг, открытие счёта в АБС).

**Архитектура**:
- 21 Go-микросервис (chi, gRPC, Temporal) + 6 AI-агентов (Python, FastAPI, vLLM)
- 2 Next.js фронтенда (web-onboarding, web-admin)
- Multi-tenant: schema-per-tenant в PostgreSQL, isolated S3-buckets, isolated Kafka topics
- Hybrid deployment: SaaS + on-prem (k8s + Helm + Vault + Keycloak + Istio mTLS)
- Observability: OpenTelemetry → VictoriaMetrics + Grafana + Loki + Tempo

**Стек безопасности**:
- УКЭП по ГОСТ через КриптоПро / VipNet (планируется к пилоту)
- Audit log с tamper-evident SHA-256 hash chain (append-only triggers)
- Vault для секретов
- mTLS между сервисами через Istio (планируется к пилоту)
- Threat model по STRIDE
- Соответствие 115-ФЗ, 375-П, 499-П, 152-ФЗ, 187-ФЗ, 63-ФЗ

**Текущий статус**: Pre-MVP, 34 Go-модуля + 6 AI-агентов с зелёными тестами; pilot-ready через 3-4 месяца после закрытия блокеров (см. § 5).

---

## 2. Документы для глубокого изучения

| Документ | Что в нём | Размер |
|----------|-----------|--------|
| [docs/security-architecture.md](./security-architecture.md) | Threat model (STRIDE), IAM, network segmentation, шифрование, криптография (ГОСТ), audit log, incident management, DR | большой |
| [docs/compliance-map.md](./compliance-map.md) | Маппинг 115-ФЗ, 375-П, 499-П, 152-ФЗ, 187-ФЗ, 63-ФЗ на возможности платформы; сертификации и сроки | большой |
| [docs/domain-model.md](./domain-model.md) | Канонические сущности: Tenant, Application, Person, LegalEntity, UBOGraph, Document, RiskAssessment, Decision, Account, AuditEvent — какие данные о клиенте платформа хранит | большой |
| [docs/tenant-configuration.md](./tenant-configuration.md) | Структура per-tenant конфига (изоляция данных банков на уровне config) | средний |
| [docs/ai-agents-automation.md](./ai-agents-automation.md) | Каталог AI-агентов с принципами объяснимости и аудит-трейсом | средний |
| [docs/pilot-readiness.md](./pilot-readiness.md) | Полный чеклист до старта пилота — что готово (🟢), что в работе (🟡), что блокер (🔴) | большой |

---

## 3. Архитектурные решения (ADRs)

Принципиальные решения зафиксированы как ADR. Все в [docs/adr/](./adr/):

| ADR | Решение | Релевантность для ИБ |
|-----|---------|----------------------|
| 0001 | Temporal для оркестрации | DR — workflows воссстановимы |
| 0002 | Schema-per-tenant в PostgreSQL | **Изоляция данных банков** |
| 0005 | Migrations strategy | drift detection через checksum |
| 0006 | ABS-adapter versioning | semver compat для интеграций |
| 0007 | Prompt management | **prompt_version в audit log** для регуляторной трассируемости |
| 0008 | Vector DB (Qdrant + pgvector) | RAG-данные изолированы |
| 0009 | On-prem update strategy | blue-green через Istio + air-gapped tarball для on-prem банков |
| 0010 | Billing & audit log | **Append-only с outbox pattern, idempotency через UUID v5, hash chain** |
| 0011 | LLM routing strategy | формальная процедура promotion модели через eval-метрики |
| 0012 | Eval-corpus governance | drift detection через KL-divergence |

---

## 4. Operational artefacts

| Артефакт | Где | Назначение |
|----------|-----|------------|
| [docs/runbooks/](./runbooks/) | Папка с runbook'ами | Пошаговые сценарии для on-call: incident-response, db-migration, audit-log-integrity, break-glass-procedure, temporal-recovery, llm-gateway-troubleshooting, outbox-dlq-recovery |
| [docs/operations/](./operations/) | Операционные процедуры | monitoring-alerts, backup-restore, onboarding-engineer |
| [docs/deployment/](./deployment/) | Deployment guides | on-prem-deploy, saas-deploy, airgapped-bundle (для on-prem банков) |
| `tools/audit-verifier/` | CLI | Валидация целостности audit-log hash chain (SHA-256), поиск пропусков |
| `tools/benchmarks/` | k6 + baselines | Performance benchmarks с baseline-сравнением для tenant-service, audit-service, llm-gateway, bff |

---

## 5. Pilot-readiness gap (что ещё не готово к пилоту)

Полный список — [docs/pilot-readiness.md](./pilot-readiness.md). Критичные (🔴) для ИБ:

| Блок | Статус | План |
|------|--------|------|
| КриптоПро / VipNet (УКЭП по ГОСТ) | 🔴 stub | Сертифицированная сборка СКЗИ + лицензии у банка-клиента; интеграция через стандартный API; УКЭП-подписание документов клиентом |
| Vault production deployment | 🟡 secrets package + factory | Vault HA-cluster с auto-unseal, AppRole login flow, Vault Agent sidecar или CSI, ротация master-key 1/год |
| mTLS между сервисами через Istio | 🔴 план только | Istio sidecar injection, SPIFFE-identity, NetworkPolicies (defence-in-depth для inter-service trust) |
| Audit log с криптографической подписью | 🟡 SHA-256 hash chain | Ed25519 / ГОСТ-2012 signature на каждое event-row; audit-verifier уже умеет проверять hash chain, добавим signature verification |
| TLS 1.3 + cert-manager | 🔴 self-signed only | cert-manager с Let's Encrypt (для SaaS) или внутренним CA (для on-prem); Auto-renewal |
| Pen-test внешним подрядчиком | 🔴 не проводился | Quarterly DR drills, ежегодный pen-test (см. security-architecture § 11) |
| WAF | 🔴 не настроен | OWASP top-10 правила; per-tenant rate limiting в API Gateway уже есть (см. services/api-gateway) |

---

## 6. Что ИБ-команда банка может проверить **уже сейчас**

Локальный запуск:
```bash
git clone <repo>
make up                  # docker-compose: postgres, kafka, redis, qdrant, minio, temporal
make smoke               # e2e happy-path
make test                # ~600 unit + integration тестов
```

ИБ-фокус-чек-листы:
- ✅ Audit log целостность: `tools/audit-verifier/cmd/verifier --check-chain`
- ✅ Tenant-изоляция: integration-тесты в `services/*/test/integration/`
- ✅ Secrets: убедиться, что `.env` файлы в `.gitignore`, секреты только в Vault (см. `packages/secrets/`)
- ✅ JWT-auth: `services/identity-service/internal/auth/jwt_test.go`
- ✅ Rate-limit: `services/api-gateway/internal/ratelimit/`
- ✅ RBAC в admin: `services/bff-admin/internal/auth/`
- ✅ MIME whitelist при upload: `services/document-service/`
- ✅ ABS-адаптеры с canonical interface: `services/abs-adapter-cft/`, `-diasoft/`, `-rs-bank/`

---

## 7. Контакты

- **Технический владелец платформы**: Главный архитектор (см. `AGENTS.md` → Зоны ответственности)
- **Security**: Security-инженер
- **Compliance**: Юрист + комплаенс-эксперт

---

## Как использовать этот bundle

1. Отправить ИБ-команде банка эту страницу как stub + ссылку на репозиторий
2. На встрече: показать executive summary § 1, ответить на вопросы по threat model и audit log
3. Для глубокого due diligence: дать read-доступ к репозиторию (приватный) + календарь встреч с архитектором
4. Pilot-readiness gap (§ 5) — открыто и честно: «вот что не готово, вот план закрытия»
