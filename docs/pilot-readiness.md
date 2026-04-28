# Pilot Readiness Checklist

Документ описывает, что должно быть готово до старта первого пилота с реальным банком-партнёром. Делится на технические, организационные, регуляторные и операционные блоки. Каждый пункт имеет owner, статус (✅/🟡/❌) и блокирует или не блокирует выход в пилот.

**Дата составления:** 2026-04-27. Состояние кодовой базы: 34 Go-модулей build+test зелёные, 12 shared-пакетов, 7 сервисов с audit-emit, 18/21 сервисов с OpenTelemetry, 10/10 critical с healthchecks.

---

## 1. Технические интеграции (что закрывает гэп между stub и production)

### 1.1 Реальные KYC API
| Что | Статус | Owner | Блокер? | Что делать |
|---|---|---|---|---|
| ФНС / ЕГРЮЛ / ЕГРИП | 🟡 framework готов | Backend + юрист | Да | Договор через Минцифры. УКЭП-сертификат (КриптоПро / VipNet). СМЭВ-3 endpoint. XML→`LegalEntity` mapper. Audit-log + circuit breaker. Включается флагом `EGRUL_LIVE=true`. |
| Росфинмониторинг (115-ФЗ) | 🟡 framework готов | Backend + юрист | Да | Соглашение с ФСФМ. Ежесуточный cron-парсер XML-фида. Локальный индекс (PostgreSQL pg_trgm). Фуззи-матчинг ФИО+дата с порогом 0.85. Флаг `RFM_LIVE=true`. |
| ФССП исполнительные производства | 🟡 framework готов | Backend + юрист | Нет (Soft-check) | Регистрация юрлица как agent-of-record. mTLS-сертификат. Закрытый IS-канал (публичный fssp.gov.ru с CAPTCHA не годится для prod). Rate limit 60 RPS. Флаг `FSSP_LIVE=true`. |
| СПАРК / Контур.Фокус | 🟡 framework готов | Backend + договор | Нет | Выбор провайдера per-tenant. API-ключ в Vault. Per-tenant контракт в `configs/tenants/<bank>/integrations/spark.yaml`. Флаг `SPARK_LIVE=true`. |
| ЕСИА для бизнеса (вход клиента) | ❌ stub | Backend + юрист | Да | Регистрация на ЕСИА как relying party. OIDC-flow. `identity-service` extension для ESIA endpoint. |

**Готовы к флипу:** Provider interface + LiveProvider stub во всех 4 ext-* сервисах. Включение — установка `*_LIVE=true` env. Сейчас все возвращают `ErrNotImplemented` с TODO-комментариями.

### 1.2 ABS-адаптеры
| Что | Статус | Owner | Блокер? | Что делать |
|---|---|---|---|---|
| ЦФТ (abs-adapter-cft) | 🟡 stub-ответы | Backend + интегратор ЦФТ | Да | Реальный SOAP-клиент к АБС банка. Контрактные тесты с mock-сервером ЦФТ. Mapper canonical→ЦФТ модель. ВПД-сертификаты. |
| Diasoft FA# (abs-adapter-diasoft) | ❌ skeleton + stub | Backend + Diasoft partner | Зависит от банка | Если банк-партнёр на Diasoft — реализация. Если на ЦФТ — оставить как backlog. |
| RS-Bank/ЦАБС | ❌ skeleton + stub | Backend | Зависит от банка | Аналогично выше. |

### 1.3 ML / AI инфраструктура
| Что | Статус | Owner | Блокер? | Что делать |
|---|---|---|---|---|
| Real LLM models (Gemma 4, Qwen 3.5, T-pro) | ❌ MockEmbedder/mock backend | ML-инженер + DevOps | Да для production-quality | Развернуть vLLM на GPU (A100/H100 в SaaS, H20/Ascend для on-prem). Загрузить веса через airgapped-bundle. Прогон eval-harness корпусов на каждой модели — выбор production-моделей по ролям per ADR-0011. |
| BGE-M3 embedder | 🟡 BGEM3Embedder client готов | ML-инженер | Да для production-RAG | TEI sidecar на GPU node. Загрузить BAAI/bge-m3 веса. Переключение через `RAG_EMBEDDER=tei` + `RAG_TEI_URL=http://tei:8080`. |
| CatBoost риск-скоринг | 🟡 StubScorer detеrministic | ML-инженер + банковский эксперт | Да | Обучение на синтетике + 100-300 размеченных кейсов от банка. ONNX export. Замена `StubScorer` на `ONNXScorer` (interface уже готов). SHAP-values для объяснимости. |
| Reranker (bge-reranker-v2-m3) | ❌ Jaccard placeholder | ML-инженер | Нет (degradation acceptable) | Подключить bge-reranker как HTTP service после dense top-10. |

### 1.4 Безопасность и шифрование
| Что | Статус | Owner | Блокер? | Что делать |
|---|---|---|---|---|
| КриптоПро CSP / VipNet (УКЭП по ГОСТ) | ❌ stub | Security + банк | Да | Сертифицированная сборка СКЗИ. Лицензии у банка-клиента. Интеграция через стандартный API. УКЭП-подписание документов клиентом. |
| Vault production deployment | 🟡 secrets package + factory | DevOps | Да для prod | Vault HA-cluster с auto-unseal. AppRole login flow вместо static `VAULT_TOKEN`. Vault Agent sidecar или CSI. Ротация master-key 1/год. |
| Vault Transit для PII-полей | ❌ только KV | Security | Нет (деградация acceptable) | Field-level encryption для серии/номера паспорта, СНИЛС, ИНН физлица per security-architecture § 5.2. |
| mTLS между сервисами через Istio | ❌ план только | DevOps + Security | Да для prod | Istio sidecar injection. SPIFFE-identity. Network policies. Defence-in-depth для inter-service trust. |
| Audit log с криптографической подписью | 🟡 SHA-256 hash chain | Security | Нет (acceptable для MVP) | Ed25519 / ГОСТ-2012 signature на каждое event-row. audit-verifier уже умеет проверять. |
| TLS 1.3 + cert-manager | ❌ self-signed только | DevOps | Да для prod | cert-manager с Let's Encrypt (для SaaS) или внутренним CA (для on-prem). Auto-renewal. |
| Pen-test внешним подрядчиком | ❌ не проводился | Security + подрядчик | Да | Quarterly DR drills, ежегодный pen-test (security-architecture § 11). |
| WAF (Cloudflare / собственный) | ❌ не настроен | DevOps | Да для prod | OWASP top-10 правила. Per-tenant rate limiting в API Gateway уже есть. |

### 1.5 Inter-service messaging
| Что | Статус | Owner | Блокер? | Что делать |
|---|---|---|---|---|
| Kafka в production | 🟡 docker-compose only | DevOps | Да | Kafka KRaft cluster (без ZooKeeper). Tiered storage. Encryption at rest (KIP-405). Партиции/репликация per topic. Topic creation policy. |
| Outbox publisher (billing) | ✅ готов | — | — | Включается через `KAFKA_BROKERS` env. Готов к работе сразу. |
| DLQ для poison messages | ❌ TODO в outbox README | Backend | Нет | Relay сейчас loop'ит forever. Добавить retry counter + dead-letter table. |
| Audit event consumer-side dedup | ❌ TODO | Backend | Нет | На стороне audit-service consumer — `ON CONFLICT (id) DO NOTHING`. |

### 1.6 Frontend & end-user UX
| Что | Статус | Owner | Блокер? | Что делать |
|---|---|---|---|---|
| web-onboarding базовые страницы | ✅ login + applications + new + detail | Frontend | — | Готов skeleton, наполнение реальной логикой документ-аплоада, UKEP-подписания. |
| web-admin operator UI | ✅ login + applications + audit + tenant + decision modal | Frontend | — | Готов skeleton. Реальная RBAC через JWT-claims. Decision modal с обязательным комментарием 20+ chars. |
| Mobile app (PWA) | ❌ deferred | — | Нет (per ADR — PWA достаточно в Y1) | Решение в product-vision: не делаем нативные мобильки в первый год. |
| Branding per-tenant | 🟡 schema готов | Frontend + design | Нет | Применение `configs/tenants/<bank>/branding/theme.json` к UI. Logo/colors/fonts override. |
| Document upload UX | 🟡 placeholder button | Frontend | Да | Drag-and-drop, preview PDF, progress bar, retry на ошибки сети. |
| ESIA login кнопка в web-onboarding | ❌ не реализован | Frontend + Backend | Да | После регистрации на ЕСИА — добавить OIDC-flow на login странице. |

---

## 2. Организационные

### 2.1 Дизайн-партнёр
| Что | Статус | Owner | Срок |
|---|---|---|---|
| Демо-материалы (видео + слайды) | ❌ нужно записать | Founder + продакт | 2 недели |
| Аудит-пакет для банковского ИБ-аудита | 🟡 security-architecture готов | Security | 1 неделя на сборку bundle |
| Интервью с CTO/CPO 5-10 банков ЦА | ❌ не проводились | Founder | 2-3 месяца |
| 2-3 LOI на пилот | ❌ | Founder | 3-4 месяца |
| DPA template | ❌ | Юрист | 1 месяц |
| SLA template | 🟡 sla.yaml в demo-bank как образец | Юрист + продакт | 1 месяц |
| Contract template (setup fee + per-event + maintenance) | ❌ | Юрист | 1-2 месяца |
| Insurance (cyber liability, D&O) | ❌ | CFO | 1 месяц |

### 2.2 Команда
По состоянию проекта `docs/technical-structure.md` § 12:
- Целевой размер к концу Y1: **18-25 человек**
- Текущий состав: основная разработка + AI-агенты ✓
- **Обязательно для пилота**:
  - Банковский эксперт-комплаенс с реальным опытом (риск средний — рынок узкий)
  - Security-инженер (для break-glass + incident response)
  - DPO (внутренний или внешний на договоре)
  - DevOps с опытом on-prem развёртываний
  - 24/7 on-call rotation (минимум 3 человека)

### 2.3 Документация для банка
| Что | Статус | Owner |
|---|---|---|
| Технический Whitepaper | ❌ | Архитектор |
| Onboarding manual для операторов банка | ❌ | Продакт + UX writer |
| Compliance-карта (`docs/compliance-map.md`) | ✅ готова | — |
| Security architecture (`docs/security-architecture.md`) | ✅ готова | — |
| Operational runbooks (`docs/runbooks/`) | ✅ готовы | — |

---

## 3. Регуляторные

### 3.1 Сертификации (приоритет)
| Сертификат | Когда нужен | Статус | Срок до получения |
|---|---|---|---|
| 152-ФЗ УЗ-2 (аттестат) | До первого пилота | ❌ | 6-9 месяцев |
| Реестр Минцифры (для участия в гос-контрактах) | Для тендеров | ❌ | 3-6 месяцев |
| ФСТЭК Приказ 17 (КИИ) | Для банков-субъектов КИИ (топ-200) | ❌ | 9-12 месяцев |
| ISO 27001 | Год 2 | ❌ | 12-18 месяцев |
| СКЗИ-сертификация (КриптоПро/VipNet) | Используем готовые от вендоров | — | — |

### 3.2 Регуляторные требования (закладываются в архитектуру)
| Требование | Статус | Где описано |
|---|---|---|
| 115-ФЗ KYC + 5-летнее хранение audit | ✅ append-only с hash-chain + audit-verifier | docs/security-architecture § 8 |
| 152-ФЗ ПДн + локализация в РФ | ✅ deployment в Yandex/VK Cloud + on-prem | docs/security-architecture + technical-structure § 10 |
| 152-ФЗ согласия на обработку | ❌ UI и flow | Frontend TODO |
| 152-ФЗ право на удаление | 🟡 archive_at field, не реальный delete | TODO Backend |
| 375-П (риск-категории + периодический пересмотр) | 🟡 categories есть в risk-engine, periodic review TODO | Phase 2 |
| 499-П (упрощённая идентификация) | 🟡 определяется в risk-policy.yaml per tenant | Phase 2 |
| 187-ФЗ КИИ (если банк-субъект) | ❌ | После первого банка-партнёра |
| Уведомление РКН в 24h при инциденте 152-ФЗ | ✅ runbook | docs/runbooks/incident-response.md |

---

## 4. Операционные

### 4.1 Инфраструктура (что нужно поднять до пилота)
| Что | Статус | Owner | Блокер? |
|---|---|---|---|
| K8s 1.30+ кластер (SaaS — Yandex Cloud / VK Cloud) | ❌ | DevOps | Да |
| Managed PostgreSQL 16 (per-tenant DBs или один cluster со schema-per-tenant) | ❌ | DevOps | Да |
| Managed Kafka KRaft | ❌ | DevOps | Да |
| Redis Cluster | ❌ | DevOps | Да |
| MinIO для object storage | ❌ | DevOps | Да |
| Vault HA-cluster | ❌ | DevOps + Security | Да |
| Keycloak SSO | ❌ | DevOps | Да |
| Temporal cluster (self-hosted, PostgreSQL backend) | 🟡 docker-compose | DevOps | Да |
| Qdrant для RAG | 🟡 docker-compose | DevOps | Да |
| GPU pool (vLLM inference) | ❌ | DevOps + ML | Да |
| Helm-чарты | ✅ 7 базовых + 2 umbrella | — | — |
| Terraform для Yandex/VK Cloud | 🟡 структура есть | DevOps | Да |
| Ansible для on-prem | 🟡 структура есть | DevOps | Зависит от банка |

### 4.2 LGTM Observability stack
| Что | Статус | Owner | Блокер? |
|---|---|---|---|
| VictoriaMetrics | ❌ | DevOps | Да для prod |
| Loki | ❌ | DevOps | Да для prod |
| Tempo | ❌ | DevOps | Да для prod |
| Grafana + дашборды | 🟡 packages/observability готов, дашборды нет | DevOps | Да |
| Alertmanager + Telegram/Mattermost | ❌ | DevOps | Да |
| OpenTelemetry SDK в сервисах | ✅ 18/21 wired | — | — |
| Audit-log агрегация (5 лет retention) | 🟡 append-only DB готов, archive policy TODO | DBA | Phase 2 |

### 4.3 Backup & DR
| Что | Статус | Owner | Блокер? |
|---|---|---|---|
| Postgres ежедневные backup + WAL | ❌ | DevOps | Да |
| MinIO bucket replication | ❌ | DevOps | Да |
| Vault encrypted backup в офлайн | ❌ | Security | Да |
| RPO 15 минут / RTO 4 часа | ❌ не verified | DevOps + Security | Да |
| Quarterly DR drill | ❌ | DevOps + Security | Год 1 |
| Cold-site в другом регионе | ❌ | DevOps | Phase 2 |

### 4.4 CI/CD
| Что | Статус | Owner |
|---|---|---|
| GitHub Actions workflows | ✅ 8 workflows готовы | — |
| GitLab CI | ✅ extended | — |
| Container registry (GHCR / private) | ❌ | DevOps |
| ArgoCD (GitOps) | ❌ | DevOps |
| Image signing (cosign) | ❌ | Security |
| SBOM scan (syft + grype) | ✅ syft в release workflow | — |
| Dependabot | ✅ | — |
| Pre-prod environment (staging) | ❌ | DevOps |

---

## 5. Бизнес и финансы

| Что | Статус | Owner | Срок |
|---|---|---|---|
| Pricing model verified (юнит-экономика) | ✅ описана в product-vision § 5 | Финансист | — |
| Биллинговые ставки в demo-bank yaml | ✅ pilot-pricing включены | — | — |
| Расчёт первого Setup Fee | ❌ | Founder + банк | На договоре |
| Stripe / банковский счёт для платежей | ❌ | CFO | 1 месяц |
| Первый счёт-фактура | ❌ | Бухгалтер | После первого пилота |

---

## 6. Roadmap до первого пилота

### Месяц 1-2: Тех-долг + Дизайн-партнёр
- Демо-материалы готовы
- Аудит-пакет для банка собран
- Интервью с 5 банками
- Backlog: KYC integrations + ML обучение начаты

### Месяц 3-4: Контракт + Pen-test
- LOI с дизайн-партнёром
- Pen-test внешним подрядчиком
- УЗ-2 аттестация запущена
- Real KYC API подключены за feature flags

### Месяц 5-6: Staging + Soak
- Staging-среда у банка-партнёра
- Shadow mode 2 недели (1-5% production traffic, без принятия решений)
- Прогон eval-harness на реальных данных банка
- Финализация SLA + DPA

### Месяц 7-9: Постепенный rollout
- 10% real traffic → 50% → 100%
- 24/7 on-call
- Quarterly DR drill #1
- УЗ-2 аттестат получен

### Месяц 10-12: Стабилизация
- Pen-test post-launch
- ISO 27001 запущена
- Второй пилотный банк

---

## 7. Самое срочное (top-10 next actions)

1. **Финансирование пилота** — оценить cash runway на 9-12 месяцев до первой выручки
2. **Найм банковского эксперта-комплаенс** — без него корпус eval-harness деградирует, риск-policy некорректен
3. **Демо-видео + слайды** — нужно для интервью с банками
4. **3-5 интервью с CTO/CPO банков 30-150 место по активам** — валидация product-market fit
5. **Договор с подрядчиком на pen-test** — 6-8 недель lead time
6. **Запуск УЗ-2 аттестации** — 6-9 месяцев lead time
7. **Договор с ФНС / Минцифры на ЕГРЮЛ API** — 2-3 месяца bureaucracy
8. **Обучение CatBoost** на синтетических кейсах — параллельно с поиском банка-партнёра
9. **Helm-чарт + Terraform deploy** в Yandex Cloud — для staging-среды
10. **Vault production deployment** — приоритетный security gap

---

**Owner документа:** CEO/Founder + продакт-менеджер. Обновляется по мере прогресса.

**Последнее обновление:** 2026-04-27.
