"""Hybrid retriever: dense (Embedder) + reranker для last-mile scoring.

Reranker — pluggable через service.reranker.Reranker. По умолчанию
LexicalReranker (Jaccard-style overlap) для Pre-MVP / CI. После подключения
GPU + bge-reranker-v2-m3 переключение через ENV ``RAG_RERANKER=bge`` без
изменений retriever-кода.
"""
from __future__ import annotations

import logging
from dataclasses import dataclass, field
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
from service.reranker import LexicalReranker, Reranker

log = logging.getLogger(__name__)

DEFAULT_TOP_K = 5
SNIPPET_MAX = 480
DENSE_WEIGHT = 0.7
LEXICAL_WEIGHT = 0.3


def _snippet(text: str, max_len: int = SNIPPET_MAX) -> str:
    text = text.strip()
    if len(text) <= max_len:
        return text
    return text[: max_len - 1].rstrip() + "…"


def _default_reranker() -> Reranker:
    return LexicalReranker()


@dataclass
class Retriever:
    store: VectorStore
    embedder: Embedder
    dense_weight: float = DENSE_WEIGHT
    lexical_weight: float = LEXICAL_WEIGHT
    reranker: Reranker = field(default_factory=_default_reranker)

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

        # Last-mile re-rank через pluggable Reranker. На сетевые ошибки
        # (например, BGE недоступен) — graceful degradation на dense-only.
        texts = [str(m.payload.get("text", "")) for m in matches]
        try:
            rerank_scores = await self.reranker.rerank(query, texts)
        except Exception as exc:  # pragma: no cover — defensive только
            log.warning(
                "reranker '%s' упал, degrade на dense-only: %s",
                getattr(self.reranker, "name", type(self.reranker).__name__),
                exc,
            )
            rerank_scores = [0.0] * len(matches)

        rescored: list[tuple[float, Match]] = []
        for m, lex in zip(matches, rerank_scores):
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
