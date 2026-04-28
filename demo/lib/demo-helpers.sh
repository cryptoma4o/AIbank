#!/usr/bin/env bash
# demo-helpers.sh — общие функции для demo-сценариев.
#
# Не самостоятельный исполняемый — sourced из scenarios/*.sh.
# Использует HTTP API сервисов через api-gateway (порт 8000) или
# напрямую к bff-onboarding (8091) и bff-admin (8092).

# ── Cross-scenario constants ──────────────────────────────────────────────
readonly API_GATEWAY="${API_GATEWAY:-http://localhost:8000}"
readonly BFF_ONBOARDING="${BFF_ONBOARDING:-http://localhost:8091}"
readonly BFF_ADMIN="${BFF_ADMIN:-http://localhost:8092}"
readonly TENANT_SERVICE="${TENANT_SERVICE:-http://localhost:8080}"
readonly IDENTITY_SERVICE="${IDENTITY_SERVICE:-http://localhost:8082}"
readonly AUDIT_SERVICE="${AUDIT_SERVICE:-http://localhost:8081}"
readonly ONBOARDING_ORCHESTRATOR="${ONBOARDING_ORCHESTRATOR:-http://localhost:8085}"

readonly C_GREEN=$'\033[0;32m'
readonly C_RED=$'\033[0;31m'
readonly C_YELLOW=$'\033[0;33m'
readonly C_BLUE=$'\033[0;34m'
readonly C_RESET=$'\033[0m'

# ── Output helpers ────────────────────────────────────────────────────────
scenario_header() {
  local num="$1" desc="$2" seed="$3"
  echo
  echo "${C_BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${C_RESET}"
  echo "${C_BLUE}Demo сценарий $num:${C_RESET} $desc"
  echo "${C_BLUE}Seed:${C_RESET} $seed"
  echo "${C_BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${C_RESET}"
}

scenario_complete() {
  local num="$1" app_id="$2"
  echo
  echo "${C_GREEN}✓ Сценарий $num завершён${C_RESET}: application_id=$app_id"
  echo "  Просмотреть полный audit-trail: ./tools/audit-verifier verify --tenant-id=demo --application-id=$app_id"
}

# ── Tenant management ────────────────────────────────────────────────────
ensure_tenant() {
  local tenant_id="$1"
  local code
  code=$(curl -s -o /dev/null -w "%{http_code}" "$TENANT_SERVICE/v1/tenants/$tenant_id")
  if [ "$code" = "404" ]; then
    echo "  → создаём tenant $tenant_id"
    curl -sS -X POST "$TENANT_SERVICE/v1/tenants" \
      -H "Content-Type: application/json" \
      -d @"$SCRIPT_DIR/../seed/tenant-demo-bank.json" >/dev/null
  fi
}

# ── Auth ─────────────────────────────────────────────────────────────────
create_applicant_token() {
  local tenant_id="$1" applicant_id="$2"
  curl -sS -X POST "$IDENTITY_SERVICE/v1/applicants/login" \
    -H "Content-Type: application/json" \
    -d "{\"tenant_id\":\"$tenant_id\",\"applicant_id\":\"$applicant_id\"}" |
    jq -r '.access_token'
}

get_admin_token() {
  curl -sS -X POST "$IDENTITY_SERVICE/v1/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"demo-admin","password":"demo-password"}' |
    jq -r '.access_token'
}

# ── Application lifecycle ────────────────────────────────────────────────
create_application() {
  local seed="$1" token="$2"
  local body
  body=$(jq -c '{applicant: .applicant, products: .products}' "$seed")
  curl -sS -X POST "$BFF_ONBOARDING/graphql" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"query\":\"mutation(\$input: ApplicationInput!) { createApplication(input: \$input) { id state } }\",\"variables\":{\"input\":$body}}" |
    jq -r '.data.createApplication.id'
}

trigger_workflow() {
  local app_id="$1"
  curl -sS -X POST "$ONBOARDING_ORCHESTRATOR/v1/workflows/$app_id/start" >/dev/null
}

upload_demo_documents() {
  local app_id="$1" seed="$2" token="$3"
  jq -c '.documents[]' "$seed" | while read -r doc; do
    local type filename
    type=$(echo "$doc" | jq -r '.type')
    filename=$(echo "$doc" | jq -r '.filename')
    # Generate synthetic content — реальные PDF делает tools/data-generator
    local tmpfile
    tmpfile=$(mktemp -t "demo-$type.XXXXXX.pdf")
    echo "%PDF-1.4 demo $type" > "$tmpfile"  # заглушка — для real demo используй data-generator
    curl -sS -X POST "$BFF_ONBOARDING/v1/applications/$app_id/documents" \
      -H "Authorization: Bearer $token" \
      -F "type=$type" \
      -F "file=@$tmpfile;filename=$filename" >/dev/null
    rm -f "$tmpfile"
  done
}

signal_documents_uploaded() {
  local app_id="$1"
  curl -sS -X POST "$ONBOARDING_ORCHESTRATOR/v1/workflows/$app_id/signal/documents_uploaded" >/dev/null
}

# ── State polling ───────────────────────────────────────────────────────
wait_for_state() {
  local app_id="$1" target_state="$2" timeout="${3:-30}"
  local elapsed=0
  while [ "$elapsed" -lt "$timeout" ]; do
    local state
    state=$(curl -sS "$BFF_ONBOARDING/graphql" \
      -H "Content-Type: application/json" \
      -d "{\"query\":\"{ application(id: \\\"$app_id\\\") { state } }\"}" |
      jq -r '.data.application.state // ""')
    if [ "$state" = "$target_state" ]; then
      echo "  ${C_GREEN}✓${C_RESET} state=$target_state"
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  echo "  ${C_RED}✗${C_RESET} timeout waiting for state=$target_state (got '$state')"
  return 1
}

# ── Decision queries ─────────────────────────────────────────────────────
get_decision() {
  local app_id="$1"
  curl -sS "$BFF_ONBOARDING/graphql" \
    -H "Content-Type: application/json" \
    -d "{\"query\":\"{ application(id: \\\"$app_id\\\") { decision { type } } }\"}" |
    jq -r '.data.application.decision.type'
}

get_risk_category() {
  local app_id="$1"
  curl -sS "$BFF_ONBOARDING/graphql" \
    -H "Content-Type: application/json" \
    -d "{\"query\":\"{ application(id: \\\"$app_id\\\") { riskAssessment { category } } }\"}" |
    jq -r '.data.application.riskAssessment.category'
}

get_account() {
  local app_id="$1"
  curl -sS "$BFF_ONBOARDING/graphql" \
    -H "Content-Type: application/json" \
    -d "{\"query\":\"{ application(id: \\\"$app_id\\\") { account { number bik } } }\"}" |
    jq -r '.data.application.account | "\(.number) (БИК \(.bik))"'
}

get_client_decision_message() {
  local app_id="$1"
  curl -sS "$BFF_ONBOARDING/graphql" \
    -H "Content-Type: application/json" \
    -d "{\"query\":\"{ application(id: \\\"$app_id\\\") { decision { clientMessage } } }\"}" |
    jq -r '.data.application.decision.clientMessage'
}

get_internal_decline_reason() {
  local app_id="$1"
  curl -sS "$BFF_ADMIN/graphql" \
    -H "Authorization: Bearer $(get_admin_token)" \
    -H "Content-Type: application/json" \
    -d "{\"query\":\"{ application(id: \\\"$app_id\\\") { decision { internalReason } } }\"}" |
    jq -r '.data.application.decision.internalReason'
}

# ── Operator actions ────────────────────────────────────────────────────
operator_decision() {
  local app_id="$1" decision="$2" comment="$3" admin_token="$4"
  curl -sS -X POST "$BFF_ADMIN/graphql" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $admin_token" \
    -d "{\"query\":\"mutation { updateApplicationDecision(id: \\\"$app_id\\\", decision: \\\"$decision\\\", comment: \\\"$comment\\\") { id } }\"}" >/dev/null
}

# ── Audit ────────────────────────────────────────────────────────────────
check_audit_events() {
  local app_id="$1"
  shift
  local expected=("$@")
  local events
  events=$(curl -sS "$AUDIT_SERVICE/v1/events?tenant_id=demo&entity_type=application&entity_id=$app_id&limit=100" |
    jq -r '.items[].event_type')
  for ev in "${expected[@]}"; do
    if echo "$events" | grep -q "^$ev$"; then
      echo "  ${C_GREEN}✓ audit:$ev${C_RESET}"
    else
      echo "  ${C_YELLOW}⚠ audit-event missing: $ev${C_RESET}"
    fi
  done
}

# ── Assertions ───────────────────────────────────────────────────────────
assert_eq() {
  local actual="$1" expected="$2"
  if [ "$actual" = "$expected" ]; then return 0; fi
  echo "  ${C_RED}✗ assertion failed: expected '$expected', got '$actual'${C_RESET}"
  exit 1
}

assert_contains() {
  local haystack="$1" needle="$2"
  if [[ "$haystack" == *"$needle"* ]]; then return 0; fi
  echo "  ${C_RED}✗ assertion failed: '$haystack' should contain '$needle'${C_RESET}"
  exit 1
}

assert_not_contains() {
  local haystack="$1" needle="$2"
  if [[ "$haystack" != *"$needle"* ]]; then return 0; fi
  echo "  ${C_RED}✗ leak detected: '$haystack' contains '$needle' (must NOT)${C_RESET}"
  exit 1
}
