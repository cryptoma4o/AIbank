# tools/benchmarks — k6 perf-тесты AIbank

Performance baselines для критических сервисов платформы. Запускаются вручную
разработчиком и в pipeline `perf-test` (deferred — k6 ещё не установлен в CI).

## Зачем

- Зафиксировать baseline latency/throughput для критических сервисов.
- Ловить регрессии до prod-rollout: `run-all.sh` падает при > 25% регрессии p(95).
- Дать метрики для capacity planning (см. `docs/product-vision.md` § 6).

## Сценарии

| Файл                              | Сервис             | VU peak | Длительность | Описание                            |
|-----------------------------------|--------------------|---------|--------------|-------------------------------------|
| `k6/tenant-service-bench.js`      | tenant-service     | 100     | ~120s        | Read-heavy CRUD                     |
| `k6/audit-service-bench.js`       | audit-service      | 500     | ~210s        | High-traffic ingestion (append-only) |
| `k6/llm-gateway-bench.js`         | llm-gateway (mock) | 50      | ~105s        | Overhead роутинга в mock-режиме     |
| `k6/bff-onboarding-bench.js`      | bff-onboarding     | 100     | ~105s        | GraphQL composite (60/30/10)        |

Распределение операций и SLA targets — в шапке каждого `.js` файла.

## Как запустить локально

```bash
# 1. Установить k6 (один раз):
#    macOS:  brew install k6
#    Linux:  https://k6.io/docs/getting-started/installation/
#    также нужен jq.

# 2. Поднять стек.
make up && make migrate-platform

# 3. Засеять тестовые данные (5 тенантов, 250 audit-событий).
bash tools/benchmarks/scripts/seed.sh

# 4. Запустить все benchmarks с автосравнением с baseline.
bash tools/benchmarks/scripts/run-all.sh

# или через Makefile:
make bench-seed
make bench
```

### Запуск отдельного сценария

```bash
make bench-tenant   # tenant-service
make bench-audit    # audit-service
make bench-llm      # llm-gateway (mock mode)
make bench-bff      # bff-onboarding GraphQL
```

### Переменные окружения

| Переменная                   | По умолчанию              | Описание                              |
|------------------------------|---------------------------|---------------------------------------|
| `BASE_URL`                   | `http://localhost:<port>` | Хост сервиса (CI overrides)           |
| `TENANT_HOST`                | `http://localhost:8080`   | для seed.sh                            |
| `AUDIT_HOST`                 | `http://localhost:8081`   | для seed.sh                            |
| `LLM_GATEWAY_FORCE_MOCK`     | (server-side env)         | для llm-gateway-bench (на стороне docker compose) |
| `MOCK_JWT`                   | (encoded mock)            | для bff-onboarding-bench               |
| `EVENTS_PER_TENANT`          | `50`                       | для seed.sh                            |
| `REGRESSION_THRESHOLD_PCT`   | `25`                       | порог fail в run-all.sh                |

## Baselines

`baselines/<service>.json` — целевые цифры из `docs/product-vision.md` § 6.
**Это TARGETS, а не measured numbers** — после первой реальной прогонки на
staging обновите числами с production-grade инфраструктуры.

### Как обновить baseline

1. Запустить `run-all.sh` несколько раз на стабильном стенде (warm cache).
2. Взять p(95) и p(99) из `results/<timestamp>/<service>.json`.
3. Округлить вверх с запасом 10-15% (нужен буфер на стохастический jitter).
4. Обновить файл в `baselines/<service>.json` и закоммитить с обоснованием в PR
   (rationale: например, «cache-hit ratio упал после migrate-tenant-pool»).

### Структура baseline JSON

```json
{
  "service": "<name>",
  "p95_ms": 100,
  "p99_ms": 250,
  "rps_min": 200,
  "source": "<источник цифры>",
  "rationale": "<почему именно столько>",
  "kind": "target"  // или "measured" после реальной прогонки
}
```

## CI

k6 пока **не установлен** в default CI workflow — perf-tests запускаются вручную
или в отдельном pipeline `perf-test` (см. `.github/workflows/perf.yml` после
включения соответствующего стейджа). На localhost запускайте перед PR в `main`,
если меняете hot path сервиса.

## Структура каталога

```
tools/benchmarks/
├── README.md                  # это
├── .gitignore                 # results/ и .seed-state.json
├── k6/
│   ├── tenant-service-bench.js
│   ├── audit-service-bench.js
│   ├── llm-gateway-bench.js
│   └── bff-onboarding-bench.js
├── scripts/
│   ├── seed.sh                # засевает 5 тенантов + 250 audit-событий
│   └── run-all.sh             # запускает все 4 сценария + diff vs baseline
├── baselines/                 # целевые SLA-цифры (commit-им)
│   ├── tenant-service.json
│   ├── audit-service.json
│   ├── llm-gateway-mock.json
│   └── bff-onboarding.json
├── results/                   # gitignored — output run'ов
└── .seed-state.json           # gitignored — состояние последнего seed.sh
```

## TODOs

- `xk6-output-prometheus-remote` для прямой выгрузки метрик в Prometheus —
  будет нужно когда подключим Grafana dashboards к benchmark-runs.
- Load test из реального гео (CDN, multi-region) — отдельный сценарий.
- Chaos engineering tests (kill-pod в середине прогона) — нужен chaos-mesh.
