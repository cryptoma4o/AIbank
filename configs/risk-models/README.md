# Production Risk Models

Каталог хранения CatBoost-моделей для `services/risk-engine`. Файлы здесь
не коммитятся в Git (большие бинарники в LFS или объектное хранилище);
этот README описывает контракт и pipeline.

## Layout

```
configs/risk-models/
├── README.md                    — этот файл
├── risk-v1.0.0.onnx             — production model v1.0.0 (LFS / S3)
├── risk-v1.0.0.metadata.json    — метаданные обучения
├── risk-v1.1.0.onnx             — следующая версия (candidate)
├── risk-v1.1.0.metadata.json
└── ...
```

## Версионирование

Filename convention: `risk-vMAJOR.MINOR.PATCH.onnx`.

- **MAJOR** меняется при изменении набора фичей (Features struct в `domain/features.go`)
- **MINOR** при существенном переобучении (новый dataset, новые гиперпараметры)
- **PATCH** при перетренировке на том же dataset для устранения багов

Метаданные модели в `*.metadata.json`:

```json
{
  "version": "v1.0.0",
  "trained_at": "2026-04-15T12:00:00Z",
  "trainer": "<имя ML-инженера>",
  "dataset_id": "synthetic-2026-04",
  "dataset_size": 5000,
  "metrics": {
    "auc_roc": 0.87,
    "f1": 0.82,
    "precision": 0.85,
    "recall": 0.80
  },
  "feature_columns": [
    "company_age_years",
    "has_blocked_okved",
    "registration_address_is_mass",
    "ubo_count",
    "has_foreign_owners",
    "capital_kopecks"
  ],
  "training_pipeline_run": "<URL on Grafana / MLFlow>",
  "approved_by": "<банковский эксперт>",
  "promoted_to_production_at": null
}
```

## Pipeline обучения

См. `docs/technical-structure.md § 7.5` (eval-harness и promotion).

1. **Подготовка датасета.** Синтетика из `tools/data-generator/` + размеченные
   реальные кейсы (минимум 100, реалистично 300-1000 для первого пилота).
2. **Обучение.** CatBoost через `python -m catboost.training` или JupyterLab
   notebook в `tools/ml-training/`. Гиперпараметры в YAML, версионируются в Git.
3. **Экспорт в ONNX.** `model.save_model("risk-v1.0.0.onnx", format="onnx",
   export_parameters={"onnx_domain": "ai.catboost", "onnx_model_version": 1})`.
   Важно: указать `--output-tree-stats` если хотим SHAP-values в выходном тензоре.
4. **Eval-harness прогон.** `python -m eval_harness scenarios/risk-scoring`
   на размеченном test-set. Регрессия > 5% по F1 — блокирует promotion.
5. **Owner approval.** Банковский эксперт проверяет 20 случайных кейсов
   на интерпретируемость (SHAP-values имеют смысл, не противоречат правилам).
6. **Promotion.** Файл копируется в production storage; `promoted_to_production_at`
   проставляется в metadata; deploy с переменной `RISK_MODEL_PATH` указывает
   на новый файл.

## Запуск с реальной моделью

```bash
# В deployment manifest или docker-compose:
SCORER=onnx
RISK_MODEL_PATH=/configs/risk-models/risk-v1.0.0.onnx
```

Risk-engine при старте:
1. Читает env, видит `SCORER=onnx`.
2. `scorer.BuildScorer(log)` пытается `NewONNXScorer(path)`.
3. На init-error → fallback на StubScorer + WARN в лог (alerting).

## Status: SKELETON (Phase 2)

`internal/scorer/onnx_scorer.go` — структурный skeleton с TODO. Реальная
ONNX-инференция требует:

- `github.com/yalue/onnxruntime_go` v1.10+ в go.mod
- `onnxruntime_go.InitializeEnvironment()` в process bootstrap
- Session creation и tensor wiring
- SHAP-tensor parsing для real explainability

См. inline TODO в `services/risk-engine/internal/scorer/onnx_scorer.go`.

## Audit и observability

Каждый Score-вызов в production должен попадать в audit log:
- model.name (= "catboost-onboarding")
- model.version (= "risk-v1.0.0", читается из ModelInfo)
- features (полный input vector — для регрессий и репродукции)
- score, factors (output)

Это требование 115-ФЗ + ADR-0010: через год компания должна суметь
объяснить любое автоматическое решение, включая «по какой именно модели
и каким именно входным данным был выставлен этот score».

## TODOs

- [ ] training-scripts в `tools/ml-training/` (Phase 2)
- [ ] dataset-spec YAML с feature dictionary (Phase 2)
- [ ] retraining cadence policy (quarterly? on-drift?)
- [ ] A/B testing скоринга через Temporal Workflow shadowing (Phase 3)
- [ ] PSI / KS drift-detection metrics в Grafana (Phase 3)
