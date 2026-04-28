# bff-admin

Backend-for-Frontend для внутренней админки банка (`apps/web-admin`).
Аудитория — `bank.admin`, `bank.compliance_officer`, `platform.admin`.
Контракт — GraphQL (см. [`graph/schema.graphqls`](graph/schema.graphqls)),
аутентификация — JWT (общий секрет с `identity-service`).

## Endpoints

| Метод | Путь | Описание |
|-------|------|----------|
| `POST` | `/graphql` | Основной GraphQL endpoint (требует admin-роль) |
| `GET`  | `/playground` | GraphiQL UI (только при `GRAPHQL_PLAYGROUND=1`) |
| `GET`  | `/health` | Liveness |
| `GET`  | `/ready`  | Readiness |

## ENV

| Переменная | Назначение |
|------------|------------|
| `JWT_SECRET` | HMAC-секрет (≥32 байт), общий с `identity-service` |
| `TENANT_SERVICE_URL` | tenant-service base URL |
| `AUDIT_SERVICE_URL` | audit-service base URL |
| `ORCHESTRATOR_URL` | onboarding-orchestrator base URL |
| `RISK_ENGINE_URL` | risk-engine base URL |
| `IDENTITY_SERVICE_URL` | identity-service base URL |
| `GRAPHQL_PLAYGROUND` | `1` чтобы включить /playground (dev only) |
| `PORT` | по умолчанию `8092` |

## Compliance Dashboard

Query `complianceDashboard(tenantId, from, to): ComplianceDashboard!`
агрегирует ключевые KPI комплаенс-офицера за период `[from, to]`.
Источник KPI — [`docs/operations/monitoring-alerts.md`](../../docs/operations/monitoring-alerts.md)
(Business KPIs) и [`docs/product-vision.md` §6](../../docs/product-vision.md).

### Поля

| Поле | Источник | Назначение |
|------|----------|------------|
| `applicationsTotal` | orchestrator | Всего заявок за период |
| `applicationsByState` | orchestrator | Map `state→count` (по всем 15 состояниям) |
| `decisionsAutoApproved` | orchestrator | Заявки в state=`auto_approved` |
| `decisionsManualReview` | orchestrator | Заявки в state=`manual_review` |
| `decisionsDeclined` | orchestrator | Заявки в state=`declined` |
| `decisionsWithEDD` | orchestrator | Заявки в state=`approved_with_edd` |
| `automationRate` | derived | `auto_approved / applicationsTotal` (0..1) |
| `averageTimeToDecisionSec` | derived | Среднее `updated_at - created_at` для terminal states |
| `uboScreeningsTotal` | ubo-graph | Всего UBO-проверок за период |
| `screeningsWithMatch` | ubo-graph | UBO-проверок с попаданием в санкционные/PEP-листы |
| `auditEventsToday` | audit-service | Sanity check: ≥1 → audit chain жива |
| `forgottenApplicantsCount` | identity-service (TODO) | Метрика 152-ФЗ §14: удалённые субъекты |

### Авторизация

- Требует роль `bank.compliance_officer`, `bank.admin` или `platform.admin`.
- `bank.operator` → `403 forbidden`.
- Cross-tenant (auth.tenant ≠ argument `tenantId`) запрещён, кроме
  `platform.admin` — он может запросить дашборд любого тенанта (поддержка платформы).

### Стратегия агрегации

Резолвер делает 3 параллельных upstream-вызова через `sync.WaitGroup`:

1. orchestrator — `GET /v1/applications?tenant_id=X&created_from&created_to`
   (date-фильтр сейчас применяется client-side; TODO upstream — добавить
   server-side фильтр для тенантов с большим объёмом).
2. ubo-graph — `GET /v1/ubo-graphs?tenant_id=X` (soft endpoint: 404 → 0).
3. audit-service — `GET /v1/audit-events?tenant_id=X` (count фильтруется
   по `occurred_at` за сегодняшний день UTC).

Частичные сбои не валят весь дашборд: фейл одного коллектора логируется
(`slog.Warn`) и соответствующее поле остаётся zero-значением — клиенту
видно, что один источник просел, но остальные KPI отдаются.

Вся математика (`automation_rate`, `averageTimeToDecisionSec`) считается
в резолвере по сырым данным, а не делегируется upstream'у.  Это даёт:
- единый источник истины (не зависит от агрегаций upstream);
- возможность считать derived KPI как чистые функции (легко тестируется,
  см. `aggregate()` в `graph/dashboard.go`).

### Ожидаемое использование

- **Daily 09:00 МСК** — экспорт в BI банка (cron в `apps/web-admin` или
  отдельный scheduled-report worker; TODO).
- **Weekly digest** — агрегация за неделю для compliance committee.
- **Real-time дашборд** — opt-in polling каждые 60с (см. `apps/web-admin/dashboard`).

### TODO

- [ ] `forgotten_applicants_count` — добавить `GET /v1/me/forgotten-count`
      в `identity-service` и подключить здесь (152-ФЗ §14 reporting).
- [ ] CSV-экспорт `complianceDashboardCSV(...)` mutation для прямой
      выгрузки без BI.
- [ ] Daily cron (`scheduled-reports` worker) → emails compliance officer'у
      и в S3 для архива.
- [ ] Server-side date filter в orchestrator (избегать full-list fetch).
- [ ] HEAD `/v1/audit-events` с `X-Total-Count` — точный count без
      выгрузки списка.

## Build / Test

```bash
go mod tidy
go build ./...
go test ./...
```

## См. также

- [ADR-0003 BFF-pattern](../../docs/adr/) — почему два BFF (`bff-admin`, `bff-onboarding`)
- [`docs/security-architecture.md` §3.2](../../docs/security-architecture.md) — RBAC
- [`docs/operations/monitoring-alerts.md`](../../docs/operations/monitoring-alerts.md) — Business KPIs
