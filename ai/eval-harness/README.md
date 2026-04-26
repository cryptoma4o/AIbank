# AI Eval Harness

Evaluation infrastructure for AIbank AI agents. Runs before any agent goes to production.

## Purpose

Measures quality of document parsing, RAG retrieval, UBO graph extraction, and reconciliation agents
against curated datasets. Integrated into CI to block regressions.

## Structure

```
eval-harness/
  runners/        # BatchRunner: loads datasets, drives agent stubs, collects results
  metrics/        # F1 (document fields), RAGAS-style faithfulness + relevancy
  datasets/       # Curated test cases per domain (git-tracked fixtures)
    docs-parsing/       # Passport, INN certificates, UPD scans
    reconciliation/     # Payment matching cases
    ubo-graphs/         # Beneficial ownership extraction
    ru-banking-chat/    # Russian-language chat intent classification
    rag-quality/        # Retrieval quality over regulatory corpus
    adversarial/        # Edge cases, adversarial inputs
  ci/             # eval_pr_check.py — fast subset runner for pull requests
```

## Running

```bash
# Full eval suite
python -m pytest ai/eval-harness/ -v

# Fast PR subset (used in CI)
python ai/eval-harness/ci/eval_pr_check.py --dataset-dir ai/eval-harness/datasets/docs-parsing
```

## Adding a Dataset

Place `.json` case files under the appropriate `datasets/` subdirectory.
Each case must have `input` and `expected_output` keys.

## Regression Threshold

CI fails if field-level F1 drops more than **5 percentage points** versus baseline stored in `ci/baseline.json`.
