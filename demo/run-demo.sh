#!/usr/bin/env bash
# run-demo.sh — orchestrator всех 3 сценариев для презентации банку.
#
# Usage:
#   make up && make migrate-platform && make smoke
#   ./demo/run-demo.sh
#
# Время работы: ~5 минут.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

C_GREEN=$'\033[0;32m'
C_RED=$'\033[0;31m'
C_BLUE=$'\033[0;34m'
C_BOLD=$'\033[1m'
C_RESET=$'\033[0m'

echo
echo "${C_BOLD}╔════════════════════════════════════════════════════════════╗${C_RESET}"
echo "${C_BOLD}║       AIbank Demo — Banking Onboarding Platform           ║${C_RESET}"
echo "${C_BOLD}║       3 сценария × ~90 секунд каждый                       ║${C_RESET}"
echo "${C_BOLD}╚════════════════════════════════════════════════════════════╝${C_RESET}"

# Pre-flight check
if ! curl -fsS http://localhost:8080/healthz >/dev/null 2>&1; then
  echo "${C_RED}✗ tenant-service не отвечает на :8080${C_RESET}"
  echo "  Сначала: ${C_BOLD}make up && make migrate-platform${C_RESET}"
  exit 1
fi

declare -a RESULTS=()
START_TS=$(date +%s)

for scenario in 01-llc-happy-path 02-ip-manual-review 03-ao-rejected; do
  echo
  if "$SCRIPT_DIR/scenarios/$scenario.sh"; then
    RESULTS+=("✓ $scenario")
  else
    RESULTS+=("✗ $scenario (FAILED)")
  fi
done

END_TS=$(date +%s)
ELAPSED=$((END_TS - START_TS))

echo
echo "${C_BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${C_RESET}"
echo "${C_BOLD}Demo Summary${C_RESET} (${ELAPSED}s)"
echo "${C_BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${C_RESET}"
for r in "${RESULTS[@]}"; do
  if [[ "$r" == ✓* ]]; then
    echo "${C_GREEN}${r}${C_RESET}"
  else
    echo "${C_RED}${r}${C_RESET}"
  fi
done

echo
echo "${C_BLUE}Дальше показать CISO банка:${C_RESET}"
echo "  1. Audit-trail integrity:  ./tools/audit-verifier verify --tenant-id=demo --source=db --dsn=\$DATABASE_URL"
echo "  2. Hash chain detection:   см. demo/README.md § 'Для CISO банка'"
echo "  3. Multi-tenant isolation: psql -c \"SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tnt_%'\""
echo "  4. Slide-deck:              docs/demo-slide-deck-outline.md"
echo "  5. Pilot-readiness gap:    docs/pilot-readiness.md"
