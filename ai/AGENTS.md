<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# ai/

## Purpose

ИИ-слой платформы — сервисы на базе LLM и ML для автоматизации KYC/KYB-проверок. Основной язык — **Python 3.12** (FastAPI, vLLM, LangGraph). Agents — stateless, оркестрируются через Temporal Activities из `services/onboarding-orchestrator/`. Статус: **директория пустая, реализация с Фазы 3 (М1)**. Eval-harness skeleton — **Фаза 0 (М0), приоритет**.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Обзор AI-слоя: агенты, модели, принципы автоматизации, ссылки на документацию |

## Planned Subdirectories

| Директория | Назначение | Фаза |
|-----------|-----------|------|
| `llm-gateway/` | Прокси к LLM: роутинг по типу задачи, биллинг токенов, PII-фильтр, кеш, гардрейлы, audit log каждого вызова | Ф3 |
| `agent-document-intake/` | Multimodal-парсинг документов (паспорт, устав, протоколы). Модель: Gemma 4 26B-MoE. Выход: JSON с confidence по полям | Ф3 М1 |
| `agent-reconciliation/` | Сверка извлечённых данных с ЕГРЮЛ. Понимает синонимы, нормализует адреса, классифицирует расхождения по severity. Модель: Gemma 4 31B | Ф3 М1 |
| `agent-ubo-tracing/` | Рекурсивное построение графа владения (УБО). Самый сложный агент. Модель: Gemma 4 31B (thinking mode) | Ф4 М2 |
| `agent-risk-scoring/` | Объяснимый скоринг. **Скоринг — CatBoost (не LLM)**; LLM только формулирует текстовое объяснение SHAP-values | Ф4 М2 |
| `agent-conversational/` | Клиентский чат-помощник. Модель: T-pro / Vikhr (русскоязычные fine-tunes). Только read-only инструменты | Ф4 М3 |
| `agent-compliance-assistant/` | Помощник комплаенс-офицера (не клиента). RAG по нормативке, сводка кейса, ссылки на 115-ФЗ | Ф7 М4 |
| `rag-service/` | RAG поверх корпуса банковской нормативки (115-ФЗ, 375-П, 499-П, методички ЦБ). Qdrant, bge-m3 эмбеддинги, hybrid search | Ф3 |
| `eval-harness/` | Оценка качества агентов на размеченных корпусах. Запускается на каждый PR в `ai/`. **Начать первым (М0)** | Ф0 М0 |

## For AI Agents

### Working In This Directory

**Принципы автоматизации (архитектурные, нельзя нарушать):**

1. ИИ **никогда** не принимает финальное решение об отказе — только рекомендация. Человек всегда.
2. Положительные решения автоматизируются только в «зелёной зоне» (все проверки пройдены, low risk).
3. Каждое решение агента — объяснимо (список фактов и весов).
4. Агенты **stateless**: состояние заявки — в Temporal workflow через `ApplicationContext`.
5. Каждый вызов агента — событие в audit log (вход, выход, версия модели, версия промпта, latency, токены).

**Стратегия моделей:**

| Роль | Кандидат №1 | Fallback |
|------|-------------|---------|
| Парсинг документов (vision) | Gemma 4 26B-MoE | Qwen 3.5-VL 32B |
| Сверка ЕГРЮЛ / UBO reasoning | Gemma 4 31B | Qwen 3.5 27B |
| UBO (thinking mode) | Gemma 4 31B thinking | Qwen 3.5 27B thinking |
| Клиентский чат (RU) | T-pro / Vikhr | Qwen 3.5 7B |
| RAG (нормативка 115-ФЗ и др.) | Gemma 4 26B-MoE | Qwen 3.5 27B |
| Эмбеддинги | bge-m3 | E5-mistral |
| Скоринг | CatBoost (не LLM) | XGBoost |

Агенты ссылаются на **роль** (`agent.document-intake.model = "production-vision"`), а не на конкретную модель. Маппинг — в `configs/tenants/{id}/ai/models.yaml`.

**Гардрейлы (обязательны, нельзя ослаблять):**
- PII-фильтр перед отправкой в LLM (паспортные данные, ИНН)
- Output safety check (запрет фраз: «гарантирую», «точно одобрят», «я уверен, что»)
- Запрет fine-tuning на production-данных (152-ФЗ)
- Версионирование промптов в Git (каждый промпт = файл в `ai/*/prompts/`)

### Testing Requirements

- **На каждый PR в `ai/`**: eval-harness fast subset (~50 кейсов), ~5 минут. Регрессия >5% по F1 — блокирует merge.
- Еженедельная полная регрессия всех корпусов.
- Pre-release gate: обязательная полная регрессия перед выкатом в prod.
- Shadow mode для новых моделей перед переключением в production (1 неделя).

### Common Patterns

Структура Python-агента:
```
agent-name/
├── main.py               # FastAPI app
├── agent.py              # LangGraph agent definition
├── tools.py              # function-calling tools
├── prompts/
│   ├── system.txt        # system prompt (версионируется в Git)
│   └── user.j2           # Jinja2 шаблон user message
├── schemas/              # Pydantic схемы input/output
├── tests/
│   └── test_agent.py
└── Dockerfile
```

Структура eval-harness:
```
eval-harness/
├── datasets/             # Тестовые корпуса (Git LFS)
│   ├── docs-parsing/     # Сканы + ground truth JSON (200 кейсов)
│   ├── reconciliation/   # Пары документ↔ЕГРЮЛ (500 кейсов)
│   ├── ubo-graphs/       # Структуры владения (100 кейсов)
│   ├── ru-banking-chat/  # Q&A пары (300 пар)
│   ├── rag-quality/      # Вопросы по нормативке (150 пар)
│   └── adversarial/      # Сложные/jailbreak кейсы (50 кейсов)
├── runners/              # Запуск через LLM Gateway
├── metrics/              # field-level F1, graph edit distance, ragas
├── reports/              # PostgreSQL + Grafana
└── ci/                   # GitLab CI интеграция
```

## Dependencies

### Internal
- `../packages/domain-model/` — типы (Python-генерация)
- `./llm-gateway/` — все LLM-вызовы идут через него (sibling-сервис внутри ai/)
- `../services/audit-service/` — логирование вызовов
- `../packages/audit-sdk/` — SDK записи событий

### External
- vLLM (inference engine, continuous batching, AWQ-квантизация)
- LangGraph (оркестрация агентов)
- FastAPI (HTTP API агентов)
- Qdrant (vector DB для RAG)
- ragas (RAG evaluation metrics)
- CatBoost + SHAP (риск-скоринг)
- bge-m3 (эмбеддинги)

<!-- MANUAL: -->
