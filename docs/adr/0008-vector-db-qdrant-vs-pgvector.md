# ADR-0008: Vector DB — Qdrant для RAG-корпуса, pgvector для служебных embedding-сэмплов

**Status:** Accepted  
**Date:** 2026-04-26  
**Authors:** Главный архитектор, ML-тех-лид  
**Reviewers:** Тех-лид backend, DBA, Security-инженер

## Context

В платформе есть два **разных по природе** сценария, требующих хранения dense-векторов:

1. **RAG-корпус для агентов и `rag-service`.** Это корпус банковской нормативки (115-ФЗ, 375-П, 499-П, методички ЦБ, типовые уставы), внутренние политики банка-тенанта, шаблоны документов. См. `docs/technical-structure.md` § 7.4. Используется агентами (`agent-conversational`, `agent-reconciliation`, `agent-risk-scoring`-explainer) для поиска релевантных фрагментов перед формированием ответа. Объём — тысячи документов, десятки тысяч чанков, миллионы токенов в сумме.
2. **Служебные embedding-сэмплы.** Production traffic sampling из § 7.5 (1–5% запросов), embedding-кэш для дедупликации одинаковых запросов клиентов в `agent-conversational`, semantic clustering audit-событий для drift-detection, near-duplicate-детекция в `agent-document-intake` (один и тот же документ загружен дважды). Объём — миллионы записей в год на тенанта, ~70–80% живёт ≤90 дней (потом аггрегируется), оставшееся хранится с PII-сэмплами в audit на 5 лет.

Эти сценарии имеют принципиально разные характеристики нагрузки и операционные требования:

| Параметр | RAG-корпус | Embedding-сэмплы |
|---|---|---|
| Read pattern | Hot path: каждый запрос агента → 1–3 поиска top-k=5..10 | Преимущественно offline (drift, дедуп, аналитика) + редкий nearest-search в hot path |
| Write pattern | Batch: ингест корпуса при онбординге тенанта + обновления раз в неделю | Streaming: события трафика идут постоянно |
| Update frequency | Низкая (дни/недели) | Высокая (секунды) |
| Latency requirement | p99 < 100 мс на поиск | p99 < 500 мс на поиск, batch-метрики допустимо считать ночью |
| Hybrid search (dense+BM25) | **Обязательно** (см. § 7.4) | Не нужен (используем только dense для clustering/dedup) |
| Per-tenant изоляция | Жёсткая (один банк — отдельный namespace, своя нормативка) | Жёсткая (per-schema из ADR-0002) |
| Объём (per-tenant год 3) | ~50–500 тыс. векторов | 1–10 млн векторов |
| Транзакционная связь с PG-сущностями | Слабая (RAG-чанк — отдельный артефакт) | **Сильная** (sample ↔ application/audit_event одной транзакцией) |

Силы, давящие на это решение:

- **Hybrid deployment.** `docs/technical-structure.md` § 1 («Hybrid deployment»), § 10.2 («air-gapped bundle для on-prem») — никаких managed-сервисов и облачных-only решений. Любой выбранный движок должен разворачиваться в контуре банка как обычный K8s-pod из tarball.
- **РФ-friendly.** § 3.3 явно фиксирует Qdrant как «РФ-продукт, on-prem дружественный». `docker-compose.yml` уже включает Qdrant — это де-факто ингредиент платформы.
- **Persistence boundary.** ADR-0002 закрепляет PostgreSQL как канонический транзакционный store со schema-per-tenant. Введение pgvector-расширения **в платформенный кластер** означает дополнительную инсталляционную зависимость, но **не отдельный сервис**.
- **Hybrid search.** § 7.4 явно требует hybrid (dense + sparse BM25) для RAG-корпуса. Это обязательное требование для качества ответов агентов на регуляторных вопросах: BM25 ловит точные термины («ст. 7.3 115-ФЗ»), dense — семантические парафразы. Без hybrid recall падает на 15–25% на нашем eval-наборе rag-quality (§ 7.5).
- **Транзакционная целостность.** Записать audit_event и его embedding в одной DB-транзакции — это естественное требование для продакшен-семплов: либо оба записаны, либо ни одного. Если embedding в Qdrant, а audit в Postgres — нужен outbox-pattern + reconciliation job.
- **Стоимость владения on-prem.** § 10.2 — банк-клиент разворачивает платформу в своём контуре. Каждый дополнительный stateful-сервис = дополнительные ресурсы (мониторинг, бэкапы, апгрейды). Минимизация числа stateful-движков снижает barrier-to-entry.
- **152-ФЗ.** Embedding-сэмплы могут содержать прокси-ПДн (вопрос клиента на естественном языке про его собственный паспорт). Хранение их рядом с audit log в одном кластере упрощает per-tenant удаление по запросу клиента (`pg_dump --schema=tnt_<id>` из ADR-0002).

Если ничего не решить, либо всё запихнём в Qdrant (потеря транзакционной целостности embedding↔audit, дополнительная сложность per-tenant удаления через двойную координацию), либо всё в pgvector (отказ от hybrid search для RAG, либо самописный BM25-индекс), либо случайный микс — каждая команда выберет своё.

## Decision

Используем **Qdrant как production vector DB для RAG-корпуса** (нормативка, внутренние политики, шаблоны документов) и **pgvector как vector store для служебных embedding-сэмплов** в платформенном PostgreSQL-кластере. Других vector-движков не вводим.

### Разделение ответственности

| Сценарий | Где живёт | Чем индексируется |
|---|---|---|
| RAG-корпус (115-ФЗ, 375-П, банковская нормативка, шаблоны) | **Qdrant**, отдельная collection на тенанта | Hybrid: dense (`bge-m3`) + sparse (BM25 / Splade-тюн) |
| Per-tenant нормативка, загруженная банком в `web-admin` | **Qdrant**, та же collection с тегом `source: tenant_uploaded` | Same |
| Кэш embedding для дедупа клиентских вопросов | **pgvector** в схеме тенанта (`tnt_<id>.embedding_cache`) | dense (`bge-m3`) |
| Production traffic sampling (1–5% LLM-запросов) | **pgvector** в схеме тенанта (`tnt_<id>.llm_sample_embeddings`) | dense |
| Near-duplicate-детекция документов в `agent-document-intake` | **pgvector** в `tnt_<id>.document_hashes_embeddings` | dense (по чанкам OCR-результата) |
| Semantic clustering audit для drift-detection | **pgvector** в `platform.drift_embeddings` (offline) | dense |
| Few-shot examples, динамически подбираемые в промпт (см. ADR-0007) | **pgvector** в `tnt_<id>.fewshot_pool` | dense |

### Архитектурные принципы разделения

**Принцип A: данные с сильной транзакционной связью с PG-сущностями → pgvector.**

Если запись «без embedding'а бессмысленна» (например, audit-sample без своего вектора-индекса невозможно использовать для drift-detection) — embedding пишется в той же транзакции, что и сама запись. Это автоматически отвечает на вопрос «куда».

**Принцип B: данные, потребляемые hot-path агентами через hybrid search → Qdrant.**

RAG-корпус — это специализированный поисковый кейс, где мы платим всю стоимость отдельного движка ради hybrid search и performance-характеристик. pgvector с собственным BM25 на FTS-индексах в принципе возможен, но требует ручного скоринга и rerank-логики, которая в Qdrant идёт из коробки.

**Принцип C: per-tenant изоляция реализуется одинаково в обоих движках.**

В Qdrant — отдельная collection на тенанта (`tnt_<id>_rag`), доступ через сервисный аккаунт с jwt-claims по тенанту. В pgvector — стандартная schema-per-tenant из ADR-0002 (тот же `search_path`-механизм для таблиц с колонкой `vector`).

### Архитектура RAG-пайплайна (Qdrant)

```
[banking law sources]   [tenant-uploaded docs]
         │                      │
         └──────────┬───────────┘
                    ▼
            [chunker (500 tokens, overlap 100)]
                    │
                    ▼
        [embedder bge-m3 + BM25 tokenizer]
                    │
                    ▼
              [Qdrant ingest]
                    │
   collection: tnt_<id>_rag
   payload: {source, doc_id, chunk_id, text, metadata}
   indexes: dense (cosine, 1024 dim), sparse (BM25)
                    │
                    ▼
[rag-service / agents] ── hybrid search top-k=10 ──► [reranker bge-reranker-v2-m3] ──► top-5 chunks
```

Hybrid search в Qdrant выполняется через native Query API: dense + sparse в одном запросе, fusion через RRF (Reciprocal Rank Fusion) или weighted sum. Это устраняет необходимость в собственной orchestration-логике.

### Архитектура embedding-сэмплов (pgvector)

```sql
-- В каждой схеме tnt_<id>:
CREATE EXTENSION IF NOT EXISTS vector;     -- применяется в platform-миграции один раз

CREATE TABLE llm_sample_embeddings (
    sample_id    UUID PRIMARY KEY,
    audit_event_id UUID NOT NULL REFERENCES audit_events(id),
    role         TEXT NOT NULL,            -- agent role
    embedding    vector(1024) NOT NULL,    -- bge-m3
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX llm_sample_embeddings_hnsw
  ON llm_sample_embeddings
  USING hnsw (embedding vector_cosine_ops)
  WITH (m = 16, ef_construction = 64);

-- Аналогично для embedding_cache, document_hashes_embeddings, fewshot_pool.
```

Запись sample происходит в одной транзакции с записью audit-события: agent → audit-sdk → `BEGIN; INSERT audit_events ...; INSERT llm_sample_embeddings ...; COMMIT`. Это даёт эффективную «строгую согласованность».

### Развёртывание

**SaaS-режим:**

- Qdrant — отдельный StatefulSet в кластере, 3 ноды для HA (raft consensus).
- Один Qdrant-кластер на платформу, изоляция через collections per-tenant (как и `tnt_<id>_<...>`-схемы в Postgres из ADR-0002).
- pgvector — расширение в существующем платформенном PostgreSQL-кластере (тот же из ADR-0002). Никаких отдельных инстансов.

**On-prem-режим:**

- Qdrant — single-node в air-gapped bundle, без HA. Бэкап на основе snapshot Qdrant (встроенная команда `POST /collections/{name}/snapshots`).
- pgvector — то же самое, что и в SaaS, но в банковском Postgres-кластере. Установка расширения через стандартную миграцию (см. ADR-0005).

### Размер и SLA

| Метрика | Qdrant (RAG) | pgvector (sampl.) |
|---|---|---|
| Размер per-tenant год 3 | 50–500 тыс. векторов × 1024 × 4 байт ≈ 200 МБ–2 ГБ | 1–10 млн × 1024 × 4 байт ≈ 4–40 ГБ |
| Read p99 | < 100 мс на hybrid top-k=10 | < 500 мс на dense top-k=20 |
| Write throughput | Batch, не критично | До 1000/сек на тенант (не превысим в реальности) |
| Index type | Default HNSW + sparse | HNSW (`m=16, ef=64`) |

### Что НЕ покрывает это решение

- **Выбор embedder-модели** (`bge-m3` vs `E5-mistral`) — определяется eval-harness (см. ADR-0011 и `technical-structure.md` § 7.5). Этот ADR фиксирует только хранение, не выбор модели.
- **Стратегия chunking** документов — операционная деталь `rag-service`, не архитектурное решение.
- **Reranker-стратегия** (`bge-reranker-v2-m3` или альтернативы) — часть pipeline `rag-service`, не vector store.
- **GraphRAG / advanced RAG-патерны** (entities, communities) — Phase 4+; если потребуются, рассматриваем как новый ADR (могут потребовать графовой БД).

### Когда пересматривается решение

Этот ADR явно подлежит пересмотру в следующих случаях:

- **Ингест RAG-корпуса > 100 ГБ** на тенанта в горячих данных — потребуется sharding Qdrant, рассмотрим Milvus/Vespa.
- **Объём embedding-сэмплов > 100 млн** в одной схеме тенанта — pgvector HNSW начнёт деградировать, нужно отдельное хранилище под холодные сэмплы (Parquet в MinIO + Qdrant для hot subset).
- **Появление 50+ тенантов в SaaS** — может потребоваться decision о dedicated Qdrant per-tenant или sharding (на момент решения у нас 1–5 пилотных банков, далеко от этого порога).
- **Pgvector-расширение не получает обновлений** в течение года при наличии security-fix-ов — миграция на отдельный store.

## Alternatives Considered

### Альтернатива 1: Только pgvector — единый store для всех сценариев

**Плюсы:**
- Один stateful-движок на инсталляцию: меньше операционных расходов, особенно в on-prem.
- Естественная транзакционность для всех записей.
- Нет дополнительного network-hop в RAG-pipeline.
- pgvector активно развивается (быстрые улучшения HNSW, новые distance metrics).
- Backup/restore тривиален: `pg_dump --schema=tnt_<id>` уже включит embedding'и автоматически.
- Per-tenant удаление по 152-ФЗ — стандартная процедура из ADR-0002.

**Минусы:**
- **Нет zero-effort hybrid search.** Sparse BM25 в Postgres реализуется через `tsvector` + `ts_rank`, fusion с dense — это руками. Качество ниже native Qdrant Query API (по нашей инженерной оценке — 5–15% recall@5 на rag-quality eval, что для регуляторного RAG критично).
- **HNSW-индекс на 5–10 млн строках одной таблицы** в Postgres начинает мешать write throughput (rebuild и cleanup при autovacuum).
- **WAL-нагрузка от vector-обновлений.** Каждый embedding × 1024 × 4 байт = 4 КБ payload, плюс индекс-обновления. На 1000 sample/сек = 4 МБ/сек WAL только от embedding'ов, удваивает нашу базовую нагрузку (учёт audit'а в ADR-0002).
- **Reindex стоит дорого.** Если меняем embedder-модель (например, после eval-harness переходим с `bge-m3` на `E5-mistral`) — пересчёт 5–10 млн векторов с обновлением HNSW в Postgres = многочасовой maintenance window. В Qdrant это новая collection с переключением алиаса.
- **`technical-structure.md` § 3.3 явно фиксирует Qdrant.** Отказ требует обновления базового документа.

**Причина отклонения:** RAG-сценарий — отдельный жанр нагрузки с обязательным hybrid search. Принуждение его в pgvector жертвует качеством ответов (а это регуляторный продукт) ради экономии одного pod-а в on-prem-инсталляции. Сложность разделения окупается.

### Альтернатива 2: Только Qdrant — единый store для всех сценариев

**Плюсы:**
- Best-in-class search performance для всего.
- Один движок для всех vector-задач.
- Hybrid search и фильтры по payload из коробки.

**Минусы:**
- **Транзакционная целостность embedding ↔ audit_event ломается.** Запись в Qdrant и Postgres — две сети, две системы. Нужен outbox-pattern + reconciliation job, который сам по себе становится источником багов (лаг, дедуп, retry). Это значимый increment сложности для каждого сценария «sample + audit».
- **Per-tenant удаление сложнее.** Сейчас по запросу клиента (152-ФЗ) — `pg_dump --schema=tnt_<id>` + DROP SCHEMA даёт полное удаление; при Qdrant нужно дополнительно дропнуть N collection'ов с проверкой полного удаления.
- **Embedding-кеш для дедупа в hot-path** — это запрос из `agent-conversational` ещё **до** записи audit. Если сделан через Qdrant, добавляется лишний network-hop на 50–100 мс на каждый клиентский вопрос. В pgvector это запрос к локальной таблице.
- **Backup/restore разделяется на два инструмента** для тенанта — снимаем атомарность бэкапа.
- **Air-gapped on-prem усложняется**: Qdrant получает write-нагрузку от каждого LLM-вызова, это доп. latency и доп. ресурсы на банковской стороне. Для embedding-кэша это нерационально.

**Причина отклонения:** служебные embedding-сэмплы по природе транзакционно связаны с PG-сущностями. Принуждение их в Qdrant создаёт outbox-сложность ради решения, которое не использует фичи Qdrant (hybrid не нужен).

### Альтернатива 3: Weaviate

**Плюсы:**
- Очень богатый feature-set: hybrid search, GraphQL API, modular embedders, multi-tenancy native.
- Активная разработка, хороший community.
- Built-in vectorization через интеграцию с моделями.

**Минусы:**
- **Не РФ-продукт.** `technical-structure.md` § 3.3 явно отдаёт предпочтение Qdrant как РФ-friendly. Установка зарубежного движка требует дополнительного юридического обоснования при on-prem-внедрении в банке.
- **GraphQL-первый API.** Наши Go-сервисы используют преимущественно REST/gRPC; GraphQL-клиент — лишняя зависимость в Go-стеке.
- **Heavier runtime** (Go + JVM-плагины для модулей).
- **Multi-tenancy в Weaviate реализована по-своему** — не совсем привязывается к нашим Postgres-схемам.

**Причина отклонения:** Qdrant закрывает все наши требования (hybrid, on-prem, РФ) с меньшим operational footprint и уже выбран в `technical-structure.md`.

### Альтернатива 4: Milvus

**Плюсы:**
- Распределённая архитектура из коробки, сильная горизонтальная масштабируемость.
- Поддержка GPU-индексации для очень больших корпусов.
- Продвинутые алгоритмы (DiskANN, IVF, HNSW и комбинации).

**Минусы:**
- **Heavy stack:** etcd + MinIO + Pulsar + несколько Milvus-компонентов. Для нашего объёма (50–500 тыс. векторов на тенанта) это overkill в SaaS, в on-prem катастрофа по operational complexity.
- **Нет нативного hybrid search в production-mode** на момент решения (был announced, но stable не для нашей версии).
- **РФ-присутствие** — отсутствует.

**Причина отклонения:** оптимизирован для случаев «миллиарды векторов» и «GPU-search» — не наш профиль. operational footprint несоразмерен задаче.

### Альтернатива 5: Elasticsearch / OpenSearch с dense_vector

**Плюсы:**
- OpenSearch уже есть в стеке (`technical-structure.md` § 3.3, для поиска и аналитики).
- Hybrid search через combined query (BM25 + kNN) в одном запросе.
- Знакомая команде технология.

**Минусы:**
- **kNN в OpenSearch уступает по latency специализированным движкам** на нашем размере корпусов (HNSW в Lucene — медленнее Qdrant примерно в 2–3 раза на recall=0.95 по нашим тестам в pre-Phase 0).
- **Использование OpenSearch как vector DB смешивает зоны ответственности**: сейчас он для логов и full-text-поиска, добавление vector-индекса делает кластер критичным для regulatory-RAG-пути. Это усложняет capacity planning.
- **kNN-индекс per-tenant** в OpenSearch требует отдельного индекса и shard-планирования; per-tenant изоляция менее чёткая, чем «collection per tenant» в Qdrant.

**Причина отклонения:** OpenSearch остаётся для full-text/log-search; для production-RAG используем специализированный движок. Это разделение зон ответственности соответствует нашему общему принципу «hexagonal services».

### Альтернатива 6: Векторы во внешнем managed-сервисе (Pinecone, Yandex Cloud Vector Search и т.п.)

**Плюсы:**
- Нулевая operational нагрузка.
- SLA гарантирован провайдером.

**Минусы:**
- **Несовместимо с hybrid deployment** (см. § 1, § 10.2). On-prem банк не может ходить в Pinecone.
- **Vendor lock-in** на критичный компонент RAG.
- **Локализация ПДн** (RAG-чанки могут содержать прокси-ПДн из загружаемых клиентом документов) — см. 152-ФЗ.

**Причина отклонения:** прямое нарушение базового принципа hybrid deployment. Не рассматриваем даже для SaaS-only.

## Consequences

### Positive

- **Hybrid search для RAG из коробки** — критично для качества ответов агентов на регуляторных вопросах (§ 7.4, § 7.5 rag-quality eval). Native Qdrant Query API устраняет boilerplate.
- **Транзакционная целостность embedding ↔ PG-сущность** для audit-сэмплов и embedding-кэша. Нет outbox-сложности на hot path.
- **Один backup-инструмент per-tenant** для основной массы данных: `pg_dump --schema=tnt_<id>` забирает всё, включая embedding-сэмплы (см. ADR-0002). Qdrant snapshot — отдельный, но не на hot-path данных.
- **Per-tenant удаление по 152-ФЗ остаётся стандартной операцией.** Embedding-сэмплы уходят с DROP SCHEMA; RAG-collection в Qdrant — отдельная команда из runbook (короткий список).
- **Reindex embedder-модели в Qdrant** — атомарная операция через collection alias swap. В pgvector это часть обычной миграции (см. ADR-0005), но мы там реже меняем модель.
- **Эксплуатационная простота on-prem.** Один Qdrant-pod (single-node) + расширение pgvector в уже стоящем Postgres. Никаких новых stateful-сервисов сверх того, что уже зафиксировано в `technical-structure.md`.
- **Естественный рост на eval-harness.** Embedding-сэмплы в pgvector доступны для analytic-запросов через стандартный SQL — drift-detection и cluster-аналитика становятся обычными ETL-job'ами, а не отдельным pipeline через Qdrant API.

### Negative

- **Два движка для команды.** Backend-разработчик должен понимать два API. Mitigation: API Qdrant и pgvector используются в разных доменах (RAG vs sampling), редко требуются одновременно одним сервисом. `rag-service` — единственный, кто работает с Qdrant; service-layer pgvector скрыт за `audit-sdk` и embedding-cache-utility.
- **pgvector-расширение требует версионной совместимости с PostgreSQL.** Mitigation: PostgreSQL 16 + pgvector 0.7+ зафиксированы в нашем base-образе и в pin-файле (см. on-prem bundle, § 10.2). Major-апгрейд Postgres = координированный апгрейд pgvector.
- **HNSW-индексы на pgvector добавляют ~10–15% к VACUUM-времени** для таблиц с высоким write-rate (audit_events). Mitigation: партиционирование audit_events по дате (см. § 3.3 принцип) и отделение embedding'а в parallel-таблицу с независимым индексом.
- **Qdrant single-node в on-prem не имеет HA.** Mitigation: для on-prem допустимо — это RAG, а не транзакционная БД; recovery time из snapshot ~5–15 минут, в течение которых агенты деградируют (RAG отключается, ответы помечаются «без RAG-контекста» в audit log) — приемлемо для квартального on-prem-цикла.
- **Eval-harness (§ 7.5) должен учитывать оба движка.** Дополнительная конфигурация в test-окружении: testcontainers Qdrant + Postgres+pgvector. Mitigation: один docker-compose для dev/test уже это решает (Qdrant + Postgres стоят в `docker-compose.yml` корня репо).

### Neutral

- **Embedding-формат стандартизуется.** Все векторы — `bge-m3` 1024 dim cosine на старте; смена модели = координированный рефактор (ADR-0011 определяет процедуру). Размер и distance metric — параметры базового образа.
- **Retention-политики разные.** RAG-корпус — без retention, обновляется при изменении нормативки. Embedding-сэмплы — TTL 90 дней для большинства, 5 лет для тех, что связаны с audit-событиями уровня «решение». Это операционная деталь, фиксируется в runbook'е.
- **Наблюдаемость.** Метрики Qdrant (request rate, p99 latency, collection size) экспортируются в Prometheus-формате — встаёт в существующий VictoriaMetrics + Grafana. pgvector-метрики идут через стандартный `pg_stat_statements`.
- **Cross-tenant-аналитика по embedding'ам платформы** (например, общие паттерны drift между банками) выполняется на агрегатах в `platform.drift_embeddings`, как описано в ADR-0002 («Cross-tenant сервисы … получают агрегаты через витрины»).

## Implementation Notes

### Этапы

| Этап | Что | Когда |
|---|---|---|
| 1 | Базовый ингест RAG-корпуса (`rag-service` + Qdrant collection per-tenant) | Phase 3 (Ф3 в § 12) |
| 2 | Hybrid search через Query API + reranker | Phase 3 |
| 3 | Embedding-кеш в pgvector для `agent-conversational` | Phase 3 |
| 4 | Schema-extension для `llm_sample_embeddings` (миграция через ADR-0005) | Phase 3 |
| 5 | Drift-detection job на основе `platform.drift_embeddings` | Phase 4+ |
| 6 | Tenant-uploaded normative ингест (банк загружает свои политики) | Phase 4 |

### Нумерация миграций (см. ADR-0005)

```
services/<service>/migrations/tenant/
├── 010_create_extension_vector.up.sql
├── 011_create_embedding_cache.up.sql
├── 012_create_llm_sample_embeddings.up.sql
└── 013_create_document_hashes_embeddings.up.sql
```

`CREATE EXTENSION vector` идёт в **platform**-миграции один раз (расширение глобально на БД), но per-schema-таблицы создаются в шаблоне tenant-миграций (см. ADR-0005, schema-per-tenant).

### Изоляция в Qdrant

```
collection name: tnt_<id>_rag
payload index on:
  - source (banking_law | tenant_uploaded | template)
  - doc_id
  - chunk_id
access:  bearer JWT с claim tenant_id, gateway проверяет соответствие
```

`rag-service` хранит соответствие `tenant_id → collection_name` в `platform.rag_collections` (новая таблица, мигрируется через ADR-0005). Это та же модель, что и `platform.tenants` для PG-схем — единый паттерн дискавери.

### Размер и planning

- **Per-tenant Qdrant footprint year 1:** ~200 МБ нормативки + ~500 МБ banker's documents → 1 ГБ. На 30 тенантов — 30 ГБ. Helm-values: `qdrant.persistence.size = 200Gi` с запасом.
- **pgvector в platform-кластере**: `llm_sample_embeddings` ~ 4–40 ГБ × N тенантов. Требует партиционирование по дате (RANGE на `created_at`) и retention-job.

### Инструменты и SDK

- Go: `qdrant-go-client` (official), `pgx` для pgvector через `pgvector/pgvector-go`.
- Python (для AI-сервисов): `qdrant-client`, `pgvector` для SQLAlchemy.
- Embedder вызовов — через `llm-gateway` с ролью `embedder` (см. ADR-0011 и `default-routing.yaml`). Это даёт единую точку учёта стоимости и переключения модели.

### Тестирование

- testcontainers Qdrant + PostgreSQL+pgvector в Go и Python тестах.
- В CI Nx-таргет `vector-store-integration` (см. ADR-0004) запускает оба контейнера и валидирует полный pipeline ингеста+поиска.
- Eval-harness fast-subset для rag-quality (§ 7.5) использует фиксированный seed и эталонные документы — регрессия recall@5 более 5% блокирует merge.

### Безопасность

- Qdrant API защищён бирберер-токеном per-сервис, не per-тенант (тенант определяется payload-фильтром на стороне gateway). Это паттерн «service identity + tenant claim в request», совместимый с ADR-0010 (claims-based authorization, `security-architecture.md` § 3).
- pgvector-таблицы шифруются при записи только если содержат прокси-PII. По умолчанию embedding-вектор сам по себе не считается ПДн (низкая обратимость к исходному тексту), но `payload`-поля и `text`-чанки в RAG могут — для них обязательно field-level encryption через Vault Transit (`security-architecture.md` § 5.2).

## References

- `docs/technical-structure.md` § 1 (multi-tenant, hybrid deployment), § 3.3 (Qdrant в стеке), § 7.2 (стратегия моделей), § 7.4 (RAG-корпус — обоснование hybrid search), § 7.5 (eval-harness — regression-критерий для RAG), § 10.2 (air-gapped bundle).
- `docs/security-architecture.md` § 5.2 (field-level encryption для прокси-ПДн), § 5.4 (PII-фильтр перед LLM).
- ADR-0001 (Temporal): RAG-вызовы внутри Activities имеют per-call timeout, fallback при недоступности Qdrant — деградация без RAG-контекста с пометкой в audit.
- ADR-0002 (Schema-per-tenant): pgvector-таблицы создаются в схеме тенанта через стандартный шаблон миграций; этот ADR — потребитель механизма ADR-0002.
- ADR-0004 (Nx): миграции pgvector и тесты Qdrant встают в Nx-граф через стандартные таргеты `db-migrate` и `integration-test`.
- ADR-0005 (Стратегия миграций БД): миграция `CREATE EXTENSION vector` и DDL embedding-таблиц идут через `db-migrator` с уровнем «backwards-compatible» (add table); политика отката — стандартная.
- ADR-0007 (Промпт-менеджмент): few-shot examples, динамически подбираемые из `tnt_<id>.fewshot_pool` через pgvector, рендерятся в Jinja2 промпт; обе системы координируются через `llm-gateway`.
- ADR-0011 (LLM routing): embedder-модель тоже резолвится через role-based routing; смена `bge-m3 → E5-mistral` затрагивает оба vector store, требует координированной миграции.
- [Qdrant documentation](https://qdrant.tech/documentation/) — Query API, hybrid search, snapshot.
- [pgvector](https://github.com/pgvector/pgvector) — HNSW параметры, version compatibility matrix.
- [bge-m3 on HuggingFace](https://huggingface.co/BAAI/bge-m3) — мульти-вектор embedder, рекомендованный в § 7.2.
- [Reciprocal Rank Fusion](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf) — алгоритм слияния dense+sparse результатов.
