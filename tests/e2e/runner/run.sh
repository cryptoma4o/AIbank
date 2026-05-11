#!/usr/bin/env bash
# E2E runner — прогон API и UI тестов, агрегация в .omc/e2e-results.jsonl.
#
# Окружение:
#   E2E_BASE_URL  — http://localhost (default) или http://206.204.106.28
#   E2E_TENANT    — demo (default)
#   E2E_SKIP_UI   — 1 чтобы пропустить Playwright (для быстрых API-only прогонов)
#   E2E_RESULTS   — путь к .omc/e2e-results.jsonl (default: .omc/e2e-results.jsonl)
#
# Exit codes:
#   0 — все тесты прошли
#   1 — хотя бы один тест упал (сам runner всегда успевает дозаписать в jsonl)
#   2 — внутренняя ошибка runner'а (тесты вообще не запустились)

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$REPO_ROOT"

API_REPORT="/tmp/aibank-e2e-api.json"
UI_REPORT="/tmp/aibank-e2e-ui.json"
RESULTS_PATH="${E2E_RESULTS:-.omc/e2e-results.jsonl}"

mkdir -p "$(dirname "$RESULTS_PATH")"

api_status=0
ui_status=0

echo "=== [1/3] API tests (pytest) ==="
if ! python3 -m pytest tests/e2e/api \
  --json-report \
  --json-report-file="$API_REPORT" \
  --json-report-omit=collectors,keywords \
  -v; then
  api_status=$?
  echo "(api: pytest exit=$api_status)"
fi

if [ "${E2E_SKIP_UI:-0}" = "1" ]; then
  echo ""
  echo "=== [2/3] UI tests (Playwright) — SKIPPED (E2E_SKIP_UI=1) ==="
  echo "{}" > "$UI_REPORT"
else
  echo ""
  echo "=== [2/3] UI tests (Playwright) ==="
  if [ -d "tests/e2e/ui/node_modules" ]; then
    pushd tests/e2e/ui >/dev/null
    PLAYWRIGHT_JSON_OUTPUT_NAME="$UI_REPORT" npx playwright test --reporter=json > "$UI_REPORT" 2> /tmp/aibank-e2e-ui.stderr
    ui_status=$?
    if [ "$ui_status" -ne 0 ]; then
      echo "(ui: playwright exit=$ui_status)"
      cat /tmp/aibank-e2e-ui.stderr >&2 || true
    else
      echo "(ui: playwright OK)"
    fi
    popd >/dev/null
  else
    echo "(ui: tests/e2e/ui/node_modules не найден — пропуск)"
    echo "{}" > "$UI_REPORT"
  fi
fi

echo ""
echo "=== [3/3] Aggregating to $RESULTS_PATH ==="
python3 tests/e2e/runner/report.py \
  --api-json "$API_REPORT" \
  --ui-json "$UI_REPORT" \
  --output "$RESULTS_PATH" \
  || {
    echo "report.py failed" >&2
    exit 2
  }

if [ "$api_status" -ne 0 ] || [ "$ui_status" -ne 0 ]; then
  echo "RESULT: FAILED (api=$api_status, ui=$ui_status)"
  exit 1
fi

echo "RESULT: OK"
exit 0
