#!/usr/bin/env bash
# setup-ollama-staging.sh — однократная инициализация Ollama на staging-сервере.
#
# Что делает:
#  1. docker compose up -d ollama (использует docker-compose.override.yml)
#  2. pull T-Lite GGUF (~4.5GB) + Qwen2.5-VL-7B (~5GB)
#  3. создаёт локальные алиасы 'tlite' и 'qwen-vl' через Modelfile
#     (соответствуют именам в staging-ollama.yaml)
#  4. перезапускает llm-gateway с новым конфигом
#  5. smoke-проверка реального ответа через gateway
#
# Идемпотентен: повторный запуск не ломает уже загруженные модели.
#
# Использование (на сервере):
#   cd /opt/aibank && bash scripts/setup-ollama-staging.sh

set -euo pipefail

OLLAMA_CONTAINER="aibank-ollama-1"
GATEWAY_URL="http://localhost:8100"
OLLAMA_URL="http://localhost:11434"

step() { printf "\n\033[1;34m→ %s\033[0m\n" "$*"; }
ok()   { printf "  \033[0;32m✓ %s\033[0m\n" "$*"; }
warn() { printf "  \033[0;33m! %s\033[0m\n" "$*"; }
fail() { printf "  \033[0;31m✗ %s\033[0m\n" "$*" >&2; exit 1; }

step "1. Запускаю Ollama"
docker compose up -d ollama
ok "контейнер $OLLAMA_CONTAINER поднят"

step "2. Жду пока Ollama станет доступной"
elapsed=0
until curl -fsS "$OLLAMA_URL/api/tags" >/dev/null 2>&1; do
  sleep 2; elapsed=$((elapsed + 2))
  if [ $elapsed -gt 60 ]; then fail "Ollama не поднялась за 60с"; fi
done
ok "Ollama API ответил за ${elapsed}с"

step "3. Загружаю text-LLM (Qwen2.5-7B, ~4.7GB — T-Lite временно недоступен через Ollama HF-proxy)"
if docker exec "$OLLAMA_CONTAINER" ollama list | grep -q "^qwen2.5:7b"; then
  ok "qwen2.5:7b уже загружен"
else
  docker exec "$OLLAMA_CONTAINER" ollama pull qwen2.5:7b
  ok "qwen2.5:7b загружен"
fi

step "4. Создаю алиас 'tlite' через Modelfile"
if docker exec "$OLLAMA_CONTAINER" ollama list | grep -q "^tlite "; then
  ok "алиас tlite уже создан"
else
  docker exec "$OLLAMA_CONTAINER" ollama create tlite -f /modelfiles/Modelfile.tlite
  ok "алиас tlite создан"
fi

step "5. Загружаю Qwen2.5-VL-7B (~5GB)"
if docker exec "$OLLAMA_CONTAINER" ollama list | grep -q "qwen2.5vl"; then
  ok "Qwen2.5-VL уже загружен"
else
  docker exec "$OLLAMA_CONTAINER" ollama pull qwen2.5vl:7b
  ok "Qwen2.5-VL загружен"
fi

step "6. Создаю алиас 'qwen-vl'"
if docker exec "$OLLAMA_CONTAINER" ollama list | grep -q "^qwen-vl "; then
  ok "алиас qwen-vl уже создан"
else
  docker exec "$OLLAMA_CONTAINER" ollama create qwen-vl -f /modelfiles/Modelfile.qwen-vl
  ok "алиас qwen-vl создан"
fi

step "7. Smoke: прямой запрос к Ollama через tlite"
response=$(curl -fsS -X POST "$OLLAMA_URL/v1/chat/completions" \
  -H "Content-Type: application/json" \
  --max-time 90 \
  -d '{"model":"tlite","messages":[{"role":"user","content":"Скажи привет одной короткой фразой."}],"max_tokens":40}')
content=$(echo "$response" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d['choices'][0]['message']['content'])")
ok "ответ от T-Lite: ${content:0:80}..."

step "8. Перезапускаю llm-gateway с новым конфигом"
docker compose up -d --force-recreate --no-deps llm-gateway
# Ждём пока поднимется healthy
elapsed=0
until curl -fsS "$GATEWAY_URL/health" >/dev/null 2>&1; do
  sleep 2; elapsed=$((elapsed + 2))
  if [ $elapsed -gt 60 ]; then fail "llm-gateway не поднялся за 60с"; fi
done
ok "gateway healthy за ${elapsed}с"

step "9. Smoke: запрос через gateway с role:ru-chat"
response=$(curl -fsS -X POST "$GATEWAY_URL/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Id: demo" \
  --max-time 120 \
  -d '{"model":"role:ru-chat","messages":[{"role":"user","content":"Какие документы нужны ИП для счёта?"}],"max_tokens":80}')
backend=$(echo "$response" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('aibank_gateway',{}).get('backend','?'))")
content=$(echo "$response" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d['choices'][0]['message']['content'])")
if [ "$backend" = "ollama" ]; then
  ok "backend=ollama, ответ: ${content:0:120}..."
else
  warn "backend=$backend (ожидался ollama)"
fi

step "DONE"
echo "Дальше:"
echo "  - проверь usage:  curl ${GATEWAY_URL}/v1/usage?tenant_id=demo | jq"
echo "  - запусти E2E:    make e2e-staging"
echo "  - eval-harness:   make eval-agents"
echo "  - откат на mock:  выставь LLM_GATEWAY_FORCE_MOCK=1 в override и перезапусти gateway"
