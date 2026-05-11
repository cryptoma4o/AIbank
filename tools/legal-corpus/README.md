# Legal Corpus — индексация нормативки в RAG

Pipeline: **скачать → распарсить → нарезать → проиндексировать** российские
нормативные акты в `rag-service` (Qdrant + BGE-M3). После этого
`agent-compliance-assistant` отвечает с цитатами из актуальных норм.

## Состав

```
tools/legal-corpus/
├── sources.yaml          # список законов с URL-fallback'ами и метаданными
├── fetch.py              # скачивание HTML в docs/legal/raw/<id>.html
├── chunk_and_index.py    # парсер + нарезка + POST в rag-service
├── pyproject.toml        # httpx, beautifulsoup4, lxml, pyyaml
└── README.md             # этот файл
```

## Быстрый старт

```bash
# 1. Зависимости (один раз):
pip install -e tools/legal-corpus

# 2. Скачать (~10-30 МБ HTML на каждый закон):
python -m fetch
# → docs/legal/raw/{115-fz,152-fz,375-p}.html

# 3. Поднять rag-service (с TEI sidecar):
docker compose up -d tei rag-service
# жди ~2-3 мин пока TEI прогреется (BGE-M3 cold-load)

# 4. Индексировать:
RAG_SERVICE_URL=http://localhost:8105 python -m chunk_and_index

# 5. Smoke search:
curl -X POST http://localhost:8105/v1/search \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"demo","query":"пороги обязательного контроля 115-ФЗ","top_k":3}' \
  | jq '.hits[].source'
```

Готовые `make`-таргеты делают всё то же одной командой — см. `make rag-index`.

## Добавить новый закон

1. Открыть `sources.yaml`, добавить запись:
   ```yaml
   - id: "590-p"
     title: "Положение Банка России от 28.06.2017 N 590-П"
     short: "590-П"
     source_type: "regulation"
     version_date: "2026-01-01"
     urls:
       - "http://pravo.gov.ru/proxy/ips/?docbody&nd=102156432"
     local_override: "docs/legal/raw/590-p.html"
   ```
2. `python -m fetch` — скачает (или используй ручной файл если pravo.gov.ru
   опять не отдаёт).
3. `python -m chunk_and_index --only 590-p` — индексирует только новый закон.

## Что делать если pravo.gov.ru не отдаёт

Часто бывает: сайт возвращает 403/captcha. Тогда:

1. Открой URL в браузере, скачай HTML через **Save Page**.
2. Положи файл в `docs/legal/raw/<id>.html` (имя как в `sources.yaml:id`).
3. Или возьми **plain text** с consultant.ru / garant.ru через копипаст и
   сохрани в `docs/legal/raw/<id>.txt` — `chunk_and_index.py` понимает .txt
   как fallback.

## Метадата в Qdrant

Каждый chunk в Qdrant payload содержит:
```json
{
  "tenant_id": "demo",
  "document_id": "115-fz_st6_p0",
  "text": "...полный текст чанка...",
  "source": "115-ФЗ ст. 6",
  "source_type": "law",
  "metadata": {
    "law_id": "115-fz",
    "short": "115-ФЗ",
    "article": "6",
    "version_date": "2026-01-01",
    "title": "Федеральный закон...",
    "source_label": "115-ФЗ ст. 6"
  }
}
```

`source_label` используется агентами как читаемая ссылка в ответах.

## Версионирование

При смене редакции закона:
1. Скачать новый HTML
2. Обновить `version_date` в `sources.yaml`
3. `python -m chunk_and_index --recreate` (TODO: реально пересоздание ещё
   не реализовано в rag-service — пока можно вручную через Qdrant API:
   `curl -X DELETE http://localhost:6333/collections/rag_demo`)
