#!/usr/bin/env bash
# postCreateCommand: одноразовая настройка после build образа Codespace.
set -euo pipefail

echo "→ Установка golangci-lint..."
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.59.0 || true

echo "→ Установка Go-моделей..."
go install golang.org/x/tools/cmd/goimports@latest || true

echo "→ Python-зависимости для validate-schema..."
pip install --user pyyaml jsonschema 2>&1 | tail -3 || true

echo "→ npm install для frontend (только если нужно)..."
if [ -d apps/web-onboarding ] && [ ! -d apps/web-onboarding/node_modules ]; then
  cd apps/web-onboarding && npm install --prefer-offline --no-audit --no-fund | tail -3 || true
  cd - >/dev/null
fi
if [ -d apps/web-admin ] && [ ! -d apps/web-admin/node_modules ]; then
  cd apps/web-admin && npm install --prefer-offline --no-audit --no-fund | tail -3 || true
  cd - >/dev/null
fi

echo "→ Проверка Go-сборки..."
make test-go 2>&1 | tail -5 || true

echo
echo "✓ AIbank Codespace готов."
echo
echo "Дальше:"
echo "  1. make up                 # поднять docker-compose стек (~5 мин)"
echo "  2. make migrate-platform   # применить миграции"
echo "  3. make smoke              # e2e smoke test"
echo "  4. ./demo/run-demo.sh      # 3 demo-сценария"
echo
echo "Frontend (запустить в отдельных терминалах):"
echo "  cd apps/web-onboarding && npm run dev    # → :3000"
echo "  cd apps/web-admin     && PORT=3001 npm run dev  # → :3001"
