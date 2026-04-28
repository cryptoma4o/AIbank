# Engineer Onboarding

## Когда использовать

- Первый день нового инженера (backend, ML, DevOps, frontend)
- Возвращение инженера после долгого перерыва (смена тимы)
- Внешний контрактор получает временный доступ к проекту
- Ревью «всё ли есть у нового сотрудника» через неделю после старта

## Цель документа

Дать инженеру за **1 рабочий день** возможность:
1. Поднять локальный стек (`make up && make smoke`)
2. Прочитать ключевые ADR и architecture-документы
3. Знать, кто owner какой части
4. Иметь все необходимые доступы

## День 1: чтение

### Обязательно (3–4 часа)

| # | Документ | Цель |
|---|---|---|
| 1 | [`README.md`](../../README.md) | Проект и быстрый старт |
| 2 | [`AGENTS.md`](../../AGENTS.md) | Соглашения по работе с кодом |
| 3 | [`docs/product-vision.md`](../product-vision.md) | Что мы строим и зачем |
| 4 | [`docs/technical-structure.md`](../technical-structure.md) | Архитектура и стек |
| 5 | [`docs/domain-model.md`](../domain-model.md) | Доменная модель (Application, LegalEntity, ...) |
| 6 | [`docs/security-architecture.md`](../security-architecture.md) | Что мы защищаем и как |
| 7 | [`docs/adr/README.md`](../adr/README.md) | Список ADR — прочитать минимум 0001, 0002, 0005, 0009, 0010, 0011 |

### По специализации (1–2 часа)

| Роль | Дополнительно |
|---|---|
| Backend (Go) | `services/<любой>/README.md`, `packages/audit-sdk/`, `tools/db-migrator/README.md` |
| Backend (ML) | `ai/llm-gateway/README.md`, `ai/eval-harness/README.md`, ADR-0007, ADR-0011, ADR-0012 |
| Frontend | `apps/web-onboarding/README.md`, `packages/ui-kit/`, ADR-0003 (GraphQL) |
| DevOps | `infrastructure/helm/README.md`, `infrastructure/terraform/README.md`, ADR-0009 |
| ABS-интеграции | `services/abs-connector/README.md`, ADR-0006, mock'и в `tools/mock-abs-*/` |
| Security | Security-architecture полностью, [`../runbooks/break-glass-procedure.md`](../runbooks/break-glass-procedure.md), ADR-0010 |

## День 1: локальный стек

```bash
# 1. Клон и зависимости
git clone <repo>
cd aibank
make deps                    # Go, Python, Node deps

# 2. Поднять стек
make up                      # docker-compose

# 3. Подождать (или Run и идти за кофе)
make wait-healthy            # ждёт healthcheck'ов

# 4. Применить миграции
make migrate-platform

# 5. Прогнать smoke
make smoke                   # = scripts/smoke.sh
# Должны пройти 9 шагов; любой fail — записан в /tmp/aibank-smoke-state/last-step

# 6. Открыть UI
open http://localhost:3000   # web-onboarding (клиент)
open http://localhost:3001   # web-admin (банк)
open http://localhost:8089   # Temporal UI
open http://localhost:9001   # MinIO console (minioadmin / minioadmin_dev)
```

Если что-то не работает:
- `docker compose logs <service>` — найти, какой сервис ругается
- `docker compose ps` — статус всех контейнеров
- См. таблицу портов ниже

### Локальные порты (docker-compose)

| Сервис | Порт | URL |
|---|---|---|
| postgres | 5432 | `postgres://aibank:aibank@localhost:5432/aibank` |
| redis | 6379 | — |
| qdrant | 6333 / 6334 | `http://localhost:6333` |
| kafka | 9092 | — |
| minio | 9000 / 9001 | `http://localhost:9001` console |
| temporal | 7233 | `tctl --address localhost:7233 ...` |
| temporal-ui | 8089 | `http://localhost:8089` |
| tenant-service | 8080 | `/health`, `/v1/tenants` |
| audit-service | 8081 | `/health`, `/v1/events` |
| identity-service | 8082 | `/health` |
| document-service | 8083 | `/health` |
| onboarding-orchestrator | 8085 | `/health` |
| risk-engine | 8086 | `/health` |
| billing-service | 8087 | `/health` |
| abs-connector | 8088 | `/health` |
| client-service | 8089 | `/health` |
| ext-egrul | 8090 | `/health` |
| ubo-service | 8090 | (collision — TBD) |
| ext-rosfinmon | 8091 | `/health` |
| bff-onboarding | 8091 | `/graphql` (collision — TBD) |
| ext-fssp | 8092 | `/health` |
| bff-admin | 8095 | `/graphql` |
| api-gateway | 8000 | gateway |
| llm-gateway | 8100 | `/healthz`, `/v1/chat/completions` |
| agent-document-intake | 8101 | `/healthz` |
| agent-reconciliation | 8102 | `/healthz` |
| agent-ubo-tracing | 8103 | `/healthz` |
| agent-conversational | 8104 | `/healthz` |
| notification-service | 8110 | (mapped from 8083) |
| abs-adapter-cft | 9001 | (collision с MinIO — TBD) |
| abs-adapter-diasoft | 9002 | — |
| abs-adapter-rs-bank | 9003 | — |
| web-onboarding | 3000 | UI |
| web-admin | 3001 | UI |

## Команды (cheat sheet)

```bash
# Стек
make up / down / restart / logs

# Тесты
make test                    # unit + integration
make test-e2e                # Playwright (только если стек запущен)

# Миграции
make migrate-platform        # platform schema
make migrate-tenant TENANT=demo SERVICE=tenant-service

# Lint / format
make lint                    # все языки
make fmt                     # форматировать
make security-scan           # SBOM + trivy

# Smoke
make smoke

# Cleanup
make clean                   # docker volumes, кеш
```

## Доступы (чеклист — зависит от роли)

- [ ] Git-доступ к репо `aibank` (read + write для своего домена)
- [ ] GitLab CI — read pipelines, run для своих сервисов
- [ ] Mattermost / Telegram — все нужные каналы:
  - [ ] `#aibank-engineering`
  - [ ] `#aibank-deploys` — авто-сообщения о релизах
  - [ ] `#oncall` (если в дежурстве)
  - [ ] `#sec-oncall` (security only)
  - [ ] `#aibank-ai` (если ML)
- [ ] Grafana / VictoriaMetrics (read для всех, edit dashboards для своей команды)
- [ ] Temporal UI staging
- [ ] Vault staging (read-only для secrets своего домена)
- [ ] Container registry (read для pull, write только для CI)
- [ ] Tempo / Loki (read)
- [ ] Yandex Cloud / VK Cloud — read для своего env (DevOps — write)
- [ ] On-call rotation (PagerDuty / Telegram-bot — для DevOps)
- [ ] MFA-токен настроен (HW-ключ выдают физически + бэкап TOTP)
- [ ] Корпоративный SSO работает

Production-доступ — отдельный процесс, не в день 1; через JIT и MFA, не «по умолчанию».

## Owners — кто ответственен за что

| Часть | Owner | Где найти |
|---|---|---|
| Backend Go (`services/`) | Tech-lead backend | `services/AGENTS.md` |
| ML / AI (`ai/`) | Tech-lead ML | `ai/AGENTS.md` |
| Frontend (`apps/`) | Frontend lead | `apps/AGENTS.md` |
| Adapters (`services/abs-adapter-*`) | ABS team lead | `services/AGENTS.md` (TBD) |
| Infrastructure (`infrastructure/`) | DevOps lead | `infrastructure/AGENTS.md` |
| Configs (`configs/tenants/*`) | CSM + DevOps | `configs/AGENTS.md` |
| Documentation (`docs/`) | Архитектор | `docs/AGENTS.md` |
| ADRs | Архитектор + reviewers per ADR | `docs/adr/AGENTS.md` |
| Security | Security-инженер | `docs/security-architecture.md` |
| Compliance / DPO | Юрист + DPO | `docs/compliance-map.md` |
| Onboarding-флоу | Tech-lead backend | `services/onboarding-orchestrator/` |
| Audit log | Security + DBA | `services/audit-service/`, ADR-0010 |
| Billing | Финансовый директор + Tech-lead | `services/billing-service/`, ADR-0010 |
| LLM Gateway | Tech-lead ML | `ai/llm-gateway/`, ADR-0011 |
| Eval-harness | ML lead + банковский эксперт | `ai/eval-harness/`, ADR-0012 |

## Первая неделя (predefined goals)

- [ ] День 1: чтение + локальный стек + smoke прошёл
- [ ] День 2–3: small starter task (заведена в трекер с тегом `good-first-issue`)
- [ ] День 4: pair programming с senior из команды
- [ ] День 5: MR → review → merge первого изменения
- [ ] День 7: 1:1 с тех-лидом, обсуждение «как первая неделя»
- [ ] День 14: подключение к on-call shadow (без полной ответственности)
- [ ] День 30: полная on-call ответственность (если применимо для роли)

## Прочитать «потом» (вторая неделя)

- [`docs/ai-agents-automation.md`](../ai-agents-automation.md) — детально про AI-агентов
- [`docs/compliance-map.md`](../compliance-map.md) — регуляторика
- [`docs/tenant-configuration.md`](../tenant-configuration.md) — конфигурация тенантов
- Все runbooks ([`../runbooks/README.md`](../runbooks/README.md)) — нужно знать, что есть и где
- Deployment docs ([`../deployment/README.md`](../deployment/README.md)) — как мы деплоимся
- Все остальные ADR (0003, 0004, 0006, 0007, 0008)

## Что НЕ делать в первую неделю

- Не commit'ить в `main` (всегда через PR)
- Не запрашивать production-доступ без активной задачи
- Не писать в `#sec-oncall` без активного инцидента (это ops-канал, не вопросы)
- Не использовать `git push --force` на shared ветках
- Не пытаться обойти security-rules «потому что мне срочно»

## Полезные ресурсы

- Temporal docs: https://docs.temporal.io/
- Istio: https://istio.io/
- Helm: https://helm.sh/
- vLLM: https://docs.vllm.ai/
- ADR template: [`docs/adr/0000-template.md`](../adr/0000-template.md)
- Conventional Commits: https://www.conventionalcommits.org/

## Связанные документы

- [`README.md`](../../README.md) — главный README
- [`AGENTS.md`](../../AGENTS.md) — соглашения
- [`../runbooks/README.md`](../runbooks/README.md) — runbook'и
- [`../deployment/README.md`](../deployment/README.md) — deployment
- [`backup-restore.md`](backup-restore.md), [`monitoring-alerts.md`](monitoring-alerts.md) — ops базис
