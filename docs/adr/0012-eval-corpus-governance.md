# ADR-0012: Eval-corpus governance — per-corpus owner, semver, drift-detection и production sampling

**Status:** Accepted
**Date:** 2026-04-26
**Authors:** Главный архитектор, ML-тех-лид, Банковский эксперт (design partner)
**Reviewers:** Тех-лид backend, Security-инженер, Комплаенс-офицер

## Context

Eval-harness — инфраструктура объективной оценки агентов и моделей (`docs/technical-structure.md` § 7.5). Без управляемых тестовых корпусов выбор моделей и промптов превращается в субъективные обсуждения, что недопустимо для системы, через которую проходят регуляторно-чувствительные решения.

ADR-0011 (Стратегия выбора и роутинга LLM) явно делегирует процедуру promotion candidate → production на eval-harness; `technical-structure.md` § 7.5 фиксирует корпуса (`docs-parsing`, `reconciliation`, `ubo-graphs`, `ru-banking-chat`, `rag-quality`, `adversarial`) и их объёмы. Существующий шаблон корпусa зафиксирован в `ai/eval-harness/datasets/docs-parsing/README.md` — синтетические данные, нет реальных ПДн, owner — ML-инженер + банковский эксперт, до назначения owner'а корпус считается v0.

Силы, давящие на это решение:

- **Без owner'а корпус деградирует за 6 месяцев.** Прямая цитата из `technical-structure.md` § 7.5: «Owner корпусов — конкретный человек (банковский эксперт), без него корпус превращается в мусор за полгода». Это означает, что governance — это не «процесс на потом», а **обязательная часть ввода корпуса в эксплуатацию**.
- **15+ корпусов в перспективе.** На момент решения активно ~6 корпусов (см. таблицу § 7.5), к Phase 4–5 — `ubo-graphs`, расширения; к Phase 7 — реальные anonymized-кейсы из production sampling. Один владелец на всё — узкое место и точка ошибки. Структура governance должна масштабироваться.
- **Регуляторная чувствительность.** Корпуса используются для gating-критериев («модель не идёт в production, если F1 на корпусе X упал на >5%»). Подделка/недокументированное изменение корпуса = подделка регуляторной отчётности. Нужны механизмы целостности.
- **Никаких реальных ПДн в корпусе.** `security-architecture.md` § 5.2 защищает PII; ADR-0007 § Implementation Notes (`secrets-in-prompts`) уже линтит few-shot examples. Этот ADR должен зафиксировать тот же принцип для корпусов.
- **Production traffic sampling 1–5%** (§ 7.5) даёт реальные кейсы, которые **должны** попадать в корпус — иначе мы тестируем синтетику, а в продакшене драйфит. Но реальный трафик содержит ПДн, и его нельзя складывать в корпус как есть.
- **SLA на регрессию.** § 7.5 говорит «нужно зафиксировать в ADR»: что считаем регрессией — F1 на 5%? на 1%? для какой задачи? Этот ADR — место фиксации.
- **Versioning корпуса.** Шаблон в `ai/eval-harness/datasets/docs-parsing/README.md` уже фиксирует: «Изменение существующих кейсов запрещено без обоснования (метрики потеряют сравнимость во времени)». Это политика, но без формального semver она нарушается через год.
- **Drift detection.** Через 12 месяцев распределение клиентских запросов в production отдрейфит относительно корпуса, который собирался в Phase 3. Без автоматизированного механизма мы не узнаем, что наши eval-метрики перестали отражать реальность.
- **Promotion v0 → v1 корпуса.** Существующий шаблон фиксирует: «До назначения owner'а корпус считается v0 — не используется как gating-критерий». Нужны формальные критерии promotion.
- **Adversarial и political/regulatory neutrality.** Кейсы должны включать сложные, спорные, jailbreak-попытки (§ 7.5 — `adversarial` корпус). Их разметка требует совместной работы security и комплаенс — отдельный жанр governance.
- **Связь с ADR-0011.** Promotion model → production требует, чтобы корпуса были стабильными между прогонами; иначе сравнение «новая модель vs baseline» некорректно.

Если ничего не решить, корпуса будут жить в Git LFS как файлы, которые периодически правит кто-то «чтобы тест прошёл», и через год eval-harness потеряет свою ценность как gating-инструмент.

## Decision

Кодифицируем **per-corpus governance**: на каждый корпус — ответственный **банковский эксперт + ML-инженер as co-owner**. Все изменения корпусов — через PR с пометкой `eval-corpus`. Корпуса версионируются через **semver в README**, breaking changes (удаление кейсов, изменение `expected`) запрещены без обоснования и записи в `CHANGELOG.md`. Production traffic sampling **1–5%** дополняет корпуса анонимизированными кейсами по формальной процедуре. **Drift detection** — ежемесячный automated comparison embedding distribution baseline vs current.

Решение состоит из семи частей: (1) owner'ство, (2) формат и semver корпуса, (3) изменения через PR, (4) promotion v0 → v1, (5) production sampling pipeline, (6) drift-detection, (7) SLA на регрессию.

### Часть 1. Owner'ство per-corpus

Каждый корпус имеет **двух co-owner'ов**:

- **Banking expert / domain owner** — определяет содержание (ground truth, edge cases, регуляторная корректность).
- **ML-инженер / technical owner** — отвечает за технический пайплайн (формат, runner, метрики).

Owner'ы фиксируются в `ai/eval-harness/datasets/<name>/README.md` секции `## Owner` (это уже соглашение, см. существующий `docs-parsing/README.md`).

| Корпус | Domain owner | Technical co-owner |
|---|---|---|
| `docs-parsing` | Банковский эксперт по документам онбординга | ML-инженер |
| `reconciliation` | Банковский эксперт по комплаенсу + DPO | ML-инженер |
| `ubo-graphs` | Банковский эксперт по комплаенсу + Архитектор | ML-инженер |
| `ru-banking-chat` | Продакт + банковский эксперт по клиентскому опыту | ML-инженер |
| `rag-quality` | Юрист по банковскому праву | ML-инженер |
| `adversarial` | Security-инженер + комплаенс-офицер | ML-инженер |

**Правило ответственности:**

- Domain owner отвечает за «правильность»: ground truth, edge cases, актуальность регуляторики.
- Technical owner отвечает за «работоспособность»: runner запускается, метрики считаются, нет битых ссылок на fixtures.
- Без обоих owner'ов корпус считается **v0** и **не может быть gating-критерием** (см. часть 4).

**Что делает owner:**

- Approve или request changes на любой PR, меняющий корпус (`CODEOWNERS` enforcement).
- Раз в квартал — review корпуса: актуальность, покрытие, статистика прохождения (отсутствие too-easy-кейсов).
- Назначает delegate на время отпуска (PR без approve owner'а **блокирован**, тимлид не может override).

**Если owner ушёл:**

- Корпус возвращается в **status: stewarded** (managed by ML-tech-lead temporarily) на ≤30 дней.
- В этот период gating всё ещё применяется, но любой regression-block требует двух approve со стороны: ML-тех-лид + продакт (для бизнес-корпусов) или security (для regulatory-корпусов).
- Через 30 дней без нового owner'а корпус **freezed**: версия не меняется, но gating сохраняется на старой baseline. Это сигнал руководству о необходимости назначить owner'а.

### Часть 2. Формат и semver корпуса

Каждый корпус — каталог в `ai/eval-harness/datasets/<name>/`:

```
ai/eval-harness/datasets/<name>/
├── README.md               -- описание корпуса, owner, версия, изменения
├── CHANGELOG.md            -- semver-история (добавлено в этом ADR)
├── manifest.yaml           -- formal-описание: версия, обязательные поля, метрики
├── cases/                  -- сами кейсы (JSONL/JSON), Git LFS для бинарных fixtures
│   ├── passport-001.json
│   ├── passport-002.json
│   └── ...
└── fixtures/               -- (опц.) PDF/PNG-сэмплы, Git LFS
```

`manifest.yaml`:

```yaml
name: docs-parsing
version: 1.4.0                      # semver
status: production                   # v0-stewarded | v0-experimental | candidate | production | deprecated
domain_owner: banking-expert-1       # ссылка на person в реестре команды
technical_owner: ml-engineer-2
created_at: 2026-04-12
last_promoted_at: 2026-04-26
total_cases: 247
adversarial_share: 0.12              # доля adversarial-кейсов в корпусе
metrics:
  primary: field_coverage_f1
  secondary: [json_validity_rate, latency_p99_ms]
gating:
  primary_threshold: 0.85             # модель проходит, если F1 ≥ 0.85
  regression_threshold_pct: 5         # блок merge если падение > 5%
  cost_per_correct_max_kop: 50        # cost-aware gate
expected_models:                     # модели, для которых корпус валиден
  - role: document-vision
license: synthetic-only              # все кейсы синтетические; запрет реальных ПДн
```

**Semver-конвенция корпуса:**

| Изменение | Bump | Примеры |
|---|---|---|
| **PATCH** | Незначимая правка без изменения метрик: typo в README, форматирование | Не должно менять scores |
| **MINOR** | Добавление новых кейсов; не меняет существующие | Промоушн, baseline пересчитывается; non-breaking |
| **MAJOR** | Удаление/изменение существующих кейсов; смена формулы метрики; смена `expected` | **Breaking** — требует архитектора, comparable-метрики теряются |

CHANGELOG.md ведёт запись каждого bump'а:

```
## 1.4.0 — 2026-04-15
### Added
- 47 кейсов charter v2 (новые форматы устава 2026 года).
### Changed
- Рекалькуляция baseline: F1 с 0.91 → 0.93 (порог поднят).
### Owner approved
- banking-expert-1, ml-engineer-2

## 1.3.0 — 2026-03-20
...
```

### Часть 3. Изменения через PR с пометкой `eval-corpus`

Любая правка в `ai/eval-harness/datasets/<name>/` обязана:

1. **Метка `eval-corpus`** на PR (CI-линтер enforce).
2. **Bump `version` в `manifest.yaml`**: PATCH/MINOR/MAJOR (CI-линтер `corpus-version-bump-required`).
3. **Запись в `CHANGELOG.md`** в категориях Added/Changed/Removed (CI-линтер).
4. **Approve обоих owner'ов** через `CODEOWNERS`-файл.
5. Для MAJOR — дополнительно approve **архитектора** и обоснование в PR description.
6. **CI-job `corpus-validate`** проверяет:
   - Manifest валиден по `corpus-manifest.schema.json`.
   - Все cases-файлы валидны по case-schema.
   - Нет PII (regex-сканер по паттернам паспортов, ИНН физлица, СНИЛС, реалистичных ФИО — описано в `security-architecture.md` § 5.2).
   - Версия в `README.md` совпадает с `manifest.yaml`.
   - Все adversarial-cases имеют поле `expected_safety_response`.

**CI-job `corpus-baseline-recompute`**: при MINOR/MAJOR bump автоматически прогоняет baseline-модели на новой версии корпуса и публикует delta в PR-комментарий: «baseline F1 был 0.91, стал 0.93 (+2%)». Owner смотрит и принимает решение.

### Часть 4. Promotion v0 → v1

**v0 (experimental)** — корпус только-только заведён:

- Минимум: README + manifest + cases. Owner — может быть только technical (ML-инженер «на старте»).
- **Не используется как gating** — eval-harness прогоняет, но регрессия не блокирует merge.
- Метрики только для информации.

**v1 (production)** — корпус становится gating-критерием:

- **≥ 100 кейсов** (для большинства корпусов; для `adversarial` — минимум 50, см. § 7.5).
- **Покрытие adversarial subset** — для всех бизнес-корпусов, минимум 5% adversarial-кейсов.
- **Назначен domain owner** (банковский эксперт), **назначен technical co-owner** (ML-инженер).
- **PR с promotion** требует:
  - Both owners approve.
  - Architecture lead approve.
  - Полный прогон baseline-моделей с фиксацией результата в `manifest.yaml.gating.primary_threshold`.
  - Запись в `CHANGELOG.md` строки `## 1.0.0 — Promoted from v0`.

**Status в Model Registry (см. ADR-0011)**: до promotion корпуса в v1 модели не могут использовать его metrics для перехода `candidate → production`. Это связь с ADR-0011 procedure (§ 3): «полная еженедельная регрессия» — на корпусах **производственного статуса**.

### Часть 5. Production traffic sampling pipeline

`technical-structure.md` § 7.5 фиксирует: **1–5% production-запросов** записываются для:

- Offline-разметки комплаенс-офицерами.
- Дообучения корпусов реальными кейсами.
- Drift detection.

**Pipeline:**

```
[агент в production] ──▶ [llm-gateway] ──▶ [LLM]
                              │
                              ▼ (1-5% sampling, per-tenant rate)
                       [llm_sample_embeddings]   (pgvector, см. ADR-0008)
                       [audit-event с raw payload — encrypted, retention 90 дней]
                              │
                              ▼ (manual selection by compliance-officer)
                       [staging corpus candidate set]
                              │
                              ▼ (anonymization + owner review)
                       [PR в eval-corpus с пометкой 'corpus-from-production']
                              │
                              ▼
                       [новая MINOR версия корпуса]
```

**Anonymization** — обязательно перед записью в корпус:

| Тип PII | Замена |
|---|---|
| ФИО | Случайное ФИО из синтетического словаря |
| ИНН физлица | Случайный валидный ИНН с корректной контрольной суммой |
| Паспорт серия+номер | Случайный валидный |
| Адрес | Случайный из синтетического |
| Дата рождения | Случайная в диапазоне ±5 лет |
| ОГРН/ИНН юрлица | Сохраняем (это публичная информация в ЕГРЮЛ) |
| Текст вопроса клиента | Сохраняем структуру, обфусцируем имена |

Анонимизация выполняется `tools/data-generator/` (см. § 2). Утилита читает sample, применяет правила, выдаёт sanitized-cases. Owner валидирует, что (а) anonymization не сломал семантику кейса, (б) ground truth корректен.

**Этический и юридический gate:** анонимизированный кейс может попасть в корпус только если:

- DPO банка-источника подтвердил (для banked-tenants — раз в квартал на batch).
- Кейс не содержит метаданных, позволяющих re-identification (например, очень специфичные ОКВЭД + регион + дата — могут идентифицировать конкретное юрлицо в малом регионе).
- Adversarial-кейсы из production требуют security-approve дополнительно.

### Часть 6. Drift detection

Ежемесячный automated job (Temporal-workflow `MonthlyCorpusDrift`):

1. Для каждого production-корпуса вычисляется **embedding distribution** (через `bge-m3`-эмбеддер, см. ADR-0008): mean vector, covariance, distribution percentiles.
2. Для production traffic sampling за последний месяц — то же самое.
3. **Сравнение через KL-divergence + Wasserstein distance** между распределениями.
4. **Threshold:**
   - KL > 0.3 → P2 алерт «drift сигнал»; owner информирован.
   - KL > 0.5 → P1 алерт «значимый drift»; запускается review корпуса; обсуждение promotion новых production-кейсов.
   - KL > 0.8 → P0 алерт «критический drift»; eval-метрики корпуса помечены `unstable`, gating ослабевает до warning-mode (но не отключается).

5. **Автоматический PR-suggester**: workflow создаёт PR `chore: corpus drift signal for <name>` с:
   - Сравнительными графиками distributions.
   - Списком top-20 production-кейсов, наиболее далёких от корпуса (кандидаты в `production sampling pipeline`).
   - Рекомендацией: extend corpus / re-baseline / no-action.

6. Owner смотрит PR, решает действие. Если no-action — закрывает с обоснованием.

**Baseline distribution** для каждого корпуса фиксируется при promotion в v1; перерасчёт — только при MAJOR bump.

### Часть 7. SLA на регрессию

Для каждой задачи фиксируется в `manifest.yaml`:

| Корпус | Главная метрика | Regression threshold | Действие при превышении |
|---|---|---|---|
| `docs-parsing` | field_coverage_f1 | > 5% | block merge |
| `reconciliation` | precision_on_discrepancies | > 3% (precision критична) | block merge |
| `ubo-graphs` | exact_match_ubo_list | > 5% | block merge |
| `ru-banking-chat` | combined (ROUGE+RU-quality+LLM-judge) | > 7% | block merge + product review |
| `rag-quality` | faithfulness (ragas) | > 3% (regulatory-критично) | block merge + legal review |
| `adversarial` | safety_pass_rate | > 0% (любая регрессия!) | block merge + security review |

Gating применяется автоматически в CI Nx-таргете `eval-fast` на каждом PR в `ai/` и в `eval-full` на еженедельном прогоне. Превышение порога → exit-code != 0 → CI-failure → merge заблокирован.

**Override-policy:** в исключительных случаях (например, корпус устарел, regression — false-positive) можно сделать override через тег `[eval-regression-ok: <reason>]` в commit-сообщении + approve owner'а корпуса + approve ML-тех-лида. Override записывается в audit log как event `eval.regression_override`.

### Что НЕ покрывает решение

- **Конкретные алгоритмы метрик** (`field_coverage_f1`, ragas-implementation) — описаны в `ai/eval-harness/metrics/` и могут эволюционировать без ADR; смена формулы метрики = MAJOR bump корпуса.
- **Шум в production sampling** (например, как обнаружить что 3% запросов — synthetic stress-test, а не реальный трафик) — операционная задача `llm-gateway`, отдельный механизм.
- **Корпуса для не-LLM моделей** (CatBoost для risk scoring, см. ADR-0011) — следуют тем же принципам governance, но метрики и формат отличаются; описаны в `ai/eval-harness/datasets/risk-scoring/`.
- **Cross-corpus correlation analysis** (например, «модель X лучше на docs-parsing но хуже на reconciliation — почему?») — research-задача, не governance.

### Когда пересматривается решение

- **Появление 30+ корпусов** — может потребоваться более структурированный owner'ский tier (lead per category).
- **Регуляторное требование внешнего аудита корпусов** (например, ЦБ требует независимого верификатора эталонной разметки) — потребует formal third-party governance.
- **Drift-detection даёт P0 чаще раза в месяц** для большинства корпусов — означает, что корпуса слишком статичны и нужен continuous-update-pipeline вместо monthly.
- **Появление federated-mode банков** (см. ADR-0011), которые хотят свои собственные корпуса — потребует расширения governance на per-tenant корпуса.

## Alternatives Considered

### Альтернатива 1: Один global owner на все корпуса

**Плюсы:**

- Простая модель ответственности.
- Единая точка координации.
- Минимум организационного overhead.

**Минусы:**

- **Узкое место:** один человек на 15+ корпусов с 100–500 кейсов каждый — невозможно поддерживать качественный review.
- **Domain expertise** разная: эксперт по документам ≠ юрист по 115-ФЗ ≠ security-инженер для adversarial. Один человек физически не может owner'ить все.
- **Bus factor**: уход одного человека = деградация всех корпусов одновременно.
- Прямо противоречит § 7.5: «Owner корпусов — конкретный человек (банковский эксперт)» — там единственное число про **отдельный** корпус, не про все вместе.

**Причина отклонения:** не масштабируется на 15+ корпусов и разную domain expertise.

### Альтернатива 2: Комитет из 3 человек на все корпуса

**Плюсы:**

- Распределённая ответственность.
- Quorum при review снижает bus-factor-риск.
- Каждый член комитета покрывает свою domain area.

**Минусы:**

- **Координация дороже:** PR ждёт quorum-approve, который собирается дольше, чем индивидуальный.
- **Индивидуальная ответственность размывается** — «комитет одобрил» снимает personal accountability, что в banking-проекте плохо.
- **Не масштабируется**: 3 человека на 15 корпусов = в среднем 5 корпусов на человека всё равно.
- **Слабая connection с domain**: комитет — generalist'ы; конкретный banking-expert по уставам ООО vs специалист по adversarial-prompt-injection — разные люди.

**Причина отклонения:** размывает ответственность, теряет domain-specialization.

### Альтернатива 3: Owner — банковский эксперт от design partner банка

**Плюсы:**

- Максимальная regulatory-correctness: эксперт в реальном банке видит «как на самом деле».
- Естественная связь с pilot-банком (Phase 7).

**Минусы:**

- **Conflict of interest**: банк-партнёр заинтересован, чтобы корпуса отражали **его** workflow, не общий рынок. Через год корпус будет gating-критерием, оптимизированным под одного банка.
- **Доступность**: эксперт банка недоступен 9-to-5 для review PR; задержки на дни.
- **Конфиденциальность**: некоторые корпуса (`adversarial`) содержат security-сценарии, которые мы не хотим раскрывать на стороне.
- **Замена банка-партнёра** = потеря owner'ов всех корпусов одновременно.

**Причина отклонения:** не выдерживает long-term sustainability и introduces conflicts. Используем банковских экспертов **внутри нашей команды** (есть штатный banking expert, см. § 12 — состав команды) с привлечением design partner'ов как advisor'ов.

### Альтернатива 4: Без semver, всё через Git-history

**Плюсы:**

- Простота: каждый коммит — версия.
- Нет дополнительной дисциплины bump'ов.

**Минусы:**

- **Невозможно сравнивать метрики во времени:** «модель X в марте была 0.91 — это на каком корпусе?» Без явной версии — гадание.
- **CI не может ловить breaking changes** автоматически — каждое изменение помечено как «потенциальный breaking».
- **CHANGELOG отсутствует** — owner вынужден читать diff'ы кейс-файлов, чтобы понять, что изменилось.
- **Промоушн v0 → v1** теряет смысл без формального semver'а.

**Причина отклонения:** Git-history достаточен для кода, но недостаточен для regulatory-eval-артефактов.

### Альтернатива 5: Live корпус (corpus = production sampling, без Git)

**Плюсы:**

- Всегда актуален, нет drift.
- Минимум manual labor.

**Минусы:**

- **Воспроизводимость eval теряется:** прогон сегодня и завтра дают разные результаты, потому что корпус разный. Невозможно сравнивать модели.
- **PII** в корпусе требует анонимизации — это ETL-pipeline, который сам по себе становится source-of-bugs.
- **Owner-control теряется:** corpus меняется каждый день, owner не успевает review каждое изменение.
- **Регуляторика**: невозможно показать «вот эталон, против которого мы тестим» — он каждый день другой.

**Причина отклонения:** static корпус — необходимый artefact для воспроизводимости; production sampling — дополняет, не заменяет.

## Consequences

### Positive

- **Корпуса не деградируют.** Per-corpus owner = personal accountability, regular quarterly review.
- **Сравнимость метрик во времени** через semver: «модель в апреле была 0.91 на docs-parsing v1.3 → в июне 0.93 на docs-parsing v1.4» — со ссылкой на CHANGELOG понятно, что изменилось.
- **Промоушн v0 → v1 — четкая формальность.** До v1 корпус не gating; после — да. Никаких «полу-gating» состояний.
- **Production sampling формализован.** Реальные кейсы попадают в корпус через явный pipeline с anonymization, owner-review, DPO-approve. Никакого «случайно скопировал прод-данные».
- **Drift-detection automated.** Месячный workflow дает раннее предупреждение, не дожидаясь, пока баг проявится в продакшене.
- **SLA на регрессию выраженный, а не подразумеваемый.** Каждый корпус знает свой threshold; CI-block — детерминированный.
- **Adversarial и regulatory-критичные корпуса под спец-контролем** (security/комплаенс — обязательные approver).
- **Совместимо с ADR-0011 (LLM routing)**: процедура promotion model требует passing на production-корпусах — корпус с status: production по этому ADR.
- **Совместимо с ADR-0007 (промпт-менеджмент)**: те же principles (PR-flow, owner-approve, версионирование, CI-lint).
- **Решает прямой gap из § 7.5**: «SLA на регрессию — нужно зафиксировать в ADR» → зафиксировано.

### Negative

- **Bus factor для одиночных domain owner'ов.** Если banking-expert уволился — корпус во временном `stewarded` режиме 30 дней. Mitigation: явный delegate-механизм; co-owner ML-инженер может временно держать линию.
- **Cost of governance.** PR на корпус требует двух approve, иногда трёх (для MAJOR). Это лишние часы экспертов. Mitigation: для MINOR (добавление кейсов) — fast-path с auto-merge при passing CI и approve обоих owner'ов; PATCH — один approve.
- **Anonymization pipeline — нетривиальный код.** `tools/data-generator/` со sanitization-режимом — отдельная утилита, требует тестирования. Mitigation: golden-tests на anonymization (regex'ы PII, валидность контрольных сумм). 
- **Drift-detection даёт false-positives.** В первые месяцы порогами KL придётся играть. Mitigation: первые 3 месяца — alert-only режим, реальное gating включается с накоплением статистики.
- **15+ корпусов = 15+ owner-pairs.** Это координационный overhead для всей команды (банковские эксперты — узкий ресурс, см. § 12). Mitigation: один banking expert может owner'ить 2–3 связанные корпуса (например, `docs-parsing` + `reconciliation`); ML-инженер — 4–5.

### Neutral

- **Дополнительная дисциплина bump'ов.** Каждый PR требует corpus-version-bump. CI-линтер enforce'ит, но это привычка, которую команда должна выработать. Документация в `ai/eval-harness/CONTRIBUTING.md`.
- **Размер репо растёт.** Корпуса в Git LFS, но истории cases-файлов и CHANGELOG'ов — обычный Git. На горизонте 5 лет — десятки МБ. Не критично.
- **Audit-events для governance-actions** (corpus-promote, corpus-freeze, regression-override) — отдельный класс audit-events; идут в общий audit log без специального treatment.
- **Compatibility с ADR-0011 model promotion procedure.** Смена baseline корпуса (MINOR/MAJOR) автоматически инвалидирует существующие model gating-результаты — модели надо re-test'ить. Это нормально и ожидаемо, но требует communication с ML-командой при больших corpus-обновлениях.

## Implementation Notes

### Структура каталога corpus

```
ai/eval-harness/datasets/<corpus-name>/
├── README.md                          -- человеко-читаемое описание
├── CHANGELOG.md                       -- semver-история
├── CODEOWNERS                         -- @banking-expert-1 @ml-engineer-2
├── manifest.yaml                      -- formal-metadata
├── baseline.yaml                      -- baseline-метрики на momentum promotion
├── cases/                             -- JSONL/JSON-кейсы
└── fixtures/                          -- (опц.) PDF/PNG, Git LFS
```

### `manifest.yaml`-схема

`ai/eval-harness/schemas/corpus-manifest.schema.json` — JSON Schema для валидации manifest'ов всех корпусов. CI-таргет `corpus-manifest-lint` проверяет каждое изменение.

### CI-таргеты Nx (см. ADR-0004)

| Таргет | Что | Когда |
|---|---|---|
| `corpus-manifest-lint` | Валидация `manifest.yaml` по schema | Каждый PR с изменением корпуса |
| `corpus-version-bump-required` | Проверка, что version в manifest изменилась | Каждый PR |
| `corpus-changelog-required` | Проверка, что CHANGELOG.md обновлён | Каждый PR |
| `corpus-pii-scan` | regex-сканер на PII в cases | Каждый PR |
| `corpus-baseline-recompute` | Прогон baseline-моделей при MINOR/MAJOR | На MINOR/MAJOR PR |
| `eval-fast` | Прогон ~50 кейсов на задачу для каждой production-модели | Каждый PR в `ai/` |
| `eval-full` | Полная регрессия на всех production-корпусах | Еженедельно |
| `corpus-drift-monthly` | Drift-detection для всех v1 корпусов | 1-го числа месяца |

### CODEOWNERS пример

```
# ai/eval-harness/datasets/docs-parsing/CODEOWNERS
* @banking-expert-1 @ml-engineer-2

# ai/eval-harness/datasets/adversarial/CODEOWNERS
* @security-engineer-1 @compliance-officer-1 @ml-engineer-2

# ai/eval-harness/datasets/rag-quality/CODEOWNERS
* @banking-lawyer-1 @ml-engineer-2
```

GitLab/GitHub `CODEOWNERS` обеспечивает обязательный approve owner'а перед merge.

### Production sampling pipeline tooling

```
tools/eval-corpus-pipeline/
├── cmd/
│   ├── select-from-sampling/main.go     -- выбор top-N кейсов из llm_sample_embeddings
│   ├── anonymize/main.go                -- применение синтетических замен к выбранным
│   └── propose-pr/main.go               -- автогенерация PR в corpus с пометкой 'corpus-from-production'
└── internal/
    └── ...
```

Используется comlpiance-офицером банка-тенанта раз в квартал (или нашим ML-инженером для cross-tenant обогащения adversarial).

### Drift-detection workflow (Temporal, см. ADR-0001)

```go
// ai/eval-harness/internal/drift/workflow.go
func MonthlyCorpusDriftWorkflow(ctx workflow.Context) error {
    corpora := workflow.ExecuteActivity(ctx, ListProductionCorpora).Get(...)
    for _, c := range corpora {
        baseline   := workflow.ExecuteActivity(ctx, LoadCorpusEmbeddings, c).Get(...)
        production := workflow.ExecuteActivity(ctx, LoadProductionSampleEmbeddings, c.Tenants, lastMonth).Get(...)
        kl, w      := workflow.ExecuteActivity(ctx, ComputeDistributionDistance, baseline, production).Get(...)
        if kl > c.GatingThresholds.DriftCritical {
            workflow.ExecuteActivity(ctx, AlertP0, c, kl, w).Get(...)
        } else if kl > c.GatingThresholds.DriftWarning {
            workflow.ExecuteActivity(ctx, AlertP2_AndProposePR, c, kl, w).Get(...)
        }
    }
    return nil
}
```

Cron-trigger: `0 3 1 * *` (3:00 МСК первого числа каждого месяца).

### Этапы внедрения

| Этап | Что | Когда |
|---|---|---|
| 1 | Базовая структура `ai/eval-harness/datasets/<name>/` с README + manifest для существующих корпусов | Phase 0 (Ф0) |
| 2 | CI-таргеты `corpus-manifest-lint`, `corpus-version-bump-required`, `corpus-pii-scan` | Phase 0 |
| 3 | CODEOWNERS для каждого корпуса с назначенными owner'ами | Phase 0–Ф1 (по мере готовности owner'ов) |
| 4 | Promotion v0 → v1 для `docs-parsing`, `rag-quality` | Phase 3 (Ф3) — параллельно с production-моделями |
| 5 | Production sampling pipeline (anonymization tool + PR-suggester) | Phase 7 (Ф7) — реальный трафик появляется |
| 6 | Drift-detection workflow | Phase 7+1 квартал (нужны данные) |
| 7 | Promotion остальных корпусов в v1 | По мере готовности owner'ов |
| 8 | Расширение adversarial корпуса production-кейсами через security-pipeline | Continuous |

### Безопасность

- **PII-scan на CI** — обязательный gate, не может быть bypassed (даже override-тегом).
- **Anonymization tool — audit-tracked**: каждый кейс, попавший в корпус через production sampling, имеет audit-trail с `original_audit_event_id`, `anonymization_algorithm_version`, `dpo_approved_by`.
- **Adversarial cases — restricted access**: каталог `ai/eval-harness/datasets/adversarial/` имеет более строгий ACL в Git (require-MFA-for-push), хотя файлы plaintext — это сигнал security-team.
- **Override regression** записывается в audit log (`eval.regression_override`) и fixated в `platform.billing_events` как event `platform.eval_override` — для платформенной отчётности (см. ADR-0010, neutral consequence).

### Координация с ADR-0011 promotion

Procedure из ADR-0011 § 3 (candidate → production model) формально ссылается на этот ADR:

> «Полная еженедельная регрессия — модель прогоняется на всех датасетах для роли. Для production-промоушена требуется: главная метрика ≥ baseline по всем датасетам для роли.»

Это означает:
- Только корпуса со status: production в этом ADR могут быть gating-критериями для model promotion.
- При MAJOR bump'е корпуса (и пересчёте baseline) автоматически инвалидируется gating всех production-моделей этой роли — нужен повторный promotion (или явный re-baseline-PR с обоснованием «модель сохраняет свойства на новом корпусе»).

## References

- `docs/technical-structure.md` § 7.2 (стратегия моделей и роли), § 7.5 (eval-harness — фундаментальный документ для этого ADR; «Owner корпусов — конкретный человек», «SLA на регрессию — нужно зафиксировать в ADR»), § 12 (фазы — состав команды с банковским экспертом).
- `docs/security-architecture.md` § 5.2 (PII-классификация — что нельзя в корпусе), § 5.4 (PII-фильтр перед LLM — параллель с anonymization), § 7.1 (Secure Development Lifecycle — корпус — артефакт SDLC), § 8.2 (audit-events для governance).
- `docs/product-vision.md` § 5 (юнит-экономика — eval-quality напрямую влияет на per-event оплату через ADR-0010).
- `ai/eval-harness/datasets/docs-parsing/README.md` — существующий шаблон корпуса, фиксирует базовые соглашения, на которых построен этот ADR.
- ADR-0001 (Temporal): drift-detection — Temporal-workflow с месячным cron-trigger.
- ADR-0002 (Schema-per-tenant): production sampling из `tnt_<id>.llm_sample_embeddings` (см. ADR-0008) — per-tenant; cross-tenant обогащение корпуса требует анонимизации **до** агрегации.
- ADR-0004 (Nx): все corpus-CI-таргеты — Nx-проекты с правильными `inputs`/`outputs` для affected detection.
- ADR-0005 (Стратегия миграций БД): corpus governance не требует миграций; `llm_sample_embeddings` (отдельный source) мигрируются стандартным flow.
- ADR-0007 (Промпт-менеджмент): параллельный паттерн governance (PR-flow, owner-approve, semver, CI-lint); промпты и корпуса — две стороны одного eval-цикла.
- ADR-0008 (Vector DB Qdrant vs pgvector): production sampling embeddings живут в `tnt_<id>.llm_sample_embeddings` (pgvector); drift-detection использует bge-m3 эмбеддер тех же характеристик.
- ADR-0010 (Биллинг и audit log): governance-events (corpus-promote, regression-override) идут в audit log; release-correlated события могут попадать в billing для платформенной отчётности (без денежной части).
- ADR-0011 (LLM routing): procedure candidate → production использует production-status корпуса этого ADR как gating-источник; MAJOR bump корпуса инвалидирует существующие model promotion'ы.
- [Semantic Versioning 2.0.0](https://semver.org/) — конвенция для корпусов.
- [Wasserstein distance for distribution comparison](https://en.wikipedia.org/wiki/Wasserstein_metric) — drift-detection метрика.
- [ragas — RAG evaluation framework](https://github.com/explodinggradients/ragas) — для `rag-quality` корпуса.
- [russe-superglue](https://russiansuperglue.com/) — основа для `ru-banking-chat` метрик.
