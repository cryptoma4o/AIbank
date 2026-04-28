# reconciliation eval corpus

Синтетический корпус для оценки качества сверки данных, извлечённых из документов,
с данными ЕГРЮЛ. Используется агентом `agent-reconciliation`.

## Состав v1

| Файл | Кейсы | Тип | Цель |
|---|---|---|---|
| `reconciliation-cases.json` | 30 | Сверка | Сравнение `extracted_data` ↔ `egrul_data` |

Подсеты:

- 10 кейсов **no-discrepancies** — данные полностью совпадают; ожидание: `matches=true`, `discrepancies=[]`, `requires_review=false`
- 10 кейсов **minor-discrepancies** — расхождения в адресе / ОКВЭД / КПП; `severity=low|medium`, без эскалации
- 10 кейсов **major-discrepancies** — разный директор, ИНН с опечаткой, отсутствует ОГРН; `severity=high`, `requires_review=true`

## Метрики

Скоринг кейса основан на сопоставлении массива `discrepancies`:

```
score = (correct_field_matches + correct_severity_matches) / total_expected
```

Кейс считается `pass` при score ≥ 0.8 (см. `harness/runner.py`).

## Источник данных

Все кейсы **синтетические**. Никаких реальных ПДн и реальных корпоративных данных.
Использованы общие генераторы (`gen_inn_legal`, `gen_ogrn`) с корректными
контрольными разрядами — те же, что и в `docs-parsing/_generate.py`.

## Owner

ML-инженер + банковский эксперт (ADR-0012, см. `docs/technical-structure.md` § 7.5).
До назначения owner'а корпус считается v0 — не используется как gating-критерий.

## Запуск

```bash
cd ai/eval-harness
python -m harness.cli run datasets/reconciliation/reconciliation-cases.json --agent-url http://localhost:8104
```

Перегенерация кейсов:

```bash
cd ai/eval-harness/datasets/reconciliation
python3 _generate.py
```

## Расширение

Новые кейсы добавляются через PR с пометкой `eval-corpus`. Изменение существующих
кейсов запрещено без обоснования (метрики потеряют сравнимость во времени).
Нумерация — последовательная по подсетам (`reconciliation-clean-011`,
`reconciliation-minor-011`, `reconciliation-major-011`).
