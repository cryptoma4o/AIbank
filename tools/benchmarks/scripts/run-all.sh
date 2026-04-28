#!/usr/bin/env bash
# Запускает все k6 benchmarks по очереди и сравнивает с baseline.
#
# Outputs JSON в tools/benchmarks/results/<timestamp>/<service>.json.
# Сравнение с baselines/<service>.json: regression > 25% по p(95) → exit 1.
#
# Color-coded output:
#   - зелёный = pass (latency в пределах baseline)
#   - жёлтый  = regression < 25% (warning, не падает)
#   - красный = regression > 25% (FAIL, exit 1)
#
# Usage:
#   bash tools/benchmarks/scripts/run-all.sh
#   BASE_URL=http://staging:8080 bash tools/benchmarks/scripts/run-all.sh
#
# Required env tools:
#   - k6 (https://k6.io/docs/getting-started/installation/)
#   - jq

set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly BENCH_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly K6_DIR="${BENCH_ROOT}/k6"
readonly BASELINES_DIR="${BENCH_ROOT}/baselines"
readonly RESULTS_ROOT="${BENCH_ROOT}/results"

readonly TIMESTAMP=$(date -u +"%Y%m%dT%H%M%SZ")
readonly RESULTS_DIR="${RESULTS_ROOT}/${TIMESTAMP}"

# Допустимая регрессия по p(95) latency.
readonly REGRESSION_THRESHOLD_PCT="${REGRESSION_THRESHOLD_PCT:-25}"

# ─── Colors ──────────────────────────────────────────────────────────────
if [ -t 1 ]; then
  C_GREEN=$'\033[0;32m'
  C_YELLOW=$'\033[0;33m'
  C_RED=$'\033[0;31m'
  C_BLUE=$'\033[0;34m'
  C_RESET=$'\033[0m'
else
  C_GREEN=""
  C_YELLOW=""
  C_RED=""
  C_BLUE=""
  C_RESET=""
fi

log()    { printf "%s→%s %s\n" "$C_BLUE" "$C_RESET" "$*"; }
ok()     { printf "  %sPASS%s %s\n" "$C_GREEN" "$C_RESET" "$*"; }
warn()   { printf "  %sWARN%s %s\n" "$C_YELLOW" "$C_RESET" "$*"; }
fail()   { printf "  %sFAIL%s %s\n" "$C_RED" "$C_RESET" "$*"; }

# ─── Pre-flight checks ───────────────────────────────────────────────────
if ! command -v k6 >/dev/null 2>&1; then
  printf "%sERROR%s: k6 не установлен. См. https://k6.io/docs/getting-started/installation/\n" \
    "$C_RED" "$C_RESET" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  printf "%sERROR%s: jq не установлен (нужен для парсинга результатов).\n" \
    "$C_RED" "$C_RESET" >&2
  exit 1
fi

mkdir -p "${RESULTS_DIR}"

# ─── Список benchmark-сценариев ──────────────────────────────────────────
# Формат: "<имя_файла_без_.js>:<имя_baseline_файла>"
BENCHMARKS=(
  "tenant-service-bench:tenant-service"
  "audit-service-bench:audit-service"
  "llm-gateway-bench:llm-gateway-mock"
  "bff-onboarding-bench:bff-onboarding"
)

OVERALL_EXIT=0

# ─── Запуск каждого benchmark ────────────────────────────────────────────
for entry in "${BENCHMARKS[@]}"; do
  bench_name="${entry%%:*}"
  baseline_name="${entry#*:}"
  bench_file="${K6_DIR}/${bench_name}.js"
  result_file="${RESULTS_DIR}/${baseline_name}.json"
  baseline_file="${BASELINES_DIR}/${baseline_name}.json"

  log "запуск ${bench_name}..."
  if [ ! -f "${bench_file}" ]; then
    fail "${bench_file} не найден"
    OVERALL_EXIT=1
    continue
  fi

  # k6 пишет JSON-summary через --summary-export.
  if ! k6 run \
    --summary-export="${result_file}" \
    --quiet \
    "${bench_file}"; then
    fail "${bench_name}: k6 завершился с ошибкой (thresholds или runtime)"
    OVERALL_EXIT=1
    # Продолжаем — пусть остальные сценарии тоже отработают.
  fi

  # ─── Извлечение метрик из summary ──────────────────────────────────────
  if [ ! -f "${result_file}" ]; then
    fail "${bench_name}: summary-файл не создан"
    OVERALL_EXIT=1
    continue
  fi

  measured_p95=$(jq -r '.metrics.http_req_duration["p(95)"] // empty' "${result_file}")
  measured_p99=$(jq -r '.metrics.http_req_duration["p(99)"] // empty' "${result_file}")
  measured_rps=$(jq -r '.metrics.http_reqs.rate // empty' "${result_file}")

  if [ -z "${measured_p95}" ]; then
    warn "${bench_name}: не удалось извлечь p(95) — формат summary неожиданный"
    continue
  fi

  printf "    measured: p95=%.1fms  p99=%.1fms  rps=%.1f\n" \
    "${measured_p95}" "${measured_p99:-0}" "${measured_rps:-0}"

  # ─── Сравнение с baseline ──────────────────────────────────────────────
  if [ ! -f "${baseline_file}" ]; then
    warn "${bench_name}: baseline (${baseline_file}) отсутствует — пропускаем сравнение"
    continue
  fi

  baseline_p95=$(jq -r '.p95_ms' "${baseline_file}")
  baseline_p99=$(jq -r '.p99_ms' "${baseline_file}")
  baseline_rps=$(jq -r '.rps_min' "${baseline_file}")

  # regression_pct = (measured - baseline) / baseline * 100
  regression_pct=$(awk -v m="${measured_p95}" -v b="${baseline_p95}" \
    'BEGIN { printf "%.1f", (m - b) / b * 100 }')

  printf "    baseline: p95=%.1fms  p99=%.1fms  rps=%.1f\n" \
    "${baseline_p95}" "${baseline_p99}" "${baseline_rps}"
  printf "    delta p95: %s%%\n" "${regression_pct}"

  is_regression=$(awk -v r="${regression_pct}" -v t="${REGRESSION_THRESHOLD_PCT}" \
    'BEGIN { print (r > t) ? "yes" : "no" }')
  is_warn=$(awk -v r="${regression_pct}" 'BEGIN { print (r > 0) ? "yes" : "no" }')

  if [ "${is_regression}" = "yes" ]; then
    fail "${bench_name}: регрессия ${regression_pct}% > ${REGRESSION_THRESHOLD_PCT}% — FAIL"
    OVERALL_EXIT=1
  elif [ "${is_warn}" = "yes" ]; then
    warn "${bench_name}: latency выросла на ${regression_pct}% (в пределах ${REGRESSION_THRESHOLD_PCT}%)"
  else
    ok "${bench_name}: latency в пределах baseline (${regression_pct}%)"
  fi
done

# ─── Итог ────────────────────────────────────────────────────────────────
echo
log "результаты сохранены в ${RESULTS_DIR}/"
if [ "${OVERALL_EXIT}" -eq 0 ]; then
  printf "%sALL BENCHMARKS PASSED%s\n" "$C_GREEN" "$C_RESET"
else
  printf "%sSOME BENCHMARKS FAILED%s — см. вывод выше\n" "$C_RED" "$C_RESET"
fi
exit ${OVERALL_EXIT}
