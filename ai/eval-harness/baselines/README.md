# Eval Baselines

Эталонные метрики качества AI-агентов, против которых проверяется регрессия
при PR в `ai/`. Регрессия > 5 percentage points (per ADR-0011) блокирует merge.

## Зачем нужно

Когда у нас будет live-vLLM (lead-time 2-3 недели на GPU, см.
`docs/pilot-readiness.md` § 1.3), переключение с mock-backend на реальные
модели (Gemma 4, Qwen 3.5, T-pro) — это рискованная операция: модель может
снизить качество vs mock-baseline.

Baseline'ы фиксируют **что считалось приемлемым качеством** на момент N.

## Структура

```
baselines/
├── README.md                          # этот файл
├── mock-2026-04-28.json               # baseline на mock-backend
└── live-vllm-<release-tag>.json       # baseline на live-моделях (заполнится после деплоя GPU)
```

Имя файла: `<backend>-<date>.json` или `<backend>-<release-tag>.json`.

## Формат файла

```json
{
  "backend": "mock",                        // "mock" | "vllm-prod" | etc.
  "model_routing": {                        // снапшот routing config
    "version": 1,
    "roles": {"document-vision": "mock-fast", ...}
  },
  "fixed_at": "2026-04-28",
  "fixed_by": "claude-flow agent (Цикл 3)",
  "datasets": {
    "docs-parsing": {
      "case_count": 50,
      "metrics": {
        "field_coverage_f1": 0.0,           // 0..1
        "exact_match_rate": 0.0,
        "avg_latency_ms": 0,
        "p95_latency_ms": 0
      }
    },
    "reconciliation": {...},
    "ubo-graphs": {...},
    "ru-banking-chat": {...},
    "rag-quality": {
      "case_count": 20,
      "metrics": {
        "ragas_faithfulness": 0.0,
        "ragas_relevancy": 0.0,
        "answer_correctness": 0.0
      }
    },
    "adversarial": {
      "case_count": 15,
      "metrics": {
        "refusal_rate": 0.0,                // должно быть высоко
        "leak_rate": 0.0                    // должно быть низко
      }
    }
  }
}
```

## Как зафиксировать новый baseline

```bash
# 1. Запустить full eval-suite на нужном backend'е
cd ai/eval-harness
LLM_GATEWAY_FORCE_MOCK=1 \
  python -m harness.cli run --all-datasets \
  --output ../eval-harness/baselines/mock-$(date +%F).json

# 2. Code review JSON: метрики разумные, не 0.0 везде
# 3. Commit + push с описанием — что за релиз, какой backend, какие модели
```

## Как сравнить регрессию

```bash
python -m harness.cli compare \
  --baseline baselines/mock-2026-04-28.json \
  --current  /tmp/current-eval.json \
  --threshold-pp 5
# Exit code 0 — passed; 1 — регрессия > 5 п.п. на любой метрике
```

CI автоматически вызывает это в `.gitlab-ci.yml` stage `eval-fast` для PR в `ai/`.

## Текущий статус (2026-04-28)

`mock-2026-04-28.json` — **placeholder**, не заполнен реальными значениями.
Нужно запустить eval-harness и зафиксировать **до** первого live-vLLM деплоя,
чтобы было с чем сравнивать. Это часть подготовки к переходу на GPU
(см. план в `~/.claude/plans/woolly-dancing-thompson.md` Цикл 3).

> ⚠️ Запуск через локальный python требует установленных pytest +
> packages/audit-sdk/python (см. ai/llm-gateway/pyproject.toml dev/audit
> sections). На pre-MVP CI делает это автоматически.

## Связанные документы

- ADR-0011 — LLM routing strategy (regression threshold = 5 п.п.)
- ADR-0012 — Eval-corpus governance (per-dataset owner, drift detection)
- `docs/pilot-readiness.md` § 1.3 — ML/AI блокеры пилота
