<!-- Parent: ./AGENTS.md -->

# Demo-script для банка-партнёра (15-20 минут)

Пошаговый walkthrough встречи с представителями банка (CTO / CISO /
Compliance / Product). Привязан к `demo/run-demo.sh` и
[demo-slide-deck-outline.md](./demo-slide-deck-outline.md).

**Аудитория**: технические + комплаенс лица в банке.
**Цель встречи**: получить LOI / договорённость о пилоте.
**Pre-prep**: за 30 минут до встречи запустить `make up && make migrate-platform && make smoke` (проверить что 21 сервис healthy).

---

## Структура (20 минут)

| Время | Блок | Что делаем |
|-------|------|------------|
| 0:00-2:00 | Intro + контекст | Slide 1-2 |
| 2:00-7:00 | Live demo: LLC happy-path | Запуск `./demo/scenarios/01-llc-happy-path.sh` |
| 7:00-10:00 | Live demo: IP manual review | Запуск `./demo/scenarios/02-ip-manual-review.sh` + web-admin |
| 10:00-13:00 | Live demo: AO rejected | Запуск `./demo/scenarios/03-ao-rejected.sh` |
| 13:00-15:00 | Security walkthrough | Audit chain integrity, tampering detection |
| 15:00-17:00 | Pilot-readiness gap | Что НЕ готово (открыто и честно) |
| 17:00-20:00 | Q&A + следующие шаги | LOI / DPA / SLA template'ы |

---

## Скрипт по блокам

### 0:00 — Intro

> «Спасибо, что нашли время. AIbank — это white-label платформа цифрового
> онбординга юрлиц для российских банков 30-200 места. Наша гипотеза:
> 70% работы по KYC/KYB можно автоматизировать через AI-агентов и
> канонические интеграции, а оставшиеся 30% — оставить compliance-офицеру
> в operator-UI с полной audit-trail.
>
> Сегодня покажем live: 3 сценария — happy-path / manual-review /
> reject. После — вопросы.»

**Покажите slide 1** (one-pager: архитектура, цифры, текущий статус
Pre-MVP / pilot 3-4 нед).

---

### 2:00 — Сценарий 01: LLC happy path

```bash
./demo/scenarios/01-llc-happy-path.sh
```

**Talking points** во время выполнения:

- **State machine**: «Заявка проходит 15 состояний — от submitted до
  account_opened. Все переходы записаны в Temporal и audit-log,
  ни один шаг нельзя пропустить или ретроактивно изменить.»
- **AI-агенты**: «Document intake извлекает данные из PDF —
  паспорт, устав, ОГРН. UBO tracing строит граф владения, проверяет
  доли ≥25%. Risk scoring выдаёт explainable score с SHAP-style
  факторами. Conversational agent подключается если у клиента
  возникли вопросы.»
- **AUTO-APPROVE**: «Risk-score 0.15 (low) → автоматическое одобрение
  без оператора. Срок: 90 секунд от submit до открытого счёта в АБС.»

> «Обратите внимание: в audit-log есть событие `ai.risk_scored` с
> полным набором факторов, что ML-инженер может в любой момент
> воспроизвести и объяснить решение.»

---

### 7:00 — Сценарий 02: IP manual review

```bash
./demo/scenarios/02-ip-manual-review.sh
```

**Talking points**:

- «У этого ИП нет ОГРНИП-выписки — risk-score 0.55 (medium).
  Платформа не одобряет автоматически, эскалирует на оператора.»
- **Откройте web-admin** (`http://localhost:3001`) → applications →
  выберите свежесозданную IP-заявку → decision modal.
- «Здесь оператор-комплаенс видит все собранные данные + reasoning
  AI-агентов + risk factors. Принимает решение с обязательным
  комментарием 20+ символов — это идёт в audit-log.»
- Operator approve → сценарий продолжает: account_opened.

> «Этот flow покрывает 20-30% заявок где AI неуверенный или документы
> неполные. Все решения, включая отказы, записываются с justification.»

---

### 10:00 — Сценарий 03: AO rejected (115-ФЗ + foreign offshore)

```bash
./demo/scenarios/03-ao-rejected.sh
```

**Talking points**:

- «АО с попаданием в синтетический 115-ФЗ список + офшорный
  совладелец без disclosure. Risk-score 0.95 (high) → автоматический
  отказ.»
- **Покажите 2 слоя сообщений**:
  - **Клиенту**: generic "Заявка не одобрена. Свяжитесь с банком." —
    БЕЗ упоминания 115-ФЗ или санкций (требование 115-ФЗ § 7).
  - **Compliance-офицеру** в web-admin: полная причина, ext-rosfinmon
    match, fuzzy-score, foreign-chain detection.
- «Это то, как 115-ФЗ требует обращаться с клиентом: отказ без
  раскрытия деталей, но полный audit-trail для регулятора.»

---

### 13:00 — Security walkthrough (для CISO)

```bash
# Audit chain integrity
./tools/audit-verifier verify --tenant-id=demo --source=db --dsn=$DATABASE_URL
# → "Цепочка: ЦЕЛАЯ"

# Tampering simulation
psql -c "ALTER TABLE audit.events DISABLE TRIGGER no_update;"
psql -c "UPDATE audit.events SET payload='{\"tampered\":true}' WHERE id=...;"
psql -c "ALTER TABLE audit.events ENABLE TRIGGER no_update;"

# Re-verify
./tools/audit-verifier verify --tenant-id=demo --source=db --dsn=$DATABASE_URL
# → "Цепочка: РАЗОРВАНА — TAMPERING ALERT (P0)"
```

**Talking points**:

- «SHA-256 hash chain делает audit-log tamper-evident. Любая
  модификация (даже DBA с прямым доступом) ломает цепочку.»
- «Дополнительно: каждое событие подписывается Ed25519
  (production) или ГОСТ-2012 (после получения КриптоПро лицензий).
  Verifier проверяет криптоподпись через `--pubkey` flag.»
- «Multi-tenant isolation: каждый банк-тенант имеет отдельную
  PostgreSQL schema, отдельные S3 buckets, отдельные Kafka topics.
  Никаких `tenant_id` в общих таблицах.»

```bash
psql -c "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tnt_%'"
# Видим только tnt_demo — для multi-bank деплоя был бы tnt_alfa, tnt_sber, ...
```

---

### 15:00 — Pilot-readiness gap (что НЕ готово)

**Talking points** — открыто и честно:

> «Часть платформы пока в pre-integration состоянии — реальные
> внешние API нужно подключить:»

| Что | Статус | Lead-time |
|-----|--------|-----------|
| ФНС/ЕГРЮЛ live | 🟡 framework + mapper готов, ждём договор через Минцифры | 4-8 нед |
| КриптоПро/VipNet (УКЭП) | 🟡 Provider-абстракция готова, ждём лицензии | 2-4 нед |
| Real LLM модели (vLLM на GPU) | 🟡 backend ready, env-overrides, ждём GPU | 2-3 нед |
| Pen-test внешний | ❌ ждём подрядчика | 2-3 нед |

> «Полный список — в [docs/pilot-readiness.md](pilot-readiness.md).
> Технически платформа готова к флипу `*_LIVE=true` для каждой
> интеграции. Демо вы видели — реальные ресурсы добавляются за
> счёт ENV без переписывания кода.»

---

### 17:00 — Следующие шаги

> «Что мы предлагаем:»

1. **Audit-bundle** для вашей ИБ-команды: `docs/audit-bundle.md` —
   threat model, ADR, runbooks, integrity checks. Можем отправить
   приватной ссылкой на репозиторий.
2. **Технический deep-dive** для DevOps команды: ADR-0013 (Vault HA)
   и ADR-0014 (mTLS Istio) — как платформа развёртывается в on-prem
   или SaaS.
3. **LOI на пилот** — мы готовим: setup fee + per-event pricing +
   maintenance. Шаблон договора готов.
4. **Sandbox для вашей команды** — можем выдать staging-доступ для
   self-validation за 1-2 недели.

---

## Если что-то пошло не так в demo

**Сервис не отвечает** (`make smoke` прошёл, но scenario fail'ит):
- Проверить `docker ps` — все ли контейнеры running
- `docker compose logs <service>` — найти ошибку
- Если AI-агент висит — `OPENAI_API_KEY` не нужен в demo (используется
  mock-backend), но если кто-то выставил `LLM_GATEWAY_FORCE_MOCK=0` —
  upstream vLLM недоступен

**Сценарий зависает в `wait_for_state`**:
- Проверить Temporal UI: `http://localhost:8080` (Temporal Web)
- Workflow может быть в waiting на signal — отправить через
  `signal_documents_uploaded` вручную

**Audit-trail пустой**:
- audit-service health: `curl http://localhost:8081/healthz`
- audit-sdk endpoint: проверить env `AUDIT_SERVICE_URL` в каждом
  сервисе

---

## После встречи

1. Отправить summary с конкретными action items (кто делает что и до
   когда — DPA, SLA review, technical deep-dive в [docs/devops-review-package.md](./devops-review-package.md))
2. Запланировать follow-up через 1-2 недели
3. Запросить feedback на demo (что было непонятно? чего не хватило?)
