#!/usr/bin/env bash
# context-light.sh — компактная карта проекта AIbank для AI-агентов.
# Цель: вывести ~200 токенов структуры вместо чтения 5+ файлов.
# Использование:
#   bash scripts/context-light.sh           # компактный вывод
#   bash scripts/context-light.sh --full    # + LOC статистика

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

FULL=0
[ "${1:-}" = "--full" ] && FULL=1

echo "=== AIbank — карта проекта ==="
echo

echo "AI-агенты (ai/):"
for d in ai/agent-* ai/llm-gateway ai/rag-service ai/eval-harness ai/_shared; do
  [ -d "$d" ] && echo "  - $d"
done

echo
echo "Сервисы (services/):"
find services -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | sed 's/^/  - /'

echo
echo "Приложения (apps/):"
find apps -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | sed 's/^/  - /'

echo
echo "Shared packages (packages/):"
find packages -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | sed 's/^/  - /'

echo
echo "Tools (tools/):"
find tools -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | sed 's/^/  - /'

echo
echo "Документы (docs/, верхний уровень):"
ls -1 docs/*.md 2>/dev/null | sed 's/^/  - /'
[ -d docs/adr ] && echo "  - docs/adr/ (ADRs)"

if [ "$FULL" = "1" ]; then
  echo
  echo "=== LOC статистика ==="
  for area in ai services apps packages tools; do
    if [ -d "$area" ]; then
      files=$(find "$area" -type f \( -name "*.go" -o -name "*.py" -o -name "*.ts" -o -name "*.tsx" \) 2>/dev/null | wc -l | tr -d ' ')
      lines=$(find "$area" -type f \( -name "*.go" -o -name "*.py" -o -name "*.ts" -o -name "*.tsx" \) -exec wc -l {} + 2>/dev/null | tail -1 | awk '{print $1}')
      printf "  %-12s %s файлов, %s LOC\n" "$area/" "${files:-0}" "${lines:-0}"
    fi
  done

  echo
  echo "=== Стек ==="
  echo "  Backend: Go 1.22 (chi, gRPC, sqlc, Temporal)"
  echo "  AI/ML:   Python 3.12 (FastAPI, vLLM, LangGraph)"
  echo "  Web:     TypeScript, Next.js 14"
  echo "  Infra:   PostgreSQL, Kafka, Redis, MinIO, Qdrant, Temporal"
fi

echo
echo "Подробнее: AGENTS.md, docs/AGENTS.md, docs/agents-rules.md"
