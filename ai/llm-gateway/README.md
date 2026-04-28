# llm-gateway

OpenAI-совместимый прокси к LLM-моделям с роль-ориентированным роутингом, per-tenant
учётом стоимости и mock-режимом для CI/eval-harness.

Соответствует разделу 4.4 `docs/technical-structure.md`. Закладка под ADR-0011
(стратегия выбора и роутинга LLM — пока в `Proposed`).

## Что даёт

- **Один endpoint** для всех AI-агентов: `POST /v1/chat/completions`.
- **Роль вместо модели.** Агент шлёт `model: "role:document-vision"` — gateway резолвит
  роль в конкретную модель и backend по конфигу. Переключение модели = изменение конфига,
  без перевыкладки агента.
- **Fallback per role.** При сбое primary-модели — автоматический повторный запрос на fallback.
- **Per-tenant учёт.** Заголовок `X-Tenant-Id` обязателен. Усреднённая стоимость в копейках
  считается на основе тарифа модели.
- **Mock-режим** (`LLM_GATEWAY_FORCE_MOCK=1`) — детерминированные ответы без реальных моделей.
  Используется в CI и в eval-harness, когда тестируется логика агентов, а не качество LLM.

## Архитектура

```
┌─────────────┐  POST /v1/chat/completions  ┌──────────────┐  POST /v1/chat/completions  ┌────────────┐
│ AI agent    │  X-Tenant-Id: bank-alpha    │  llm-gateway │  (resolved real model name) │ vLLM /     │
│ (intake)    │  model: role:document-vision│              │ ─────────────────────────►  │ external   │
│             │                             │  router      │                             │ API        │
└─────────────┘                             │  ↓           │  ◄──── chat.completion ──── └────────────┘
                                            │  usage tracker (in-memory)                  
                                            │  ↓                                          
                                            │  +aibank_gateway in response                
                                            └──────────────┘                              
                                                                                          
       fallback flow: vllm primary fails → retry with role.fallback model         
       force_mock mode: every request → MockBackend (deterministic by prompt hash)
```

## Конфиг

Дефолтный YAML — `configs/default-routing.yaml`. Переменная `LLM_GATEWAY_CONFIG`
задаёт путь к кастомному конфигу.

```yaml
backends:
  vllm-gemma: {type: openai_compatible, url: http://vllm-gemma:8000}
  mock:       {type: mock}

models:
  gemma-4-26b:
    backend: vllm-gemma
    cost_per_1k_input_kop: 50
    cost_per_1k_output_kop: 150

roles:
  document-vision:
    model: gemma-4-26b
    fallback: qwen-3-vl-32b
```

Pydantic-валидатор гарантирует, что любые ссылки (model→backend, role→model)
указывают на существующие сущности — некорректный конфиг падает на старте.

## Запросы

### Базовый запрос с role

```bash
curl -X POST http://localhost:8100/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Id: bank-alpha" \
  -d '{
    "model": "role:document-vision",
    "messages": [{"role": "user", "content": "Извлеки поля из паспорта..."}]
  }'
```

### Через X-Agent-Role (если payload — стандартный OpenAI)

```bash
curl -X POST http://localhost:8100/v1/chat/completions \
  -H "X-Tenant-Id: bank-alpha" \
  -H "X-Agent-Role: ru-chat" \
  -d '{
    "model": "ignored-when-role-header-set",
    "messages": [{"role": "user", "content": "Что нужно для открытия счёта ИП?"}]
  }'
```

### Получить отчёт о потреблении

```bash
curl http://localhost:8100/v1/usage?tenant_id=bank-alpha
```

## Mock-режим для CI

```bash
LLM_GATEWAY_FORCE_MOCK=1 python -m uvicorn main:app --port 8100
```

В этом режиме gateway игнорирует upstream URL'ы и выдаёт детерминированный ответ
по хешу промпта. Подходит для:

- Юнит-тестов агентов: одинаковый промпт → одинаковый ответ → стабильная регрессия.
- eval-harness в CI: можно валидировать pipeline (request → response → metric)
  без затрат на реальные модели.

## Тесты

```bash
pip install -e .[dev]
pytest
```

Покрытие:

- `test_router.py` — резолвинг ролей, моделей, fallback, force_mock; валидация конфига
- `test_usage.py` — аккумулирование, изоляция по тенанту, расчёт стоимости
- `test_api.py` — TestClient по всем endpoint'ам, проверка обязательного X-Tenant-Id,
  X-Agent-Role override, smoke-test реального YAML

## Что осознанно НЕ реализовано в MVP

- **Streaming-ответы** (SSE). Не нужны на этапе скелетов агентов.
- **Per-tenant rate limiting.** Закладка есть в YAML-конфиге, реализация в Phase 1+.
- **PII-фильтр на входе/выходе.** Гардрейл из `docs/security-architecture.md` § 5.4 — отдельная задача.
- **Persistent usage** в Kafka → billing-service. Сейчас только in-memory; ADR-0010 опишет.
- **Prompt versioning.** Будет в ADR-0007.
- **Точные токенизаторы** (sentencepiece, tiktoken). Сейчас грубая оценка `len(text)//4`.
  Точные счётчики появятся, когда выбраны production-модели после eval-harness.

## Зависимости

- `fastapi`, `uvicorn`, `httpx`, `pydantic`, `pydantic-settings`, `pyyaml`
