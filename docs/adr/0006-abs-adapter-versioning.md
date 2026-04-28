# ADR-0006: Версионирование ABS-адаптеров — отдельный gRPC-сервис на адаптер с semver и поддержкой N-2

**Status:** Accepted
**Date:** 2026-04-26
**Authors:** Главный архитектор, Тех-лид backend
**Reviewers:** Security-инженер, DevOps, Тех-лид интеграций

## Context

Универсальный коннектор к АБС описан в `docs/technical-structure.md` § 6: `abs-connector` принимает канонические команды и маршрутизирует их в один из адаптеров (`abs-adapter-cft`, `abs-adapter-diasoft`, `abs-adapter-rs-bank`). Адаптер инкапсулирует proprietary-протокол конкретной АБС (SOAP/MQ/SFTP/собственный SDK) и реализует контракт `Translate / Execute / MapResponse / IsIdempotent`.

Силы, давящие на решение о версионировании:

- **Гетерогенный технологический стек адаптеров.** ЦФТ — Go-сервис с собственным HTTP/SOAP-клиентом (`services/abs-adapter-cft/cmd/server/main.go` — слушает `:9001`, plain HTTP, минимальный handler). Diasoft — частично Java 21 (см. § 3.1, «Адаптеры АБС (legacy)»), потому что некоторые их SDK официально только для JVM. RS-Bank — Go или Java, конкретный выбор за командой адаптера. Версионирование должно одинаково работать через границу языка.
- **Разный жизненный цикл АБС у разных банков.** Один банк апгрейдит ЦФТ до новой минорной версии раз в полгода; второй держит фикс-версию RS-Bank годами и не готов мигрировать. Адаптер для банка А должен поддерживать новую модель, адаптер для банка Б — старую, и **они оба собираются из одного релиза платформы**.
- **Изменения в каноническом контракте.** `packages/proto/` (см. § 2 и `default-routing.yaml`-pattern, § 4.2) содержит каноническую модель команд. Изменение канонической команды (например, добавление поля `external_reference_number` в `OpenAccount`) должно быть совместимо со всеми существующими адаптерами без форсирования синхронного апгрейда у банка.
- **Идемпотентность через dedup-store** (§ 6, § 4.3). Дедуп-ключ — это часть контракта; смена семантики идемпотентности — breaking change адаптера, не canon.
- **Golden-tests** с mock-АБС (§ 6, `tools/mock-abs-cft/`, `tools/mock-abs-diasoft/`). Каждый адаптер имеет свой golden-набор; регрессии ловятся в CI. Версионирование адаптера = версионирование его golden-набора.
- **On-prem-цикл квартальный** (§ 11, «On-prem: квартальные релизы со стабилизационным циклом», и ADR-0009). За квартал в одном банке могут одновременно лежать **несколько версий одного адаптера** — потому что банк не готов мигрировать на новую за один релиз. Платформа поддерживает «N–2 версии on-prem одновременно» (§ 11) — это формулируется как «3 поколения протокола адаптера живут одновременно».
- **Регуляторика и audit.** Каждая операция в АБС записывается в audit log (`security-architecture.md` § 8.2, ADR-0010), включая версию адаптера, через который прошла. По 5-летнему сроку хранения это значит: через 4 года расследования инцидента нужно уметь восстановить, какая именно версия адаптера ЦФТ выполнила перевод.
- **Команды адаптеров эволюционируют независимо.** Команда, поддерживающая `abs-adapter-cft`, не должна быть заблокирована релизом `abs-adapter-diasoft`. Это требование к **независимому версионированию**.
- **Существующая закладка.** `services/abs-adapter-cft/cmd/server/main.go` — отдельный исполняемый файл со своим HTTP-портом и lifecycle. Это уже отдельный сервис, ADR кодифицирует «как именно он отдельный».

Если ничего не решить, разные команды примут разные подходы: одна — общий Go-модуль, другая — Java-плагин, третья — gRPC-сервис. Через год оркестрация релизов станет невозможной: неясно, какая версия общей библиотеки совместима с какой версией АБС банка X.

## Decision

Каждый ABS-адаптер — **отдельный сервис со своим go.mod (или Maven/Gradle для Java) и своим жизненным циклом**, с **собственным semantic version** и **gRPC-контрактом к `abs-connector`**. Поддерживаем **N–2 версии протокола одновременно** (3 поколения).

Решение состоит из пяти частей: (1) физическая декомпозиция, (2) контракт через канонические proto, (3) semver per adapter, (4) backwards-compat policy, (5) golden-tests и release pipeline.

### Часть 1. Физическая декомпозиция

```
services/
├── abs-connector/                      # Один на платформу
│   └── go.mod                          # canonical command router
│
├── abs-adapter-cft/                    # Адаптер ЦФТ
│   ├── cmd/server/main.go              # gRPC + HTTP health server (см. main.go: :9001)
│   ├── internal/
│   │   ├── handler/                    # gRPC-сервер (impl AdapterService)
│   │   ├── translator/                 # canonical → CFT-протокол
│   │   ├── client/                     # CFT SOAP/HTTP-клиент
│   │   └── mapper/                     # CFT-ответ → canonical event
│   ├── go.mod                          # независимый модуль
│   ├── VERSION                         # 1.4.2 — semver, читается build-системой
│   └── golden/                         # тесты с mock-CFT
│
├── abs-adapter-diasoft/                # Адаптер Diasoft (Java 21 если требует SDK)
│   ├── src/main/java/...
│   ├── pom.xml / build.gradle
│   ├── VERSION                         # 0.9.5
│   └── golden/
│
└── abs-adapter-rs-bank/                # Адаптер RS-Bank
    ├── cmd/server/main.go
    ├── go.mod
    ├── VERSION
    └── golden/

packages/proto/
└── abs/
    ├── adapter_v1.proto                # gRPC-контракт «adapter side» — канонические команды
    ├── adapter_v2.proto                # следующая major версия (живёт параллельно)
    ├── adapter_v3.proto                # ещё следующая
    └── canonical/                      # типы команд и событий, не привязанные к версии
```

Каждый адаптер — Docker-образ `aibank/abs-adapter-<name>:<adapter-version>`, деплоится Helm-чартом, доступен `abs-connector` по имени сервиса в Kubernetes. На один банк-тенант — **только один активный экземпляр адаптера** (выбранный тенант-конфигом, см. § 9 `configs/tenants/<bank>/integrations/abs.yaml`).

### Часть 2. Контракт через канонические proto

`abs-connector` и адаптер общаются через **канонические команды**, описанные в `packages/proto/abs/`. Это и есть Anti-Corruption Layer (§ 6):

```protobuf
// packages/proto/abs/adapter_v1.proto
syntax = "proto3";
package aibank.abs.adapter.v1;

service AdapterService {
  rpc Execute(CanonicalCommand) returns (CanonicalEvent);
  rpc HealthCheck(HealthRequest) returns (HealthResponse);
  rpc Capabilities(CapabilitiesRequest) returns (CapabilitiesResponse);
}

message CanonicalCommand {
  string idempotency_key = 1;
  string tenant_id = 2;
  oneof payload {
    OpenAccount open_account = 10;
    AttachSignatureCard attach_sig = 11;
    ReserveAccountNumber reserve = 12;
    // ...
  }
  CommandMetadata metadata = 100;
}
```

Внутреннее устройство адаптера — proprietary, но **гибридная модель** обеспечивает:

- Контракт `abs-connector` ↔ адаптер: канонический gRPC из `packages/proto/abs/`.
- Контракт адаптер ↔ АБС банка: SOAP/MQ/SDK конкретной АБС.

`abs-connector` не знает ни одной строчки про SOAP CFT, MQ Diasoft или SFTP RS-Bank. Адаптер — не знает других тенантов, кроме своего активного.

`Capabilities()` — критическая часть: адаптер сообщает, какие версии canonical-API он поддерживает (`["v1", "v2"]`) и какие команды реализует. `abs-connector` использует это для версионного маршрутизирования.

### Часть 3. Semver per adapter

Каждый адаптер имеет **независимую версию** в формате `MAJOR.MINOR.PATCH`:

| Тип изменения | Bump | Примеры |
|---|---|---|
| **PATCH** | Bug-fix без изменения наблюдаемого поведения | Исправление таймаута SOAP-клиента, retry на конкретный код ошибки |
| **MINOR** | Новая функциональность, обратно совместимая | Поддержка нового типа счёта, новая опциональная команда |
| **MAJOR** | Breaking change в контракте адаптер ↔ АБС или адаптер ↔ canonical | Смена canonical-версии (`v1 → v2`), изменение семантики idempotency-key, удаление команды |

Версия читается из `services/abs-adapter-*/VERSION` файла, проставляется в Docker-image-tag, возвращается через `Capabilities()`-RPC, **записывается в audit log на каждую операцию**:

```json
{
  "action": "abs.execute",
  "adapter": "cft",
  "adapter_version": "1.4.2",
  "canonical_version": "v1",
  "command": "OpenAccount",
  "idempotency_key": "..."
}
```

### Часть 4. Backwards-compat policy: N–2

`abs-connector` поддерживает **3 поколения canonical proto одновременно**: `v(N)`, `v(N-1)`, `v(N-2)`. Это значит:

- В момент решения: канонический proto — `v1`. Адаптеры могут поддерживать `v1` (все).
- При выпуске `v2`: новые адаптеры поддерживают `v1 + v2` через `Capabilities()`. Старые — продолжают `v1`. Connector сам выбирает версию исходя из `Capabilities()` адаптера.
- При выпуске `v3`: connector прекращает использовать `v1` для новых команд, но поддерживает `v1` ещё один квартал для уже-в-полёте команд (idempotent retry).

**Правила breaking change в canonical proto:**

- Поле `optional` добавляется без bump major — это MINOR изменение `packages/proto/abs/`.
- Удаление поля → новая major proto-версия (`adapter_v2.proto`).
- Семантический breaking (тот же proto, изменилось значение) — тоже major, через комментарий + новая proto-версия.
- При выпуске новой major — старые адаптеры **продолжают работать на старой proto-версии**, без переписывания.

**Правила breaking change в адаптере:**

- Adapter может bump'ать свой major независимо от canonical proto. Это означает breaking в **внутреннем** контракте (например, Diasoft v3.0 → v4.0 у банка с переходом на новую версию SDK).
- Каждый банк-тенант в `configs/tenants/<bank>/integrations/abs.yaml` фиксирует версию адаптера, на которую он подписан:

```yaml
# configs/tenants/bank-alpha/integrations/abs.yaml
abs:
  adapter: cft
  adapter_version_constraint: "^1.4"   # принимает 1.4.x, 1.5.x; не принимает 2.x
  canonical_version: v1
  endpoint: https://abs-adapter-cft.bank-alpha.svc.cluster.local:9001
  credentials_ref: vault://kv/bank-alpha/abs/cft
```

`abs-connector` при старте валидирует: `adapter.Capabilities() ∋ canonical_version`, иначе `panic` с понятной ошибкой.

### Часть 5. Golden-tests и release pipeline

Каждый адаптер обязан иметь:

1. **Mock АБС** — в `tools/mock-abs-<name>/` (упомянут в § 2). Mock эмулирует proprietary-протокол АБС.
2. **Golden-набор** — пары `(canonical_command, expected_canonical_event)` для каждой версии canonical proto. Хранятся в `services/abs-adapter-*/golden/v1/`, `golden/v2/` и т.д. Изменение содержимого golden-файла = breaking change, требует bump MINOR/MAJOR в зависимости от природы.
3. **CI-job** в Nx (см. ADR-0004): `nx test abs-adapter-cft` запускает testcontainers с mock-АБС, проверяет golden-набор для всех поддерживаемых canonical-версий.
4. **Контрактный тест с `abs-connector`**: `nx run abs-connector:contract-test` поднимает все адаптеры в тестовом режиме и проверяет совместимость их `Capabilities()` с известным набором тенантов.

**Release pipeline** для адаптера:

- Любая правка в `services/abs-adapter-cft/` требует bump `VERSION` (CI-линтер `adapter-version-bump-required` ловит).
- Major bump требует подпись архитектора в PR.
- Релизный PR публикует Docker-образ `aibank/abs-adapter-cft:1.4.2`, обновляет каталог релизов в `infrastructure/helm/services/abs-adapter-cft/values.yaml`.
- Каждый банк-тенант мигрирует на новую версию адаптера на своей скорости через GitOps (Argo CD читает `configs/tenants/<bank>/integrations/abs.yaml`).

### Что НЕ покрывает решение

- **Конкретный API внутри адаптера** (SOAP-схемы CFT, MQ-формат Diasoft) — proprietary, описан в `services/abs-adapter-*/docs/protocol.md`.
- **Стратегия миграции банка с одной major версии адаптера на другую** — операционная задача, отдельный runbook.
- **Sharding адаптеров** (если у банка два инстанса CFT) — не закладываем в MVP, рассмотрим при появлении кейса.
- **Cross-tenant адаптеры** — не существуют по определению: один тенант — один адаптер.

## Alternatives Considered

### Альтернатива 1: Один общий go-модуль для всех адаптеров

**Плюсы:**

- Один `go.mod` — простая навигация и refactoring между адаптерами.
- Shared utility-код (логирование, метрики, retry) переиспользуется без копирования.
- Единый CI-пайплайн.

**Минусы:**

- **Java 21 для Diasoft невозможен** — `go.mod` чужд JVM. Это сразу режет один из адаптеров.
- **Coupling релизов.** Изменение в `abs-adapter-cft` затрагивает зависимости всех адаптеров в общем модуле; команда CFT не может сделать релиз без согласования с командами Diasoft и RS-Bank.
- **Versioning неоднозначен.** Один тег `v1.4.2` на репо — что он означает для трёх адаптеров с разной зрелостью? CFT в production, RS-Bank в alpha, Diasoft в beta — всё в одной версии — путаница.
- **Размер пакета и compile-time** растут с числом адаптеров.

**Причина отклонения:** не закрывает Java-сценарий, делает релизы coupled.

### Альтернатива 2: Один go-модуль на адаптер, но **общий процесс** через Go plugins

**Плюсы:**

- In-process — нет network-hop между connector и адаптером, минимальная latency.
- Один Docker-образ на dev/prod деплой.

**Минусы:**

- **Go plugins** имеют известные проблемы: версии Go должны точно совпадать, glibc-версии должны совпадать, plugin-API статически линкует основной бинарь — расхождение vendored-зависимостей даёт runtime-падение. В banking-проде это отказ.
- **Не решает проблему Java/JVM** для Diasoft.
- **Любой crash адаптера = crash connector**, нет процессной изоляции.
- **Hot-reload плагина** в production — отдельная больная история (Go plugin API не поддерживает unload).
- **Безопасность**: компрометация одного адаптера = компрометация process с доступом ко всем тенантам (`abs-connector` обслуживает многих).

**Причина отклонения:** Go plugins не production-ready для нашей нагрузки и кросс-языковых требований.

### Альтернатива 3: gRPC-сервис каждый адаптер с **monorepo-shared semver** (one version for all)

**Плюсы:**

- gRPC решает language-agnostic проблему.
- Одна цифра версии для всей платформы, проще в коммуникации с банком («у вас платформа 1.4»).

**Минусы:**

- **Coupling релизов через версионирование.** Bug-fix в `abs-adapter-cft` тянет bump для всех адаптеров, потому что версия общая — лишний noise в Diasoft и RS-Bank релизных PR'ах.
- **Невозможно сказать «у банка А версия CFT 1.4.2, у банка Б версия RS-Bank 2.0.1»** — одна версия на платформу скрывает реальную картину.
- **Non-coupled life-cycles** — главная цель решения — нарушены.

**Причина отклонения:** semver per adapter лучше отражает реальность независимых интеграций.

### Альтернатива 4: HTTP/REST вместо gRPC

**Плюсы:**

- Стандартный, понятный, проще тестировать руками.
- `abs-adapter-cft` уже стартует HTTP-сервер на `:9001` (см. `main.go`) — не пришлось бы переписывать.

**Минусы:**

- **Слабая типизация контракта.** OpenAPI хороша, но gRPC + protobuf — строже, codegen надёжнее, evolution-rules более чёткие.
- **Streaming.** Если в будущем потребуется streaming статуса операции (например, длинная open-account) — gRPC даёт это естественно, REST — через SSE/WebSocket с самописной семантикой.
- **Performance.** На внутрикластерных вызовах gRPC + protobuf быстрее REST + JSON в 2–5 раз; для batch open-account это значимо.
- **Канонический proto — единственный источник правды**. Если REST — нужен OpenAPI и proto одновременно или выбор.

**Причина отклонения:** gRPC лучше ложится на наш канонический модель и `packages/proto/`. Текущий HTTP в `main.go` — переходное состояние, плановая миграция на gRPC — часть Implementation Notes этого ADR. Health-эндпоинт остаётся HTTP для удобства K8s-probes.

### Альтернатива 5: Каждый адаптер — Lambda/serverless

**Плюсы:**

- Pay-per-use, нет idle-cost.
- Автомасштабирование.

**Минусы:**

- **Hybrid deployment** (§ 1, § 10.2) запрещает managed-сервисы. On-prem банк не разворачивает Lambda.
- **Cold start** — для SOAP-клиентов с длинной TLS-handshake-цепочкой это +500 мс на первый вызов; в banking SLA значимо.
- **Stateful-клиенты** к АБС (long-lived TLS-сессии, MQ-connection-pool) плохо ложатся на serverless.

**Причина отклонения:** прямое нарушение принципа hybrid deployment.

## Consequences

### Positive

- **Команды эволюционируют независимо.** Bug-fix в CFT — релиз CFT 1.4.3 без касания Diasoft.
- **Херо-уровень изоляции отказов.** Crash adapter-cft не валит adapter-diasoft и не валит connector. Caller в connector видит timeout/error и возвращает clean failure для compensating SAGA-action в Temporal (см. ADR-0001).
- **Полное audit-tracing.** Каждое действие в АБС связано с `(adapter, adapter_version, canonical_version)` — реконструкция через 5 лет всегда возможна, даже если за это время вышло 20 версий адаптера CFT.
- **Гибкость per-tenant.** Один банк на CFT v1.4, другой на CFT v2.0 — оба собраны в одном release-bundle; миграция банка — изменение `configs/tenants/<bank>/integrations/abs.yaml`, не изменение кода.
- **Language-flexibility.** Diasoft на JVM, ЦФТ и RS-Bank на Go — не требует heroic-glue.
- **Естественный путь добавления адаптера.** Новый банк с новой АБС = новый сервис в `services/abs-adapter-<name>/`, новый Helm-чарт, новый golden-набор. Никаких изменений в connector, кроме обновления списка известных адаптеров.
- **Backwards-compat N–2** даёт банкам реальное окно для квартальной миграции (см. ADR-0009): 3 квартала на смену canonical proto major.

### Negative

- **Network hop между connector и адаптером.** mTLS (`security-architecture.md` § 4.1) добавляет ~1–3 мс per-call. Для большинства операций (открытие счёта = 5–15 секунд в АБС) это незаметно, но в batch-сценариях аккумулируется. Mitigation: gRPC-streaming для batch operations, persistent connection.
- **Operational footprint.** В on-prem на банк — 1 экземпляр connector + 1 экземпляр активного адаптера + mock'и в стейджинге. Это +2 pods к минимальному набору. Mitigation: на банк разворачивается только нужный адаптер (CFT — без Diasoft), не все три.
- **Версионная матрица сложнее в коммуникации с банком.** «Какая у вас версия адаптера CFT?» — нужно объяснять, что это не «версия платформы». Mitigation: tenant-cli даёт команду `abs-version --tenant=<id>`, которая показывает полную картину; релиз-ноты включают per-adapter changelog.
- **CI-build множится.** Каждый адаптер — свой docker build, свой test, свой golden-set. Mitigation: Nx affected (см. ADR-0004) — на PR с правкой только CFT не пересобираются Diasoft и RS-Bank.
- **N–2 обязывает поддерживать старые canonical proto** на стороне connector. Это код, который никто не любит трогать, и легко зарасти legacy-debt. Mitigation: explicit deprecation policy с датой EOL, регулярный (раз в полгода) review «какие тенанты на каких версиях».

### Neutral

- **Semver-дисциплина** — это процесс, не technical control. CI ловит «забыл bump VERSION», но не ловит «bump'нул minor вместо major». Mitigation: описание правил в `services/abs-adapter-*/CONTRIBUTING.md`, ревью архитектора на major bumps.
- **Mock'и для каждого адаптера** — отдельный артефакт, который тоже нужно поддерживать. На длинной дистанции окупается через golden-tests, но в первые месяцы — overhead.
- **Существующий HTTP-сервер в `main.go`** становится переходным; план миграции на gRPC — часть Phase 2 (Mock-АБС и коннектор, см. § 12), не в одном PR.
- **Capabilities-discovery** добавляет один RPC при старте connector. Это не runtime-нагрузка, а часть init-flow. Mitigation: cached-результат на 60 секунд, ре-discovery при HUP-сигнале.

## Implementation Notes

### Файл VERSION и build-вход

`services/abs-adapter-cft/VERSION`:

```
1.4.2
```

В `cmd/server/main.go` (расширение существующего `main.go`):

```go
//go:embed VERSION
var version string

func main() {
    log.Printf("abs-adapter-cft %s starting", strings.TrimSpace(version))
    // ... gRPC + HTTP health server
}
```

Docker-build передаёт `VERSION` в image-tag через Nx-таргет `docker` (см. ADR-0004 пример).

### Минимальный gRPC-skeleton (Go)

```go
type adapterServer struct {
    abs.UnimplementedAdapterServiceServer
    cftClient *cft.Client
    version   string
}

func (a *adapterServer) Capabilities(ctx context.Context, _ *abs.CapabilitiesRequest) (*abs.CapabilitiesResponse, error) {
    return &abs.CapabilitiesResponse{
        AdapterName:        "cft",
        AdapterVersion:     a.version,
        CanonicalVersions:  []string{"v1"},
        SupportedCommands:  []string{"OpenAccount", "ReserveAccountNumber", "AttachSignatureCard"},
    }, nil
}

func (a *adapterServer) Execute(ctx context.Context, cmd *abs.CanonicalCommand) (*abs.CanonicalEvent, error) {
    if a.cftClient.IsIdempotent(cmd.IdempotencyKey) {
        return a.cftClient.GetCachedEvent(cmd.IdempotencyKey), nil
    }
    cftCmd, err := translator.Translate(cmd)
    if err != nil { return nil, err }
    cftResp, err := a.cftClient.Execute(ctx, cftCmd)
    if err != nil { return nil, err }
    event := mapper.MapResponse(cftResp)
    a.cftClient.StoreIdempotent(cmd.IdempotencyKey, event)
    return event, nil
}
```

### Capabilities → routing в connector

```go
// services/abs-connector/internal/router/router.go
func (r *Router) Init(ctx context.Context, tenants []TenantConfig) error {
    for _, t := range tenants {
        adapterConn := mustDial(t.Endpoint)
        caps, err := abs.NewAdapterServiceClient(adapterConn).Capabilities(ctx, &abs.CapabilitiesRequest{})
        if err != nil { return fmt.Errorf("tenant %s adapter unreachable: %w", t.ID, err) }

        if !contains(caps.CanonicalVersions, t.CanonicalVersion) {
            return fmt.Errorf("tenant %s wants canonical %s but adapter %s@%s supports %v",
                t.ID, t.CanonicalVersion, caps.AdapterName, caps.AdapterVersion, caps.CanonicalVersions)
        }
        r.tenants[t.ID] = adapterConn
    }
    return nil
}
```

### Golden-test layout

```
services/abs-adapter-cft/golden/
├── v1/
│   ├── open_account_individual.golden.yaml
│   ├── open_account_legal.golden.yaml
│   ├── reserve_number.golden.yaml
│   └── attach_signature_card.golden.yaml
└── v2/
    └── ... (новые кейсы для v2)
```

Файл `open_account_individual.golden.yaml`:

```yaml
input:
  command: OpenAccount
  idempotency_key: "test-key-001"
  tenant_id: "test-tenant"
  payload:
    client_id: "test-client-1"
    account_type: "current_rub"
expected_event:
  status: success
  abs_account_id: "40817810099900012345"
  events:
    - type: ClientCreated
    - type: AccountReserved
    - type: AccountActivated
mock_state_before:
  client_exists: false
mock_state_after:
  client_count: 1
  account_count: 1
```

Runner — `tools/golden-runner/`, поднимает `tools/mock-abs-cft/` в testcontainers, прогоняет golden-файлы, diff-ит реальный output.

### Релиз-pipeline (Nx + GitLab CI)

```
PR с правкой services/abs-adapter-cft/
    ↓
nx affected lint test build
    ↓
adapter-version-bump-required (CI-линтер) — проверяет, что VERSION bumped
    ↓
golden-test:cft (testcontainers с mock-CFT)
    ↓
contract-test (capabilities-discovery с реальным connector)
    ↓
docker-build:cft → image aibank/abs-adapter-cft:1.4.2
    ↓
push в registry
    ↓
PR в configs/tenants/<bank>/integrations/abs.yaml — bump constraint
    ↓
ArgoCD sync per-tenant (см. § 11)
```

### Backwards-compat: пример эволюции

Сценарий: добавляем поле `external_reference_number` в `OpenAccount`.

1. **Решение**: optional поле — это MINOR в `packages/proto/abs/canonical/`. Не требует новой proto-версии.
2. PR в `packages/proto/abs/canonical/open_account.proto`:
   ```protobuf
   message OpenAccount {
       string client_id = 1;
       string account_type = 2;
       string external_reference_number = 3;  // optional, default empty
   }
   ```
3. `abs-connector` после регенерации proto умеет передавать новое поле.
4. Адаптеры, не знающие нового поля, его игнорируют (proto3 forward-compat).
5. Адаптеры, знающие поле, поддерживают его в новой минорной версии (`abs-adapter-cft 1.5.0`).
6. Банки мигрируют на 1.5.0 на своей скорости — feature backwards-compatible.

Сценарий: меняется идемпотентность, теперь dedup-key должен включать `tenant_id`.

1. **Решение**: семантический breaking — MAJOR в canonical proto (`adapter_v2.proto`).
2. Создаём `packages/proto/abs/adapter_v2.proto` с обновлёнными comment-ами и/или новыми типами.
3. `abs-connector` поддерживает оба: `v1` и `v2`.
4. Адаптеры, переписанные на `v2`, объявляют `Capabilities.CanonicalVersions = ["v1", "v2"]`.
5. Каждый банк-тенант в `abs.yaml` указывает желаемую версию; миграция — отдельный PR на банк.
6. Через 2 квартала после выпуска `v2` — `v1` помечен deprecated; через ещё 2 квартала — connector прекращает поддержку `v1` (это уход за пределы N–2, требует отдельного coordinated release).

### Release-cadence

| Тип | Кадетенция | Approver |
|---|---|---|
| PATCH | По мере необходимости | Tech lead адаптера |
| MINOR | Ежемесячно или по нужде | Tech lead + контрактный тест |
| MAJOR | Раз в квартал, синхронно с on-prem-релизом (см. ADR-0009) | Архитектор + tech lead |

### Безопасность

- **mTLS** между `abs-connector` и адаптерами (Istio, см. `security-architecture.md` § 4.1).
- **Service identity** — каждый адаптер имеет SPIFFE-ID; connector проверяет identity при `Capabilities()`-discovery.
- **Outbound от адаптера** — только в банковский АБС, IP-allowlist в network policy (см. § 4.2).
- **Audit log** на каждой операции включает `adapter`, `adapter_version`, `canonical_version`, `idempotency_key` (см. `security-architecture.md` § 8.2).
- **Compromise одного адаптера** не даёт доступ к другим — process isolation + service identity.

### Этапы внедрения

| Этап | Что | Когда |
|---|---|---|
| 1 | Канонический `packages/proto/abs/adapter_v1.proto` + skeleton connector | Phase 2 (Ф2, см. § 12) |
| 2 | `abs-adapter-cft` — переход с HTTP на gRPC, golden-набор для mock-CFT | Phase 2 |
| 3 | `abs-adapter-diasoft` (Java 21) с тем же контрактом | Phase 2 |
| 4 | `abs-adapter-rs-bank` | Phase 4 или позже |
| 5 | CI-линтеры `adapter-version-bump-required`, `proto-compat-check` | Phase 2 |
| 6 | Релизный pipeline для on-prem (см. ADR-0009) | Phase 7 |

## References

- `docs/technical-structure.md` § 1 (hybrid deployment), § 2 (структура монорепо: `services/abs-adapter-*`), § 3.1 (Java 21 для legacy-адаптеров), § 4.3 (Integration Layer), § 6 (универсальный ABS-коннектор — фундамент этого ADR), § 11 (CI/CD, on-prem N–2), § 12 (Phase 2 как точка реализации).
- `docs/security-architecture.md` § 4.1 (mTLS), § 4.2 (outbound в адаптер-зоне), § 8.2 (audit-логирование версии адаптера).
- `services/abs-adapter-cft/cmd/server/main.go` — текущая HTTP-реализация-заглушка, базис для миграции на gRPC.
- ADR-0001 (Temporal): adapter-команды выполняются как Temporal Activities; retry / compensating actions на уровне SAGA не зависят от адаптерной semver — они работают с canonical-командами.
- ADR-0002 (Schema-per-tenant): `tenant_id` в каноне маппится на схему БД для записи `abs_operations` и audit-events; адаптер сам в БД не пишет.
- ADR-0004 (Nx): per-adapter Nx-проекты с независимым `project.json`, affected-detection критичен для CI.
- ADR-0005 (Стратегия миграций): adapter-side миграции (например, dedup-store schema) идут через стандартный `db-migrator`; canonical proto-versioning — отдельный artifact, не БД-миграция.
- ADR-0009 (Стратегия on-prem обновлений): квартальный цикл диктует темп MAJOR bump'ов; blue-green обновления адаптера должны учитывать активные SAGA в Temporal.
- ADR-0010 (Биллинг и audit log): events `abs.execute` с `adapter_version` идут в `billing_events`; биллинг учитывает per-операцию из `product-vision.md` § 5.
- [gRPC](https://grpc.io/) и [Protocol Buffers v3](https://protobuf.dev/) — основной IPC между connector и адаптерами.
- [SemVer 2.0.0](https://semver.org/) — спецификация версионирования.
- [Buf](https://buf.build/) — рекомендован как линтер для канонических proto, проверяет breaking changes.
