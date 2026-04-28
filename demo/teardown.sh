#!/usr/bin/env bash
# teardown.sh — очистка demo-state после презентации.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "→ Останавливаем стек и удаляем demo-данные..."
cd "$SCRIPT_DIR/.." && make smoke-clean

# Удалить tmp-файлы demo
rm -rf /tmp/aibank-demo-state /tmp/demo-*.pdf 2>/dev/null || true

echo "✓ teardown complete. Готов к следующему запуску через ./demo/run-demo.sh"
