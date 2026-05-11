#!/usr/bin/env python3
"""fetch.py — скачивание текстов российских законов в docs/legal/raw/.

Алгоритм:
  1. Читает sources.yaml
  2. Для каждого документа:
     a. Если есть local_override и файл существует — пропускаем (его уже
        положили вручную, ничего не качаем).
     b. Иначе перебираем urls[] по порядку, первый отдавший 200 — сохраняем
        в docs/legal/raw/<id>.html.
  3. С --force — игнорировать существующие файлы (форс-перекачка).

Идемпотентен. Используется stdlib + httpx + pyyaml.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import httpx
import yaml

ROOT = Path(__file__).resolve().parent.parent.parent
RAW_DIR = ROOT / "docs" / "legal" / "raw"
SOURCES = Path(__file__).resolve().parent / "sources.yaml"

USER_AGENT = "Mozilla/5.0 (compatible; AIbank-legal-corpus/0.1; +https://aibank.local)"


def load_sources() -> list[dict]:
    data = yaml.safe_load(SOURCES.read_text(encoding="utf-8"))
    return data.get("documents", [])


def fetch_one(doc: dict, force: bool) -> tuple[str, Path | None]:
    """Скачать один документ. Возвращает (status, путь-к-файлу-или-None)."""
    doc_id = doc["id"]
    out = RAW_DIR / f"{doc_id}.html"

    # 1. local_override has priority.
    local = doc.get("local_override")
    if local:
        local_path = ROOT / local
        if local_path.exists() and local_path.stat().st_size > 100:
            return ("local", local_path)

    # 2. existing cached file
    if out.exists() and not force:
        return ("cached", out)

    # 3. try urls
    headers = {"User-Agent": USER_AGENT, "Accept": "text/html,application/xhtml+xml"}
    for url in doc.get("urls", []):
        try:
            with httpx.Client(timeout=30.0, follow_redirects=True, headers=headers, verify=False) as client:
                resp = client.get(url)
            if resp.status_code != 200:
                print(f"  [{doc_id}] {url} → HTTP {resp.status_code}, пробую следующий", file=sys.stderr)
                continue
            if len(resp.content) < 1000:
                print(f"  [{doc_id}] {url} → ответ слишком короткий ({len(resp.content)} байт), пробую следующий", file=sys.stderr)
                continue
            out.parent.mkdir(parents=True, exist_ok=True)
            out.write_bytes(resp.content)
            return (f"fetched({len(resp.content)//1024}KB)", out)
        except httpx.HTTPError as e:
            print(f"  [{doc_id}] {url} → {e!r}, пробую следующий", file=sys.stderr)
            continue

    return ("failed", None)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--force", action="store_true", help="игнорировать кэш, перекачать всё")
    args = ap.parse_args()

    docs = load_sources()
    if not docs:
        print("ERROR: нет документов в sources.yaml", file=sys.stderr)
        return 2

    print(f"[fetch] {len(docs)} документов, output={RAW_DIR}")
    print()

    failed = 0
    for doc in docs:
        status, path = fetch_one(doc, force=args.force)
        print(f"  {doc['id']:<10} [{status}] {path if path else '— скачать вручную в ' + doc.get('local_override', '?')}")
        if status == "failed":
            failed += 1

    print()
    if failed:
        print(f"WARNING: {failed} из {len(docs)} документов не удалось скачать.")
        print("Положи файл вручную в docs/legal/raw/<id>.html и перезапусти.")
        return 1
    print(f"OK: все документы доступны в {RAW_DIR}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
