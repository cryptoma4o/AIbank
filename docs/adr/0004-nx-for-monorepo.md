# ADR-0004: Nx как инструмент управления монорепо

**Status:** Accepted  
**Date:** 2026-04-26  
**Authors:** Главный архитектор  
**Reviewers:** Тех-лид backend, Тех-лид frontend, DevOps

## Context

Платформа — монорепо с разнородным стеком (см. `docs/technical-structure.md` § 2):

- **Go 1.22+** — 21 микросервис в `services/` + 8 shared-пакетов в `packages/`
- **Python 3.12** — 7 AI-сервисов в `ai/` + tools-скрипты
- **TypeScript 5+** — 2–4 Next.js приложения в `apps/` + ui-kit + tenant-config-schema
- **Java 21** — отдельные ABS-адаптеры (legacy SOAP/MQ)
- **Protobuf / OpenAPI** — кодогенерация в Go/TS/Python из единого источника (`packages/proto/`, `packages/openapi/`)

Без специализированного оркестратора монорепо проблемы накапливаются быстро:

- **CI «сборка всего на каждый PR».** При 30+ сервисах изменение README одного сервиса прогоняет линт/тесты/сборку всех — десятки минут CI на тривиальный коммит. Через 2 года это становится несовместимо с практикой trunk-based development.
- **Граф зависимостей не виден.** Если меняется `packages/domain-model/`, неясно, какие сервисы должны пересобраться. Ручная навигация по `go.mod`/`package.json`/`requirements.txt` ошибочна.
- **Кодогенерация рассинхронизирована.** Изменение `packages/proto/onboarding.proto` должно переcгенерировать клиенты в Go/TS/Python. Без оркестратора это или ручная процедура (забытый шаг = баг), или GitLab pipeline в 200 строк YAML.
- **Caching.** Один и тот же `go build` на одном и том же коммите выполняется в каждом из 30+ pipeline-job. Без распределённого кеша это килотонны GPU/CPU-минут впустую.
- **Скрипты разработчика разные у каждого сервиса.** «Как запустить тест?» — `cd services/X && go test ./...` для одного, `cd ai/Y && pytest` для второго, `cd apps/Z && npm test` для третьего. Должна быть единая команда уровня репо.

Если ничего не делать, к Фазе 3 (10+ сервисов в активной разработке, ИИ-агенты с кодогеном) команда будет тратить заметную долю времени на инфраструктурные тренья. У нас 18–25 человек целевой команды — каждый процент эффективности критичен.

## Decision

Используем **Nx** как корневой оркестратор монорепо.

- **Версия:** Nx 19.x (latest stable на момент принятия).
- **Workspaces:**
  - Go-проекты (`services/*`, `packages/*` с `go.mod`) подключаются через [`@nx-go/nx-go`](https://github.com/nx-go/nx-go)
  - TypeScript-приложения (`apps/*`) и пакеты (`packages/ui-kit`, `packages/tenant-config-schema/ts/`) — через native Nx generators
  - Python-проекты (`ai/*`, `tools/data-generator/`) — через [`@nxlv/python`](https://github.com/lucasvieirasilva/nx-plugins) (poetry-based) либо через custom run-commands executors
  - Java-проекты (legacy ABS-адаптеры) — через `@jnxplus/nx-gradle` или просто через run-commands к Gradle
- **Cache:** self-hosted Nx Replay поверх MinIO (никаких managed-сервисов, см. принцип hybrid deployment). Nx Cloud SaaS не используем.
- **Affected detection:** CI-job вычисляет `nx affected -t test,lint,build` от base-ветки и запускает только затронутые проекты.
- **Codegen:** генерация из `packages/proto/`, `packages/openapi/`, `packages/domain-model/schema.json` оформляется как Nx-таргет с явными `inputs`/`outputs` для корректной инвалидации кеша.
- **Единые команды разработчика:**
  - `nx test <project>` / `nx run-many -t test`
  - `nx serve <app>` для локального запуска
  - `nx graph` — визуализация графа зависимостей
  - `nx generate` — скаффолдинг новых сервисов по шаблону

**Что НЕ покрывает Nx:**
- Контейнеризация — остаётся на `docker compose` / Helm для прод.
- Деплой в K8s — ArgoCD + Helm.
- Управление инфраструктурой — Terraform/Ansible.

Nx отвечает только за «уровень исходников»: build, test, lint, generate, dep-graph, cache.

## Alternatives Considered

### Альтернатива 1: Bazel

**Плюсы:**
- Hermetic builds — настоящая воспроизводимость до бита. Каждый `bazel build` детерминирован.
- Отличная поддержка Java и proto-кодогенерации (rules_proto, rules_grpc).
- Rules для Go, Python, TypeScript существуют и поддерживаются.
- Sandboxing по умолчанию.
- Distributed builds (RBE) при необходимости.

**Минусы:**
- **Высокий cost of entry.** Каждый каталог требует `BUILD.bazel`-файла; миграция Go-проекта с `go.mod` на `rules_go` нетривиальна (gazelle помогает, но не закрывает всё).
- **Java-центричная экосистема.** Лучшие инструменты Bazel — для JVM-мира; для нашего стека (Go-first + TS + Python) ощущения вторые-третьи.
- **Steep learning curve.** Команде из 18–25 человек, большинство которых не работали с Bazel, потребуется 1–2 месяца для уверенного владения. Это прямой удар по timeline Фаз 1–3.
- **TypeScript-DX страдает.** `rules_nodejs`/`rules_js` в активной перестройке несколько лет; Next.js-сборка через Bazel — отдельный жанр боли.
- **Hermetic-философия конфликтует с реальностью банковских АБС.** Sandboxed-окружение Bazel плохо дружит с проприетарными SDK от КриптоПро/VipNet, которые требуют установки в системный путь.

**Причина отклонения:** инвестиция в Bazel оправдана при ≥1000 разработчиков и ≥10 лет жизни кодовой базы (Google, Pinterest, Uber). Для нас на горизонте 2–3 лет ROI отрицательный — мы заплатим месяцами setup-а и обучения, не получив свойств, которые реально используем.

### Альтернатива 2: Make + bash-скрипты

**Плюсы:**
- Нулевая внешняя зависимость, понятно всем.
- Простые ad-hoc задачи покрываются за 5 минут.

**Минусы:**
- Нет ни кеша, ни affected-detection — основные вещи, ради которых нужен оркестратор монорепо.
- Граф зависимостей выражается вручную, разъезжается с реальностью.
- Кросс-платформенность плохая (Make на macOS и GNU Make различаются).
- Через год превращается в 2000 строк Makefile с вкраплениями bash, который никто не понимает.

**Причина отклонения:** не закрывает основные требования. Сохраняем Make для верхнеуровневых convenience-команд (`make up`, `make logs`, etc.), но не как оркестратор.

### Альтернатива 3: Turborepo

**Плюсы:**
- Очень простой setup для JS/TS-монорепо.
- Хороший cache, affected detection.
- От Vercel — активная разработка.

**Минусы:**
- **JS/TS-первый.** Поддержка Go и Python только через generic «run command», без понимания специфики (sourceRoot, dep-graph по `go.mod`).
- Codegen из proto в Go/Python через Turborepo — это просто wrapped shell-скрипт, без преимуществ перед Make.
- Нет встроенных generator-ов для скаффолдинга новых сервисов в нашей структуре.

**Причина отклонения:** оптимизирован под совсем другую структуру (mono-language JS-приложения). Для нашего polyglot-репо проигрывает Nx по поддержке Go/Python.

### Альтернатива 4: Pants v2

**Плюсы:**
- Сильная Python-поддержка (одна из лучших на рынке среди мульти-языковых билд-систем).
- Hermetic, как Bazel, но с меньшим cost-of-entry.
- Rust-движок, быстрый.

**Минусы:**
- **Слабая TypeScript-поддержка.** Rules для JS/TS только базовые, Next.js не из коробки.
- **Меньшее community.** Меньше плагинов, меньше StackOverflow-ответов.
- Go-поддержка появилась относительно недавно и менее зрелая, чем `@nx-go/nx-go`.

**Причина отклонения:** хорош для Python-первых команд. Наш фронтенд (Next.js + ui-kit + tenant-config-schema TypeScript) важнее, чем выигрыш в Python — Python-сервисов меньше и они изолированы.

### Альтернатива 5: Lerna (или Yarn/pnpm workspaces без оркестратора)

**Плюсы:**
- Лёгкий, многим знаком.

**Минусы:**
- Только JS/TS.
- Lerna в режиме maintenance (Nx купил Lerna в 2022, основная разработка идёт в Nx).

**Причина отклонения:** не закрывает Go и Python.

## Consequences

### Positive

- **Affected detection в CI экономит CI-минуты.** На тривиальных PR (один сервис) — 2–5 минут CI вместо 20–30 без affected-detection. Это компаундирует во времени.
- **Cache переиспользуется между разработчиками и CI.** Self-hosted Nx Replay поверх MinIO означает, что один разработчик собрал сервис → у второго и в CI это уже из кеша.
- **Visualization графа.** `nx graph` показывает всем (включая нетехнических стейкхолдеров) реальную картину зависимостей.
- **Скаффолдинг новых сервисов.** `nx generate @nx-go/nx-go:application <name>` создаёт новый Go-сервис по шаблону за секунды. Это критично для платформы с 21+ сервисом — без шаблонизации у каждого сервиса свой layout.
- **Codegen встроен в граф.** Изменение `.proto` автоматически инвалидирует все потребители; ручная синхронизация не нужна.
- **Nx-cloud-friendly без vendor lock-in.** В будущем можно переключиться на Nx Cloud или остаться на self-hosted.

### Negative

- **Команде нужно изучить Nx.** Минимальный onboarding — 1–2 дня на разработчика. Пик кривой обучения — первые 2–3 недели.
- **Nx иногда «магичен».** При необычных конфигурациях (например, custom Go-проект с нестандартной структурой) приходится разбираться в `executors` и `targets`. Документация хороша, но не всегда покрывает edge cases.
- **Зависимость от плагинов сообщества.** `@nx-go/nx-go`, `@nxlv/python` — сторонние плагины. Если автор бросит — придётся форкать или мигрировать. Mitigation: плагины open-source, миграция на голые run-commands возможна.
- **`nx.json` и `project.json`-файлы в каждом проекте.** Дополнительный layer конфигурации к `go.mod`/`package.json`.
- **Nx Replay self-hosted требует MinIO + минимальной обвязки.** Это уже есть в нашей инфраструктуре, но конфигурацию backup/retention нужно настроить.

### Neutral

- **Версионирование Nx.** Major-апгрейды (раз в полгода) обычно требуют миграционных скриптов (`nx migrate`). Они работают, но требуют внимания на стороне DevOps.
- **Размер репо.** Nx добавляет `node_modules/` в корне репо — около 500 МБ. Не критично, но входит в бюджет дискового пространства dev-машин и CI-runner-ов.
- **Codegen-таргеты должны быть аккуратно описаны.** Без явных `inputs`/`outputs` Nx будет либо перегенерировать всё, либо пропускать обновления. Это однократная инвестиция в правильную конфигурацию.

## Implementation Notes

**Минимальная структура корневых файлов после миграции:**

```
/
├── nx.json                  # глобальная конфигурация Nx + cache settings
├── package.json             # Nx, плагины, root scripts
├── pnpm-workspace.yaml      # для TS-пакетов (или yarn workspaces)
├── tsconfig.base.json
├── go.work                  # Go workspaces для shared dev-experience
├── pyproject.toml           # root pyproject для общих dev-tools (ruff, mypy)
└── tools/nx/
    ├── workspace-tools/     # custom executors при необходимости
    └── codegen-rules/       # обвязка для proto/openapi
```

**Пример `project.json` для Go-сервиса (`services/tenant-service/project.json`):**

```jsonc
{
  "name": "tenant-service",
  "sourceRoot": "services/tenant-service",
  "projectType": "application",
  "tags": ["scope:platform", "lang:go", "tier:core"],
  "targets": {
    "build": {
      "executor": "@nx-go/nx-go:build",
      "outputs": ["{workspaceRoot}/dist/services/tenant-service"]
    },
    "test": { "executor": "@nx-go/nx-go:test" },
    "lint": { "executor": "@nx-go/nx-go:lint" },
    "docker": {
      "executor": "nx:run-commands",
      "options": { "command": "docker build -t aibank/tenant-service services/tenant-service" }
    }
  }
}
```

**CI-job (GitLab):**

```yaml
test:affected:
  stage: test
  script:
    - npx nx affected -t test --base=$CI_MERGE_REQUEST_DIFF_BASE_SHA --parallel=4
```

**Поэтапная миграция (не делаем всё сразу):**

| Этап | Что | Когда |
|---|---|---|
| 1 | Базовый `nx.json`, root `package.json`, импорт TS-приложений (`apps/*`) | Сразу, до Фазы 1 |
| 2 | Подключение Go-сервисов через `@nx-go/nx-go`, единый `nx test`/`nx build` | Параллельно с реализацией первых сервисов в Фазе 1 |
| 3 | Подключение Python (`ai/*`) через `@nxlv/python` или run-commands | Перед Фазой 3 (ИИ-слой) |
| 4 | Codegen-таргеты для `proto/`, `openapi/`, `domain-model/` | Параллельно с этапом 2 |
| 5 | Self-hosted Nx Replay поверх MinIO | После того как накопится 50+ ежедневных CI-job |
| 6 | Custom generator-ы для скаффолдинга новых ABS-адаптеров и AI-агентов | Перед Фазой 4 |

**Ответственность:**
- Корневая Nx-конфигурация — DevOps + тех-лид backend.
- Конфигурация на уровне отдельного сервиса (`project.json`) — owner-команда сервиса.
- Custom executors и generator-ы — DevOps.

## References

- `docs/technical-structure.md` § 2 (структура монорепо), § 11 (CI/CD).
- ADR-0006 (Версионирование ABS-адаптеров) — будет использовать Nx generators для скаффолдинга нового адаптера.
- [Nx documentation](https://nx.dev/)
- [`@nx-go/nx-go`](https://github.com/nx-go/nx-go)
- [`@nxlv/python`](https://github.com/lucasvieirasilva/nx-plugins/tree/main/packages/nx-python)
- [Why Nx instead of Bazel for our scale (внутр. сравнение)](../research/monorepo-tools-comparison.md) (TBD)
