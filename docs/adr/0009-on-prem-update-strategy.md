# ADR-0009: Стратегия on-prem обновлений — blue-green с Istio routing и подписанным air-gapped bundle

**Status:** Accepted
**Date:** 2026-04-26
**Authors:** Главный архитектор, DevOps-лид
**Reviewers:** Тех-лид backend, Security-инженер, DBA

## Context

В контуре банка-клиента у нас нет прямого сетевого доступа. Релизный цикл — квартальный (`docs/technical-structure.md` § 11: «On-prem: квартальные релизы со стабилизационным циклом»). Развёртывание — air-gapped bundle с Helm-чартами, docker-образами и весами LLM-моделей (§ 10.2). Это создаёт принципиально другие требования, чем у SaaS-режима.

Силы, давящие на решение:

- **No remote access.** Мы не можем зайти в банковский кластер по SSH; обновление инициирует банковский DevOps по нашему runbook. Любой сценарий «нажми кнопку → подождите → если что — мы починим» **на нашей стороне** не работает.
- **Maintenance window 02:00–04:00 МСК.** Банк-клиент типично готов давать 2-часовое окно ночью раз в квартал. За это время нужно: (а) обновиться, (б) убедиться, что работает, (в) при проблемах — откатиться. Если откат занимает > 30 минут — мы выходим за окно с инцидентом для банка.
- **Длинные SAGA в Temporal** (см. ADR-0001). На момент обновления в кластере живут активные workflow онбординга — некоторые из них могут быть на стадии «ждём документы от клиента 14 дней». Любая стратегия обновления обязана либо мигрировать эти workflow без потери прогресса, либо аккуратно завершить.
- **Миграции БД forward-compatible.** ADR-0005 уже задал политику: «политика отката — это всегда forward fix в проде, не migrate down»; «Backwards-compatible миграции в любое время; деструктивные — quarterly + 7 дней предупреждения». Это означает, что **обе версии платформы (старая и новая) должны уметь работать с одной и той же схемой БД хотя бы в течение окна обновления**.
- **Stateful-зависимости.** PostgreSQL, Kafka, MinIO, Vault, Qdrant — все persistent. Мы не можем «развернуть всё заново» — это потеря состояния, неприемлемо. Stateful-сервисы обновляются in-place либо по своим runbook'ам (отдельный жанр), не как часть платформенного релиза.
- **Stateless-сервисы.** Большинство наших сервисов (`tenant-service`, `bff-*`, `agent-*`, `abs-connector`, `abs-adapter-*`) — stateless относительно K8s. Их можно перевыкатывать без потери данных, важна только корректность routing.
- **Поддержка N–2 версий on-prem одновременно** (§ 11). У нас в полевых развёртываниях три поколения платформы могут жить одновременно (банк А — 1.4, банк Б — 1.5, банк В — 1.6); это не про обновление одного банка, а про поддержку версионной матрицы. Это решение касается **процедуры обновления внутри одного банка**.
- **LLM-веса в bundle** (§ 10.2). Веса измеряются десятками гигабайт. Любая стратегия обновления обязана не качать их повторно, если они не изменились между релизами; если изменились — обеспечить downtime-free переключение моделей на новую версию.
- **Audit log непрерывен.** 5-летний срок хранения (ADR-0007 § 8.1) и hash-цепочка (`security-architecture.md` § 8.3) запрещают любой gap в audit log даже на 5 минут. Любая стратегия обновления обязана не разрывать audit-цепочку.
- **Истанбульская специфика РФ-банков.** Часто банк выделяет K8s, который он администрирует сам (OKD/Deckhouse, см. § 10.2); мы — гость на их кластере. Минимум магии, максимум стандартных K8s-примитивов.
- **Безопасность и регуляторика.** Любой обновлённый артефакт банк хочет проверить: подпись, SBOM, security scan-результаты. Bundle обязан быть подписан, артефакты — пинены по digest.

Если ничего не решить, банк столкнётся с обновлением, которое требует 6-часового downtime, ломает live workflow, и при сбое не имеет 5-минутного отката. Это убивает доверие после первого инцидента.

## Decision

Используем **blue-green развёртывание с двумя параллельными namespaces в одном K8s-кластере банка**, с **переключением трафика через Istio VirtualService** и **синхронным отказом-ready-bundle**.

Решение состоит из шести частей: (1) топология namespaces, (2) flow обновления, (3) переключение трафика, (4) state-handling, (5) формат bundle и подписи, (6) роль банка vs наша роль.

### Часть 1. Топология namespaces

```
[Кластер банка-клиента]
├── platform-blue/           — текущая активная версия (например, v1.4.2)
│   ├── tenant-service
│   ├── bff-onboarding
│   ├── bff-admin
│   ├── onboarding-orchestrator (Temporal worker)
│   ├── document-service
│   ├── ... (всё stateless)
│   └── abs-adapter-cft
├── platform-green/          — параллельная новая версия (например, v1.5.0)
│   └── ... (тот же набор, но новые версии образов)
├── platform-data/           — stateful, единый, не дублируется
│   ├── postgres-cluster
│   ├── kafka
│   ├── temporal-cluster (PG-backed, см. ADR-0001)
│   ├── qdrant
│   ├── minio
│   ├── vault
│   └── redis
├── platform-mesh/           — Istio control-plane, gateway, VirtualService
│   ├── istio-gateway
│   └── platform-vs          — VirtualService, маршрутизирующий на blue или green
└── platform-ai/             — отдельно из-за GPU и весов LLM (см. § 10.1, 10.2)
    └── llm-gateway / vllm-pool
```

**Ключевое правило:** **stateless-сервисы дублируются (blue+green), stateful — в едином `platform-data`**. Это значит: обе версии работают **с одной и той же БД, одним Kafka, одним Vault**. Совместимость обеспечивается forward-compatible миграциями (ADR-0005).

### Часть 2. Flow обновления

```
T-0 (квартальный релиз готов в SaaS, прошёл стабилизацию):
  → формируется air-gapped bundle (см. часть 5)
  → bundle передаётся банку (физический носитель / outbound proxy банка с whitelist)

T-7 дней (банк проверяет bundle):
  → банк сверяет подпись, разворачивает в pre-prod, smoke-tests
  → банк подтверждает: «готовы к окну»

T+0 (начало maintenance window 02:00 МСК):
  Шаг 1. Банк применяет миграции БД (forward-compatible, idempotent — см. ADR-0005)
  Шаг 2. Банк деплоит новую версию в namespace `platform-green`
  Шаг 3. K8s readiness probes ждут healthy → все pods Ready
  Шаг 4. Smoke-tests против green namespace через приватный VS
            (test-VirtualService направляет /v1/__smoke на green)
  Шаг 5. Если smoke pass: банк нажимает «активировать новую версию»
            (tenant-cli activate-release --version=1.5.0 --target=green)
  Шаг 6. tenant-cli обновляет VirtualService:
            old: → platform-blue
            new: → platform-green
            (Istio выполняет blue→green переключение атомарно)
  Шаг 7. Мониторинг 30 минут — error rate, latency, audit-completeness
  Шаг 8. При проблеме: tenant-cli rollback-release (один CLI-call возвращает VS на blue)
  Шаг 9. После 24 часов стабильной работы: blue namespace может быть scaled down
            (но не удаляется ещё 7 дней — backstop откат)

T+7 дней (стабилизация):
  → blue namespace удаляется
  → green становится «текущим» (то есть в следующем релизе он станет blue, новая версия — green)
```

Полный технический rollback (зеркальный flow) занимает **5–10 минут**, потому что blue namespace всё ещё работает и trafficа просто не получает.

### Часть 3. Переключение трафика через Istio VirtualService

```yaml
# platform-mesh/platform-vs.yaml (пример)
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: platform-router
  namespace: platform-mesh
spec:
  hosts:
    - bank-alpha.platform.internal
  http:
    - match:
        - uri: { prefix: "/v1/__smoke" }
      route:
        - destination:
            host: bff-onboarding.platform-green.svc.cluster.local
    - route:
        - destination:
            host: bff-onboarding.platform-blue.svc.cluster.local   # active = blue
          weight: 100
        - destination:
            host: bff-onboarding.platform-green.svc.cluster.local
          weight: 0
```

**Активация green:** меняем weights `100/0 → 0/100`. Istio делает это атомарно для новых соединений; in-flight HTTP-запросы доезжают до старого backend, новые идут на новый.

**Атомарность для multiple services.** Routing меняется одним applied-манифестом для всех hosts (`bff-onboarding`, `bff-admin`, `api-gateway`, etc.) — `kubectl apply` атомарен для одного yaml-файла.

**Canary as option (не default).** В стандартном on-prem-флоу мы делаем 100% blue → 100% green одной командой; canary 10/25/50/100 — opt-in через флаг `tenant-cli activate-release --canary=5min` для банков, готовых к более длинному окну.

### Часть 4. State-handling

#### БД (см. ADR-0005)

- Миграции применяются **до** деплоя green namespace (Шаг 1 во flow).
- Все миграции **forward-compatible**: blue работает на новой схеме (новые поля игнорирует, новых таблиц не пишет, но и не падает).
- Деструктивные миграции (drop column) разделены на 2 квартальных релиза: «релиз N — перестал писать» / «релиз N+1 — drop column» (политика ADR-0005).
- Если миграция всё-таки сломается на старте: forward fix следующим квартальным релизом, в окне — ручная коррекция DBA банка по runbook'у.

#### Temporal workflow

- Один Temporal-кластер на банк, обслуживает обе версии.
- **Temporal worker'ы (часть `onboarding-orchestrator`) дублируются**: blue-worker и green-worker оба подключены к одному Temporal-кластеру, обрабатывают одни и те же task queues.
- Долгоживущие workflow (например, «жду документы 14 дней») продолжают `await`-блокироваться независимо от того, какой worker их подхватит после рестарта (это естественное свойство Temporal — workflow определяется state-store, не worker'ом).
- **Versioned workflow code.** Temporal встроенно поддерживает versioning (`workflow.GetVersion`); breaking changes в workflow-логике — через явный `Patch` (см. ADR-0001).
- Когда blue scaled down (T+24 часа), его worker'ы отписываются, остальной трафик task queues подхватывает green-worker.

#### Kafka

- Один Kafka-кластер обслуживает оба namespace.
- Topic-схемы (через Confluent Schema Registry или эквивалент в РФ-варианте) эволюционируют forward-compatible — новый consumer читает старые сообщения, старый consumer игнорирует новые поля.
- Consumer group именования включает namespace: `bff-onboarding-blue`, `bff-onboarding-green`. Это даёт независимый offset, что важно при canary-тестах (green читает свой offset, не «съедает» сообщения у blue).
- В normal-flow (после переключения weight=100% на green) blue consumer'ы дотягивают до баланса и scaled down.

#### Audit log

- Append-only PostgreSQL (см. § 8.3 security-architecture). Обе версии пишут в одну таблицу.
- Hash-цепочка непрерывна: каждая запись ссылается на hash предыдущей независимо от того, какой namespace её вписал.
- Запись о самом релизе — отдельный audit-event `platform.release_activated` с `from_version`, `to_version`, `tenant_id`, написанный `tenant-cli` в момент переключения.

#### Vault и Qdrant

- Vault обслуживает оба namespace по разным service-identity (blue и green имеют разные K8s-SA).
- Qdrant — единый, обе версии используют те же collections (`tnt_<id>_rag`, см. ADR-0008). Если новая версия требует переиндексации (например, смена embedder-модели — см. ADR-0011) — отдельный flow с alias-swap, не часть стандартного blue-green.

#### LLM-веса

- В отдельном `platform-ai` namespace, обновляются по своему циклу. Если новый релиз требует новой модели — bundle включает её, веса распаковываются в `platform-ai/models/` и подключаются через `llm-gateway`-конфиг (alias swap по аналогии с Qdrant).
- Если веса не изменились между релизами — они переиспользуются, bundle их не везёт повторно (см. часть 5).

### Часть 5. Формат bundle и подписи

```
aibank-platform-1.5.0-onprem.tar.gz
├── manifest.yaml                   — список артефактов с digest и version
├── manifest.yaml.sig               — подпись cosign / GPG
├── docker-images/
│   ├── platform/
│   │   ├── tenant-service-1.5.0.tar
│   │   ├── bff-onboarding-1.5.0.tar
│   │   └── ...
│   └── adapters/
│       ├── abs-adapter-cft-1.4.3.tar       — могут быть свои версии (ADR-0006)
│       └── ...
├── helm-charts/
│   ├── platform-onprem-1.5.0.tgz   — главный umbrella-chart
│   └── charts/
│       └── ...
├── migrations/
│   └── db-migrator-1.5.0.tar       — Job-образ с embedded миграциями (ADR-0005)
├── models/                          — только если веса изменились
│   ├── gemma-4-26b-v1.3/
│   └── checksum.txt
├── runbooks/
│   ├── upgrade.md                   — пошаговая инструкция для банк-DevOps
│   ├── rollback.md
│   └── verify.md
└── changelog.md                     — что изменилось с предыдущего релиза
```

**Подпись:** `manifest.yaml.sig` — cosign-подпись приватным ключом нашей релизной CI; банк проверяет публичным ключом из своего trust-store. Каждый docker-image и helm-chart дополнительно подписан (cosign sign image, helm sign).

**SBOM:** к каждому docker-image приложен Software Bill of Materials (SPDX или CycloneDX) — банк-ИБ может запустить свой scanner (см. `security-architecture.md` § 7.1, § 10.2 «SBOM + scan» в чеклисте ИБ-аудита).

**Reproducibility:** sha256 digest каждого артефакта в `manifest.yaml`; банк может верифицировать неизменность bundle на своей стороне.

### Часть 6. Роли в процессе обновления

| Кто | Что делает |
|---|---|
| **Наша релизная команда** | Формирует bundle, подписывает, передаёт банку. Поддержка горячей линии в день обновления. |
| **Банковский DevOps** | Принимает bundle, проверяет подпись, разворачивает в pre-prod, выполняет runbook в prod-окне, нажимает «активировать». |
| **Банковский DBA** | Применяет миграции БД через `db-migrator` Job (см. ADR-0005). |
| **Банковский ИБ** | Сканит SBOM, проверяет docker-images своим сканером, авторизует развёртывание. |
| **`tenant-cli`** | Утилита, поставляемая в bundle: единая точка для переключения VS, отката, статуса миграций. На стороне банка. |
| **`web-admin` (Phase 7+)** | Опционально — UI-кнопка «активировать релиз», обёртка над `tenant-cli`. В MVP — только CLI. |

### Что НЕ покрывает решение

- **Обновление stateful-сервисов** (PostgreSQL major upgrade, Kafka cluster upgrade) — отдельный класс операций, отдельные runbook'и в `docs/runbooks/`. Не часть платформенного релиза.
- **Обновление инфраструктурного слоя банка** (K8s, Istio control-plane) — ответственность банка, мы только указываем минимально совместимые версии в release-notes.
- **SaaS-режим обновления** — там используется ArgoCD + rolling update + canary через Argo Rollouts; это другой ADR-кандидат при необходимости.
- **Federated-mode LLM** (см. ADR-0011 часть «Federated mode») — обновление банковских custom-моделей не в scope нашего bundle.

### Когда пересматривается решение

- **Появление 50+ on-prem банков** — может потребоваться более автоматизированная система доставки (закрытый CDN, GitOps-pull от банка).
- **Регуляторное требование zero-downtime** — текущая схема даёт <1 минуту switching-time, но requestов в полёте может всё-таки терять единицы; если ЦБ потребует strict zero-loss — нужны session affinity и graceful drain с длинным окном.
- **Появление кейса, где stateful-сервис обязан обновляться синхронно с stateless** — потребует переосмысления топологии.

## Alternatives Considered

### Альтернатива 1: Rolling update Helm chart in-place

**Плюсы:**

- Стандартная K8s-практика. `helm upgrade` — одна команда.
- Минимум ресурсов: новая версия pod'а замещает старый, никакого дублирования.
- Простой mental model для банковского DevOps.

**Минусы:**

- **Откат — это второй `helm rollback`,** который занимает столько же, сколько upgrade — несколько минут на pod, итого 10–15 минут на сервис; если у нас 21 сервис в платформе — это полчаса rollback при инциденте.
- **Между «старая инстанция убита» и «новая ready» — Pod в Init/CrashLoop** трафик не получает; для нескольких сервисов в одной session — водопад ошибок.
- **Долгоживущие соединения** (gRPC stream, WebSocket для будущих subscriptions) обрываются на каждом pod-restart.
- **Нет parallel smoke-test** новой версии до переключения трафика — мы либо в проде, либо нет.
- **Temporal worker'ы** при rolling restart могут дать timeout на activity; в маленьком окне это норма, но в нашем 2-часовом окне это съедает запас.

**Причина отклонения:** не даёт быстрого отката в окне 2 часов; rolling — стандарт SaaS, для on-prem с квартальными релизами нужно что-то более safety-net-ориентированное.

### Альтернатива 2: Immutable infrastructure — новый K8s namespace на каждый релиз

**Плюсы:**

- Максимальная изоляция: каждая версия — свой namespace, без shared mutable state на уровне манифестов.
- Прозрачная история: namespace'ы `platform-1.4.2`, `platform-1.5.0`, `platform-1.6.0` лежат рядом.
- Откат к любой версии простым переключением VS.

**Минусы:**

- **Хранилище данных не immutable.** PostgreSQL, Kafka, Vault всё равно общие. Это не настоящая immutable infrastructure, а blue-green с другими именами.
- **Распухание namespace-ов.** За год 4 namespace'а при стандартном цикле, плюс failed-релизы; уборка — отдельная процедура.
- **Vault-интеграция** требует разных service-identity на каждый namespace — это management-overhead.
- **Конфликт со словарём** банковского DevOps — они привыкли к понятию «среда» (`prod`, `staging`), а не к именам с версиями.

**Причина отклонения:** дополнительная сложность без реальной выгоды над blue-green; функционально эквивалентно нашему решению, но с худшей читаемостью.

### Альтернатива 3: Argo Rollouts с canary через VirtualService

**Плюсы:**

- Декларативное описание canary-флоу: 5% → 25% → 50% → 100% с автоматическим analysis-template (метрики из VictoriaMetrics).
- Auto-rollback при срабатывании analysis-rule.
- Хорошо интегрируется с Istio.

**Минусы:**

- **Argo Rollouts — отдельный controller** в кластере банка. Дополнительная зависимость (CRD-driven), которую банк-DevOps должен поддерживать.
- **Длительность canary 5min × 4 шага = 20 минут** — занимает половину 2-часового окна; в случае проблем на последнем шаге банк успевает rollback, но запас тонкий.
- **Long-running activity в Temporal** не дружит с canary очень хорошо: запрос «открыть счёт» не должен распиливаться между versions on flight.
- **Auto-rollback** — приятная фича, но в banking-контексте мы хотим, чтобы человек принимал решение в реальном времени, а не automation.

**Причина отклонения:** Argo Rollouts хорош для SaaS с непрерывной выкаткой и небольшими изменениями, для квартального on-prem-релиза с ручной валидацией — overkill. Сохраняем canary как opt-in через тот же `tenant-cli`-флаг (часть 3), но не делаем default.

### Альтернатива 4: Полный red-черный (отдельный K8s-кластер на каждую версию)

**Плюсы:**

- Абсолютная изоляция версий.
- Параллельный live-traffic на обе версии возможен.

**Минусы:**

- **Дублирует stateful-сервисы или требует cross-cluster БД** — оба варианта неприемлемы (ресурсы / задержки).
- **Банк не выделит второй кластер на платформу** — это удвоение всех сертификатов, IAM, мониторинга.
- **Failover между кластерами** требует cross-cluster Istio mesh — неподъёмная сложность для квартального обновления.

**Причина отклонения:** избыточно для нашего сценария; blue-green в одном кластере даёт 95% выгоды без 95% сложности.

### Альтернатива 5: «Update as code» через GitOps (pull-based банком)

**Плюсы:**

- Банк сам тянет изменения из Git, нет физического bundle.
- Полная декларативность через ArgoCD.
- Естественное audit-trail в Git.

**Минусы:**

- **Air-gapped on-prem** (см. § 10.2) — банк не имеет outbound в наш Git. Это базовое ограничение.
- **Подписанные артефакты** в Git технически возможны, но Git не стандартный канал для распространения LLM-весов в десятки гигабайт.
- **Pull-based** для квартального цикла — лишняя гибкость; нам нужна detailed runbook-управляемая процедура.

**Причина отклонения:** прямое противоречие с air-gapped принципом. GitOps работает в SaaS, не в on-prem.

## Consequences

### Positive

- **Откат за 5–10 минут** через перезаписывание VS — значимо для 2-часового окна.
- **Параллельный smoke-test** новой версии до переключения trafficа — банк может убедиться, что green работает на их данных, до того, как пользователи это увидят.
- **Минимум новых компонентов.** Istio уже в стеке (`technical-structure.md` § 3.4), ArgoCD/Helm — стандарт. Новой инфраструктуры — только namespace-конвенция и `tenant-cli`-команды.
- **Stateful-сервисы не дублируются** — нет проблем с миграцией данных, бэкапами, синхронизацией.
- **Air-gapped-friendly.** Bundle самодостаточен, подписан, не требует внешних вызовов.
- **Audit-completeness.** Hash-цепочка не разрывается; событие `platform.release_activated` явно фиксирует переключение.
- **Temporal-workflow живут.** Долгоживущие workflow не теряют состояние.
- **Согласовано с ADR-0005** (forward-compatible миграции), ADR-0001 (Temporal versioning), ADR-0006 (адаптеры со своим semver — обновляются независимо или вместе с платформой через bundle).

### Negative

- **2x ресурсов на stateless-сервисы во время окна.** Для 21+ сервиса с 2–3 replicas — 50+ pod'ов параллельно. Mitigation: окно короткое (3–24 часа максимум blue+green одновременно), банковские кластера обычно имеют 30–50% запаса; release-notes явно указывают peak-resource-usage.
- **Forward-compatible миграции — дополнительная дисциплина в разработке.** Каждая миграция должна оставлять старую версию работоспособной. Mitigation: уже зафиксировано в ADR-0005 как политика; lint-правило в CI ловит обратимость.
- **Istio dependency.** Если банк не хочет/не умеет Istio — наш blue-green ломается. Mitigation: альтернативный механизм через nginx ingress + service swap описан в `runbooks/upgrade-no-istio.md`; но это slower-rollback (1–2 минуты), и feature parity не 100%.
- **Тренировка банка на runbook.** Первое обновление — пилотное с нашим engineer-on-call в видеоконференции. Mitigation: tabletop-exercise с банком за месяц до первого реального обновления.
- **Сложнее quick-fix.** В rolling-сценарии «хотфикс» — это `helm upgrade --reuse-values`. В blue-green — нужен новый bundle (даже если patch). Mitigation: hot-fix bundle упрощённый (только изменённые images, без LLM-весов и без миграций); цикл подготовки сжат до 4 часов вместо 4 недель.
- **Storage-overhead.** Blue и green namespaces используют PVC от common-data, но временные ConfigMaps/Secrets дублируются. Незаметно (~МБ), но факт.

### Neutral

- **Уборка blue-namespace через 7 дней** — операционная процедура, делает банк по runbook'у; не automation, но понятная команда.
- **Bundle-формат стабилен** по версиям — это хорошо для долгосрочной воспроизводимости (manifest можно прочесть через 5 лет), но требует sklerotic-дисциплины при изменениях формата.
- **Versioned bundle-storage у банка.** Банк хранит N последних bundle'ов — возможность откатиться на 2 релиза назад. Это ответственность банка, мы документируем рекомендацию (хранить 4 последних).
- **Релизный кадетенция кварт** = 4 окна в год. Этот ADR не меняет кадетенцию, только процедуру внутри окна.

## Implementation Notes

### Bundle build pipeline (на нашей стороне, см. ADR-0004 для Nx)

```
GitLab CI:
  → nx run-many -t build:onprem-bundle
       ↓
       packs all docker images, helm charts, migrations, models (если изменились)
       ↓
  → cosign sign manifest.yaml
  → cosign sign --recursive все docker images
  → helm sign все charts
  → SBOM-generation (syft) для каждого image
  → tar.gz → upload в защищённое хранилище
  → notify банк-DevOps по согласованному каналу (физическая передача / outbound proxy банка)
```

### Verification на стороне банка

Banking DevOps выполняет (`runbooks/verify.md`):

```bash
# Проверка подписи
cosign verify-blob --key aibank-release.pub --signature manifest.yaml.sig manifest.yaml

# Распаковка
tar -xzf aibank-platform-1.5.0-onprem.tar.gz

# Проверка digest каждого артефакта
yq -r '.images[] | .name + " " + .digest' manifest.yaml | while read name digest; do
  actual=$(sha256sum "docker-images/${name}.tar" | awk '{print $1}')
  [[ "$actual" == "$digest" ]] || { echo "FAIL: $name"; exit 1; }
done

# Загрузка в banking docker-registry
for img in docker-images/**/*.tar; do
  docker load -i "$img"
  docker tag ... bank-registry.local/...
  docker push bank-registry.local/...
done

# SBOM-scan через trivy (или банковский эквивалент)
syft images...   # см. runbook
```

### Активация и откат через tenant-cli

```bash
# Активация
tenant-cli release activate \
  --target=green \
  --version=1.5.0 \
  --tenant=bank-alpha \
  --canary=none           # default — instant 100% switch

# Статус
tenant-cli release status --tenant=bank-alpha
# Output:
#   active: green (1.5.0) since 2026-04-26 02:34
#   standby: blue (1.4.2)
#   migrations: applied 14 of 14
#   audit: hash-chain unbroken (last 192410 events)

# Rollback
tenant-cli release rollback --tenant=bank-alpha
# Output:
#   switching VS to blue (1.4.2)... done
#   audit event 'platform.release_rolled_back' written
```

`tenant-cli` под капотом — kubectl-обёртка, выполняет idempotent `apply` манифестов. Audit-events пишет в Postgres напрямую (через service-аккаунт `platform-cli` с min-privilege).

### Health и smoke-tests

```yaml
# приватный VS для smoke-tests:
# /v1/__smoke/* всегда на green (или указанную версию)
- match:
    - uri: { prefix: "/v1/__smoke" }
    - headers:
        x-release-test: { exact: "true" }
  route:
    - destination: { host: "bff-onboarding.platform-green.svc" }
```

Smoke-набор:

- Health-check каждого сервиса (`/health`).
- Один полный e2e-cycle онбординга на синтетическом тенанте (`__smoke_tenant`).
- Audit-completeness: запись + чтение последнего hash.
- Temporal: список активных workflow в обоих namespace должен совпадать (после activation).

Если smoke fails — `tenant-cli` сам не делает activation, требуется явный `--force-activate`.

### Истанбульская специфика

| Кейс | Mitigation |
|---|---|
| Банк использует Deckhouse (вместо ванильного K8s) | Helm-чарты совместимы, Istio есть в Deckhouse-CE; runbook включает Deckhouse-specific commands |
| Банк не разрешает cluster-admin для нашего CLI | `tenant-cli` работает с ограниченным RBAC (only namespace `platform-mesh` для VS, only namespace `platform-{blue,green}` для read) |
| Банк не разрешает cosign в air-gapped | Включаем cosign-binary в bundle; verification — banking offline-tool с pre-distributed public key |
| Банк требует дополнительной подписи их KMS (ГОСТ-2012) | Параллельная подпись опциональна: `manifest.yaml.gost.sig`; описано в release-process |

### Этапы внедрения

| Этап | Что | Когда |
|---|---|---|
| 1 | Bundle-build pipeline в CI | Phase 7 (Ф7, см. § 12 — первый пилот) |
| 2 | `tenant-cli release activate / rollback / status` | Phase 7 |
| 3 | Runbook'и upgrade.md / rollback.md / verify.md | Phase 7 |
| 4 | Tabletop-exercise с design-partner банком | Phase 7, до первого реального обновления |
| 5 | Первое реальное обновление с инженером on-call | Phase 7+1 квартал |
| 6 | Отладка автоматизации smoke-набора | Continuous |

### Безопасность

- **Подпись bundle**: cosign с приватным ключом нашей релизной CI; ключ ротируется раз в год; revocation list распространяется через release-notes.
- **mTLS** между blue и green namespaces сохраняется через Istio; service identity отличаются (`bff-onboarding.platform-blue` ≠ `bff-onboarding.platform-green`), что позволяет точно отслеживать, какая версия что вызвала в audit log.
- **Vault**: разные service-account для blue и green namespace; rotate credentials при scaling-down старого namespace.
- **Network policy**: blue и green не могут общаться напрямую — это защищает от accidental leakage между versions через side-channels; общение только через `platform-data` и `platform-mesh`.
- **Audit-events**: `platform.release_started`, `platform.release_activated`, `platform.release_rolled_back`, `platform.release_completed` — каждое с `actor` (тот, кто нажал tenant-cli), `from_version`, `to_version`, hash-цепочка с предыдущим event.

## References

- `docs/technical-structure.md` § 1 (hybrid deployment), § 8.1 (изоляция тенантов — K8s namespace), § 10 (развёртывание), § 10.2 (air-gapped bundle — основа этого ADR), § 10.3 (Helm-структура), § 11 (CI/CD, on-prem квартальный цикл, N–2).
- `docs/security-architecture.md` § 4.1 (Istio mTLS), § 4.2 (white-list outbound — ограничение для bundle delivery), § 7.1 (Secure Development Lifecycle — image signing), § 8.3 (audit log непрерывность), § 9.3 (DR — RPO/RTO), § 10.2 (подготовка к ИБ-аудиту банка — SBOM, signed artifacts).
- ADR-0001 (Temporal): Temporal-workflow versioning через `GetVersion`/Patch — обязательная техника при изменении workflow-логики между релизами.
- ADR-0002 (Schema-per-tenant): обновление не затрагивает изоляцию схем; миграции применяются ко всем `tnt_*` схемам через `db-migrator` (ADR-0005).
- ADR-0004 (Nx): bundle-build — Nx-таргет с правильно описанными `inputs`/`outputs`, кеш Nx Replay переиспользует не изменившиеся слои.
- ADR-0005 (Стратегия миграций БД): forward-compatible миграции — фундаментальное предположение этого ADR; политика отката («forward fix в проде») — следствие.
- ADR-0006 (Версионирование ABS-адаптеров): адаптер обновляется независимо или вместе с платформой; bundle включает совместимые версии адаптеров.
- ADR-0010 (Биллинг и audit log): release-events — отдельный класс audit-events, идущих в `platform.billing_events` через `correlation_id` для отчётности.
- ADR-0011 (LLM routing): обновление модели — отдельная процедура внутри `platform-ai` namespace, не часть стандартного blue-green; alias swap для атомарности.
- [Istio VirtualService traffic shifting](https://istio.io/latest/docs/concepts/traffic-management/#virtual-services)
- [cosign — keyless and key-based signing](https://docs.sigstore.dev/cosign/overview/)
- [Helm Chart provenance and integrity](https://helm.sh/docs/topics/provenance/)
- [Temporal Workflow Versioning](https://docs.temporal.io/workflows#workflow-versioning)
