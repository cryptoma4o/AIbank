# ADR-0010: Биллинг-модель и audit log — отдельный billing-service с Kafka и correlation_id связью с audit

**Status:** Accepted
**Date:** 2026-04-26
**Authors:** Главный архитектор, Финансовый директор, Тех-лид backend
**Reviewers:** Security-инженер, DBA, Продакт

## Context

Биллинговая модель платформы описана в `docs/product-vision.md` § 5: per-event тарификация (200–400 ₽ за открытие счёта ИП, 800–1500 ₽ за ООО, 2000–4000 ₽ за АО, проверка УБО — отдельная позиция), плюс maintenance fee, плюс премиум-фичи (например, white-label-кастомизации, расширенные интеграции). Юнит-экономика — 17.5 млн ₽ с банка в год — критически зависит от того, что мы корректно учитываем каждое тарифицируемое событие.

Силы, давящие на это решение:

- **Per-event billing требует надёжного потока событий.** Каждый «открытый счёт ИП» = одна билинг-запись. Потеря 1% событий = потеря 1% выручки = десятки тысяч рублей в месяц на одного банка. Нужна at-least-once-доставка с дедупликацией.
- **Audit log как доказательная база.** При согласовании счёта с банком должна быть возможность сказать: «вот 1247 случаев `account.opened` за апрель — вот audit-события каждого случая». Без этой связи биллинг превращается в споры на ровном месте.
- **Cross-tenant платформенная природа.** Биллинг — это **платформенный** концерн: мы биллим банков, а не их клиентов. Это первая истинно cross-tenant подсистема в платформе. ADR-0002 явно описывает, как такие подсистемы живут: схема `platform`, не `tnt_<id>`.
- **Audit log уже есть и устроен.** `security-architecture.md` § 8.3 фиксирует: append-only PostgreSQL, hash-цепочка, подпись каждой записи, 5-летнее хранение. Audit-events пишет `audit-service` через `audit-sdk` (`packages/audit-sdk/`, см. § 4.5). Биллинг не должен ломать эту картину или превращать audit log во второй биллинг.
- **Идемпотентность.** При retry от источника события (например, после сбоя `onboarding-orchestrator`) повторное `account.opened` для той же заявки не должно увеличивать биллинг.
- **Регуляторика.** Audit log хранится 5 лет (115-ФЗ). Билинг-события — финансовая отчётность, по НК РФ хранятся 4 года, но мы выбираем тот же 5-летний срок для согласованности.
- **Premium-фичи и maintenance fee.** Это не per-event, а subscription-style (плата за период). Биллинг должен поддерживать оба типа.
- **Сверка с банком.** Раз в месяц банк хочет получить выгрузку «вот N открытий счетов, вот M проверок УБО, вот премиум-фичи» с возможностью по каждой строчке посмотреть, какой именно applicant это был. Без `correlation_id` к audit log это невозможно.
- **Кросс-сервисный источник событий.** События билинга порождают разные сервисы: `onboarding-orchestrator` (открытие счёта), `agent-ubo-tracing` (УБО-проверка как отдельный billable event), `tenant-service` (premium-фичи), `web-admin` (экспорт данных как отдельная услуга). Все они должны единообразно публиковать билинг-события.
- **Интеграция с LLM-биллингом.** ADR-0011 фиксирует: каждый LLM-вызов имеет `cost_kop` для resolved-модели; этот `cost_kop` — либо часть нашего биллинга банку (если банк не self-host), либо часть нашей внутренней себестоимости.
- **Связь с миграциями.** ADR-0005 в Implementation Notes говорит: «Журнал применения миграций экспортируется в наш билинг при следующей синхронизации (см. ADR-0010)». Биллинг — единое место сбора operational events для платформенной отчётности, в том числе релизных.

Если ничего не решить, биллинг распылится: один сервис пишет в свою таблицу, второй — Kafka-event без consumer'а, третий — файл. Через полгода — discrepancy с банком, которое невозможно разрешить иначе как ручным аудитом.

## Decision

Создаём **отдельный `billing-service`** с собственной таблицей `platform.billing_events` (cross-tenant, в схеме `platform`, ADR-0002). События в биллинг публикуются через **Kafka topic `platform.billing.events`** всеми сервисами-источниками. Связь с audit log — через **`correlation_id`** (audit_event_id хранится в billing_event для сверки и реконсиляции). Идемпотентность — через **event_id (UUID v4, генерируется источником)**.

Решение состоит из шести частей: (1) сервис, (2) поток событий, (3) схема `billing_events`, (4) связь с audit, (5) идемпотентность, (6) аналитика и отчётность.

### Часть 1. `services/billing-service/`

Новый Go-сервис в `services/billing-service/` (упомянут в `technical-structure.md` § 4.5 как платформенный сервис). Ответственности:

- Consume `platform.billing.events` Kafka topic.
- Идемпотентная запись в `platform.billing_events`.
- Cross-tenant аналитика: монтаж per-tenant отчётов, экспорт в банковский формат (XLSX/JSON).
- HTTP API для `web-admin` платформы (наша админка, не банковская): дашборды, корректировки, выгрузки.
- gRPC API для других сервисов: «сколько событий типа X у тенанта Y за период Z».
- Прескриптивные правила (rate cards) — версионируемая yaml-конфигурация, какая цена за какой `event_type` в зависимости от тенанта.
- Расчёт суммы invoicing раз в месяц (cron / Temporal-workflow, см. ADR-0001) с генерацией PDF/XML.

`billing-service` подключается к **схеме `platform`** общего PostgreSQL-кластера (не к `tnt_<id>`-схемам), потому что биллинг — платформенный концерн. Это полностью соответствует ADR-0002 («Cross-tenant сервисы … подключаются к схеме `platform`»).

### Часть 2. Поток событий

```
┌─────────────────────────────┐
│ onboarding-orchestrator     │ — публикует account.opened
└──────────────┬──────────────┘
┌──────────────┴──────────────┐
│ agent-ubo-tracing           │ — публикует ubo.verification.completed
└──────────────┬──────────────┘
┌──────────────┴──────────────┐
│ tenant-service              │ — публикует premium.feature.activated, maintenance.month
└──────────────┬──────────────┘
┌──────────────┴──────────────┐
│ llm-gateway (через Phase 3) │ — публикует llm.usage (агрегат за минуту)
└──────────────┬──────────────┘
               │
               ▼
   ┌───────────────────────────┐
   │ Kafka topic               │
   │ platform.billing.events   │  (compacted=false, retention 14 дней — backstop)
   └────────────┬──────────────┘
                ▼
       ┌────────────────┐
       │ billing-service│ ── записывает в platform.billing_events
       └────────┬───────┘
                ▼
       ┌────────────────┐
       │ aggregator     │ ── VictoriaMetrics для dashboard'ов
       └────────────────┘
```

**Правила публикации события:**

1. Сервис-источник вызывает метод доменной операции (например, `OpenAccount` в Temporal-workflow).
2. **В одной транзакции** с записью результата операции и audit-event пишется outbox-запись в `tnt_<id>.billing_outbox`.
3. Отдельный outbox-publisher (часть `audit-sdk` или отдельная Go-горутина) публикует outbox-записи в Kafka topic `platform.billing.events` и помечает их published.
4. `billing-service` consume'ит, идемпотентно пишет в `platform.billing_events`.

**Outbox pattern критичен:** прямая публикация в Kafka из сервиса теряет события при сбое между «БД commit» и «Kafka publish». Outbox + publisher — стандартный SAGA-паттерн, который мы уже применяем для audit.

### Часть 3. Схема `platform.billing_events`

```sql
CREATE TABLE platform.billing_events (
    event_id           UUID PRIMARY KEY,                 -- v4, генерируется источником
    tenant_id          TEXT NOT NULL REFERENCES platform.tenants(id),
    event_type         TEXT NOT NULL,                    -- 'account.opened.individual', 'ubo.verification', 'premium.web_admin_extra', 'maintenance.month', 'llm.usage.day', ...
    event_subtype      TEXT,                              -- доп. дискриминатор: 'ip' / 'llc' / 'jsc' для account.opened
    occurred_at        TIMESTAMPTZ NOT NULL,             -- бизнес-время события (когда счёт был открыт), не время записи
    recorded_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    correlation_id     UUID,                              -- audit_event_id, ссылка на запись в audit log
    audit_chain_position BIGINT,                          -- позиция в hash-цепочке для быстрого lookup
    source_service     TEXT NOT NULL,                    -- 'onboarding-orchestrator' / 'agent-ubo-tracing' / ...
    quantity           NUMERIC(12, 4) NOT NULL DEFAULT 1, -- штуки или дробное (например, 0.0124 для частичного maintenance)
    rate_card_version  TEXT NOT NULL,                    -- 'rates-2026q2-v3' — версия rate card, действовавшая на момент события
    unit_price_kop     BIGINT,                            -- nullable, заполняется billing-service по rate card
    amount_kop         BIGINT,                            -- nullable, quantity × unit_price_kop
    currency           TEXT NOT NULL DEFAULT 'RUB',
    metadata           JSONB,                             -- доп. контекст (application_id, model_used, etc.)
    invoiced_in        TEXT,                              -- nullable, 'invoice-2026-04-bank-alpha' после включения в счёт
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Индексы для типовых запросов:
CREATE INDEX billing_events_tenant_period
    ON platform.billing_events (tenant_id, occurred_at);
CREATE INDEX billing_events_correlation
    ON platform.billing_events (correlation_id) WHERE correlation_id IS NOT NULL;
CREATE INDEX billing_events_uninvoiced
    ON platform.billing_events (tenant_id, occurred_at)
    WHERE invoiced_in IS NULL;
CREATE INDEX billing_events_type
    ON platform.billing_events (tenant_id, event_type, occurred_at);

-- Append-only enforcement через триггер (UPDATE / DELETE запрещены):
CREATE OR REPLACE FUNCTION platform.billing_events_immutable()
RETURNS TRIGGER AS $$ BEGIN
    RAISE EXCEPTION 'platform.billing_events is append-only';
END; $$ LANGUAGE plpgsql;

CREATE TRIGGER billing_events_no_update
    BEFORE UPDATE OR DELETE ON platform.billing_events
    FOR EACH ROW EXECUTE FUNCTION platform.billing_events_immutable();
```

Обновление `invoiced_in` — единственное исключение, реализуется через специальную хранимую процедуру `platform.mark_invoiced(event_ids[], invoice_id)` с `SECURITY DEFINER` и аудит-логом. Прямые UPDATE невозможны.

### Часть 4. Связь с audit log: correlation_id

Каждое билинг-событие имеет соответствующий audit-event. Connection — через `correlation_id`:

```
┌────────────────────────────┐         ┌──────────────────────────────┐
│ tnt_alfa.audit_events      │         │ platform.billing_events      │
│   id            UUID PK    │◀────────│   correlation_id    UUID FK  │
│   action='account.opened'  │         │   event_type='account.       │
│   subject_id    UUID       │         │       opened.llc'            │
│   data          JSONB      │         │   tenant_id  'bank-alpha'    │
│   hash_chain    TEXT       │         │   amount_kop      120000     │
└────────────────────────────┘         └──────────────────────────────┘
```

**Зачем не embed billing-fields прямо в audit:**

- Audit-event описывает «что произошло»; biling-event — «как это монетизировалось». Это разные жизненные циклы (биллинг может быть пересчитан задним числом в случае ошибки rate card, audit — нет; rate card версионируется, audit-payload — нет).
- Audit per-tenant (tnt_<id>.audit_events), биллинг — cross-tenant (`platform.billing_events`); embedding ломает изоляцию (см. ADR-0002).
- При запросе банка «покажи все ваши биллинг-события за месяц» мы не должны давать доступ к audit-payload (там часто есть PII).

**Зачем `correlation_id` важен:**

- Спор с банком «откуда эти 50 000 ₽ за апрель за УБО-проверки» → 27 строчек в `billing_events` → 27 audit-events с подробным контекстом → банк сам видит, что да, это было.
- Регуляторный запрос «покажи все события, повлиявшие на счёт за период» → cross-table join через correlation_id.
- Реконсиляция: периодически (раз в неделю) `billing-service` запускает Temporal-job, который проверяет: каждый `billing_event` имеет рабочий `audit_event` (если correlation_id указан); каждый audit-event типа `account.opened` имеет соответствующий `billing_event`. Расхождения — алерт.

**`audit_chain_position`** — денормализованная позиция в hash-цепочке для быстрого lookup без полного table scan по audit per-tenant.

### Часть 5. Идемпотентность

`event_id` — UUID v4, **генерируется источником** в момент порождения события (не в момент публикации в Kafka). Это значит:

- Retry с источника (например, Temporal Activity повторила свою работу из-за ошибки сети до commit) генерирует **тот же** `event_id`, потому что `event_id` детерминирован от `(workflow_id, activity_id, attempt_n=0)` или от естественного бизнес-ключа (`application_id + event_type`).
- При двойной публикации в Kafka (consumer-group rebalance) — `billing-service` ловит конфликт по primary key и логирует as duplicate без записи.
- При двойной обработке у consumer — то же самое.

**Соглашение для источников:**

```go
// onboarding-orchestrator/internal/billing/emit.go
func EmitAccountOpened(ctx context.Context, app *Application) BillingEvent {
    return BillingEvent{
        EventID:       uuid.NewSHA1(uuid.NameSpaceOID, []byte("account.opened|"+app.ID)).String(),
        TenantID:      app.TenantID,
        EventType:     "account.opened",
        EventSubtype:  app.LegalEntity.Type, // 'ip' | 'llc' | 'jsc'
        OccurredAt:    app.AccountOpenedAt,
        CorrelationID: app.LastAuditEventID,
        SourceService: "onboarding-orchestrator",
        Quantity:      1,
        RateCardVersion: ratecard.CurrentVersion(ctx, app.TenantID),
        Metadata: map[string]any{
            "application_id": app.ID,
            "abs_account_id": app.ABSAccountID,
        },
    }
}
```

UUID v5 (детерминированный SHA1-based) от `(event_type, business_key)` гарантирует один и тот же `event_id` при retry.

### Часть 6. Аналитика и отчётность

**VictoriaMetrics** (см. § 3.5):

- Метрики `billing_events_total{tenant, event_type, source_service}` экспортируются `billing-service`.
- Grafana-дашборды:
  - «События за день» (для нашего ops-team).
  - «События за месяц на тенанта» (для CSM-менеджера).
  - «Расхождения audit ↔ billing» (для финансов).
  - «Latency публикации события» (для DevOps — выявляет медленный outbox-publisher).

**Месячный invoicing**:

- Temporal-workflow `MonthlyInvoicing` стартует 1-го числа каждого месяца:
  1. Для каждого тенанта — выбор всех `billing_events` за прошлый месяц где `invoiced_in IS NULL`.
  2. Применение rate card → расчёт `unit_price_kop` и `amount_kop`.
  3. Группировка по `event_type` → строки счёта.
  4. Генерация invoice PDF/XML (УПД-формат для РФ).
  5. Atomic update `invoiced_in = 'invoice-YYYY-MM-<tenant>'`.
  6. Отправка банку по согласованному каналу.

**Ad-hoc корректировки**: через `tools/billing-cli` с обоснованием в audit log (`platform.billing_adjustment` event); прямой SQL запрещён.

### Что НЕ покрывает решение

- **Конкретные ставки rate card** — определяются `product-vision.md` § 5 и периодически пересматриваются продактом без необходимости в новом ADR.
- **Налоговая часть** (НДС, авансовые платежи) — отдельный финансовый workflow, использует `billing-service` как источник данных, но реализуется в `1С` или эквиваленте на стороне финансового отдела.
- **Биллинг конечных клиентов банка** — это дело банка, не наше; мы биллим банк.
- **Federated billing** для on-prem банков, которые сами хотят учитывать события без нашего участия — отдельный режим, опциональный, не в MVP (рассмотрим как Phase 7+ feature).
- **Кросс-валютный учёт** — все события в RUB; multi-currency — будущее расширение.

### Когда пересматривается решение

- **Появление 50+ тенантов** — `platform.billing_events` начнёт расти быстро, потребуется партиционирование по дате.
- **Регуляторное требование онлайн-инвойсинга** (как в ОФД для розничной торговли) — потребует push-events с задержкой <1 сек, что требует другой архитектуры (Kafka Streams aggregator вместо batch).
- **Появление платформы B2B клиентов помимо банков** (например, мы продаём `agent-document-intake` как отдельный сервис) — потребуется multi-product биллинг.

## Alternatives Considered

### Альтернатива 1: In-DB billing_events таблица в каждом сервисе-источнике

**Плюсы:**

- Нулевая дополнительная инфраструктура.
- Транзакционная гарантия с доменной операцией.

**Минусы:**

- **Каждый сервис пишет в свою таблицу** — по сервисам. Отчётность — `UNION` по всем доменным БД. С учётом schema-per-tenant из ADR-0002 — `UNION` по 30 тенантов × N сервисов = десятки таблиц. Аналитика становится ad-hoc-кошмаром.
- **Cross-tenant отчётность невозможна без отдельного aggregator'а** — то есть всё равно нужен `billing-service`, только теперь он тянет из 30 баз в 30 местах.
- **Rate card применяется в момент записи** = при изменении rate card задним числом нужно переписывать события (нарушение append-only) или хранить две суммы.
- **Нет единого аудита публикации события** — отладка «почему этого события нет в счёте» требует похода в исходную таблицу сервиса.

**Причина отклонения:** размывает ответственность, делает отчётность дорогой; не решает кросс-сервисную природу биллинга.

### Альтернатива 2: Kafka events → ClickHouse aggregator

**Плюсы:**

- ClickHouse ОПТИМИЗИРОВАН для аналитики событий — миллиарды записей с low latency.
- Отлично подходит для dashboards с временными аггрегациями.
- Стандарт в индустрии для event-aggregation.

**Минусы:**

- **ClickHouse — еще один stateful-сервис** в платформе. Hybrid deployment (§ 1, § 10.2) принципиально это не запрещает, но каждый дополнительный stateful-pod в on-prem банковском кластере = дополнительный operational footprint, дополнительный backup, мониторинг. На наш масштаб (миллионы, не миллиарды событий за год на тенанта) — overkill.
- **Месячный invoicing удобнее в PostgreSQL** — там же транзакционные данные, joins с `tenant_keys`, `tenants`.
- **Audit reconciliation** через ClickHouse↔PostgreSQL — cross-database, медленно и хрупко.
- **Append-only с подписью** в ClickHouse реализуется хуже, чем в PG (нет нативных constraint-triggers).

**Причина отклонения:** аналитический оптимум, но операционный pessimum для нашего масштаба и принципа hybrid. Возвращаемся к рассмотрению при превышении 100 млн events/год на тенанта.

### Альтернатива 3: Embed билинг в audit log как special event type

**Плюсы:**

- Один store, одна цепочка — простая реконсиляция «биллинг = подмножество audit».
- Нулевая новая инфраструктура.
- Единая модель подписи и hash-цепочки.

**Минусы:**

- **Cross-tenant природа биллинга нарушает изоляцию audit** (per-tenant из ADR-0002). Запрос «выгрузка биллинга всех тенантов» требует чтения из всех `tnt_<id>.audit_events` — ровно то, что ADR-0002 запрещает делать прямыми JOIN.
- **Rate card применяется задним числом** — невозможно, audit immutable.
- **Размер audit log растёт** — биллинг события льются с высокой частотой, audit для регуляторики оптимизируется под доказательность, не throughput.
- **Aggregator всё равно нужен** — но он теперь читает audit на 30 тенантов вместо одной cross-tenant таблицы.
- **Рискованно с т.з. ИБ-аудита банка**: аудитор задаёт «зачем платформа читает наш audit cross-tenant?» — длинное и плохое объяснение.

**Причина отклонения:** конфликтует с ADR-0002 (cross-tenant aggregation запрещён через JOIN), смешивает разные жизненные циклы (audit immutable vs billing recalculable), нарушает разделение platform vs tenant.

### Альтернатива 4: Отдельный billing-service со своим store **в облаке/SaaS**

**Плюсы:**

- Не нужно деплоить в on-prem.
- Готовые фичи (Stripe, Chargebee, Recurly).

**Минусы:**

- **152-ФЗ:** биллинг-события содержат корреляции с ПДн (через correlation_id и application_id) — отправка в зарубежный SaaS = нарушение локализации.
- **Hybrid deployment** запрещает managed-сервисы (§ 1).
- **Audit-связь** через зарубежный store невозможна.

**Причина отклонения:** прямое нарушение базовых принципов.

### Альтернатива 5: Один большой `billing-events` поток без разделения по `event_type`

**Плюсы:**

- Минимальная схема, всё в JSONB.

**Минусы:**

- **Аналитика становится дороже:** все запросы — по `metadata->>'subtype'`, без индексов это N table scan.
- **Rate card matching** требует разобрать JSONB на стороне `billing-service` — ошибки в типах ловятся позже.
- **Schema evolution** для новых типов сложнее без явных колонок.

**Причина отклонения:** компромисс не оправдывает потерю в аналитике; явные колонки `event_type` / `event_subtype` дают и гибкость JSONB (через `metadata`), и скорость аналитики.

## Consequences

### Positive

- **Cross-tenant природа биллинга явно отражена** в архитектуре: `platform`-схема, отдельный сервис, единая Kafka topic. Нет соблазна тащить `tenant_id` в общие таблицы (принцип multi-tenant соблюдён).
- **Audit и biling независимы**, но связаны через `correlation_id` — даёт точную сверку без архитектурного сцепления.
- **Append-only с триггером** = при ИБ-аудите банк видит «биллинг тоже append-only, как audit», что закрывает категорию «можете ли вы подделать счета?».
- **Идемпотентность через `event_id`** — стандартное решение, решает retry-проблемы во всех источниках.
- **Outbox pattern** для публикации = at-least-once без потерь при сбоях; уже знакомый паттерн (применяется для audit и для ABS, см. ADR-0006).
- **Rate card версионируется** = можно изменить тарифы с следующего месяца без переписывания истории; corner-case «банк хочет старую цену» решается через grandfathered-rate-card-version в `tenants.config`.
- **Реконсиляция автоматизирована** через еженедельный Temporal-workflow (см. ADR-0001) — расхождения audit ↔ billing видны до того, как банк их найдёт.
- **Совместимо с ADR-0009 (on-prem обновления)**: blue и green namespace оба пишут в одну `platform.billing_events`, hash-цепочка не страдает; `platform.release_activated` — отдельный billing-event nullable-amount (для отчётности «какие релизы прошли в месяце»).
- **Совместимо с ADR-0011 (LLM routing)**: `cost_kop` каждого LLM-вызова идёт в `billing-service` агрегированно (раз в минуту, не по каждому вызову — это снизит throughput до контролируемого уровня).

### Negative

- **Outbox pattern overhead** — каждое доменное действие пишет +1 запись в `tnt_<id>.billing_outbox`, plus отдельный publisher-горутина в каждом сервисе-источнике. Mitigation: уже есть для audit, переиспользуем pattern в `audit-sdk`.
- **Дополнительный сервис в платформе.** `billing-service` нужно деплоить, мониторить, обновлять. Mitigation: единственный сервис, обслуживающий cross-tenant операции; малое stateless-приложение (один Helm-chart).
- **Lag между событием и счётом.** Outbox → Kafka → consumer → DB write → invoicing — суммарно секунды, но в худшем случае (сбой Kafka) — часы. Mitigation: outbox retention 14 дней — backstop восстановление; реконсиляция-job ловит то, что не попало.
- **Сложность отладки кросс-сервисной публикации.** Если `account.opened` не дошёл до `billing_events`, причина может быть в outbox / Kafka / consumer. Mitigation: дашборд «events emitted vs events consumed» с алертом при расхождении >5%; trace-id в каждом событии.
- **Размер `platform.billing_events` без партиционирования** — при 30 тенантах × 5000 событий/мес × 60 мес = 9 млн строк за 5 лет; не критично, но потребует партиционирования при росте до 50+ тенантов.

### Neutral

- **Rate card конфигурация.** `configs/rates/rates-2026q2-v3.yaml` — версионируется в Git. Изменения через PR с продактом. Не привязывает к ADR.
- **Корректировки** через `billing-cli` — это операционный инструмент, не automation; ответственность лежит на финансовом менеджере.
- **JSONB metadata** растёт со временем по schema. Mitigation: документация в `services/billing-service/docs/event-types.md` фиксирует, какие `metadata`-поля для какого `event_type` ожидаются.
- **Источник даты события (`occurred_at`).** Бизнес-время, может отличаться от `recorded_at` на дни (например, retroactive event для исторических кейсов). Mitigation: invoicing использует `occurred_at` для группировки в месяце.

## Implementation Notes

### Структура сервиса

```
services/billing-service/
├── cmd/
│   ├── server/main.go            -- gRPC + HTTP server
│   ├── invoicer/main.go           -- Temporal worker для MonthlyInvoicing
│   └── reconciler/main.go         -- Temporal worker для weekly reconciliation
├── internal/
│   ├── consumer/                  -- Kafka consumer для platform.billing.events
│   ├── store/                     -- PostgreSQL access (sqlc)
│   ├── ratecard/                  -- yaml-конфигурация и резолюция цен
│   ├── invoicing/                 -- генерация PDF/XML
│   └── reconciliation/            -- audit ↔ billing дельта
├── migrations/
│   └── platform/                  -- DDL для platform.billing_events (см. ADR-0005)
├── proto/                         -- gRPC API (биллинг-данные для web-admin платформы)
├── go.mod
└── project.json                   -- Nx-таргеты
```

### Outbox-паттерн в источниках

Каждый сервис-источник имеет в своей tenant-схеме:

```sql
CREATE TABLE tnt_<id>.billing_outbox (
    id              UUID PRIMARY KEY,
    event_payload   JSONB NOT NULL,                   -- готовый BillingEvent
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ
);
```

В рамках доменной транзакции:

```go
tx, _ := db.Begin(ctx)
    tx.Exec("INSERT INTO applications ... ")
    auditID, _ := auditSDK.Write(ctx, tx, "account.opened", ...)
    tx.Exec(`INSERT INTO billing_outbox VALUES ($1, $2, NOW(), NULL)`,
        evt.EventID, evt.WithCorrelation(auditID).JSON())
tx.Commit()
```

Параллельная горутина (или CronJob) каждые 5 секунд тащит unpublished-rows, публикует в Kafka, помечает published.

### Kafka topic config

```yaml
topic: platform.billing.events
partitions: 12                     # хватает для всех тенантов и источников
replication_factor: 3              # на SaaS; для on-prem — 1 или 3 в зависимости от банковского Kafka
retention.ms: 1209600000            # 14 дней — backstop
cleanup.policy: delete              # не compacted — все события важны
config:
  min.insync.replicas: 2
  compression.type: lz4
```

Schema: Confluent Schema Registry-совместимая (или Apicurio для РФ-friendly), `BillingEvent` Avro/JSON-schema. Forward/backward совместимость обязательна.

### Идемпотентный consumer

```go
func (c *Consumer) handle(msg *kafka.Message) error {
    var evt BillingEvent
    if err := json.Unmarshal(msg.Value, &evt); err != nil { return err }

    _, err := c.db.Exec(ctx, `
        INSERT INTO platform.billing_events (event_id, tenant_id, event_type, ...)
        VALUES ($1, $2, $3, ...)
        ON CONFLICT (event_id) DO NOTHING
    `, evt.EventID, evt.TenantID, evt.EventType /*, ...*/)
    return err
}
```

`ON CONFLICT DO NOTHING` обеспечивает идемпотентность — повторное событие просто игнорируется без ошибки.

### Реконсиляция (еженедельная)

Temporal workflow `WeeklyReconciliation`:

1. Для каждого тенанта читает `tnt_<id>.audit_events` за прошлую неделю с `action IN ('account.opened', 'ubo.verification', 'premium.feature.activated')`.
2. Для каждой такой записи проверяет существование соответствующего `platform.billing_events` через `correlation_id`.
3. Расхождения публикуются как `platform.billing_reconciliation` event и алертится в Telegram/Mattermost (см. § 3.5).

Threshold: >5 расхождений на тенанте за неделю → P1 алерт (см. `security-architecture.md` § 8.4).

### Rate card example

```yaml
# configs/rates/rates-2026q2-v3.yaml
version: rates-2026q2-v3
effective_from: 2026-04-01
effective_to: null                  # до отзыва
events:
  account.opened:
    ip:    { unit_price_kop: 30000 }     # 300 ₽
    llc:   { unit_price_kop: 110000 }    # 1100 ₽
    jsc:   { unit_price_kop: 280000 }    # 2800 ₽
  ubo.verification:
    standard:   { unit_price_kop: 15000 }
    deep:       { unit_price_kop: 50000 }
  premium.feature.activated:
    custom_branding:  { unit_price_kop: 100000, recurrence: monthly }
    extended_api:     { unit_price_kop: 200000, recurrence: monthly }
  maintenance.month:
    base:  { unit_price_kop: 5000000 }   # 50 000 ₽/мес
  llm.usage.day:
    # формула: agg cost_kop за день из llm-gateway audit
    formula: passthrough
overrides:
  bank-alpha:
    account.opened.llc: { unit_price_kop: 100000 }   # пилотная скидка
```

`billing-service` загружает rate card при старте + reload по SIGHUP / ConfigMap change.

### gRPC API (для `web-admin` платформы)

```protobuf
service BillingQuery {
  rpc GetTenantUsage(TenantUsageRequest) returns (TenantUsageResponse);
  rpc GetMonthlyInvoice(MonthlyInvoiceRequest) returns (MonthlyInvoiceResponse);
  rpc GetReconciliationStatus(ReconciliationRequest) returns (ReconciliationResponse);
  rpc ExportEventsCSV(ExportRequest) returns (stream ExportChunk);
}
```

Доступ — только роли `platform.admin`, `platform.finance` (см. `security-architecture.md` § 3.2). Аккаунт-менеджеры конкретного банка — отдельная роль `platform.csm` с ограниченным доступом «только свои тенанты».

### Этапы внедрения

| Этап | Что | Когда |
|---|---|---|
| 1 | DDL миграции (`platform.billing_events`, outbox-таблицы, immutability trigger) | Phase 1 (Ф1, см. § 12) |
| 2 | `audit-sdk` — outbox-вспомогательные функции для билинга | Phase 1 |
| 3 | `billing-service` MVP: consumer + storage + базовый gRPC API | Phase 2 |
| 4 | Источники: `onboarding-orchestrator`, `tenant-service`, `agent-ubo-tracing` публикуют события | Phase 4 (Ф4, когда УБО появляется) |
| 5 | Rate card resolution + monthly invoicing workflow | Phase 4 |
| 6 | Реконсиляция, дашборды | Phase 4 |
| 7 | LLM-usage агрегация из llm-gateway | Phase 3 (Ф3) — параллельно с llm-gateway billing |
| 8 | Premium-фичи billing | Phase 5+ |

### Безопасность

- **Append-only enforced на DB-уровне** через trigger; `billing-service` использует роль `platform_billing_owner` без UPDATE/DELETE прав.
- **`SECURITY DEFINER`-функция `mark_invoiced`** — единственный способ модифицировать `invoiced_in`, аудит-логирует каждый вызов.
- **PII в metadata запрещён** — CI-линтер `billing-no-pii-metadata` проверяет, что `metadata` не содержит ключей, помеченных в `security-architecture.md` § 5.2 как PII (паспорт, СНИЛС, ИНН физлица). Application-level подменён на `application_id` (UUID).
- **Доступ к таблице** — только service-account `billing_app` с грантом INSERT (consumer) и SELECT (reporting).
- **Шифрование at-rest** — стандартный TDE PostgreSQL (`security-architecture.md` § 5.1).

## References

- `docs/product-vision.md` § 5 — биллинг-модель: per-event тарифы и юнит-экономика, фундамент этого ADR.
- `docs/technical-structure.md` § 4.5 (`billing-service` упомянут как платформенный сервис), § 8.3 (audit log structure), § 11 (CI/CD — релизные события идут в биллинг).
- `docs/security-architecture.md` § 3.2 (роли `platform.admin`, `platform.finance`), § 5.1 (at-rest шифрование), § 5.2 (PII классификация — что нельзя в metadata), § 8.2 (audit log — обязательное логирование решений), § 8.3 (5-летний срок).
- ADR-0001 (Temporal): `MonthlyInvoicing` и `WeeklyReconciliation` — Temporal workflow.
- ADR-0002 (Schema-per-tenant): cross-tenant природа биллинга → схема `platform`; outbox-таблицы — в `tnt_<id>` для транзакционности с доменной операцией.
- ADR-0005 (Стратегия миграций): миграции `platform.billing_events` идут как `platform`-миграции (один раз), outbox-миграции — как tenant-template (на каждый тенант).
- ADR-0006 (Версионирование ABS-адаптеров): операции через адаптер, влияющие на биллинг (например, `account.opened` через ABS), включают `adapter_version` в `metadata`.
- ADR-0007 (Промпт-менеджмент): aggregated `llm.usage` events содержат `prompt_id` и `prompt_version` в metadata — для cost-attribution.
- ADR-0009 (Стратегия on-prem обновлений): release-events идут в `billing_events` с `amount_kop = NULL` — для платформенной отчётности «какие релизы прошли в месяц», без денежной части.
- ADR-0011 (LLM routing): `resolved_model` и `cost_kop` агрегируются в LLM-usage events.
- ADR-0012 (Eval-corpus governance): корпус-адаптация и production traffic sampling сами по себе не billable (это R&D), но события sampling могут идти в audit для регуляторики.
- [Outbox Pattern (Chris Richardson)](https://microservices.io/patterns/data/transactional-outbox.html)
- [Kafka idempotent consumer](https://kafka.apache.org/documentation/#consumer)
- [PostgreSQL trigger-based immutability](https://www.postgresql.org/docs/current/triggers.html)
