#!/usr/bin/env bash
# AIbank end-to-end smoke test (happy path).
#
# Walks through tenant creation, audit logging, LLM gateway (mock mode),
# conversational agent, and usage reporting. Designed to validate that the
# core onboarding pipeline is wired correctly after `make up`.
#
# Usage:
#   make up && make migrate-platform && make smoke
#
# Exit codes:
#   0  — all steps passed
#   1  — any step failed (last failed step recorded in /tmp/aibank-smoke-state)

set -euo pipefail

# ── Constants ─────────────────────────────────────────────────────────────
readonly STATE_DIR="/tmp/aibank-smoke-state"
readonly TENANT_ID="demo"
readonly TENANT_HOST="http://localhost:8080"
readonly AUDIT_HOST="http://localhost:8081"
readonly IDENTITY_HOST="http://localhost:8082"
readonly LLM_HOST="http://localhost:8100"
readonly CHAT_HOST="http://localhost:8104"
readonly HEALTH_TIMEOUT_SECONDS=60

# ── Colors (disabled if not a TTY) ────────────────────────────────────────
if [ -t 1 ]; then
  readonly C_GREEN=$'\033[0;32m'
  readonly C_RED=$'\033[0;31m'
  readonly C_YELLOW=$'\033[0;33m'
  readonly C_BLUE=$'\033[0;34m'
  readonly C_RESET=$'\033[0m'
else
  readonly C_GREEN=""
  readonly C_RED=""
  readonly C_YELLOW=""
  readonly C_BLUE=""
  readonly C_RESET=""
fi

# ── State tracking ────────────────────────────────────────────────────────
mkdir -p "$STATE_DIR"
CURRENT_STEP=0
CURRENT_DESC=""

step_begin() {
  CURRENT_STEP=$((CURRENT_STEP + 1))
  CURRENT_DESC="$1"
  printf "%s→ Step %d:%s %s\n" "$C_BLUE" "$CURRENT_STEP" "$C_RESET" "$CURRENT_DESC"
  echo "$CURRENT_STEP|$CURRENT_DESC|in_progress" > "$STATE_DIR/last-step"
}

step_ok() {
  printf "  %s✓ Step %d OK%s\n" "$C_GREEN" "$CURRENT_STEP" "$C_RESET"
  echo "$CURRENT_STEP|$CURRENT_DESC|ok" > "$STATE_DIR/last-step"
}

on_error() {
  local exit_code=$?
  printf "\n%s✗ smoke FAILED at step %d: %s%s\n" \
    "$C_RED" "$CURRENT_STEP" "$CURRENT_DESC" "$C_RESET" >&2
  echo "$CURRENT_STEP|$CURRENT_DESC|failed|exit=$exit_code" > "$STATE_DIR/last-step"
  echo "  → state recorded in $STATE_DIR/last-step" >&2
  echo "  → tail logs: docker compose logs <service>" >&2
  exit 1
}
trap on_error ERR

# ── Helpers ───────────────────────────────────────────────────────────────
poll_health() {
  # poll_health <url> <name> [timeout]
  local url="$1"
  local name="$2"
  local timeout="${3:-$HEALTH_TIMEOUT_SECONDS}"
  local elapsed=0
  while [ $elapsed -lt "$timeout" ]; do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      printf "  %s↳ %s healthy (%ss)%s\n" "$C_YELLOW" "$name" "$elapsed" "$C_RESET"
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  echo "  ↳ $name failed to become healthy within ${timeout}s" >&2
  return 1
}

assert_status() {
  # assert_status <expected> <actual> <context>
  local expected="$1"
  local actual="$2"
  local ctx="$3"
  if [ "$actual" != "$expected" ]; then
    echo "  ↳ $ctx: expected HTTP $expected, got $actual" >&2
    return 1
  fi
}

# ── Step 1: verify docker compose is up ───────────────────────────────────
step_begin "verify docker compose stack is running"
running_count=$(docker compose ps --services --filter "status=running" 2>/dev/null | wc -l | tr -d ' ')
if [ "$running_count" -lt 5 ]; then
  echo "  ↳ only $running_count services running; expected at least 5" >&2
  echo "  ↳ run 'make up' first" >&2
  exit 1
fi
echo "  ↳ $running_count services running"
step_ok

# ── Step 2: wait for core services to be healthy ──────────────────────────
step_begin "wait for tenant/audit/identity/llm-gateway to be healthy (timeout ${HEALTH_TIMEOUT_SECONDS}s)"
poll_health "$TENANT_HOST/health"    "tenant-service"
poll_health "$AUDIT_HOST/health"     "audit-service"
poll_health "$IDENTITY_HOST/health"  "identity-service"
poll_health "$LLM_HOST/healthz"      "llm-gateway"
poll_health "$CHAT_HOST/healthz"     "agent-conversational"
step_ok

# ── Step 3: run platform migrations ───────────────────────────────────────
step_begin "apply platform migrations (db-migrator-platform)"
docker compose --profile migrate run --rm db-migrator-platform >"$STATE_DIR/migrate.log" 2>&1
echo "  ↳ migration log: $STATE_DIR/migrate.log"
step_ok

# ── Step 4: create tenant ─────────────────────────────────────────────────
step_begin "create tenant '$TENANT_ID' via tenant-service"
create_payload=$(cat <<EOF
{
  "id": "$TENANT_ID",
  "name": "Демо банк",
  "bik": "044525593",
  "inn": "7728168971",
  "deployment_mode": "saas"
}
EOF
)
create_status=$(curl -sS -o "$STATE_DIR/tenant-create.json" -w "%{http_code}" \
  -X POST "$TENANT_HOST/v1/tenants" \
  -H "Content-Type: application/json" \
  -d "$create_payload")
# Idempotent: 201 on first run, 409 on re-run is also acceptable.
if [ "$create_status" = "409" ]; then
  echo "  ↳ tenant already exists (HTTP 409) — continuing"
else
  assert_status "201" "$create_status" "POST /v1/tenants"
fi
step_ok

# ── Step 5: verify tenant exists ──────────────────────────────────────────
step_begin "verify tenant '$TENANT_ID' is retrievable"
get_status=$(curl -sS -o "$STATE_DIR/tenant-get.json" -w "%{http_code}" \
  "$TENANT_HOST/v1/tenants/$TENANT_ID")
assert_status "200" "$get_status" "GET /v1/tenants/$TENANT_ID"
step_ok

# ── Step 6: write audit event ─────────────────────────────────────────────
step_begin "write audit event via audit-service"
audit_payload=$(cat <<EOF
{
  "tenant_id": "$TENANT_ID",
  "entity_type": "test",
  "entity_id": "smoke-001",
  "event_type": "smoke.run",
  "actor_id": "smoke-script",
  "actor_type": "system",
  "payload": {}
}
EOF
)
audit_status=$(curl -sS -o "$STATE_DIR/audit-create.json" -w "%{http_code}" \
  -X POST "$AUDIT_HOST/v1/events" \
  -H "Content-Type: application/json" \
  -d "$audit_payload")
assert_status "201" "$audit_status" "POST /v1/events"
step_ok

# ── Step 7: call llm-gateway in mock mode ────────────────────────────────
step_begin "call llm-gateway /v1/chat/completions (mock mode)"
chat_payload=$(cat <<'EOF'
{
  "model": "role:ru-chat",
  "messages": [
    {"role": "user", "content": "Привет, проверка smoke-теста."}
  ]
}
EOF
)
chat_status=$(curl -sS -o "$STATE_DIR/llm-chat.json" -w "%{http_code}" \
  -X POST "$LLM_HOST/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Id: $TENANT_ID" \
  -d "$chat_payload")
assert_status "200" "$chat_status" "POST /v1/chat/completions"
if ! grep -q "aibank_gateway" "$STATE_DIR/llm-chat.json"; then
  echo "  ↳ response missing 'aibank_gateway' metadata" >&2
  echo "  ↳ payload: $(cat "$STATE_DIR/llm-chat.json")" >&2
  exit 1
fi
step_ok

# ── Step 8: call agent-conversational ─────────────────────────────────────
step_begin "call agent-conversational /v1/chat"
agent_payload=$(cat <<EOF
{
  "tenant_id": "$TENANT_ID",
  "messages": [
    {"role": "user", "content": "Какие документы нужны для открытия счёта?"}
  ]
}
EOF
)
agent_status=$(curl -sS -o "$STATE_DIR/agent-chat.json" -w "%{http_code}" \
  -X POST "$CHAT_HOST/v1/chat" \
  -H "Content-Type: application/json" \
  -d "$agent_payload")
assert_status "200" "$agent_status" "POST /v1/chat"
step_ok

# ── Step 9: report usage ──────────────────────────────────────────────────
step_begin "fetch usage report from llm-gateway"
usage_status=$(curl -sS -o "$STATE_DIR/usage.json" -w "%{http_code}" \
  "$LLM_HOST/v1/usage?tenant_id=$TENANT_ID")
assert_status "200" "$usage_status" "GET /v1/usage"
if ! grep -qE '"data"\s*:\s*\[' "$STATE_DIR/usage.json"; then
  echo "  ↳ usage report missing 'data' array" >&2
  echo "  ↳ payload: $(cat "$STATE_DIR/usage.json")" >&2
  exit 1
fi
step_ok

# ── Done ──────────────────────────────────────────────────────────────────
printf "\n%s✓ smoke OK%s — %d steps passed\n" "$C_GREEN" "$C_RESET" "$CURRENT_STEP"
echo "  artifacts: $STATE_DIR/"
echo "$CURRENT_STEP|all|ok" > "$STATE_DIR/last-step"
