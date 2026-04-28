#!/usr/bin/env bash
# Demo сценарий 03: АО с sanctions match → declined.
#
# Демонстрирует negative path: ext-rosfinmon hit + foreign offshore в
# UBO chain → high risk → automatic decline. Показывает audit-trail
# причины отказа без раскрытия деталей клиенту (115-ФЗ требование).

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SEED="$SCRIPT_DIR/../seed/ao-sanctions.json"

source "$SCRIPT_DIR/../lib/demo-helpers.sh"

scenario_header "03" "АО Declined → 115-ФЗ + foreign offshore" "$SEED"

ensure_tenant "demo"
TOKEN=$(create_applicant_token "demo" "sidorov-demo")

APP_ID=$(create_application "$SEED" "$TOKEN")
echo "  → application_id=$APP_ID"

trigger_workflow "$APP_ID"
wait_for_state "$APP_ID" "collecting_documents"

upload_demo_documents "$APP_ID" "$SEED" "$TOKEN"

signal_documents_uploaded "$APP_ID"
wait_for_state "$APP_ID" "ai_processing"

# UBO-tracing обнаруживает foreign offshore без disclosure
# Risk-scoring + ext-rosfinmon hit (синтетика) → high risk
wait_for_state "$APP_ID" "decision_pending" 60

DECISION=$(get_decision "$APP_ID")
RISK=$(get_risk_category "$APP_ID")
echo "  → risk=$RISK decision=$DECISION"
assert_eq "$DECISION" "declined"
assert_eq "$RISK" "high"

# Клиенту показываем generic-сообщение без деталей (115-ФЗ § 7)
CLIENT_MESSAGE=$(get_client_decision_message "$APP_ID")
echo "  → клиенту показано: \"$CLIENT_MESSAGE\""
assert_contains "$CLIENT_MESSAGE" "Заявка не одобрена"
assert_not_contains "$CLIENT_MESSAGE" "санкци"  # детали никогда не раскрываем
assert_not_contains "$CLIENT_MESSAGE" "115-ФЗ"

# В audit-log есть полная причина для compliance-officer
INTERNAL_REASON=$(get_internal_decline_reason "$APP_ID")
echo "  → внутренняя причина (видна только compliance): $INTERNAL_REASON"
assert_contains "$INTERNAL_REASON" "115-ФЗ"

# Audit события
check_audit_events "$APP_ID" \
  "application.submitted" \
  "documents.uploaded" \
  "ai.ubo_traced" \
  "ai.risk_scored" \
  "ext.rosfinmon.match_found" \
  "decision.declined" \
  "client.notification_sent"

scenario_complete "03" "$APP_ID"
