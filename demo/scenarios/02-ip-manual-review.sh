#!/usr/bin/env bash
# Demo сценарий 02: ИП без ОГРНИП-выписки → manual review.
#
# Демонстрирует escalation в operator UI (web-admin) и flow с
# дозапросом документов клиентом.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SEED="$SCRIPT_DIR/../seed/ip-partial.json"

source "$SCRIPT_DIR/../lib/demo-helpers.sh"

scenario_header "02" "ИП Manual Review → operator escalation" "$SEED"

ensure_tenant "demo"
TOKEN=$(create_applicant_token "demo" "petrov-demo")

APP_ID=$(create_application "$SEED" "$TOKEN")
echo "  → application_id=$APP_ID"

trigger_workflow "$APP_ID"
wait_for_state "$APP_ID" "collecting_documents"

# Только паспорт — нет ОГРНИП-выписки
upload_demo_documents "$APP_ID" "$SEED" "$TOKEN"

signal_documents_uploaded "$APP_ID"
wait_for_state "$APP_ID" "ai_processing"

# AI-агенты обнаруживают отсутствие ОГРНИП → risk medium
wait_for_state "$APP_ID" "manual_review" 60

DECISION=$(get_decision "$APP_ID")
RISK=$(get_risk_category "$APP_ID")
echo "  → risk=$RISK decision=$DECISION"
assert_eq "$DECISION" "manual_review"
assert_eq "$RISK" "medium"

# Operator смотрит в web-admin и принимает решение через decision modal
echo "  → Operator открывает заявку в web-admin (http://localhost:3001)"
echo "  → Mutation updateApplicationDecision через bff-admin"

ADMIN_TOKEN=$(get_admin_token)
operator_decision "$APP_ID" "approved" "Документы валидны при доп. проверке через ЕГРИП API" "$ADMIN_TOKEN"

wait_for_state "$APP_ID" "account_opened" 30
ACCOUNT=$(get_account "$APP_ID")
echo "  → счёт открыт после manual review: $ACCOUNT"

check_audit_events "$APP_ID" \
  "application.submitted" \
  "documents.uploaded" \
  "ai.risk_scored" \
  "decision.manual_review" \
  "operator.decision_made" \
  "abs.account_opened"

scenario_complete "02" "$APP_ID"
