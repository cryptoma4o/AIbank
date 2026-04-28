# rag-service

Семантический + лексический поиск по корпусу банковской нормативки. Используется
агентами `agent-conversational`, `agent-compliance-assistant`, `agent-reconciliation`,
`agent-risk-scoring` (explainer-режим). См. `docs/technical-structure.md` § 4.4
и § 7.4, ADR-0008.

## Эндпоинты

| Метод | Путь          | Назначение                                                     |
|-------|---------------|----------------------------------------------------------------|
| POST  | `/v1/search`  | Поиск top-k фрагментов по тенанту с опциональным фильтром.    |
| POST  | `/v1/index`   | Загрузка/обновление документов в коллекцию тенанта.            |
| GET   | `/healthz`    | Liveness-пробка + информация об embedder-е и backend-е.        |

`/healthz` возвращает:

```json
{
  "status": "ok",
  "service": "rag-service",
  "backend": "qdrant",
  "embedder": "tei",
  "dim": 1024
}
```

## Embedders

Поддерживаются два варианта embedding-провайдера, выбор — через env `RAG_EMBEDDER`:

| Значение  | Класс           | dim  | Зависимости               | Когда используется                  |
|-----------|-----------------|------|---------------------------|-------------------------------------|
| `mock`    | `MockEmbedder`  | 384  | нет (чистый Python)       | CI, локальный dev, юнит-тесты       |
| `tei`     | `BGEM3Embedder` | 1024 | TEI-сидекар (HTTP)        | production                          |

`MockEmbedder` — детерминированный hash-based embedder для CI и тестов
retriever-а (см. `tests/test_retriever.py`). Он не использует ML-зависимостей
и стабилен между запусками.

`BGEM3Embedder` — тонкий HTTP-клиент к
[Text Embeddings Inference (TEI)](https://huggingface.github.io/text-embeddings-inference/).
TEI-сидекар запускает модель `BAAI/bge-m3` на GPU-ноде и принимает запросы
вида `POST /embed` с телом `{"inputs": ["text", ...]}`. Клиент:

- L2-нормирует векторы (чтобы dot-product в retriever-е оставался эквивалентом cosine);
- ретраит на `httpx.HTTPError` (по умолчанию 2 повтора с экспоненциальным backoff);
- авто-батчит вход, если он длиннее `batch_size` (по умолчанию 32);
- валидирует размерность ответа.

> **NB.** Мы намеренно НЕ тащим `sentence-transformers`/`torch` в зависимости
> rag-service. Модель живёт в TEI-сервере (отдельный pod), сервис общается с ней
> только по HTTP. Это держит rag-service легковесным и развязанным от GPU-стека.

### Production deployment (TEI sidecar)

В production-кластере TEI поднимается отдельным Deployment в namespace
`aibank-rag` с GPU-реквестом и nodeSelector-ом на GPU-пул. Веса BGE-M3
(~2.5 ГБ) грузятся в init-container из локального registry / S3-совместимого
хранилища банка (для air-gapped инсталляций — см. `docs/technical-structure.md`
§ 10.2).

Полная конфигурация — в `configs/sample-prod.yaml`. Пример Helm-values
лежит в `infrastructure/helm/services/rag-service/`.

## Переменные окружения

| Имя             | Default              | Назначение                                                                 |
|-----------------|----------------------|---------------------------------------------------------------------------|
| `RAG_EMBEDDER`  | `mock`               | Какой embedder использовать: `mock` или `tei`.                            |
| `RAG_TEI_URL`   | `http://tei:8080`    | URL TEI-сидекара (используется только при `RAG_EMBEDDER=tei`).            |
| `RAG_EMBED_DIM` | `384` (mock) / `1024`| Размерность вектора. Должна совпадать с dim collection в Qdrant.          |
| `RAG_BACKEND`   | `qdrant`             | Vector store: `qdrant` (production) или `memory` (тесты/dev).             |
| `QDRANT_URL`    | `http://qdrant:6333` | URL Qdrant.                                                                |
| `PORT`          | `8105`               | HTTP-порт rag-service.                                                    |

## Локальный запуск (без GPU)

```bash
cd ai/rag-service
python3.13 -m venv .venv && . .venv/bin/activate
pip install -e .[dev]

# Mock embedder + in-memory store — нет внешних зависимостей.
RAG_EMBEDDER=mock RAG_BACKEND=memory python -m uvicorn main:app --port 8105
```

## Тесты

```bash
cd ai/rag-service
pip install -e .[dev]
pip install respx                       # для тестов BGEM3Embedder
python -m pytest tests/ -v
```

Тесты покрывают:

- `tests/test_retriever.py` — retriever + MockEmbedder + MemoryStore.
- `tests/test_api.py` — FastAPI-эндпоинты (HTTP-уровень).
- `tests/test_embedder.py` — фабрика + BGEM3Embedder с замоканным httpx
  (happy path, L2-нормализация, retry, авто-батчинг).

## TODO / roadmap

- **Sparse-векторы для hybrid search** (BM25 / SPLADE) — TEI поддерживает
  `splade` как альтернативный backend, Qdrant `>= 1.10` принимает sparse.
  Сейчас лексическая часть hybrid-а реализована Jaccard-rerank-ом в
  `retriever.py` — это placeholder из MVP, см. ADR-0008 § «Что НЕ покрывает».
- **sentence-transformers fallback** для air-gapped без GPU (TEI требует
  cuda — без GPU latency p99 ~ 1–2с). В таком случае допустимо локальный
  CPU-инференс через ONNX runtime, но это отдельный профиль deps.
- **Per-tenant embed-model override** — некоторые банки могут потребовать
  модель, обученную на их домене. В этом случае фабрика должна научиться
  читать `tenant.yaml`-секцию `ai.embedder`, а Qdrant-collection-имя
  включать model-id (для безопасной миграции через alias swap).
- **Reranker** (`bge-reranker-v2-m3`) после top-k=10 dense recall — см.
  ADR-0008 § «Архитектура RAG-пайплайна».
