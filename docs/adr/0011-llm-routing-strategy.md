# ADR-0011: Стратегия выбора и роутинга LLM — multi-model role-based pool с per-tenant overrides

**Status:** Accepted  
**Date:** 2026-04-26  
**Authors:** Главный архитектор, ML-тех-лид  
**Reviewers:** Тех-лид backend, Security-инженер, Продакт

## Context

Платформа использует LLM в нескольких сервисах с принципиально разными требованиями к модели (см. `docs/technical-structure.md` § 7.2):

- **`agent-document-intake`** — multimodal-парсинг паспортов, уставов, протоколов. Нужны OCR-навыки, длинный контекст (256K для устава целиком), function calling для возврата структурированного JSON.
- **`agent-reconciliation`** — сверка извлечённых данных с ЕГРЮЛ. Reasoning-модель, не multimodal.
- **`agent-ubo-tracing`** — рекурсивный reasoning по графу владения. Thinking-mode, long context.
- **`agent-conversational`** — клиентский чат на русском. Естественный русский, банковская терминология, лёгкая модель (latency p99 < 2 c).
- **`rag-service`** — генерация ответов по нормативке с цитированием. Faithfulness важнее latency.
- **Embedder** для RAG и embedding-сэмплов (см. ADR-0008) — отдельная модель класса `bge-m3` или `E5-mistral`.

Силы, давящие на это решение:

- **Уже зафиксированный архитектурный принцип «пул моделей, а не одна».** `technical-structure.md` § 7.2 явно постулирует: «Принципиальное решение: не выбираем одну модель на платформу. Вместо этого строим пул моделей-кандидатов и роутим запросы по типу задачи. Финальный выбор для каждой роли определяется эмпирически по eval-harness, а не по теоретическим бенчмаркам». ADR-0011 кодифицирует это решение и его границы, но не пересматривает.
- **Разные классы задач требуют разных моделей по объективным причинам.** Multimodal-OCR-задача и текстовая reasoning-задача в принципе оптимально решаются разными моделями. Принуждение всех задач к одной модели = заведомо subobtimal качество либо неоправданно дорогая модель на простые задачи.
- **Per-tenant overrides — обязательное требование.** `technical-structure.md` § 7.2 («разные банки могут использовать разные модели — один Gemma, другой Qwen, третий GigaChat»), § 9 (`configs/tenants/<bank>/ai/models.yaml` — какие модели использовать). Один банк в Phase 7 пилотится self-hosted Gemma, второй — не готов разворачивать GPU и просит GigaChat API. Архитектура должна это поддерживать **без форка кода**.
- **Eval-harness как механизм выбора.** § 7.5 описывает eval-инфраструктуру: датасеты, метрики, regression gate, shadow-mode, cost-aware ranking. Решение «модель X — production для роли Y» **не может приниматься субъективно** — оно следует из eval-метрик. Этот ADR фиксирует процедуру, по которой кандидат превращается в production.
- **Graceful degradation.** § 7.3 («Retry с деградацией — если 32B недоступна, fallback на 7B + флаг для ручной перепроверки»). Любой роутинг должен поддерживать fallback per-role при сбое primary.
- **Биллинг по фактической модели.** Стоимость вызова различается между моделями в разы (см. `default-routing.yaml`: gemma-4-31b — 80/220 коп., qwen-3-27b — 40/120 коп., t-pro — 30/90 коп. за 1k токенов). Биллинг тенанта (ADR-0010) должен учитывать **резолвенную модель**, а не роль (иначе тенант с GigaChat-fallback платит как за Gemma — что неверно).
- **Текущая закладка в `llm-gateway`.** `default-routing.yaml` уже реализует role-based routing с fallback и per-tenant API. README сервиса прямо ссылается на ADR-0011 как кодификатор существующей реализации. Этот ADR не вводит новую систему — он фиксирует правила, по которым существующая система живёт и развивается.
- **Hybrid deployment.** § 1, § 10.2 — air-gapped on-prem. Невозможно завязывать роутинг на single-vendor SaaS API.

Если ничего не решить — каждый агент выберет свою модель в коде (как в Альтернативе 4 ниже), биллинг будет некорректным, выбор production-моделей превратится в субъективные обсуждения, а добавление нового банка с другим набором моделей потребует merge в `main`. Через 6 месяцев окажемся в ситуации, когда «у Альфа-банка работает Qwen, а у Бета-банка пилотится GigaChat, и обновление любого агента требует регрессии на обоих».

## Decision

Кодифицируем **multi-model role-based pool** как production-стратегию роутинга в `llm-gateway`, **с per-tenant overrides** через файлы конфигурации тенанта. Federated-mode (банк сам управляет полным набором моделей) — опциональный режим, активируется только в on-prem и только для конкретных банков-клиентов; в SaaS не используется.

Решение состоит из четырёх взаимосвязанных частей: (1) роутинг, (2) Model Registry, (3) процедура promotion candidate → production, (4) биллинг и audit.

### Часть 1. Роутинг

**Базовый принцип**: агент обращается к LLM **через роль**, не модель.

```http
POST /v1/chat/completions
X-Tenant-Id: bank-alpha
X-Agent-Role: document-vision        # или model: "role:document-vision"
```

Резолюция роли (детерминированная) в `llm-gateway`:

1. **Tenant override.** Чтение `configs/tenants/<bank>/ai/models.yaml`. Если для роли есть запись — используется указанная модель.
2. **Default mapping.** Если override отсутствует — используется глобальный `configs/default-routing.yaml` (`role → model`).
3. **Resolve model → backend.** По имени модели находится backend (vLLM endpoint или external API).
4. **Invocation.** Запрос отправляется в backend с timeout-ом из конфига.
5. **Fallback при сбое.** Если primary недоступна (network error, 5xx, timeout) — повторный запрос на `fallback`-модель этой же роли. Ответ помечается флагом `degraded: true` в metadata, в audit log пишется и primary-failure, и fallback-success.
6. **Cost calculation.** Стоимость считается по фактически использованной модели, не по роли.

**Текущий набор ролей** (фиксируется этим ADR, далее меняется только PR-ом с правкой ADR):

| Роль | Описание | Production model (на момент решения) | Fallback |
|---|---|---|---|
| `document-vision` | Multimodal OCR паспортов, уставов | (определяется eval-harness, см. § 7.5; на момент решения candidate — `gemma-4-26b`) | `qwen-3-vl-32b` |
| `text-reasoning` | Общий reasoning без vision | candidate — `gemma-4-31b` | `qwen-3-27b` |
| `reconciliation` | Сверка с ЕГРЮЛ | candidate — `gemma-4-31b` | `qwen-3-27b` |
| `ubo-tracing` | Графовый reasoning УБО | candidate — `gemma-4-31b` (thinking) | `qwen-3-27b` (thinking) |
| `ru-chat` | Клиентский чат на русском | candidate — `t-pro` | `qwen-3-27b` |
| `rag` | RAG-ответы по нормативке | candidate — `gemma-4-26b` | `qwen-3-27b` |
| `embedder` | Embedding для RAG и сэмплов | candidate — `bge-m3` | (single-only, без fallback в MVP) |
| `mock-eval` | Test-only, всегда mock | `mock-fast` | n/a |

**Важно:** «production model» в момент принятия этого ADR — формально `candidate`. Перевод в `production` происходит через процедуру § 3 ниже, после полного прохода eval-harness в Phase 3 (Ф3, см. § 12 `technical-structure.md`).

**Запрет хардкода модели в агенте.** Агент **никогда** не пишет `model: "gemma-4-26b"` напрямую. Только через роль. CI-линтер в Nx-таргете `agent-prompts-lint` ловит хардкод имён моделей в коде агента (исключение — eval-harness, который явно тестирует конкретные кандидаты).

### Часть 2. Model Registry

Реестр моделей (см. `technical-structure.md` § 7.2 «Model Registry») — централизованное описание всех моделей в пуле:

```yaml
# configs/models/registry.yaml (single-source-of-truth для команды ML)
models:
  gemma-4-26b:
    backend_type: openai_compatible
    weights_uri: s3://platform-models/gemma-4-26b/v1.2/        # для vLLM self-host
    license: Apache-2.0
    languages:
      ru: tier-2          # tier-1=excellent, tier-2=good, tier-3=basic
      en: tier-1
    capabilities: [text, vision, function_calling]
    context_window: 256000
    vram_required_gb: 48
    latency_p50_ms: 1200
    latency_p99_ms: 3400
    cost_per_1k_input_kop: 50
    cost_per_1k_output_kop: 150
    status: candidate     # experimental | candidate | production | deprecated
    eval_results:
      docs-parsing-f1: 0.91
      reconciliation-precision: 0.88
      rag-faithfulness: 0.93
      last_evaluated: 2026-04-15
    promoted_to_production_on: null
    deprecated_on: null
  qwen-3-27b: { ... }
  t-pro: { ... }
  gigachat-pro:
    backend_type: external_api
    api_endpoint: https://gigachat.devices.sberbank.ru/api/v1/
    license: proprietary
    languages: { ru: tier-1, en: tier-2 }
    cost_per_1k_input_kop: 70
    cost_per_1k_output_kop: 200
    status: candidate     # доступен как fallback для on-prem банков, не готовых к self-hosted
```

Registry — единственный source-of-truth для метаданных модели. `default-routing.yaml` ссылается на модели **по имени из registry**; CI валидирует, что все имена в роутинг-конфиге существуют в registry. Дрифт между файлами невозможен.

### Часть 3. Procedure: candidate → production

Перевод модели в `production` для роли — **формальная процедура**, не subjective decision:

1. **Candidate registration.** ML-инженер добавляет модель в `registry.yaml` со `status: experimental`. Заводится PR.
2. **Initial eval.** На fast-subset eval-harness (см. § 7.5) — все актуальные датасеты для роли (`docs-parsing` для `document-vision`, `rag-quality` + `ru-banking-chat` для `rag` и т.д.).
3. **Если fast-subset показал ≥ baseline – 5%** на ключевой метрике — статус повышается до `candidate`.
4. **Полная еженедельная регрессия** — модель прогоняется на всех датасетах (`adversarial` тоже). Для production-промоушена требуется:
   - **Главная метрика**: ≥ baseline (предыдущая production) **по всем датасетам** для роли.
   - **Cost-aware ranking**: cost-per-correct-answer не выше +20% от baseline (если задача регуляторно-критична — `reconciliation`, `rag` — этот лимит послабляется до +50% при выигрыше в качестве).
   - **Latency p99**: не хуже SLA роли (для `ru-chat` — < 2 с, для `document-vision` — < 5 с, для остальных — < 10 с).
   - **Faithfulness/safety**: для `rag` и `ru-chat` — не ниже 0.90 на adversarial-наборе.
5. **Shadow-mode две недели.** Модель получает копию production-трафика (см. § 7.5 «Shadow mode»), её ответы только логируются. Сравниваются:
   - Аналогичные метрики на реальных запросах.
   - Расхождения с production: для `agent-conversational` — DSAT-аналоги через offline-разметку комплаенс-офицером.
6. **Approval gate.** ADR-style review с минимум двумя ревьюверами:
   - **ML-тех-лид** (отвечает за метрики).
   - **Security-инженер или комплаенс-офицер** (для регуляторно-критичных ролей: `reconciliation`, `rag`, `ru-chat`).
7. **Promotion PR.** Меняется `status: production` в registry, обновляется `default-routing.yaml`, старая production переходит в `deprecated`. Один и тот же PR.
8. **Deploy с feature flag.** Первые 7 дней — gradual rollout через Unleash (`technical-structure.md` § 11): 10% → 25% → 50% → 100% трафика. На каждом шаге метрики мониторятся; деградация = автомат-rollback через Argo.

**Что не позволяется:**

- Перевод модели в production без shadow-mode (даже в emergency — там используется fallback, не promotion).
- Перевод без approval security/комплаенс для регуляторно-критичных ролей.
- Skip eval-harness — даже если «модель явно лучше», это субъективное мнение.
- «Только для одного тенанта пробую» — это не promotion в production, это per-tenant override (см. часть 4).

### Часть 4. Per-tenant overrides

Тенант может переопределить модель для роли в своём `models.yaml` (см. § 9):

```yaml
# configs/tenants/bank-alpha/ai/models.yaml
overrides:
  ru-chat:
    model: gigachat-pro          # банк хочет API, не self-hosted
    fallback: t-pro               # если GigaChat недоступен — деградируем на платформенный
  document-vision:
    model: gemma-4-31b            # банк предпочитает 31B вместо 26B-MoE
    # fallback по умолчанию из default-routing.yaml
```

**Правила override:**

- Override указывает только модель/fallback. **Никаких custom-параметров** (temperature, max_tokens) на этом уровне; такие параметры — часть промпта (см. ADR-0007).
- Модель в override **должна существовать в Model Registry** (CI-валидатор).
- Модель в override **должна быть `candidate` или `production`** — не `experimental` и не `deprecated`. CI ловит.
- **Tenant override не требует passing eval-gate** — это решение банка-клиента под его ответственность. Но в onboarding-документе для банка явно прописано: «выбор модели вне нашего default-маппинга означает, что вы принимаете ответственность за качество ответов на ваших данных».

### Federated mode (опциональный)

В on-prem-режиме банк может управлять **полным** набором своих моделей: имена, веса, backend-endpoints. В этом случае платформенный `default-routing.yaml` заменяется на `configs/tenants/<bank>/ai/routing.yaml` (полный конфиг), и `Model Registry` дополняется банковским `bank-registry.yaml`. Это режим **только для on-prem пилотов**, опциональный, требует отдельного Helm-overlay. По умолчанию даже в on-prem банк наследует наш registry и default-routing.

Federated mode не используется в SaaS — там tenant override (часть 4) более чем достаточен.

### Биллинг и audit

Каждый вызов LLM пишет в audit log (см. `security-architecture.md` § 8.2, ADR-0007 для интеграции с promp-управлением):

```json
{
  "action": "llm.invocation",
  "data": {
    "agent": "document-intake",
    "role": "document-vision",
    "resolved_model": "gemma-4-26b",
    "resolved_via": "default",
    "fallback_used": false,
    "tokens_in": 412,
    "tokens_out": 158,
    "cost_kop": 174,
    "prompt_id": "document-intake.extract-passport",
    "prompt_version": 3,
    "latency_ms": 1820,
    "deploy_sha": "abc123..."
  }
}
```

`cost_kop` = `tokens_in × cost_per_1k_input_kop + tokens_out × cost_per_1k_output_kop` для **резолвенной** модели. Биллинг тенанта (ADR-0010) агрегирует эти значения, не делая допущений «такая роль = такая модель».

### Что НЕ покрывает это решение

- **Выбор конкретной модели для каждой роли** — это динамическое решение, обновляется по результатам eval-harness, фиксируется в Model Registry без открытия нового ADR.
- **Структура промптов для роли** — отдельно в ADR-0007.
- **Embedder-модели для vector search** — выбор `bge-m3` vs `E5-mistral` решается eval-harness; этот ADR фиксирует только то, что эмбеддер тоже резолвится через role.
- **Гардрейлы** (PII-фильтр, запрет на autonomous decline) — отдельно в `technical-structure.md` § 7.3 и `security-architecture.md` § 5.4. Этот ADR — про роутинг и выбор модели.
- **GPU capacity planning** для self-hosted моделей — операционная задача DevOps, не архитектурное решение.

### Когда пересматривается решение

- **Появление single-modal-модели radically лучше всех остальных** во всех задачах (gpt-7-class) — рассмотрим переход на single-vendor.
- **Больше 10 банков-тенантов с уникальными модельными требованиями** — может потребоваться полный federated mode по умолчанию.
- **Появление модели с 1M+ context, function calling и сильным русским** — может упростить пул до 2–3 ролей.
- **Регуляторное изменение**: если ЦБ потребует локализации всех LLM-вызовов в РФ — отказываемся от GigaChat/YandexGPT в наборе ролей и работаем только self-hosted.

## Alternatives Considered

### Альтернатива 1: Single-vendor — всё на одной модели (Gemma 4 31B или Qwen 3.5 27B)

**Плюсы:**
- Простейшая операционная схема: один pool of replicas, одна модель в memory.
- Никакого роутинга — все агенты идут в один endpoint.
- Минимальный operational footprint в on-prem (одна модель в air-gapped bundle).
- Один объект в registry, один контракт API.

**Минусы:**
- **Multimodal-задачи объективно требуют другой архитектуры**, чем reasoning. Принуждение `agent-document-intake` к Qwen 3.5 (без VL-варианта) или к Gemma 4 (где русский слабее) — оба варианта дают на 5–15% меньше точности на eval-датасетах.
- **Стоимость одной модели = стоимость всех вызовов.** Чат с клиентом, который мог бы работать на 7B-модели за 30 коп./1k, идёт на 31B за 220 коп./1k — это 7× overcharge.
- **`technical-structure.md` § 7.2 явно постулирует обратное.** Single-vendor-выбор требовал бы отзыва базового архитектурного решения.
- **Tenant override становится бессмысленным** — нечего overrid-ить, кроме backend-URL.
- **Vendor risk concentration**: если у Gemma найдётся юридическая проблема (изменение лицензии, регуляторное ограничение) — ломаем сразу всю платформу.

**Причина отклонения:** прямо нарушает принцип `technical-structure.md` § 7.2; экономика и качество страдают одновременно.

### Альтернатива 2: Federated per-tenant — каждый банк выбирает свой пул целиком

**Плюсы:**
- Максимальная гибкость: банк формирует политику моделей под себя.
- Естественно ложится на банковскую культуру «у меня свой стек».
- Облегчает on-prem self-service.

**Минусы:**
- **Платформа теряет обучающий сигнал.** Eval-harness строится на agreggate данных от всех тенантов. Если каждый банк на своём пуле — нет общей regression-кривой, каждый тенант — отдельный остров.
- **Поддержка усложняется кратно.** При 30 тенантах × 6 ролей × 2 моделей на роль = 360 пар (tenant, role, model), для каждой нужно регрессионное тестирование.
- **Биллинг и cost-aware ranking ломаются.** Cost-per-correct-answer считать в разрезе кросс-тенантного пула невозможно.
- **Onboarding нового банка дороже:** вместо «у вас наши defaults, можете override-ить отдельные роли» — «вам нужно выбрать 6 моделей и обосновать выбор».
- **Регулятор-aware промптинг** размывается — у каждого банка свой пул — невозможно обеспечить consistency на уровне платформы.

**Причина отклонения:** избыточная гибкость для текущей фазы (1–5 пилотных банков). Сохраняем federated как опциональный режим для on-prem-банков с особыми требованиями (часть 4 решения).

### Альтернатива 3: Per-call dynamic routing — выбираем модель по характеристикам запроса

**Плюсы:**
- Теоретически оптимальный cost-quality trade-off для каждого запроса.
- Можно отправлять простые запросы на 7B, сложные — на 31B.

**Минусы:**
- **Классификатор сложности — отдельная LLM-задача**, добавляет latency и стоимость на КАЖДОМ запросе.
- **Воспроизводимость теряется.** Один и тот же запрос завтра может уйти на другую модель — eval становится noisy.
- **Audit log усложняется**: нужно логировать не только модель, но и решение классификатора.
- **Stability concern**: классификатор сам — точка отказа. Если он деградирует — деградируют все агенты.
- **Не решает проблему multimodal vs text.** Multimodal-запрос всё равно нужен на multimodal-модель.

**Причина отклонения:** premature optimization. Role-based routing решает 90% сценария; per-call dynamic — кейс на Phase 5+, если eval-данные покажут реальный потенциал.

### Альтернатива 4: Per-agent hard-coded — каждый агент сам выбирает модель в коде

**Плюсы:**
- Простейшая реализация: агент инициализирует клиент vLLM с конкретным URL.
- Нет роутинг-сервиса как зависимости.

**Минусы:**
- **Tenant override невозможен.** Каждый банк = форк кода агента. Прямо ломает требование hybrid deployment.
- **A/B-тестирование невозможно.** Смена модели = redeploy агента.
- **Биллинг централизованно невозможен.** Каждый агент сам считает токены и стоимость, форматы расходятся.
- **Гардрейлы (PII-фильтр) распыляются.** Если каждый агент сам зовёт модель — PII-фильтр нужно встраивать N раз. `llm-gateway` нужен именно как централизованный gate.
- **`technical-structure.md` § 4.4 явно описывает llm-gateway как единый прокси.**

**Причина отклонения:** прямо нарушает архитектуру и хуже по всем измерениям.

### Альтернатива 5: External multi-model API (OpenRouter и т.п.)

**Плюсы:**
- Готовый роутинг ко множеству моделей через единый API.
- Нет инфраструктурных затрат.

**Минусы:**
- **Несовместимо с hybrid deployment.** On-prem банк не ходит в OpenRouter.
- **Локализация ПДн.** Запросы клиентов могут содержать паспортные данные — отправлять в зарубежный SaaS = нарушение 152-ФЗ.
- **Стоимость в копейках** — внешний роутер берёт markup; для нашего объёма дешевле self-hosted.
- **SLA вне нашего контроля** — для регуляторно-критичной системы недопустимо.

**Причина отклонения:** базовое нарушение принципа hybrid deployment.

## Consequences

### Positive

- **Архитектурный принцип «pool, не one»** реализован в коде, не только в документе.
- **Per-tenant overrides без форка** позволяют onboard'ить банки с разными требованиями (self-hosted Gemma vs GigaChat API) изменяя только конфиг тенанта.
- **A/B и shadow** реализуются через изменение конфига или registry — нулевой код-импакт на агенты.
- **Биллинг корректен**: тенант платит за фактически использованную модель, fallback не маскируется под primary.
- **Объективная процедура promotion** убирает субъективность из решения «какую модель использовать в продакшене»; вместо «архитектор сказал» — eval-метрики на эталонных датасетах с подписью двух ревьюверов.
- **Graceful degradation** через role-fallback: при сбое 31B-модели роль автоматически уходит на 27B-fallback, с пометкой `degraded` для последующей ручной перепроверки (§ 7.3).
- **Audit-completeness**: каждая запись содержит role + resolved_model + resolved_via + fallback_used → полная реконструкция «почему этот клиент получил такой ответ».
- **Eval-driven culture**: команда привыкает обосновывать выбор модели цифрами, не чувствами.

### Negative

- **`llm-gateway` — single-point-of-failure для всего LLM-трафика.** Mitigation: HA-deployment (3 replica с stateless-сервисом, конфиги перечитываются по SIGHUP), health-check на каждом backend, circuit-breaker на роль с автоматическим перевхождением в fallback.
- **Model Registry — дополнительная единица, требующая дисциплины.** Если запись в registry рассыпается с реальностью (модель удалена, а запись осталась) — runtime-ошибки. Mitigation: CI-проверка консистентности registry vs default-routing vs реально доступных моделей в weekly job.
- **Per-tenant overrides увеличивают surface для тестирования.** В eval-harness нужно прогонять не только default mapping, но и активные overrides per-bank. Mitigation: matrix-test в Nx (см. ADR-0004) с tag-фильтрацией: тестируем все production-маппинги + active overrides пилотных банков.
- **Procedure candidate → production имеет lag.** От «новая модель появилась» до «production» — минимум 2–3 недели. Это плата за регуляторную надёжность; на этапе experimentation модель доступна как override для конкретного тенанта.
- **Cost calculation зависит от точности токенизаторов.** В MVP `llm-gateway` использует грубую оценку `len(text)//4` (см. README сервиса). Это даёт ±10–20% ошибку в биллинге. Mitigation: Phase 3+ — точные токенизаторы (sentencepiece, tiktoken-equivalents) per-model. Документировано как Phase 3 deliverable.
- **Tenant override без eval-gate — потенциальный квалитативный регресс**. Если банк выбрал модель хуже, чем default — мы не блокируем (это его выбор), но можем не заметить. Mitigation: weekly cross-tenant report по quality metrics; если override-модель показывает деградацию — алертим CSM банка.

### Neutral

- **Часть моделей в pool за пределами нашего контроля** (GigaChat, YandexGPT) — их обновления не зависят от нас. Mitigation: registry содержит `api_endpoint` с pin-версией, мониторинг breaking changes провайдеров. Для on-prem self-hosted — версии моделей фиксированы в bundle.
- **Список ролей расширяется по мере появления агентов.** Добавление роли = PR с обновлением этого ADR + registry + default-routing. Это нормальная эволюция, не reorg.
- **Embedder также резолвится через role** — единый паттерн для всего LLM-трафика; integration с pgvector и Qdrant (см. ADR-0008) идёт через тот же gateway.
- **Mock-роль (`mock-eval`)** существует для тестов — это ожидаемый артефакт, не загрязнение пула.

## Implementation Notes

### Текущее состояние

В `ai/llm-gateway/` уже реализовано:

- Role-based routing через `default-routing.yaml`.
- Fallback per role.
- Per-tenant учёт стоимости (in-memory, persist в Kafka — Phase 3).
- Mock-режим для CI.

Что **этот ADR добавляет к существующей реализации**:

| Элемент | Текущий статус | Что нужно сделать |
|---|---|---|
| Model Registry (`configs/models/registry.yaml`) | Нет | Создать в Phase 3 как первый шаг |
| CI-валидатор registry ↔ routing ↔ runtime | Нет | Nx-таргет `model-registry-lint` |
| Per-tenant override (`configs/tenants/<bank>/ai/models.yaml`) | Schema есть, реализации в gateway нет | Phase 3 |
| Procedure candidate → production | Описана здесь | Закрепить в ADR-0012 (eval-corpus governance) |
| Audit `resolved_model` + `cost_kop` | Частично | Persist в Kafka в Phase 3, см. ADR-0010 |
| Точные токенизаторы | `len(text)//4` | Phase 3 как часть выбора production-моделей |

### Этапы внедрения (привязаны к фазам в § 12)

| Этап | Что | Когда |
|---|---|---|
| 1 | Model Registry + CI-валидаторы | Phase 0 (Ф0) |
| 2 | Per-tenant override в gateway, schema валидация | Phase 3 (Ф3) |
| 3 | Persistence usage в Kafka → billing-service | Phase 3 (Ф3) |
| 4 | Полный eval-harness regression + cost-aware ranking | Phase 3 (Ф3) |
| 5 | Shadow-mode для production-промоушена | Phase 3 (Ф3) |
| 6 | Точные токенизаторы per-model | Phase 3 (Ф3, после выбора production-моделей) |
| 7 | Federated mode (если будет конкретный on-prem кейс) | Phase 7 (Ф7) или позже |

### CI-проверки в Nx (см. ADR-0004)

- **`model-registry-lint`**: парсит `registry.yaml`, валидирует обязательные поля, lifecycle-консистентность (`deprecated_on > promoted_to_production_on > created_at`), отсутствие дубликатов.
- **`routing-config-lint`**: каждая модель в `default-routing.yaml` существует в registry со статусом `production` (не candidate). Каждая роль имеет fallback.
- **`tenant-override-lint`**: каждая модель в любом `configs/tenants/*/ai/models.yaml` существует в registry со статусом `candidate` или `production`.
- **`llm-gateway-integration-test`**: testcontainers с mock-backend, проверка резолюции role + override + fallback на полном set'е тестовых тенантов.

### Безопасность

- Per-tenant `bearer JWT` к `llm-gateway` с claim `tenant_id` (см. `security-architecture.md` § 3.1, ADR-0010).
- PII-фильтр на входе в gateway — гардрейл из `security-architecture.md` § 5.4 — обязательный для `external_api`-моделей (GigaChat, YandexGPT). Для self-hosted моделей в нашей инфраструктуре — рекомендуемый, не обязательный (поскольку модель в периметре).
- Promotion требует подписи security-инженера в PR для регуляторно-критичных ролей — это процессный контроль, не technical, но без него `production` PR не мерджится.
- Audit log хранится 5 лет (см. § 8.1) с хеш-цепочкой записей; promotion-события (смена статуса модели) — отдельные события `audit.model_promoted`.

### Operations

- **Health-check на backend** в gateway: каждые 30 с пинг на `/health`, при 3 fail-ах — backend помечается как down, role-fallback активируется автоматически.
- **Circuit breaker per role**: при > 50% failures на роль за 1 минуту — все запросы 5 минут идут на fallback с алертом DevOps.
- **Capacity**: GPU-pool в SaaS планируется per-роль на основе requests/sec и token-throughput; on-prem — банк планирует свою capacity на основе registry-метрики `vram_required_gb` и ожидаемого QPS.

## References

- `docs/technical-structure.md` § 4.4 (llm-gateway как единый прокси), § 7.2 (стратегия моделей и роли — фундаментальный документ для этого ADR), § 7.3 (гардрейлы), § 7.5 (eval-harness — определяет процедуру promotion), § 9 (per-tenant конфигурация), § 11 (CI/CD и feature flags), § 12 (привязка к фазам).
- `docs/security-architecture.md` § 3.1 (идентификация субъектов, JWT для service-to-service), § 5.4 (PII-фильтр перед LLM), § 8.1 (срок хранения LLM-логов 5 лет), § 8.2 (что логируется в audit).
- `ai/llm-gateway/README.md` — описание текущей реализации, ссылка на этот ADR как кодификатор.
- `ai/llm-gateway/configs/default-routing.yaml` — конкретный production-конфиг, на который этот ADR опирается.
- ADR-0001 (Temporal): LLM-вызовы из Activities получают role-based routing через тот же gateway; retry-механика Temporal комплементарна fallback-логике gateway.
- ADR-0002 (Schema-per-tenant): tenant-id в JWT-клейме маппится на схему БД; в gateway — на ключ конфига `configs/tenants/<bank>/`.
- ADR-0004 (Nx): `model-registry-lint`, `routing-config-lint`, `tenant-override-lint`, integration-tests — все встают как Nx-таргеты с правильными `inputs`/`outputs`.
- ADR-0005 (Стратегия миграций): persistence usage в `billing_events` (платформенная схема) и связанный partitioning — мигрируется через стандартный flow.
- ADR-0007 (Промпт-менеджмент): резолюция промпта и резолюция модели — параллельные операции в gateway, обе резолвятся по `(tenant, role)` с независимыми overrides.
- ADR-0008 (Vector DB): embedder-роль резолвится через тот же механизм; смена embedder-модели координируется со re-ingest корпуса в Qdrant и пересчётом pgvector-индексов.
- ADR-0010 (Биллинг и audit log): получает события usage с уже резолвенным `resolved_model` и `cost_kop`; стоимость считается от фактической, не от роли.
- ADR-0012 (Eval-corpus governance): процедура candidate → production опирается на eval-датасеты; этот ADR ссылается на ADR-0012 как на детальную спецификацию governance.
- [vLLM](https://github.com/vllm-project/vllm) — основной inference-движок для self-hosted моделей.
- [SGLang](https://github.com/sgl-project/sglang) — альтернатива для structured output (упомянут в § 3.6).
