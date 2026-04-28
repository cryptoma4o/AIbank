# ADR-0007: Промпт-менеджмент: Git+filesystem с явным версионированием

**Status:** Accepted  
**Date:** 2026-04-26  
**Authors:** Главный архитектор, ML-тех-лид  
**Reviewers:** Тех-лид backend, Security-инженер, Комплаенс-офицер

## Context

ИИ-агенты платформы (`agent-document-intake`, `agent-reconciliation`, `agent-ubo-tracing`, `agent-conversational`, `agent-risk-scoring`-explainer и `rag-service`) используют LLM через `llm-gateway`. Промпт здесь — это не вспомогательная строка, а ключевой артефакт, определяющий поведение системы:

- От формулировки промпта зависит, **какие поля паспорта будут извлечены** (`agent-document-intake`).
- От формулировки промпта зависит, **какие расхождения с ЕГРЮЛ будут отмечены как существенные** (`agent-reconciliation`).
- От формулировки промпта зависит, **какие вопросы клиенту сформулирует чат-агент** в спорной ситуации (`agent-conversational`).

Каждое из этих решений в конечном счёте влияет на регуляторно-чувствительные действия: одобрение/отказ/доработку заявки на открытие счёта. Это означает, что промпт по сути является **версионируемым артефактом конфигурации, имеющим тот же вес, что код или миграция БД**.

Силы, давящие на это решение:

- **Регуляторика и audit log.** Согласно `docs/security-architecture.md` § 8.2, каждый вызов LLM логируется на 5 лет. Для каждой записи в audit log должно быть однозначно зафиксировано, **какая именно версия промпта** породила ответ — иначе через 2 года при ИБ-аудите банка или запросе РКН по 152-ФЗ невозможно реконструировать, какое именно правило вызвало решение. Это прямое требование `docs/security-architecture.md` § 7.3 («Версионирование промптов — каждый промпт в Git, версия в логе вызова») и `technical-structure.md` § 7.3 (гардрейл «Версионирование промптов»).
- **Hybrid deployment.** Платформа работает в SaaS (10–30 тенантов на одном кластере) и в on-prem-контуре банка (см. `technical-structure.md` § 1, принцип «Hybrid deployment»). On-prem-инсталляция часто **air-gapped** (см. § 10.2 — air-gapped bundle). SaaS-only сервисы для промптов (Langfuse Cloud, PromptLayer, Helicone Cloud) исключены — их нельзя развернуть в контуре банка без сетевых вызовов наружу.
- **A/B-тестирование.** В Phase 3 (см. § 12, Ф3) предусмотрено сравнение моделей и промптов через `eval-harness` и shadow-mode на production-трафике. Менеджмент промптов должен это поддерживать без переписывания кода агентов.
- **Per-tenant overlays.** Конфигурация тенанта в `configs/tenants/<bank>/ai/prompts.yaml` (см. § 9) содержит «кастомные промпты поверх базовых». Один банк хочет более формальный тон в чате, другой — упоминание собственных продуктов в ответах. Это значит, что промпт-менеджер обязан поддерживать **слоистую модель** (base + tenant override) и резолвить её детерминированно.
- **Контракт ревью.** Изменение промпта должно проходить тот же gate, что и изменение кода: PR с code review, security-проверкой (нет ли prompt-injection-уязвимостей в шаблоне), eval-регрессией. Промпт нельзя менять «по живому» в admin-UI без следа.
- **Стоимость владения.** Команда — 18–25 человек на год 1 (см. § 12). Развёртывание и сопровождение отдельного сервиса промпт-менеджмента (БД, UI, миграции, бэкапы, мониторинг) требует ресурсов, которые лучше потратить на eval-harness и доменных агентов.
- **Текущая закладка в `llm-gateway`.** README сервиса (`ai/llm-gateway/README.md`) явно фиксирует «Prompt versioning. Будет в ADR-0007». Конфиг роутинга (`configs/default-routing.yaml`) уже резолвит роли, не модели — естественное место присоединить и резолв промпта по роли.

Если ничего не решить, разные агенты примут разные подходы (один захардкодит промпт в коде, другой будет тянуть из Redis, третий — из переменной окружения), и через 6 месяцев восстановить «какой промпт работал в продакшене 14 марта» будет невозможно. Это P0-блокер для регуляторного аудита.

## Decision

Используем **Git+filesystem с YAML-промптами** как основной механизм хранения, версионирования и доставки промптов. Каждый вызов LLM сопровождается явным `prompt_version` в audit log. Внешний admin-UI **не** строим в Phase 1; рассматриваем как опцию Phase 2+ при появлении конкретного запроса от банка-клиента.

### Структура

Промпты живут в монорепо (см. ADR-0004 про Nx и `technical-structure.md` § 2):

```
ai/prompts/
├── base/                                # Базовые промпты, общие для всех тенантов
│   ├── document-intake/
│   │   ├── extract-passport.v3.yaml
│   │   ├── extract-charter.v2.yaml
│   │   └── extract-protocol.v1.yaml
│   ├── reconciliation/
│   │   └── compare-with-egrul.v4.yaml
│   ├── ubo-tracing/
│   │   └── trace-ownership.v2.yaml
│   ├── conversational/
│   │   ├── greeting.v1.yaml
│   │   └── faq-answer.v3.yaml
│   └── rag/
│       └── answer-with-citations.v2.yaml
├── shared/                              # Шаблоны и фрагменты
│   ├── safety-disclaimer.j2
│   └── russian-formal-tone.j2
└── schemas/
    └── prompt-record.schema.json        # JSON Schema валидации промптов

configs/tenants/<bank>/ai/prompts.yaml   # Per-tenant overlay (см. ниже)
```

### Анатомия записи промпта

```yaml
# ai/prompts/base/document-intake/extract-passport.v3.yaml
id: document-intake.extract-passport
version: 3
status: production         # draft | candidate | production | deprecated
created_at: 2026-03-12
authors:
  - ml-engineer-1
  - banking-expert-2
related_eval:
  dataset: docs-parsing
  baseline_f1: 0.91
template: |
  Ты помощник, извлекающий поля из скана паспорта РФ.
  Верни строго валидный JSON со схемой {{ schema_json }}.
  Документ: {{ document_text }}
parameters:
  temperature: 0.1
  max_tokens: 2048
  response_format: json_object
inputs:                    # Контракт переменных шаблона
  - name: document_text
    type: string
    required: true
  - name: schema_json
    type: string
    required: true
fingerprint: sha256:b41a...    # Считается автоматически по template+parameters
```

- `id` — стабильный идентификатор (не меняется между версиями).
- `version` — целочисленный, монотонно растущий. **Семантический смысл фиксированной версии — иммутабельный**: после merge в `main` файл `extract-passport.v3.yaml` не редактируется. Любая правка — это новый файл `v4`.
- `fingerprint` — SHA-256 от нормализованного `template + parameters`. Считается CI-хуком при создании PR. Несовпадение между файлом и fingerprint при загрузке = ошибка старта `llm-gateway`.
- `status` отражает жизненный цикл: `draft` → `candidate` (eval-harness shadow runs) → `production` → со временем `deprecated`. Только один `production`-промпт на пару `(id, tenant)` в любой момент.

### Резолв промпта в `llm-gateway`

Агент обращается не к конкретному промпту, а к **prompt-роли**:

```http
POST /v1/chat/completions
X-Tenant-Id: bank-alpha
X-Agent-Role: document-intake
X-Prompt-Ref: document-intake.extract-passport
```

`llm-gateway` резолвит запрос в три шага:

1. Тенант-overlay: ищется `configs/tenants/bank-alpha/ai/prompts.yaml` запись для `document-intake.extract-passport`. Если есть — используется указанная версия (например, `v3-banka` — банк форкнул базовый промпт под свой стиль).
2. Базовый промпт: если overlay отсутствует — выбирается `production`-версия из `ai/prompts/base/document-intake/extract-passport.v*.yaml`.
3. Загрузка и рендер: Jinja2 с строгим режимом (`StrictUndefined`) рендерит шаблон, gateway отправляет результат в выбранную модель (см. ADR-0011).

Резолюция полностью **детерминирована**: один и тот же `(tenant, prompt_id, deploy_sha)` всегда даёт один и тот же `(prompt_id, version, fingerprint)`. Это необходимо для воспроизводимости eval-харнес-прогонов и аудита.

### Запись в audit log

Каждый вызов LLM пишет в `audit_events` (см. `technical-structure.md` § 8.3, `security-architecture.md` § 8.2):

```json
{
  "action": "llm.invocation",
  "subject": {"type": "application", "id": "..."},
  "data": {
    "agent": "document-intake",
    "prompt_id": "document-intake.extract-passport",
    "prompt_version": 3,
    "prompt_fingerprint": "sha256:b41a...",
    "model_role": "document-vision",
    "resolved_model": "gemma-4-26b",
    "tenant_overlay_applied": false,
    "deploy_sha": "abc123...",
    "tokens_in": 412,
    "tokens_out": 158,
    "cost_kop": 174
  }
}
```

Связка `(prompt_id, prompt_version, prompt_fingerprint, deploy_sha)` достаточна для полной реконструкции состояния системы в момент вызова через `git checkout deploy_sha` + чтение файла промпта. `fingerprint` дополнительно защищает от ситуации «файл подменили, не двигая версию».

### A/B и shadow-режимы

`eval-harness` (см. `technical-structure.md` § 7.5) поддерживает прогон одного датасета через несколько `prompt_version` параллельно — это просто параметризация runner-а:

```bash
nx run eval-harness:run --dataset=docs-parsing \
  --candidates="document-intake.extract-passport@v3,document-intake.extract-passport@v4-draft"
```

В production shadow-режим (см. § 7.5, «Shadow mode для новых моделей») распространяется и на промпты: новый `candidate`-промпт получает копию запроса, его ответ только логируется, не возвращается клиенту. Через неделю shadow-данных принимается решение о промоушене в `production`. Промоушен — это PR со сменой `status: candidate → production` и понижением старого до `deprecated`.

### Per-tenant overlay

```yaml
# configs/tenants/bank-alpha/ai/prompts.yaml
overrides:
  conversational.greeting:
    version_override: 1-bank-alpha    # ссылка на ai/prompts/base/conversational/greeting.v1-bank-alpha.yaml
  faq-answer:
    version_override: 3
    parameter_overrides:
      temperature: 0.0                # банк хочет максимально детерминированный тон
```

Параметрические оверрайды разрешены только из явного allow-list (`temperature`, `top_p`, `max_tokens`). Структурный оверрайд (другой `template`) делается через отдельный файл с суффиксом `-<tenant_id>` в `ai/prompts/base/<role>/`. Это сохраняет всё в Git — overlay не означает «прячем код тенанта в `configs/`».

### Что НЕ покрывает это решение

- **Промпты внутри SaaS-сервисов нашей платформы (например, в admin-UI)**, не отправляемые в LLM, остаются обычными UI-строками в i18n-файлах. Это не промпты в смысле этого ADR.
- **System-промпты модели**, прошитые в чекпойнт во время fine-tuning, выходят за рамки ADR (это часть Model Registry, см. ADR-0011).
- **Few-shot examples**, динамически подбираемые из RAG по семантической близости, — отдельный механизм (см. ADR-0008 про vector DB), но статические examples внутри template — это часть промпта и подпадают под этот ADR.

## Alternatives Considered

### Альтернатива 1: In-code constants (промпты как Go/Python-строки в коде сервисов)

**Плюсы:**
- Нулевая инфраструктурная сложность: текст промпта живёт в `agent_document_intake/prompts.py` рядом с кодом, который его использует.
- Type-checking при наличии (Python `Literal`, Go `const`).
- Atomic deploy кода и промпта: PR с правкой промпта это PR с правкой кода.
- Никаких runtime-зависимостей на YAML-парсер, файловую систему, кеш промптов.

**Минусы:**
- **A/B-тестирование требует merge в `main` и redeploy** — для нашего trunk-based потока это значит, что shadow-режим промпта = развёрнутый сервис с двумя if-ветками в коде. Через 3 месяца имеем спагетти из feature-флагов на промпты.
- **Per-tenant overlay требует кода**: либо матрица `if tenant == "bank-alpha"`, либо отдельный пакет на банк. Оба варианта не масштабируются на 10+ банков.
- **Промпт смешивается с логикой**: при code review backend-разработчик ревьюит и Python-код и текст промпта на русском языке. Банковский эксперт не имеет доступа к коду — то есть владелец промпта не владеет файлом, в котором тот лежит.
- **Audit-trail по промптам теряется в общей истории кода**. Запрос «покажи все версии промпта `extract-passport` за год» превращается в `git log -p -- agent_document_intake/prompts.py | grep ...`.

**Причина отклонения:** A/B-тесты и per-tenant overlay — обязательные требования (см. Context, eval-harness и `configs/tenants/*/ai/prompts.yaml`). In-code constants ломают оба сценария.

### Альтернатива 2: Внешний managed prompt service (Langfuse Cloud, PromptLayer, Helicone, Humanloop)

**Плюсы:**
- Готовые admin-UI с историей, A/B-режимом, метриками, comments.
- Roles & permissions для нетехнических пользователей (банковских экспертов, продактов).
- Интеграция с трейсингом LLM-вызовов «из коробки».

**Минусы:**
- **Несовместимо с hybrid/on-prem.** Banки в РФ не разрешают outbound-трафик к зарубежным SaaS из периметра (см. `security-architecture.md` § 4.2 — outbound только по white-list). Langfuse Cloud, PromptLayer, Helicone — все хостятся в США/ЕС. Self-hosted Langfuse возможен, но это уже Альтернатива 3.
- **Локализация и юрисдикция данных.** Промпт может содержать примеры (few-shot) с реальными данными — это потенциально ПДн (152-ФЗ требует локализации). Хранение их в зарубежном SaaS — нарушение.
- **Vendor lock-in на критичный артефакт**. Если сервис закрылся / поменял ценник / потерял доступность — мы не можем выкатить новую версию агента.
- **Дополнительный SLA на 5 лет**. Аудиторская трассируемость требует, чтобы версия промпта от 2026-04 была доступна в 2031. Внешний сервис может перестать существовать.

**Причина отклонения:** прямо нарушает принцип hybrid deployment и требование air-gapped on-prem.

### Альтернатива 3: Self-hosted prompt service (Langfuse self-hosted, собственный сервис на PostgreSQL + admin-UI)

**Плюсы:**
- Admin-UI для нетехнических пользователей.
- A/B-метрики и трассировка вызовов в одном UI.
- Можно развернуть в on-prem.
- При выборе Langfuse — open-source MIT, есть community.

**Минусы:**
- **Дополнительный сервис на эксплуатацию.** PostgreSQL-схема, миграции (см. ADR-0005), HA-режим, мониторинг, бэкапы, security-обновления. Для команды из 18–25 человек это заметная нагрузка ради функциональности, которую в Phase 1 обеспечит просто Git.
- **Дублирование Source of Truth.** Если промпты живут в БД admin-сервиса, то Git перестаёт быть источником правды → теряется PR-flow, code review, blame-trail. Если живут и там и там — нужен механизм синхронизации, который сам по себе становится источником багов.
- **Air-gapped on-prem усложняет жизнь.** Каждое обновление промпта требует развёртывания admin-сервиса в банке + обучения банковского эксперта. Для квартального on-prem-цикла (см. ADR-0005, `technical-structure.md` § 11) это перебор.
- **Регуляторика «промпты в Git» в `security-architecture.md` § 7.3** прямо постулирует Git. Это означает, что даже при наличии БД-сервиса нам всё равно нужна синхронизация в Git для аудита.

**Причина отклонения:** дополнительная инфраструктура без явной выгоды по сравнению с Git+filesystem на Phase 1. Возвращаемся к рассмотрению в Phase 2+ при появлении конкретных пользовательских сценариев (банковский эксперт хочет править промпты без PR — что само по себе спорно с точки зрения SDLC).

### Альтернатива 4: Database-backed (промпты в PostgreSQL, без отдельного сервиса)

**Плюсы:**
- Использует уже существующую PostgreSQL-инфраструктуру (см. ADR-0002).
- Легко добавить admin-UI как часть `web-admin` приложения.
- Удобный поиск/фильтрация через SQL.

**Минусы:**
- **Те же проблемы дублирования с Git**, что у Альтернативы 3. Либо БД — единственный источник правды (нет PR-flow), либо БД синхронизируется с Git (overhead без явной выгоды).
- **`fingerprint` и иммутабельность** в БД достижимы только через жёсткие триггеры/правила; в Git это естественное свойство.
- **Per-tenant overlay в БД требует таблиц с кросс-тенантными записями**, что плохо ложится на schema-per-tenant из ADR-0002.
- **Развёртывание промптов рассинхронизируется с deploy-ом сервисов**: если промпт в БД обновился, а сервис ещё не задеплоен с новой схемой ввода — рассинхрон между ожидаемыми переменными и реальным шаблоном.

**Причина отклонения:** Git естественно решает все задачи (immutability, audit, branching, blame, review) лучше БД. БД оптимален для часто меняющихся данных, а промпт — это не «данные», а «версионируемая конфигурация уровня кода».

### Альтернатива 5: GitOps через ArgoCD (промпты как Kubernetes ConfigMaps)

**Плюсы:**
- Полностью соответствует уже выбранному GitOps-подходу для конфигов тенантов (см. `technical-structure.md` § 9).
- Автоматическая выкатка изменений промптов в кластер.

**Минусы:**
- ConfigMap-лимит 1 МБ на ресурс — теоретически достаточно, но при большом числе промптов с few-shot examples приближается к границе.
- Наблюдаемость хуже: чтобы посмотреть текущий промпт в кластере, нужно `kubectl get configmap`, а не `cat ai/prompts/...yaml`.
- ArgoCD-sync задерживает выкатку относительно prompt-резолюции в `llm-gateway` (ConfigMap mounted as volume → пара секунд reload).

**Причина отклонения:** не альтернатива, а **возможный механизм доставки**. В рамках принятого решения промпты по факту доедут до пода через образ или ConfigMap — оба варианта совместимы с Git-as-source-of-truth. Конкретный механизм доставки решается отдельно в Helm-чарте `llm-gateway` и не требует ADR.

## Consequences

### Positive

- **Audit log → Git → версия промпта → восстановление контекста за 5 лет.** Стандартная процедура регуляторного аудита: по `prompt_version + deploy_sha` из audit-записи восстанавливается точный текст промпта через `git show`. Никаких дополнительных систем.
- **PR-flow на промпты идентичен PR-flow на код.** Code review (включая security-проверку на prompt injection в шаблоне), eval-харнес-регрессия в CI, ответственный мерджер — всё уже есть.
- **Atomic deploy кода и промптов** при коммите в одной ветке: меняем сигнатуру `inputs` промпта и логику агента в одном PR.
- **Air-gapped on-prem работает из коробки.** Промпты бандлятся в Docker-образ `llm-gateway` (или в `ai/prompts/`-volume Helm-чарта), никаких сетевых вызовов наружу.
- **A/B и shadow-режим — изменение конфига роутинга, не кода.** Достаточно добавить новый `candidate`-файл с `v4` и прописать его в eval-harness.
- **Per-tenant overlay естественно ложится на `configs/tenants/<bank>/ai/prompts.yaml`** (см. `technical-structure.md` § 9), который уже выбран как способ кастомизации.
- **Никакого нового сервиса.** Команда из 18–25 человек экономит ~1–2 месяца на не-разработке промпт-сервиса; эти ресурсы идут в eval-harness и доменных агентов.

### Negative

- **Нет ergonomic-UI для банковского эксперта.** Чтобы предложить улучшение промпта, эксперту нужен PR — то есть базовое знание Git/GitLab. Mitigation: оборачиваем процесс в простой markdown-runbook («как предложить правку промпта»), большие банки обычно имеют выделенного человека в команде с этим навыком.
- **A/B-метрики не приходят бесплатно.** Для сравнения двух версий нужна ручная работа в `eval-harness` или ручной анализ audit log. В managed-сервисах такие дашборды уже есть. Mitigation: Grafana-дашборд поверх audit log с разрезом по `prompt_version` (часть § 7.5 и Phase 3).
- **Резолюция промпта добавляет latency на первый запрос** после рестарта `llm-gateway`. Mitigation: промпты загружаются и парсятся при старте сервиса, кешируются в памяти; reload через signal `SIGHUP` или ConfigMap update.
- **`fingerprint`-чек на старте может блокировать deploy** при человеческой ошибке (правка файла без bump-а версии). Mitigation: pre-commit hook + CI-job `prompt-lint` ловят это до merge.
- **Promotion `candidate → production` требует дисциплины.** Если забыть понизить старую `production`-версию до `deprecated`, имеем две одновременно. Mitigation: CI-валидатор ловит «более чем одну production версию на (id, tenant)» и блокирует merge.

### Neutral

- **Размер репо растёт.** Промпты с few-shot могут быть 5–50 КБ на файл; с историей версий за 2 года — несколько МБ. На фоне `node_modules` и `go.sum` это незаметно.
- **Перевод промптов между языками** (если когда-нибудь понадобится английский UI) — отдельная задача, не покрытая ADR. Сейчас все промпты на русском.
- **Промпты — потенциальный канал prompt-injection.** Это покрывается security-review в PR-процессе (как и любой другой код). Не уникальная проблема нашего решения.
- **Размер CHANGELOG.** История промптов превратится в значимую часть истории репозитория — это плюс для audit, нейтрально для DX.

## Implementation Notes

### Этапы внедрения

| Этап | Что | Когда |
|---|---|---|
| 1 | Структура `ai/prompts/`, JSON Schema записи, `prompt-lint` CI-job | Phase 0 (до Ф1, см. § 12) |
| 2 | Резолвер промптов в `llm-gateway`, чтение `prompt_id`/`prompt_version` через заголовки, рендер Jinja2 | Phase 3 параллельно с подключением реальных моделей |
| 3 | Запись `prompt_version`/`fingerprint` в audit log через `audit-sdk` | Phase 3 |
| 4 | Per-tenant overlay из `configs/tenants/<bank>/ai/prompts.yaml` | Phase 3 |
| 5 | A/B-шину поверх audit log + Grafana-дашборды | Phase 3+, после первой production-модели |
| 6 | Опциональный admin-UI как часть `web-admin` (только если будет конкретный customer-запрос) | Phase 2+ от старта релизов |

### CI-проверки в Nx-пайплайне (см. ADR-0004)

- `prompt-lint`: парсит YAML, валидирует по `prompt-record.schema.json`, проверяет монотонность версий, уникальность `production`-статуса, корректность `fingerprint`.
- `prompt-eval-fast`: для PR с правкой промпта запускает соответствующий fast-subset eval-harness (см. § 7.5, «На каждый PR в `ai/`»). Регрессия >5% блокирует merge.
- `secrets-in-prompts`: ищет паттерны API-ключей, токенов, реальных ИНН/паспортов в шаблонах. Few-shot examples должны быть синтетическими.

### Жизненный цикл промпта

```
draft (PR создан)
   ↓ после code review + eval-fast pass
candidate (merge в main, статус 'candidate', shadow-mode в проде)
   ↓ через >=7 дней shadow + полная еженедельная регрессия
production (PR со сменой статуса)
   ↓ замещение более новой версией
deprecated (хранится бессрочно, перестаёт использоваться)
```

`deprecated` промпты **не удаляются** — это требование 5-летнего хранения.

### Договорённости code review

- Промпт-владелец = banking expert или ML-engineer (не backend).
- Минимум один reviewer — security или комплаенс — для промптов с output-сценариями, влияющими на решения (decline/approve/risk-score).
- Изменение `inputs`-контракта = breaking change, требует bump major-версии в коде агента, использующего промпт. Lint ловит расхождение по `inputs`.

### Безопасность

- `template`-рендеринг в **strict mode** (`Jinja2 StrictUndefined`): любая неподставленная переменная = ошибка, не пустая строка.
- `inputs`, помеченные как PII (по таблице из `security-architecture.md` § 5.2), на этапе рендера должны проходить через PII-маскирование (см. § 7.3 гардрейл и `security-architecture.md` § 5.4) **до** подстановки в шаблон. Тест на это — обязательная часть unit-тестов агента.
- В audit log пишется `prompt_fingerprint`, но **не сам текст промпта** (он живёт в Git и реконструируется по `deploy_sha`). Это сокращает объём audit DB (см. ADR-0005 про partitioning) и одновременно соответствует принципу «логи не содержат секретов» (`security-architecture.md` § 8.3).

## References

- `docs/technical-structure.md` § 7.2 (стратегия моделей и роли), § 7.3 (гардрейлы, версионирование промптов), § 7.5 (eval-harness), § 9 (per-tenant конфигурация).
- `docs/security-architecture.md` § 7.3 (промпты в Git как требование), § 8.2 (что обязательно логируется), § 5.4 (PII-фильтр перед LLM).
- ADR-0001 (Temporal): Activity, инициирующая LLM-вызов, передаёт `prompt_id`/`prompt_version` в context для трассировки.
- ADR-0002 (Schema-per-tenant): per-tenant overlay не использует БД-таблиц, поэтому schema-per-tenant сохраняется без изменений.
- ADR-0004 (Nx): `prompt-lint`, `prompt-eval-fast`, `secrets-in-prompts` оформляются как Nx-таргеты с явными `inputs`/`outputs` для корректной инвалидации кеша.
- ADR-0005 (Стратегия миграций БД): миграция аудит-таблиц включает поля `prompt_version` и `prompt_fingerprint`. Изменение схемы аудита проходит через тот же migration flow.
- ADR-0008 (Vector DB Qdrant vs pgvector): динамический подбор few-shot из vector store — отдельный механизм, ссылающийся на промпты этого ADR.
- ADR-0011 (LLM routing): резолвер `llm-gateway` объединяет prompt-резолюцию (этот ADR) и model-резолюцию (ADR-0011) в одной транзакции.
- `ai/llm-gateway/README.md` — секция «Что осознанно НЕ реализовано в MVP», пункт «Prompt versioning. Будет в ADR-0007».
- [Jinja2 StrictUndefined](https://jinja.palletsprojects.com/en/3.1.x/api/#jinja2.StrictUndefined) — режим, в котором рендерим шаблоны.
- [MADR 3.0 template](https://adr.github.io/madr/) — общий формат ADR.
