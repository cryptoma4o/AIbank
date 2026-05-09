<!-- Parent: AGENTS.md -->
<!-- Generated: 2026-05-08 -->
<!-- Status: Draft v1 — sales/architecture overview -->
<!-- Owner: ML/AI lead + продакт -->

# AI/ML возможности AIbank

Документ описывает что именно делает AI/ML в платформе, как устроен
стек, какие задачи решает и что **сознательно** не делает по соображениям
compliance и объяснимости.

Связано с:
- [docs/system-overview.md](system-overview.md) — общая архитектура
- [docs/security-architecture.md](security-architecture.md) §8 — обработка ПДн в AI-пайплайнах
- [docs/adr/0011-llm-model-selection.md](adr/0011-llm-model-selection.md) — выбор моделей по ролям
- [docs/services-roadmap.md](services-roadmap.md) — план развития AI-агентов

---

## Архитектурный обзор

AI/ML работает в 3 разных слоях, каждый — для конкретной бизнес-задачи:

```
┌─────────────────────────────────────────────────────────────────┐
│  6 LLM-агентов (узкие операции, не финальные решения)           │
│  ai/agent-{document-intake, reconciliation, ubo-tracing,        │
│            risk-scoring, conversational, compliance-assistant}   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  ai/llm-gateway — единая точка прокси к моделям                  │
│  vLLM на GPU: Gemma 4 (vision+text), Qwen 3.5 (reasoning),      │
│  T-pro / Vikhr (ru-chat). Per-tenant model routing.              │
└─────────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
┌──────────────┐       ┌──────────────┐       ┌──────────────┐
│ ai/rag-      │       │ services/    │       │ ai/eval-     │
│ service      │       │ risk-engine  │       │ harness      │
│              │       │              │       │              │
│ Qdrant +     │       │ CatBoost     │       │ Регрессион-  │
│ BGE-M3 +     │       │ ONNX +       │       │ ные тесты    │
│ reranker     │       │ SHAP         │       │ на каждом PR │
└──────────────┘       └──────────────┘       └──────────────┘
   knowledge-base         deterministic ML       quality gate
```

Принцип: **LLM делает интерпретацию и поиск, ML — численное решение,
человек — финальный approve/reject.**

---

## Слой 1. LLM-агенты (6 шт)

Все живут в `ai/`, ходят в `llm-gateway`. Каждый агент решает одну
бизнес-задачу с фиксированным контрактом (input/output JSON).

### 1.1 agent-document-intake — извлечение полей из документов

**Когда вызывается:** этап 6 формы — после загрузки документа в
document-service.

**Что делает:**
- Получает PDF/JPG (паспорт, устав, лист записи ЕГРЮЛ, протокол ЕИО, лицензия, отчётность)
- Запускает OCR (Tesseract или vision-LLM Gemma 4-vision для сложных layout'ов)
- LLM извлекает структурированные поля по схеме типа документа
- Возвращает JSON `{fields: {...}, confidence: 0.0-1.0, low_confidence_fields: [...]}`

**Примеры extractions:**
- Из паспорта: серия/номер/дата выдачи/код подразделения/место рождения/ФИО
- Из устава: дата создания, наименование, размер уставного капитала, виды деятельности, сведения об ЕИО
- Из листа записи ЕГРЮЛ: ИНН/ОГРН/КПП/адрес/дата регистрации/учредители

**Где экономия:** оператор не вбивает поля руками — только review низкоконфидентных (`confidence < 0.85`).

### 1.2 agent-reconciliation — сверка анкета vs ЕГРЮЛ vs документы

**Когда вызывается:** этап 7 — параллельно AML-проверкам.

**Что делает:**
- Берёт три источника: что заявил applicant в анкете (этап 2), что ЕГРЮЛ вернул через ext-egrul, что agent-document-intake извлёк из устава
- Находит расхождения по правилам: «строгое совпадение» (ИНН, ОГРН, дата регистрации) и «семантическое» (адрес — учитываем сокращения, наименование — учитываем кавычки/тире)
- Возвращает `{ok: bool, discrepancies: [{field, in_form, in_egrul, in_document, severity}]}`

**Severity-уровни:**
- `critical` — ИНН/ОГРН не совпадают → автоматический `reject`
- `major` — наименование != ЕГРЮЛ → переход в `manual_review`
- `minor` — адрес немного отличается → flag для оператора

### 1.3 agent-ubo-tracing — распутывание цепочки владения

**Когда вызывается:** этап 5 — после получения списка учредителей юрлица.

**Что делает:**
- Получает учредителей из ЕГРЮЛ
- Для каждого учредителя-ЮЛ рекурсивно идёт глубже (через ext-egrul) до тех пор, пока не найдёт физлиц
- Считает effective stake (произведение долей по цепочке)
- Помечает физлиц с effective stake ≥ 25% как UBO (115-ФЗ требование)
- Если цепочка обрывается на иностранном ЮЛ — возвращает status `requires_manual_check` с указанием юрисдикции

**Output:** `UBOGraph` (см. `packages/domain-model/schema.json $defs.UBOGraph`) — узлы (Person/LegalEntity) + рёбра (ownership stake) + ownership_chains.

**Где экономия:** распутывание 3-уровневой цепочки занимает у аналитика 2-4 часа; агент справляется за 30 сек + 5 мин review.

### 1.4 agent-risk-scoring — объяснение риск-скора (SHAP → текст)

**Когда вызывается:** этап 8 — после CatBoost-inference.

**Что делает:**
- Получает risk_score + raw SHAP-values от CatBoost (числовые impacts на каждый признак)
- Превращает в человеческий текст: «Высокий cash-share (45%) добавляет 18 пунктов к риску, потому что превышает порог 30% по 375-П»
- Структурирует Top-5 факторов риска и Top-3 митигирующих фактора

**Output для compliance-офицера:**
```json
{
  "score": 67,
  "level": "high",
  "top_factors": [
    {"name": "okved_high_risk_category", "impact": +28, "explanation": "ОКВЭД 64.99 в high-risk списке per per-tenant policy"},
    {"name": "cash_share_45pct", "impact": +18, "explanation": "Доля наличных операций 45% превышает порог 30% для UMC"}
  ],
  "mitigating": [
    {"name": "company_age_8_years", "impact": -8, "explanation": "Компания старше 5 лет, история операций"}
  ]
}
```

**Compliance benefit:** объяснимость отказа для регулятора (115-ФЗ ст. 7) — нельзя «потому что модель так сказала».

### 1.5 agent-conversational — чат-бот в кабинете клиента

**Когда вызывается:** в любой момент в /applications/{id}.

**Что делает:**
- Принимает вопрос на русском языке
- Идёт в RAG (см. слой 2) за контекстом: либо из регуляторики, либо из per-tenant FAQ, либо из текущего состояния заявки
- Отвечает с цитатами на конкретные документы
- Не выдумывает — если RAG не нашёл релевантного контекста, отвечает «обратитесь к support@bank»

**Типовые вопросы:**
- «Какие документы нужны для ИП?»
- «Когда мне ответят?»
- «Что значит статус documents_pending?»
- «Можно ли загрузить устав сканом?»
- «Какой тариф мне подойдёт?»

### 1.6 agent-compliance-assistant — помощник bank-комплаенсу

**Когда вызывается:** в админке web-admin при manual_review.

**Что делает:**
- Помогает оператору принять решение: «можно ли открыть счёт компании с ОКВЭД 96.09 при Росфинмон-флаге?»
- Идёт в RAG: 115-ФЗ + 375-П + 639-П + per-tenant policies
- Возвращает структурированный совет: «По 115-ФЗ ст. 7.4 нельзя без EDD; по policy банка рекомендация — отказ»
- НЕ принимает решение сам — финальный approve/reject делает человек

**Compliance benefit:** оператор тратит 2-3 минуты на review совета вместо 20-30 минут на ручной поиск регуляторных норм.

---

## Слой 2. RAG-сервис — единая база знаний

`ai/rag-service`. Используется agent-conversational и agent-compliance-assistant.

### Стек

| Компонент | Назначение |
|-----------|------------|
| **Qdrant** | Vector database (HNSW index, ~10K-100K векторов) |
| **BGE-M3 embedder** | Multi-lingual embeddings (1024-dim) — TEI sidecar на GPU |
| **bge-reranker-v2-m3** | Reranking top-20 → top-5 после dense retrieval |

### Корпус (что индексируется)

**Глобальный корпус (для всех тенантов):**
- Полные тексты: 115-ФЗ, 152-ФЗ, 187-ФЗ, 375-П, 499-П, 639-П
- Положения ЦБ РФ: 590-П, 153-И
- Письма ЦБ РФ (релевантные банковские)
- ФНС-методички по ЕГРЮЛ

**Per-tenant корпус:**
- Тарифные планы и документы оферты банка
- Внутренние процедуры комплаенс-отдела
- Per-tenant policy: blocked ОКВЭД, риск-пороги, regional restrictions
- Шаблоны типовых решений (одобрение / EDD / отказ)
- Исторические кейсы (разобранные заявки с метаданными)

### Поток retrieval

1. Вопрос → BGE-M3 → embedding (1024-dim)
2. Qdrant ищет top-20 ближайших векторов (HNSW)
3. bge-reranker-v2-m3 переранжирует → top-5
4. Top-5 chunks + question → LLM (Qwen 3.5 для reasoning, T-pro для русского чата) → answer с цитатами

---

## Слой 3. CatBoost риск-скоринг (детерминистическая ML)

`services/risk-engine` + ONNX runtime.

### Зачем CatBoost, а не LLM?

Регуляторика требует:
- **Воспроизводимость**: одинаковый input → одинаковый output (LLM дрейфует, CatBoost — нет)
- **Объяснимость**: SHAP-values по каждому признаку (LLM "потому что так показалось" не пройдёт ЦБ)
- **Версионируемость**: модель снятая 2026-Q1 vs 2026-Q3 — diff'ы признаков и весов
- **Аудит**: можно показать, что в production-decision использовалась модель с конкретным `model_version`

### Признаки модели (примеры)

| Категория | Признаки |
|-----------|----------|
| **Юрлицо** | ОКВЭД-кат (high/medium/low risk), возраст компании, тип ОПФ, географическая принадлежность, кол-во учредителей |
| **Финансы** | оборот заявленный, оборот по ЕГРЮЛ, доля наличных, ВЭД, валютные операции |
| **Учредители** | ИП vs ЮЛ, страна резидентства, наличие ПДЛ, FATCA/CRS флаги |
| **Внешние сигналы** | ЗСК-светофор, FSSP количество производств, sanctions match score |
| **Behavioural (anti-fraud)** | device fingerprint similarity, IP-geolocation match, time-of-day подачи |

### Output

```json
{
  "score": 67,
  "risk_level": "high",
  "recommendation": "manual_review",
  "factors": [...],  // SHAP-values
  "model_version": "v1.3.2",
  "transaction_limits": {
    "daily_outgoing": {"amount": 500000, "currency": "RUB"},
    "monthly_outgoing": {"amount": 5000000, "currency": "RUB"}
  }
}
```

### Обучение модели

| Этап | Данные |
|------|--------|
| Старт (cold) | Синтетика из `tools/data-generator` + 100-300 размеченных кейсов от банка-партнёра |
| Через 6-12 мес | Real-data из production (с тенантской подписью) — periodic retrain |
| Drift detection | `ai/eval-harness` ловит регрессию >5% на эталонном корпусе |

---

## Слой 4. Eval-harness — quality gate для AI

`ai/eval-harness`. Запускается на каждом PR с изменениями в `ai/`.

### Что проверяется

| Метрика | Корпус | Threshold |
|---------|--------|-----------|
| OCR field-extraction accuracy | 50 эталонных документов (паспорт + устав + лицензия) | ≥ 90% точных полей |
| Reconciliation F1 | 100 кейсов с известными discrepancies | ≥ 0.92 F1 |
| UBO-tracing recall | 30 цепочек 1-5 уровней | ≥ 95% UBO найдены |
| Conversational hallucination rate | 200 вопросов с known ground-truth | ≤ 2% галлюцинаций |
| Compliance-assistant accuracy | 80 кейсов с экспертно-разобранным ответом | ≥ 85% совпадение recommendation |
| CatBoost AUC | Hold-out test set | ≥ 0.84 (baseline 0.80) |

### Регрессионная политика

- Регрессия > 5% → блокирует merge
- Регрессия 2-5% → требует approve от ML-lead
- Улучшение → автоматический merge после code-review

---

## Что AI/ML НЕ делает (compliance boundaries)

| Запрет | Причина |
|--------|---------|
| **Не принимает финальное approve/reject** | 115-ФЗ требует human-decision; модель только рекомендует |
| **Не подписывает документы** | УКЭП ставит сам клиент через КриптоПро/VipNet |
| **Не делает payments** | Это АБС банка, AI здесь не лезет |
| **Не передаёт сырые ПДн в LLM-prompt** | 152-ФЗ + per-tenant policy: ПДн хешируются или токенизируются перед отправкой в модель |
| **Не использует cloud-LLM (OpenAI/Anthropic) для реальных данных** | 152-ФЗ локализация: только self-hosted vLLM в РФ |
| **Не auto-rejects «по подозрению»** | High-risk score → manual_review, не auto-decline (за исключением sanctions hit и Росфинмон match) |
| **Не "общается" вне темы** | agent-conversational ограничен RAG-корпусом, на off-topic отвечает «обратитесь к support» |

---

## Бизнес-эффект (где AI/ML экономит банку деньги)

При **5 000 заявок/мес** на банк (типичный среднего банк МСБ):

| Задача | Без AI | С AI | Экономия в час/мес |
|--------|--------|------|-------------------|
| Manual KYC review одной заявки | 4-8 ч оператора | 15-30 мин на review автоматических флагов | ~3500 ч/мес |
| OCR extraction документа | 5-10 мин ручного ввода | 2-5 сек + 0-5 мин review | ~700 ч/мес |
| Поиск UBO в 3-уровневой цепочке | 2-4 ч аналитика | 30 сек + 5 мин review | ~150 ч/мес |
| Ответ на типовой вопрос клиента | 2-24 ч ticket | мгновенно через чат-бот | ~250 ч саппорта |
| Объяснение отказа регулятору | 30-60 мин compliance-офицера | 0 (авто-генерация SHAP) | ~80 ч/мес |
| **ИТОГО** | | | **~4680 ч/мес** |

При ставке compliance-офицера ₽800-1500/час — экономия **₽3.7-7M/мес** на банк.

**TCO LLM-стека** (vLLM на 1× A100 80GB): ₽120-180K/мес. **ROI** ≈ 20-50× от затрат на инфру AI.

---

## Roadmap AI/ML

### M1 (до пилота)
- ✅ Архитектура агентов готова, mock backends работают
- 🟡 vLLM real deployment с Gemma 4 / Qwen 3.5 / T-pro
- 🟡 BGE-M3 + reranker (TEI sidecar)
- 🟡 CatBoost stub → ONNX-real
- 🟡 RAG-корпус: загрузить регуляторику + per-tenant policies

### M2 (production)
- 🆕 agent-fraud-detection (anti-fraud при подаче: device + IP + behavior)
- 🆕 agent-explanation расширенный — объяснение для регулятора по запросу
- Continuous learning: periodic retrain CatBoost на real-data
- Drift detection в eval-harness

### M3 (scale, year 2)
- 🆕 agent-translator — multi-language onboarding для иностранных компаний
- 🆕 agent-document-classifier — авто-классификация типа загруженного документа
- 🆕 vector-store auto-update: новые регуляторные письма ЦБ → автоматический re-index
- Advanced ML: neural retrieval, generative tax-planner для высоко-net-worth клиентов

---

## Связанные документы

- [docs/system-overview.md](system-overview.md) — общая архитектура платформы
- [docs/security-architecture.md](security-architecture.md) — обработка ПДн в AI-пайплайнах
- [docs/adr/0011-llm-model-selection.md](adr/0011-llm-model-selection.md) — выбор моделей по ролям
- [docs/services-roadmap.md](services-roadmap.md) — план развития
- [docs/commercial-proposal.md](commercial-proposal.md) — коммерческое предложение
- [ai/AGENTS.md](../ai/AGENTS.md) — гайд для разработчиков AI-агентов
