"""Hybrid retriever: dense (Embedder) + lexical-overlap re-rank.

В MVP «sparse» — это упрощённое пересечение токенов между запросом и
сниппетом (BM25-style placeholder). Когда в Phase-1 добавится bge-m3
плюс настоящий BM25 (через Qdrant 1.10+ sparse vectors или OpenSearch),
заменяем эту функцию `_lexical_score` — интерфейс retriever-а не меняется.
"""
from __future__ import annotations

import logging
import re
from dataclasses import dataclass
from typing import Any

from models.schemas import (
    IndexDocument,
    SearchFilters,
    SearchHit,
)
from service.embedder import Embedder
from service.qdrant_client import (
    Match,
    Point,
    VectorStore,
    collection_name,
)

log = logging.getLogger(__name__)

DEFAULT_TOP_K = 5
SNIPPET_MAX = 480
DENSE_WEIGHT = 0.7
LEXICAL_WEIGHT = 0.3


_TOKEN_RE = re.compile(r"[\wа-яёА-ЯЁ]+", re.UNICODE)


def _tokenize(text: str) -> set[str]:
    return {m.group(0).lower() for m in _TOKEN_RE.finditer(text)}


def _lexical_score(query: str, text: str) -> float:
    """Jaccard-style overlap. 0..1."""
    q = _tokenize(query)
    t = _tokenize(text)
    if not q or not t:
        return 0.0
    inter = q & t
    return len(inter) / len(q)


def _snippet(text: str, max_len: int = SNIPPET_MAX) -> str:
    text = text.strip()
    if len(text) <= max_len:
        return text
    return text[: max_len - 1].rstrip() + "…"


@dataclass
class Retriever:
    store: VectorStore
    embedder: Embedder
    dense_weight: float = DENSE_WEIGHT
    lexical_weight: float = LEXICAL_WEIGHT

    async def index(
        self, tenant_id: str, documents: list[IndexDocument]
    ) -> tuple[str, int]:
        col = collection_name(tenant_id)
        await self.store.ensure_collection(col, self.embedder.dim)
        if not documents:
            return col, 0

        vectors = await self.embedder.embed([d.text for d in documents])
        points = [
            Point(
                id=d.id,
                vector=v,
                payload={
                    "document_id": d.id,
                    "text": d.text,
                    "source": d.source,
                    "source_type": d.source_type,
                    "tenant_id": tenant_id,
                    **(d.metadata or {}),
                },
            )
            for d, v in zip(documents, vectors)
        ]
        await self.store.upsert(col, points)
        return col, len(points)

    async def search(
        self,
        tenant_id: str,
        query: str,
        top_k: int = DEFAULT_TOP_K,
        filters: SearchFilters | None = None,
    ) -> list[SearchHit]:
        col = collection_name(tenant_id)
        # Embed → dense recall (over-fetch для last-mile re-rank-а лексикой).
        [qvec] = await self.embedder.embed([query])
        recall_k = max(top_k * 4, top_k)
        flt = self._filter_dict(filters)
        matches: list[Match] = await self.store.search(col, qvec, recall_k, flt)
        if not matches:
            return []

        rescored: list[tuple[float, Match]] = []
        for m in matches:
            text = str(m.payload.get("text", ""))
            lex = _lexical_score(query, text)
            blended = self.dense_weight * m.score + self.lexical_weight * lex
            rescored.append((blended, m))
        rescored.sort(key=lambda x: x[0], reverse=True)

        out: list[SearchHit] = []
        for score, m in rescored[:top_k]:
            payload = m.payload
            out.append(
                SearchHit(
                    document_id=str(payload.get("document_id", m.id)),
                    score=round(float(score), 6),
                    snippet=_snippet(str(payload.get("text", ""))),
                    source=str(payload.get("source", "")),
                    source_type=str(payload.get("source_type", "")),
                    metadata={
                        k: v
                        for k, v in payload.items()
                        if k not in {"document_id", "text", "source", "source_type", "tenant_id"}
                    },
                )
            )
        return out

    @staticmethod
    def _filter_dict(filters: SearchFilters | None) -> dict[str, Any] | None:
        if not filters:
            return None
        d = {k: v for k, v in filters.model_dump().items() if v is not None}
        return d or None
