#!/usr/bin/env bash
# Demo сценарий 01: ООО happy path → auto-approved.
#
# Демонстрирует full e2e: создание заявки → upload документов →
# document-intake AI-agent → ubo-tracing → risk-scoring → decision →
# открытие счёта в ABS-CFT mock.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SEED="$SCRIPT_DIR/../seed/llc-happy.json"

source "$SCRIPT_DIR/../lib/demo-helpers.sh"

scenario_header "01" "ООО Happy Path → auto-approve" "$SEED"

# 1. Создать tenant (если не создан)
ensure_tenant "demo"

# 2. Получить applicant token (для онбординг-flow)
TOKEN=$(create_applicant_token "demo" "ivanov-demo")

# 3. Создать заявку
APP_ID=$(create_application "$SEED" "$TOKEN")
echo "  → application_id=$APP_ID"

# 4. Запустить onboarding workflow (Temporal)
trigger_workflow "$APP_ID"
wait_for_state "$APP_ID" "collecting_documents"

# 5. Загрузить документы (3 шт.)
upload_demo_documents "$APP_ID" "$SEED" "$TOKEN"

# 6. Сигнал completing документов → AI-агенты работают
signal_documents_uploaded "$APP_ID"
wait_for_state "$APP_ID" "ai_processing"

echo "  → AI agents working: document-intake → ubo-tracing → risk-scoring"
wait_for_state "$APP_ID" "decision_pending" 60

# 7. Risk-engine выдаёт low-risk → auto-approve
DECISION=$(get_decision "$APP_ID")
RISK=$(get_risk_category "$APP_ID")
echo "  → risk=$RISK decision=$DECISION"
assert_eq "$DECISION" "auto_approved"
assert_eq "$RISK" "low"

# 8. Открытие счёта через ABS-CFT
wait_for_state "$APP_ID" "account_opened" 30
ACCOUNT=$(get_account "$APP_ID")
echo "  → счёт открыт: $ACCOUNT"

# 9. Audit log должен содержать все ключевые события
check_audit_events "$APP_ID" \
  "application.submitted" \
  "documents.uploaded" \
  "ai.document_intake.completed" \
  "ai.ubo_traced" \
  "ai.risk_scored" \
  "decision.auto_approved" \
  "abs.account_opened"

scenario_complete "01" "$APP_ID"
