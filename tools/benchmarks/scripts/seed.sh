#!/usr/bin/env bash
# Pre-test data seeder для k6 benchmarks.
#
# Создаёт детерминированный набор тестовых данных:
#   - 5 тенантов (bench-alpha, bench-beta, bench-gamma, bench-delta, bench-epsilon)
#   - Для каждого тенанта: 50 audit-событий (используется GET /v1/events).
#
# Идемпотентен: повторные запуски с теми же id не ломают состояние:
#   - tenant create вернёт 409 при дубликате — игнорируется.
#   - audit events просто добавятся в журнал (append-only by design).
#
# Результаты записываются в tools/benchmarks/.seed-state.json
# для consume benchmark-скриптами (если нужно динамически узнавать id).
#
# Usage:
#   bash tools/benchmarks/scripts/seed.sh
#   TENANT_HOST=http://staging:8080 AUDIT_HOST=http://staging:8081 \
#       bash tools/benchmarks/scripts/seed.sh

set -euo pipefail

# ─── Constants ───────────────────────────────────────────────────────────
readonly TENANT_HOST="${TENANT_HOST:-http://localhost:8080}"
readonly AUDIT_HOST="${AUDIT_HOST:-http://localhost:8081}"

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly BENCH_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly STATE_FILE="${BENCH_ROOT}/.seed-state.json"

# Фиксированный набор tenant_id — должен совпадать с k6 скриптами.
readonly TENANTS=("bench-alpha" "bench-beta" "bench-gamma" "bench-delta" "bench-epsilon")
readonly EVENTS_PER_TENANT="${EVENTS_PER_TENANT:-50}"

# ─── Colors ──────────────────────────────────────────────────────────────
if [ -t 1 ]; then
  C_GREEN=$'\033[0;32m'
  C_YELLOW=$'\033[0;33m'
  C_BLUE=$'\033[0;34m'
  C_RED=$'\033[0;31m'
  C_RESET=$'\033[0m'
else
  C_GREEN=""
  C_YELLOW=""
  C_BLUE=""
  C_RED=""
  C_RESET=""
fi

log()  { printf "%s→%s %s\n" "$C_BLUE" "$C_RESET" "$*"; }
ok()   { printf "  %s✓%s %s\n" "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf "  %s!%s %s\n" "$C_YELLOW" "$C_RESET" "$*"; }
err()  { printf "  %s✗%s %s\n" "$C_RED" "$C_RESET" "$*" >&2; }

# ─── Health check ────────────────────────────────────────────────────────
log "проверка health сервисов: ${TENANT_HOST}, ${AUDIT_HOST}"
if ! curl -fsS --max-time 3 "${TENANT_HOST}/health" >/dev/null 2>&1; then
  err "tenant-service не отвечает на ${TENANT_HOST}/health"
  err "запустите 'make up && make migrate-platform' перед сидингом"
  exit 1
fi
if ! curl -fsS --max-time 3 "${AUDIT_HOST}/health" >/dev/null 2>&1; then
  err "audit-service не отвечает на ${AUDIT_HOST}/health"
  exit 1
fi
ok "сервисы доступны"

# ─── Сидинг тенантов (идемпотентно) ──────────────────────────────────────
log "создаём ${#TENANTS[@]} тенантов..."
created=0
existed=0
for tid in "${TENANTS[@]}"; do
  payload=$(cat <<EOF
{
  "id": "${tid}",
  "name": "Bench Tenant ${tid}",
  "bik": "044525593",
  "inn": "7728168971",
  "deployment_mode": "saas"
}
EOF
)
  status=$(curl -sS -o /dev/null -w "%{http_code}" \
    -X POST "${TENANT_HOST}/v1/tenants" \
    -H "Content-Type: application/json" \
    -d "${payload}")
  case "${status}" in
    201)
      ok "создан тенант ${tid}"
      created=$((created + 1))
      ;;
    409|500)
      # 409 — duplicate; 500 — provisioner может вернуть это при повторном create
      # на уже инициализированной схеме. Оба случая = "уже существует".
      warn "тенант ${tid} уже существует (HTTP ${status})"
      existed=$((existed + 1))
      ;;
    *)
      err "неожиданный статус ${status} при создании ${tid}"
      exit 1
      ;;
  esac
done
ok "тенанты: создано=${created}, существовало=${existed}"

# ─── Сидинг audit-событий ────────────────────────────────────────────────
log "сидинг ${EVENTS_PER_TENANT} событий на тенант (${#TENANTS[@]} тенантов)..."
total_events=0
for tid in "${TENANTS[@]}"; do
  for i in $(seq 1 "${EVENTS_PER_TENANT}"); do
    payload=$(cat <<EOF
{
  "tenant_id": "${tid}",
  "entity_type": "application",
  "entity_id": "app-${tid}-${i}",
  "event_type": "application.created",
  "actor_id": "seed-script",
  "actor_type": "system",
  "payload": {"seed_index": ${i}, "source": "bench-seed"}
}
EOF
)
    status=$(curl -sS -o /dev/null -w "%{http_code}" \
      -X POST "${AUDIT_HOST}/v1/events" \
      -H "Content-Type: application/json" \
      -d "${payload}")
    if [ "${status}" != "201" ]; then
      err "не удалось добавить событие ${tid}/${i} (HTTP ${status})"
      exit 1
    fi
    total_events=$((total_events + 1))
  done
  ok "тенант ${tid}: ${EVENTS_PER_TENANT} событий"
done
ok "всего событий добавлено: ${total_events}"

# ─── Запись seed-state ───────────────────────────────────────────────────
log "запись состояния в ${STATE_FILE}"
timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
# Собираем JSON-массивы.
tenant_json=$(printf '"%s",' "${TENANTS[@]}")
tenant_json="[${tenant_json%,}]"

# Application IDs соответствуют тому, что используется в bff-onboarding-bench.js.
app_ids="["
first=1
for i in $(seq 1 "${EVENTS_PER_TENANT}"); do
  if [ ${first} -eq 1 ]; then
    app_ids="${app_ids}\"app-bench-alpha-${i}\""
    first=0
  else
    app_ids="${app_ids},\"app-bench-alpha-${i}\""
  fi
done
app_ids="${app_ids}]"

cat > "${STATE_FILE}" <<EOF
{
  "seeded_at": "${timestamp}",
  "tenant_host": "${TENANT_HOST}",
  "audit_host": "${AUDIT_HOST}",
  "tenants": ${tenant_json},
  "events_per_tenant": ${EVENTS_PER_TENANT},
  "total_events": ${total_events},
  "application_ids": ${app_ids}
}
EOF
ok "состояние записано"
ok "seed завершён успешно"
