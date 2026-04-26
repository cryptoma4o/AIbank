# ai/

ИИ-сервисы и агенты, включая LLM-инфраструктуру и eval-harness.

См. [docs/ai-agents-automation.md](../docs/ai-agents-automation.md) для детальной спецификации.

## Структура

- `llm-gateway/` — OpenAI-совместимый прокси к моделям с гардрейлами
- `agent-document-intake/` — multimodal-парсинг документов
- `agent-reconciliation/` — сверка с ЕГРЮЛ
- `agent-ubo-tracing/` — построение графа владения
- `agent-risk-scoring/` — объяснимый скоринг (CatBoost + LLM-объяснение)
- `agent-conversational/` — клиентский чат-помощник
- `agent-compliance-assistant/` — помощник комплаенс-офицера
- `rag-service/` — RAG поверх банковской нормативки
- `eval-harness/` — оценка качества агентов и моделей

## Принципы

- Stateless-агенты, всё состояние — в `onboarding-orchestrator`
- Все вызовы LLM — только через `llm-gateway`
- Каждый агент имеет свой eval-корпус и метрики
- Промпты версионируются в Git, не хардкодятся
