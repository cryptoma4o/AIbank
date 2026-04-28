#!/usr/bin/env bash
# changed-services.sh — определяет, какие services/ai/apps/packages затронуты
# в текущем git diff (uncommitted + staged + последний commit).
# Помогает агенту/PR-чеклисту понять scope без полного исследования.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Объединяем три источника изменений: uncommitted, staged, последний commit.
CHANGED_FILES=$(
  {
    git diff --name-only 2>/dev/null
    git diff --cached --name-only 2>/dev/null
    git diff --name-only HEAD~1 HEAD 2>/dev/null || true
  } | sort -u
)

if [ -z "$CHANGED_FILES" ]; then
  echo "Изменений не обнаружено."
  exit 0
fi

extract_components() {
  local prefix="$1"
  echo "$CHANGED_FILES" \
    | grep "^${prefix}/" \
    | awk -F/ '{print $1"/"$2}' \
    | sort -u
}

echo "=== Затронутые компоненты ==="
echo

for area in ai services apps packages tools infrastructure docs configs; do
  components=$(extract_components "$area")
  if [ -n "$components" ]; then
    echo "$area/:"
    echo "$components" | sed 's/^/  - /'
  fi
done

echo
echo "=== Файлов изменено: $(echo "$CHANGED_FILES" | wc -l | tr -d ' ') ==="

# Подсказки по необходимым проверкам
echo
echo "=== Рекомендуемые проверки ==="
if echo "$CHANGED_FILES" | grep -q "^ai/"; then
  echo "  - make eval-agents  (изменения в ai/)"
fi
if echo "$CHANGED_FILES" | grep -q "^services/"; then
  echo "  - make test-go && make lint  (изменения в services/)"
fi
if echo "$CHANGED_FILES" | grep -q "^packages/domain-model/"; then
  echo "  - make generate && make validate-schema  (доменная модель)"
fi
if echo "$CHANGED_FILES" | grep -q "^configs/tenants/"; then
  echo "  - make validate-schema  (тенант-конфиги)"
fi
if echo "$CHANGED_FILES" | grep -q "^apps/"; then
  echo "  - npm run lint && npm test в соответствующем apps/"
fi
