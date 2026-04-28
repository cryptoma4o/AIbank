# LLM Gateway Troubleshooting

## Когда использовать

- Алерт `llm-gateway.error_rate > 5%`, `llm-gateway.p95_latency > 5s`
- Агенты (`agent-document-intake`, `agent-conversational`, `agent-reconciliation`) возвращают 5xx
- Подозрение, что primary backend (`vllm-gemma`) упал и не fallback'ится
- Один тенант исчерпал per-tenant квоту, нужно расследовать
- Подозрение на утечку PII в логи модели

Ссылки: [ADR-0011 LLM routing](../adr/0011-llm-routing-strategy.md), [`technical-structure.md` § 7](../technical-structure.md), [`security-architecture.md` § 8.3](../security-architecture.md).

## Архитектура (краткая)

```
[агенты] → [llm-gateway :8100] → [vLLM pool] / [GigaChat API] / [mock]
              ↓
        [audit-service] (логирование вызовов)
              ↓
        [billing-service] (агрегат cost_kop)
```

Gateway знает про **роли** (например, `role:vision-doc-parsing`, `role:ru-chat`), маппинг роль→backend в `configs/llm/routes.yaml` per-tenant override.

## Mock-режим

Gateway поддерживает `LLM_GATEWAY_FORCE_MOCK=1` — отвечает заглушкой без обращения к backend. Используется для smoke-тестов и dev-окружения.

```bash
# Проверка, в каком режиме gateway
kubectl exec -n platform deploy/llm-gateway -- env | grep LLM_GATEWAY_FORCE_MOCK
# Если "1" — это mock-режим. В prod должно быть пусто или "0"

# Принудительно включить mock на короткое время (debug)
kubectl set env deploy/llm-gateway -n platform LLM_GATEWAY_FORCE_MOCK=1
kubectl rollout status deploy/llm-gateway -n platform

# Выключить
kubectl set env deploy/llm-gateway -n platform LLM_GATEWAY_FORCE_MOCK-
```

В smoke-скрипте (`scripts/smoke.sh`, шаг 7) gateway вызывается именно в mock-режиме — проверка интеграции, не реальной модели.

## Ключевые endpoint'ы

| URL | Назначение | Авторизация |
|---|---|---|
| `/healthz` | Liveness probe | — |
| `/readiness` | Готовность принимать трафик (включая backend health) | — |
| `/v1/chat/completions` | OpenAI-compatible chat | `X-Tenant-Id` обязателен |
| `/v1/completions` | OpenAI-compatible completions | `X-Tenant-Id` |
| `/v1/embeddings` | Эмбеддинги (bge-m3 default) | `X-Tenant-Id` |
| `/v1/usage?tenant_id=<id>` | Аудит потребления per tenant (см. ADR-0011) | platform.admin |
| `/v1/models` | Список доступных моделей и ролей | — |
| `/metrics` | Prometheus-формат | — |

Аудит потребления (per-tenant):

```bash
# Сегодняшнее потребление
curl -fsS http://llm-gateway:8100/v1/usage?tenant_id=<tenant>

# Период
curl -fsS "http://llm-gateway:8100/v1/usage?tenant_id=<tenant>&from=2026-04-01&to=2026-04-26"

# Sample output
# {
#   "data": [
#     {"role":"role:ru-chat","model":"qwen-3.5-7b","tokens_in":12450,"tokens_out":4321,"cost_kop":215},
#     ...
#   ],
#   "totals": {"tokens_in": ..., "cost_kop": ...}
# }
```

## Сценарий 1: primary backend (vllm-gemma) failed, fallback не сработал

**Признаки:** rate ошибок 502/503 от gateway, в логе backend connection refused.

```bash
# 1. Подтвердить, что backend упал
kubectl get pods -n platform-ai -l app=vllm-gemma
kubectl logs -n platform-ai -l app=vllm-gemma --tail=100

# 2. Проверить readiness gateway
curl -fsS http://llm-gateway:8100/readiness | jq .
# Должен показать {"backends": {"vllm-gemma": "down", "qwen": "up", ...}}

# 3. Если fallback роль определён, но не сработал — проверить routes.yaml
kubectl exec -n platform deploy/llm-gateway -- cat /app/configs/routes.yaml
# Каждая роль должна иметь fallback:
#   role:vision-doc-parsing:
#     primary:  vllm-gemma-vision
#     fallback: qwen-3.5-vl

# 4. Ручное переключение роли на резервный backend
# Edit ConfigMap (либо patch values.yaml + helm upgrade)
kubectl edit configmap llm-gateway-routes -n platform
# Меняем primary: с vllm-gemma на qwen, сохраняем

# 5. Rolling restart gateway, чтобы перечитал config
kubectl rollout restart deploy/llm-gateway -n platform
kubectl rollout status deploy/llm-gateway -n platform

# 6. Verify
curl -fsS -X POST http://llm-gateway:8100/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Id: <tenant>" \
  -d '{"model":"role:vision-doc-parsing","messages":[{"role":"user","content":"ping"}]}'

# 7. Восстановить vllm-gemma — отдельная задача (см. agent-инцидент)
# После восстановления вернуть routes.yaml к исходному состоянию
```

## Сценарий 2: Quota exhaustion для одного тенанта

**Признаки:** один тенант получает 429, другие работают; `llm.quota_exceeded{tenant=<x>}` алерт.

```bash
# 1. Включить детальное логирование
kubectl set env deploy/llm-gateway -n platform LLM_GATEWAY_LOG_REQUESTS=true
kubectl rollout status deploy/llm-gateway -n platform

# 2. Распределение запросов по тенантам за последний час
kubectl logs -n platform deploy/llm-gateway --since=1h | \
  grep -oE 'X-Tenant-Id: [a-z0-9-]+' | sort | uniq -c | sort -rn

# 3. Текущая квота тенанта (из tenant config)
kubectl exec -n platform deploy/tenant-service -- \
  tenant-cli get-quota --tenant=<tenant> --resource=llm

# 4. Если квота исчерпана легитимно — обсудить с CSM подъём лимитов
# 5. Если необычный pattern (массовый retry, infinite loop в агенте) — найти в логах:
kubectl logs -n platform deploy/llm-gateway --since=1h | \
  grep "X-Tenant-Id: <tenant>" | grep -oE 'X-Trace-Id: [a-f0-9-]+' | sort | uniq -c | sort -rn
# Top trace_id с 100+ вызовами — подозрительный

# 6. Trace через Tempo
# Открыть Grafana → Tempo → Search trace_id=<id>
# Найти source-сервис и баг в нём

# 7. Временный bump квоты (только при подтверждённой не-malicious причине)
kubectl exec -n platform deploy/tenant-service -- \
  tenant-cli set-quota --tenant=<tenant> --resource=llm --rpm=2000 --duration=1h \
  --reason="incident-<id>"

# 8. Не забыть выключить debug-логирование (PII в логах!)
kubectl set env deploy/llm-gateway -n platform LLM_GATEWAY_LOG_REQUESTS-
```

## Сценарий 3: PII попал в логи

**Признаки:** на review логов видны фрагменты ИНН, паспортных данных, СНИЛС.

```bash
# 1. НЕМЕДЛЕННО — отключить детальное логирование
kubectl set env deploy/llm-gateway -n platform LLM_GATEWAY_LOG_REQUESTS-
kubectl rollout status deploy/llm-gateway -n platform

# 2. Эскалация в security-офицер (P0 если PII попал в Loki/persistent log)

# 3. Аудит масштаба
kubectl logs -n platform deploy/llm-gateway --since=24h | grep -E '\b\d{12}\b'  # ИНН-like
# (использовать шаблоны из security-architecture § 8.3)

# 4. Удаление из Loki — через retention policy override (DPO банка обязан approve)
# Loki retention API:
curl -X POST http://loki:3100/loki/api/v1/delete \
  --data-urlencode 'query={service="llm-gateway"}' \
  --data-urlencode 'start=<from>' --data-urlencode 'end=<to>'

# 5. Проверить, что PII-фильтр работает
kubectl exec -n platform deploy/llm-gateway -- cat /app/configs/pii-filter.yaml
# Должен быть включён: enabled: true, mode: redact (не log_only)

# 6. Test PII filter
curl -fsS -X POST http://llm-gateway:8100/v1/chat/completions \
  -H "Content-Type: application/json" -H "X-Tenant-Id: test" \
  -d '{"model":"role:ru-chat","messages":[{"role":"user","content":"Мой ИНН 7728168971"}]}'
# В логе ИНН должен быть [REDACTED-INN]

# 7. Post-mortem обязательно (P0 per security-architecture § 9.1)
```

Что должно быть отфильтровано (`security-architecture.md` § 5.2, § 8.3):
- Серия и номер паспорта
- ИНН физлица (не путать с ИНН ЮЛ — он публичный)
- СНИЛС
- API-ключи, токены
- Полные паспортные данные

Что МОЖЕТ быть в логах:
- ИНН ЮЛ (это открытые данные ЕГРЮЛ)
- application_id (UUID)
- tenant_id
- prompt_id, model_used (для биллинга)

## Сценарий 4: Агент возвращает ошибки, но gateway healthy

```bash
# 1. Прямой ping gateway из pod агента
kubectl exec -n platform deploy/agent-conversational -- \
  curl -fsS http://llm-gateway:8100/healthz

# 2. Network policy не блокирует?
kubectl get netpol -n platform | grep agent-conversational

# 3. mTLS handshake (если Istio)
kubectl exec -n platform deploy/agent-conversational -c istio-proxy -- \
  curl -fsS http://llm-gateway:8100/v1/models
# Если 503 — Istio sidecar не propagates mTLS, проверить DestinationRule

# 4. Конкретный test case
kubectl exec -n platform deploy/agent-conversational -- \
  python -c "import httpx; r=httpx.post('http://llm-gateway:8100/v1/chat/completions', \
    json={'model':'role:ru-chat','messages':[{'role':'user','content':'тест'}]}, \
    headers={'X-Tenant-Id':'demo'}); print(r.status_code, r.text[:200])"
```

## Сценарий 5: vLLM OOM на GPU

**Признаки:** pod restart loop в `platform-ai`, в логах CUDA OOM.

```bash
# 1. GPU memory utilization
kubectl exec -n platform-ai deploy/vllm-gemma -- nvidia-smi

# 2. Текущий max-batch-size
kubectl exec -n platform-ai deploy/vllm-gemma -- env | grep VLLM_MAX_

# 3. Уменьшить max-batch-size временно
kubectl set env deploy/vllm-gemma -n platform-ai VLLM_MAX_NUM_SEQS=64
kubectl rollout status deploy/vllm-gemma -n platform-ai

# 4. Если повторяется — escalation в ML-команду
#    Вероятные причины: длинный context (документ устава полностью),
#    quantization не применён, увеличилась нагрузка от тенантов
```

## Health checks

```bash
# Gateway
curl -fsS http://llm-gateway:8100/healthz       # liveness
curl -fsS http://llm-gateway:8100/readiness     # backends + deps

# Per-backend
kubectl exec -n platform-ai deploy/vllm-gemma -- curl -fsS http://localhost:8000/v1/models
kubectl exec -n platform-ai deploy/vllm-qwen -- curl -fsS http://localhost:8000/v1/models

# Метрики (ключевые)
kubectl exec -n platform deploy/llm-gateway -- curl -fsS http://localhost:8100/metrics | \
  grep -E '(llm_request_duration_seconds|llm_tokens_total|llm_quota_remaining)'
```

## Связанные документы

- [ADR-0011](../adr/0011-llm-routing-strategy.md) — стратегия роутинга и пул моделей
- [ADR-0010 § «LLM-биллинг»](../adr/0010-billing-and-audit-log.md) — `cost_kop` и агрегация
- [`technical-structure.md` § 7](../technical-structure.md) — ИИ-инфраструктура
- [`security-architecture.md` § 8.3](../security-architecture.md) — PII-фильтрация в логах
- [`incident-response.md`](incident-response.md) — эскалация P0 при PII leak
