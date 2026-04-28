# ADR-0003: GraphQL для BFF — single endpoint per BFF без федерации в MVP

**Status:** Accepted
**Date:** 2026-04-26
**Authors:** Главный архитектор, Тех-лид frontend
**Reviewers:** Тех-лид backend, Security-инженер, Продакт

## Context

Слой Backend-for-Frontend описан в `docs/technical-structure.md` § 4.1: отдельный BFF на каждое UI-приложение, агрегирующий данные из доменных микросервисов. На момент решения активно проектируются два BFF:

- **`bff-onboarding`** — для `apps/web-onboarding` (кабинет конечного клиента: заявитель, юрлицо, документы, статус)
- **`bff-admin`** — для `apps/web-admin` (рабочее место сотрудника банка: очередь заявок, риск-карточка, разбор кейса, отчёты)

Силы, давящие на выбор протокола между BFF и frontend:

- **Агрегация из 5+ микросервисов на один экран.** Открытие карточки заявки в `web-admin` требует данных из `client-service` (профиль), `document-service` (загруженные документы), `risk-engine` (скор + факторы), `ubo-service` (граф УБО), `audit-service` (история действий), часто плюс `tenant-service` (политики банка для конкретного экрана). REST-агрегация = N запросов из BFF к доменным сервисам + параллельный fetch на стороне UI = water-fall, который видит клиент.
- **Разный shape данных у двух UI.** `web-onboarding` показывает клиенту минимум: «вашу заявку проверяют, осталось донести устав». `web-admin` для того же `Application` показывает 50+ полей. Это означает либо два разных REST-эндпоинта на каждую сущность (комбинаторный взрыв `*-summary`, `*-full`, `*-with-risk`), либо клиент-driven выбор полей.
- **Типизация важна.** Стек фронта — TypeScript 5+ (см. `technical-structure.md` § 3.1). Канонические доменные типы живут в `packages/domain-model/` с генерацией для TS. Любой выбор протокола обязан давать типобезопасные клиенты с генерацией из контракта.
- **Schema versioning.** API-first и contract-driven — принцип № 6 в `technical-structure.md` § 1. Контракт BFF меняется чаще, чем доменные API: каждый экран — новая выборка полей. Нужен механизм эволюции без `/v2/`-эндпоинтов.
- **Аутентификация и tenant routing.** Каждый запрос имеет `X-Tenant-Id` и JWT, BFF транслирует это в downstream-вызовы. Роли (`bank.operator`, `client.applicant`, см. `security-architecture.md` § 3.2) определяют, какие поля фронту вообще доступны — поле `risk_score_factors` на `web-admin` отдаётся `compliance_officer`, на `web-onboarding` не отдаётся никогда.
- **Hybrid deployment.** Принцип hybrid (§ 1) запрещает managed-сервисы; всё, что выбираем, должно одинаково работать в SaaS и в air-gapped on-prem (§ 10.2).
- **Наблюдаемость и audit log.** `security-architecture.md` § 8.2 требует логировать «обращения к ПДн» — это значит, что BFF должен ясно видеть, к каким полям идёт запрос. На уровне REST «GET /applications/123» это всё; на уровне чего-то более структурированного — это явный список полей.
- **Команда.** 18–25 человек на год 1, фронтенд — 2–3 человека. Введение тяжёлой инфраструктуры (Apollo Federation, отдельный gateway) обязано окупаться функциональностью, а не «потому что модно».

Если ничего не решить, два BFF выберут разные подходы: один — REST с per-screen эндпоинтами, второй — что-то ещё. Через 3 месяца фронт-команда жонглирует двумя клиентскими моделями, генераторы кода расходятся, типы между BFF и UI рассинхронизируются.

## Decision

Используем **GraphQL как протокол между BFF и frontend, single endpoint на каждый BFF, без федерации в MVP**. Конкретно:

- В `services/bff-onboarding/` и `services/bff-admin/` каждый сервис экспонирует **один HTTP endpoint** `POST /graphql` (плюс `GET /graphql` для playground в dev).
- На стороне сервера — **`gqlgen`** (Go, schema-first, codegen-driven). Это естественно ложится на наш Go-стек (см. `technical-structure.md` § 3.2: chi + sqlc + gRPC + temporal-go-sdk).
- На стороне фронта — **Apollo Client** или **urql** + **GraphQL Code Generator** для TS-типов из `.graphql`-схемы. Окончательный выбор клиентской библиотеки делает фронт-команда; ADR фиксирует только что клиент должен поддерживать persisted queries и normalized cache.
- **Schema-first, не code-first.** Каноническая схема — `services/bff-*/schema/*.graphql`, проверяется в CI на breaking changes (см. Implementation Notes).
- **GraphQL не выходит за пределы BFF.** Между BFF и доменными микросервисами по-прежнему gRPC + REST по контрактам из `packages/proto/` и `packages/openapi/` (см. § 4.2). Federation, mesh, schema stitching — не используются.
- **Один и тот же endpoint обслуживает нескольких клиентов одного приложения** (web + потенциально мобильный): разные клиенты делают разные запросы к одной схеме.
- **Authorization выполняется на BFF**, на каждом резолвере, по claims из JWT (см. `security-architecture.md` § 3.2). Запрос к полю, на которое у роли нет прав, — возвращает `null` с типизированной ошибкой `UnauthorizedField`.

**Что НЕ покрывает решение:**

- API между frontend и third-party-системами (Госуслуги, ЕГРЮЛ) — он остаётся в доменных сервисах через REST/SOAP, BFF — фасад.
- API между микросервисами — gRPC + REST, не GraphQL.
- Subscriptions / WebSockets для long-running операций (статус заявки) — рассматривается отдельно, в MVP polling каждые 5–10 с.

### Когда пересматривается решение

- **Появление третьего BFF** (например, `bff-courier` для `apps/courier-tablet`) с кросс-доменным запросом «одна заявка по всем BFF одновременно». Тогда — Apollo Router + Federation v2.
- **Превышение latency budget** на single-endpoint per BFF (p99 BFF > 500 мс из-за heavy resolver-chains). Сначала смотрим dataloaders, persisted queries, кеши; только потом — федерация.
- **Появление reusable GraphQL-схем между BFF** (50%+ типов общие) — рассмотрим shared subgraph.

## Alternatives Considered

### Альтернатива 1: REST + per-screen aggregation endpoints

**Плюсы:**

- Знаком всем backend-разработчикам, нулевая кривая входа.
- Простой кеш через HTTP-уровень (CDN, ETag, `Cache-Control`).
- Точное соответствие OpenAPI-генерации, которая уже есть в `packages/openapi/`.
- Легко логируется и отлаживается стандартными инструментами (curl, Postman).
- BFF-резолверы можно положить «прямо на handler» без отдельного query-парсера.

**Минусы:**

- **Комбинаторный взрыв эндпоинтов.** Один `Application` отображается на 6–8 экранах, каждый со своим shape: `GET /applications/{id}/summary`, `/full`, `/with-risk`, `/for-compliance`, `/timeline`. Через полгода — 80+ эндпоинтов, и большинство — копипаст с разной выборкой полей.
- **Over-fetching и under-fetching.** Клиент или получает всё (нагрузка на сеть и БД) или делает 3–5 параллельных запросов на один экран (water-fall в браузере).
- **Типизация рассинхронизируется.** OpenAPI генерит типы, но обновление схемы требует git-флоу с pinned-версиями; в практике фронт и бэк часто рассинхронизированы по версии типов.
- **Versioning через `/v1/`, `/v2/`.** Изменение поля = либо breaking, либо `?include=`-флаги, которые сами становятся неявным API.

**Причина отклонения:** на двух BFF с богатыми экранами (особенно `web-admin`) комбинаторный взрыв превращает поддержку BFF в сизифов труд. GraphQL решает именно эту проблему, ради которой он создавался.

### Альтернатива 2: GraphQL Federation (Apollo Router) с самого начала

**Плюсы:**

- Каждый BFF (или доменный сервис) — subgraph; единая supergraph-схема.
- Легко добавить третий-четвёртый BFF без рефакторинга.
- Apollo Router написан на Rust, низкая latency.
- Хорошо ложится на DDD: subgraph = bounded context.

**Минусы:**

- **Дополнительный сервис** (Apollo Router) + дополнительная супер-схема + composition-pipeline в CI. Для двух BFF это инфраструктура ради инфраструктуры.
- **Apollo Router GPL/Elastic-License-2.0** — лицензионные ограничения требуют юридической оценки для дистрибутива в banking on-prem (приемлемо, но дополнительный пункт в чек-листе).
- **Кривая обучения** — `@key`, `@requires`, `@provides`, entity resolvers; для команды из 2–3 фронтов и 4–5 бэков — лишний слой когнитивной нагрузки.
- **Доменные сервисы становятся subgraph-aware**, что протекает GraphQL-абстракцией в доменный слой. Принцип «GraphQL не выходит за BFF» при federation размывается.
- **Federation решает проблему, которой ещё нет**: у нас два BFF, каждый со своим UI; кросс-BFF-запросы пока не нужны.

**Причина отклонения:** premature optimization. Возвращаемся к рассмотрению при появлении третьего BFF или явной потребности в кросс-агрегации.

### Альтернатива 3: tRPC

**Плюсы:**

- End-to-end типизация без codegen — типы выводятся из TS-сигнатур серверных процедур.
- Очень низкий boilerplate, отлично подходит для TS-only стека.
- Активная экосистема, хорошие интеграции с Next.js.

**Минусы:**

- **TypeScript-only.** Наш backend — Go. tRPC требует, чтобы сервер был на TS. Это означало бы переписать BFF на Node.js, отказавшись от единого стека и от существующих Go-паттернов (`packages/audit-sdk` Go-биндинги, sqlc-клиенты к доменным сервисам).
- **Отсутствие introspection** в форме, привычной для тулинга (нет GraphiQL/Playground).
- **Schema versioning** — отсутствует в явном виде, всё через TS-типы; в hybrid on-prem развёртываниях с годовыми циклами это рискованно.

**Причина отклонения:** требует смены языка BFF, что несоразмерно дорого ради DX-выигрыша.

### Альтернатива 4: RPC + Protobuf (gRPC-Web для фронта)

**Плюсы:**

- Та же модель контрактов, что и между сервисами (см. `packages/proto/`).
- Бинарный формат — быстрее REST на больших payload.
- Сильная типизация, codegen для всех языков.
- Естественный путь, если хотим единый протокол сверху вниз.

**Минусы:**

- **gRPC-Web хуже совместим с современным Next.js / SSR.** Streaming через server actions работает, но debugging-tooling на порядок беднее GraphQL.
- **Per-screen aggregation требует RPC-метода на экран** — та же проблема, что у REST (Альтернатива 1).
- **DevTools для бизнес-аналитика.** GraphQL Playground — мощный инструмент для продакта/QA «посмотреть, что вернёт сервер для такого-то экрана»; в gRPC-Web аналогичного UX нет.
- **Авторизация по полю** в Protobuf неудобна — каждое поле либо в сообщении, либо нет; динамическая выборка через `FieldMask` возможна, но это самописная инфраструктура поверх Protobuf.

**Причина отклонения:** для BFF—frontend плюсы Protobuf не перевешивают потерю DX. RPC оставляем там, где он действительно нужен — service-to-service.

### Альтернатива 5: BFF без оркестрации (frontend-direct calls в API Gateway)

**Плюсы:**

- Нет дополнительного сервиса.
- Минимальная latency.

**Минусы:**

- **Нарушает принцип BFF.** `technical-structure.md` § 4.1 явно описывает BFF как обязательный слой, в том числе для поддержки разных shape данных на разные клиенты.
- **Авторизация и audit log** размазываются по доменным сервисам; каждый из них должен фильтровать поля по роли клиента — нарушение DRY и точка ошибок.
- **API Gateway — не место для бизнес-логики агрегации**: его задача — routing, rate limiting, JWT-проверка (см. § 4.1), не композиция доменов.

**Причина отклонения:** ломает уже принятую архитектуру.

## Consequences

### Positive

- **Один экран — один запрос.** Frontend получает ровно те поля, которые нужны для экрана; latency падает с водопада из 3–5 REST-вызовов до одного round-trip BFF.
- **Schema-first контракт.** `.graphql`-файлы в монорепо — единый источник правды; codegen для TS (фронт) и Go (gqlgen) запускается в Nx-таргете (см. ADR-0004), любая правка схемы видна обеим сторонам в одном PR.
- **Эволюция без `/v2/`.** Добавление поля — non-breaking by design. Удаление — через `@deprecated` + grace period, что естественно ложится на квартальные on-prem-релизы (см. ADR-0009).
- **Field-level authorization** становится явной: каждый резолвер сам отвечает на «можно ли этой роли это поле». Аудит «обращения к ПДн» (`security-architecture.md` § 8.2) пишется ровно на тех полях, которые помечены `@pii: true` в директивах схемы.
- **DX для всех ролей в команде.** GraphQL Playground в dev/staging — банковский эксперт, продакт, QA могут сами посмотреть «что будет на экране», не разворачивая весь стек.
- **Independent BFFs.** `bff-onboarding` и `bff-admin` эволюционируют в своих темпах, у каждой команды — своя схема, никаких общих федеративных типов в MVP.
- **Готовность к Federation в будущем.** Single endpoint per BFF — корректный subgraph-кандидат; миграция к Apollo Router в Phase 5+ не требует переписывания резолверов.

### Negative

- **N+1-запросы — стандартная проблема GraphQL.** Mitigation: dataloaders в gqlgen (batching и per-request кеш) обязательны на каждом резолвере, отдающем коллекцию. CI-линтер `graphql-no-naive-resolvers` ловит резолверы без dataloader.
- **Запрос произвольной сложности → DoS-вектор.** Mitigation: query depth limit (max 8), complexity scoring (max 1000), persisted queries в production (whitelist хеш-схема — на фронте только id-запроса), per-tenant rate-limit в API Gateway (см. `security-architecture.md` § 4.3).
- **HTTP-кеш не работает «из коробки».** Все запросы — `POST /graphql`, ETag-кеш на стороне CDN неприменим. Mitigation: persisted queries позволяют использовать `GET /graphql?id=...&vars=...`, после чего стандартный CDN-кеш возможен. На MVP — просто in-memory нормализованный кеш Apollo Client.
- **Ошибки маскируются под `null`.** GraphQL-конвенция «частичные ответы с массивом ошибок» противоречит REST-привычке «4xx/5xx». Mitigation: договоримся что в production — `errors` пустой при success, а partial success возвращается как доменный union-тип (`Result = Success | DomainError`).
- **Schema breaking changes ловятся не статически.** Один может удалить поле, фронт сломается. Mitigation: CI-job `graphql-schema-diff` (см. Implementation Notes) в каждом PR на BFF; breaking changes требуют explicit override-тега.
- **Авторизация по полю — больше резолверов, больше тестов.** Mitigation: directive-based auth (`@authz(role: "compliance_officer")`) выносит логику в схему; интеграционный тест прогоняет матрицу `(role × field)`.

### Neutral

- **Логи длиннее.** Запрос-payload содержит запрос-документ (1–10 КБ) — логи BFF растут. Mitigation: pesisted-queries-id вместо полного документа в production-логах, полный документ — только в trace.
- **Команда выучит ещё одну технологию.** Кривая обучения GraphQL для бэк-разработчика — 1–2 недели до уверенного владения. На горизонте года — окупится.
- **Subscriptions откладываются.** Long-running статус заявки сейчас через polling, потом — Server-Sent Events или GraphQL subscriptions. Это операционное решение, не архитектурное.
- **Schema живёт в монорепо** рядом с BFF-кодом — стандартный путь для Nx-проекта (см. ADR-0004), не создаёт нового жанра дисциплины.

## Implementation Notes

### Структура каталогов

```
services/bff-onboarding/
├── cmd/server/main.go
├── schema/
│   ├── schema.graphql              # корневая схема
│   ├── application.graphql         # типы для Application
│   ├── document.graphql            # типы для Document
│   └── directives.graphql          # @authz, @pii, @deprecated
├── internal/
│   ├── resolvers/                  # gqlgen-резолверы
│   ├── dataloaders/                # batching для downstream-вызовов
│   ├── auth/                       # claim → role mapping
│   └── clients/                    # gRPC-клиенты к доменным сервисам
├── gqlgen.yml
└── project.json                    # Nx-таргеты

services/bff-admin/
└── ... (аналогично)

apps/web-onboarding/
├── src/
│   ├── graphql/
│   │   ├── operations/             # *.graphql — клиентские запросы
│   │   └── generated/              # codegen output
│   └── ...
└── codegen.ts

apps/web-admin/
└── ... (аналогично)
```

### Анатомия схемы (фрагмент `bff-admin`)

```graphql
directive @authz(role: String!) on FIELD_DEFINITION
directive @pii(level: PIILevel = MEDIUM) on FIELD_DEFINITION

enum PIILevel { LOW MEDIUM HIGH }

type Application {
  id: ID!
  status: ApplicationStatus!
  legalEntity: LegalEntity!
  applicant: Person! @pii(level: HIGH) @authz(role: "bank.operator")
  riskAssessment: RiskAssessment @authz(role: "bank.compliance_officer")
  documents: [Document!]!
  uboGraph: UBOGraph @authz(role: "bank.compliance_officer")
  auditTrail: [AuditEvent!]! @authz(role: "bank.compliance_officer")
}

type Query {
  application(id: ID!): Application
  applicationsQueue(filter: QueueFilter, page: PageInput): ApplicationsConnection!
}
```

`@pii` — маркер для `audit-sdk`: чтение поля автоматически пишется как `audit_event.action = "read.pii"` (см. `security-architecture.md` § 8.2).
`@authz` — checked в `gqlgen` middleware на основе claims из JWT.

### Codegen и Nx-таргеты (см. ADR-0004)

| Таргет | Что делает | Когда запускается |
|---|---|---|
| `graphql-server-codegen` | gqlgen генерирует Go-резолверы из `services/bff-*/schema/*.graphql` | На каждый PR с правкой схемы |
| `graphql-client-codegen` | GraphQL Code Generator → TS-типы для `apps/web-*` | Тот же PR |
| `graphql-schema-lint` | `eslint-plugin-graphql` + custom rules: no naked `String` для PII, обязательный `@authz` на полях с `@pii` | Каждый PR |
| `graphql-schema-diff` | Сравнение схемы с базовой веткой; breaking changes блокируют merge без override-тега `[graphql-breaking-ok]` в commit | Каждый PR |
| `graphql-query-validate` | Валидация всех `.graphql` в `apps/web-*/src/graphql/operations/` против актуальной серверной схемы | Каждый PR |
| `graphql-persisted-queries` | Сборка персистентного манифеста для production deploy | На pre-release |

### Лимиты безопасности

- **Query depth**: 8 уровней (`gqlgen` middleware).
- **Query complexity**: 1000 единиц (стандартная gqlgen-метрика, поля × коэффициент сложности).
- **Persisted queries в production**: `apollo-server` middleware принимает только pre-registered queries по hash; ad-hoc запросы возвращают `403 Forbidden`. В dev/staging — открыто.
- **Rate limit**: per-tenant в API Gateway (см. `security-architecture.md` § 4.3), 100 запросов/мин на `client.applicant`, 1000 на `bank.operator`.
- **Field-level audit**: каждый резолвер с `@pii` пишет в audit log, batched per-request.

### Подключение к downstream-сервисам

BFF — тонкий слой агрегации. Резолверы не содержат бизнес-логики, только:

1. JWT → claims → role-check (`@authz`).
2. dataloader-batched gRPC/REST вызовы к доменным сервисам.
3. Маппинг ответа в GraphQL-тип.
4. Запись audit-события через `audit-sdk` для PII-полей.

Это сохраняет принцип BFF как «UI-shape adapter, не бизнес-домен» (`technical-structure.md` § 4.1).

### Этапы внедрения

| Этап | Что | Когда |
|---|---|---|
| 1 | Базовая схема `bff-onboarding`, gqlgen, codegen Nx-таргеты | Phase 1 (Ф1, см. § 12) |
| 2 | Auth-directives, audit-интеграция, persisted queries в staging | Phase 1 |
| 3 | `bff-admin` со своей схемой | Phase 4 (Ф4 — расширение для оператора банка) |
| 4 | DataLoader-инфраструктура, complexity limits в production | Phase 4 |
| 5 | Migration от polling к SSE/subscriptions для статусов | Phase 7+ если будет потребность |

### Тестирование

- **Unit**: каждый резолвер тестируется с mock-клиентами downstream-сервисов.
- **Schema-test**: golden-snapshot файлы `*.graphql.snap` — изменения схемы видны в diff.
- **Integration**: testcontainers с Postgres + mock доменных сервисов; e2e-запросы через гошный gqlclient.
- **Authz-matrix**: матрица `(role × top-level query × field)` — обязательный тест на каждый новый `@authz`.

## References

- `docs/technical-structure.md` § 1 (multi-tenant, hybrid, API-first), § 2 (структура монорепо: `services/bff-onboarding`, `services/bff-admin`), § 4.1 (Edge Layer, BFF — отдельный для каждого приложения, GraphQL), § 4.2 (доменные сервисы — потребители BFF), § 9 (per-tenant конфигурация — UI-параметры читаются BFF из tenant-service).
- `docs/security-architecture.md` § 3.2 (RBAC и роли — `bank.operator`, `bank.compliance_officer`, `client.applicant`), § 4.3 (rate limiting в API Gateway), § 8.2 (логирование обращений к ПДн).
- ADR-0001 (Temporal): BFF читает workflow-состояние через temporal-go-sdk и доменные сервисы; запросы статуса заявки — query-резолвер на `Application.status`.
- ADR-0002 (Schema-per-tenant): BFF извлекает `tenant_id` из JWT и пробрасывает в downstream-вызовы; сам не ходит в БД.
- ADR-0004 (Nx): codegen-таргеты `graphql-*` встают в граф с явными `inputs`/`outputs`.
- ADR-0007 (Промпт-менеджмент): BFF не вызывает LLM напрямую — это делают агенты; BFF получает уже сформированные ответы.
- ADR-0010 (Биллинг и audit log): GraphQL-резолверы с `@pii` — источник `read.pii` audit-событий, которые консолидируются в `billing_events` для платформенной отчётности.
- [gqlgen](https://gqlgen.com/) — schema-first GraphQL для Go.
- [GraphQL Code Generator](https://the-guild.dev/graphql/codegen) — TS-типы для фронта.
- [Apollo Federation v2](https://www.apollographql.com/docs/federation/) — оставляем как путь будущего, но не используем в MVP.
- [Persisted Queries](https://www.apollographql.com/docs/apollo-server/performance/apq/) — стратегия защиты production-эндпоинтов.
- [GraphQL spec](https://spec.graphql.org/) — каноническая спецификация.
