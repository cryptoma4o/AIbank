#!/usr/bin/env python3
"""chunk_and_index.py — парсит HTML законов, режет на чанки, индексирует в rag-service.

Алгоритм:
  1. Читает sources.yaml + соответствующие файлы из docs/legal/raw/.
  2. BeautifulSoup4 → выделение основного текста (приоритет: <div class="document">,
     fallback на <body>).
  3. Нарезка на чанки:
     - Сначала пытается найти «Статью N» / «Пункт N» как маркеры.
     - Если структура не найдена — fixed-window 1200 chars, overlap 150.
  4. POST batch'ами (32 chunks) в rag-service /v1/index.

Метадата каждого chunk:
  {law_id, short, article (если есть), version_date, char_offset}

Параметры (env):
  RAG_SERVICE_URL  — http://localhost:8105 (default)
  RAG_TENANT_ID    — demo (default)
"""

from __future__ import annotations

import argparse
import os
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Iterator

import httpx
import yaml
from bs4 import BeautifulSoup

ROOT = Path(__file__).resolve().parent.parent.parent
RAW_DIR = ROOT / "docs" / "legal" / "raw"
SOURCES = Path(__file__).resolve().parent / "sources.yaml"

CHUNK_TARGET = 1200
CHUNK_OVERLAP = 150
CHUNK_MIN = 200
# TEI CPU image имеет hard-limit max_batch_size=8 (выводит WARN при старте).
# Если послать больше — embedding падает с 500. Поэтому 8.
BATCH_SIZE = 8

# Паттерны структуры — пробуем по убыванию специфичности.
ARTICLE_RE = re.compile(r"(?:^|\n)\s*(Статья\s+(\d+(?:\.\d+)?)[^\n]{0,150})", re.MULTILINE)
PUNKT_RE = re.compile(r"(?:^|\n)\s*(\d+(?:\.\d+){0,3})\.\s+", re.MULTILINE)


@dataclass
class Chunk:
    chunk_id: str
    text: str
    metadata: dict = field(default_factory=dict)


def load_sources() -> list[dict]:
    return yaml.safe_load(SOURCES.read_text(encoding="utf-8")).get("documents", [])


def extract_text(html: bytes) -> str:
    """HTML → чистый текст."""
    soup = BeautifulSoup(html, "lxml")
    # удалить script/style/nav
    for tag in soup(["script", "style", "nav", "header", "footer"]):
        tag.decompose()
    # приоритет 1: основной контейнер документа на pravo.gov.ru
    main = soup.find("div", class_="document") or soup.find("div", id="document_text") or soup.body
    if main is None:
        return ""
    text = main.get_text(separator="\n", strip=True)
    # сжать множественные newline
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text


def split_by_articles(text: str) -> list[tuple[str | None, str]]:
    """Разбить текст на блоки по «Статья N». Если статьи не найдены — один блок."""
    matches = list(ARTICLE_RE.finditer(text))
    if not matches:
        return [(None, text)]
    blocks: list[tuple[str | None, str]] = []
    for i, m in enumerate(matches):
        start = m.start()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(text)
        article_num = m.group(2)
        block = text[start:end].strip()
        blocks.append((article_num, block))
    # текст до первой статьи (преамбула) — отдельным блоком без article
    pre = text[: matches[0].start()].strip()
    if pre:
        blocks.insert(0, (None, pre))
    return blocks


def window_split(text: str, target: int = CHUNK_TARGET, overlap: int = CHUNK_OVERLAP) -> Iterator[str]:
    """Sliding window по символам. Для блоков длиннее target."""
    if len(text) <= target:
        yield text
        return
    start = 0
    while start < len(text):
        end = min(start + target, len(text))
        # стараемся резать по абзацу
        if end < len(text):
            nl = text.rfind("\n\n", start + target // 2, end)
            if nl != -1:
                end = nl
        yield text[start:end].strip()
        if end >= len(text):
            break
        start = end - overlap


def chunk_document(doc: dict, text: str) -> list[Chunk]:
    """Превращает текст одного закона в Chunk[]."""
    chunks: list[Chunk] = []
    blocks = split_by_articles(text)
    for article_num, block in blocks:
        for i, piece in enumerate(window_split(block)):
            if len(piece) < CHUNK_MIN:
                continue
            chunk_id_parts = [doc["id"]]
            if article_num:
                chunk_id_parts.append(f"st{article_num}")
            chunk_id_parts.append(f"p{i}")
            chunk_id = "_".join(chunk_id_parts)

            source_label = doc["short"]
            if article_num:
                source_label = f"{doc['short']} ст. {article_num}"

            chunks.append(
                Chunk(
                    chunk_id=chunk_id,
                    text=piece,
                    metadata={
                        "law_id": doc["id"],
                        "short": doc["short"],
                        "article": article_num,
                        "version_date": doc.get("version_date"),
                        "title": doc["title"],
                        "source_label": source_label,
                    },
                )
            )
    return chunks


def index_batch(client: httpx.Client, base_url: str, tenant_id: str, chunks: list[Chunk], doc: dict) -> dict:
    payload = {
        "tenant_id": tenant_id,
        "documents": [
            {
                "id": c.chunk_id,
                "text": c.text,
                "source": c.metadata["source_label"],
                "source_type": doc["source_type"],
                "metadata": c.metadata,
            }
            for c in chunks
        ],
    }
    resp = client.post(f"{base_url}/v1/index", json=payload, timeout=120)
    resp.raise_for_status()
    return resp.json()


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--recreate", action="store_true", help="(reserved) пересоздать коллекцию")
    ap.add_argument("--dry-run", action="store_true", help="только парсить, не отправлять в rag-service")
    ap.add_argument("--only", help="индексировать только указанные id, через запятую")
    args = ap.parse_args()

    base_url = os.environ.get("RAG_SERVICE_URL", "http://localhost:8105").rstrip("/")
    tenant_id = os.environ.get("RAG_TENANT_ID", "demo")
    only = set(args.only.split(",")) if args.only else None

    docs = load_sources()
    print(f"[chunk-index] base={base_url} tenant={tenant_id} docs={len(docs)} dry_run={args.dry_run}")
    print()

    total_chunks = 0
    failed = 0

    with httpx.Client() as client:
        for doc in docs:
            if only and doc["id"] not in only:
                continue
            raw = RAW_DIR / f"{doc['id']}.html"
            if not raw.exists():
                # попробуем .txt
                txt = RAW_DIR / f"{doc['id']}.txt"
                if not txt.exists():
                    print(f"  [{doc['id']}] ✗ файл {raw.name} не найден; запусти fetch.py", file=sys.stderr)
                    failed += 1
                    continue
                text = txt.read_text(encoding="utf-8", errors="replace")
            else:
                text = extract_text(raw.read_bytes())

            if len(text) < 500:
                print(f"  [{doc['id']}] ✗ текст слишком короткий: {len(text)} chars", file=sys.stderr)
                failed += 1
                continue

            chunks = chunk_document(doc, text)
            print(f"  [{doc['id']}] {len(text)} chars → {len(chunks)} chunks")
            total_chunks += len(chunks)

            if args.dry_run:
                for c in chunks[:3]:
                    print(f"    sample: id={c.chunk_id} len={len(c.text)} meta={c.metadata}")
                continue

            # batch отправка
            for i in range(0, len(chunks), BATCH_SIZE):
                batch = chunks[i : i + BATCH_SIZE]
                try:
                    result = index_batch(client, base_url, tenant_id, batch, doc)
                    indexed = result.get("indexed", len(batch))
                    print(f"    batch {i // BATCH_SIZE + 1}: indexed={indexed}")
                except httpx.HTTPError as e:
                    print(f"    batch {i // BATCH_SIZE + 1}: ✗ {e!r}", file=sys.stderr)
                    failed += 1

    print()
    if failed:
        print(f"WARNING: {failed} ошибок")
        return 1
    print(f"OK: всего {total_chunks} chunks проиндексировано в tenant={tenant_id}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
