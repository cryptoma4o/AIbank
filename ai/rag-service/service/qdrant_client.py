"""Thin wrapper around qdrant-client + in-memory stub for tests.

Контракт `VectorStore`:

    ensure_collection(collection: str, dim: int) -> None
    upsert(collection, points: list[Point]) -> None
    search(collection, vector, top_k, filters) -> list[Match]

Production-реализация (`QdrantStore`) использует синхронный QdrantClient
вызовами через `asyncio.to_thread`, чтобы не тащить вторую (async)
библиотеку и сохранить совместимость с тестовым стабом.

В тестах используется `MemoryStore` — он реализует тот же интерфейс
поверх обычного списка в памяти, не требуя поднимать Qdrant.
"""
from __future__ import annotations

import asyncio
import logging
import os
import uuid
from dataclasses import dataclass, field
from typing import Any, Protocol

log = logging.getLogger(__name__)


def default_dim() -> int:
    """Размерность вектора по умолчанию.

    Берётся из env `RAG_EMBED_DIM`. По умолчанию 384 (для MockEmbedder /
    обратной совместимости). В production с BGE-M3 — 1024 (см. ADR-0008).
    """
    try:
        return int(os.environ.get("RAG_EMBED_DIM", "384"))
    except ValueError:
        return 384


def collection_name(tenant_id: str) -> str:
    """Per-tenant collection name. Tenant isolation is structural."""
    safe = "".join(c for c in tenant_id if c.isalnum() or c in "-_") or "default"
    return f"rag_{safe}"


@dataclass
class Point:
    id: str
    vector: list[float]
    payload: dict[str, Any] = field(default_factory=dict)


@dataclass
class Match:
    id: str
    score: float
    payload: dict[str, Any]


class VectorStore(Protocol):
    backend_name: str

    async def ensure_collection(self, collection: str, dim: int) -> None: ...
    async def upsert(self, collection: str, points: list[Point]) -> None: ...
    async def search(
        self,
        collection: str,
        vector: list[float],
        top_k: int,
        filters: dict[str, Any] | None = None,
    ) -> list[Match]: ...


class MemoryStore:
    """In-memory vector store for tests and dev.

    Реализует косинусное сходство руками — `Embedder` уже выдаёт
    нормированные векторы, поэтому это чистый dot-product.
    """

    backend_name = "memory"

    def __init__(self) -> None:
        # collection -> list[Point]
        self._data: dict[str, list[Point]] = {}

    async def ensure_collection(
        self, collection: str, dim: int | None = None
    ) -> None:
        # dim не нужен MemoryStore (хранит произвольные списки), но
        # сохраняем сигнатуру совместимой с QdrantStore.
        _ = dim if dim is not None else default_dim()
        self._data.setdefault(collection, [])

    async def upsert(self, collection: str, points: list[Point]) -> None:
        bucket = self._data.setdefault(collection, [])
        existing = {p.id: i for i, p in enumerate(bucket)}
        for p in points:
            if p.id in existing:
                bucket[existing[p.id]] = p
            else:
                bucket.append(p)

    async def search(
        self,
        collection: str,
        vector: list[float],
        top_k: int,
        filters: dict[str, Any] | None = None,
    ) -> list[Match]:
        bucket = self._data.get(collection, [])
        scored: list[Match] = []
        for p in bucket:
            if filters and not _payload_matches(p.payload, filters):
                continue
            score = sum(a * b for a, b in zip(vector, p.vector))
            scored.append(Match(id=p.id, score=score, payload=dict(p.payload)))
        scored.sort(key=lambda m: m.score, reverse=True)
        return scored[:top_k]


def _payload_matches(payload: dict[str, Any], filters: dict[str, Any]) -> bool:
    for k, v in filters.items():
        if v is None:
            continue
        if payload.get(k) != v:
            return False
    return True


class QdrantStore:
    """Production wrapper around qdrant-client.QdrantClient (sync, off-thread)."""

    backend_name = "qdrant"

    def __init__(self, url: str | None = None, api_key: str | None = None) -> None:
        # Lazy import — keeps test environments without qdrant-client working.
        from qdrant_client import QdrantClient  # type: ignore

        self._url = url or os.environ.get("QDRANT_URL", "http://qdrant:6333")
        self._client = QdrantClient(url=self._url, api_key=api_key)

    async def ensure_collection(
        self, collection: str, dim: int | None = None
    ) -> None:
        from qdrant_client.http import models as rest  # type: ignore

        size = dim if dim is not None else default_dim()

        def _ensure() -> None:
            existing = {c.name for c in self._client.get_collections().collections}
            if collection in existing:
                return
            self._client.create_collection(
                collection_name=collection,
                vectors_config=rest.VectorParams(
                    size=size, distance=rest.Distance.COSINE
                ),
            )

        await asyncio.to_thread(_ensure)

    async def upsert(self, collection: str, points: list[Point]) -> None:
        from qdrant_client.http import models as rest  # type: ignore

        rest_points = [
            rest.PointStruct(
                id=_to_qdrant_id(p.id),
                vector=p.vector,
                payload=p.payload,
            )
            for p in points
        ]

        def _do() -> None:
            self._client.upsert(collection_name=collection, points=rest_points)

        await asyncio.to_thread(_do)

    async def search(
        self,
        collection: str,
        vector: list[float],
        top_k: int,
        filters: dict[str, Any] | None = None,
    ) -> list[Match]:
        from qdrant_client.http import models as rest  # type: ignore

        qfilter: Any = None
        if filters:
            must = [
                rest.FieldCondition(key=k, match=rest.MatchValue(value=v))
                for k, v in filters.items()
                if v is not None
            ]
            if must:
                qfilter = rest.Filter(must=must)

        def _do() -> list[Match]:
            results = self._client.search(
                collection_name=collection,
                query_vector=vector,
                limit=top_k,
                query_filter=qfilter,
                with_payload=True,
            )
            return [
                Match(
                    id=str(r.payload.get("document_id", r.id)) if r.payload else str(r.id),
                    score=float(r.score),
                    payload=dict(r.payload or {}),
                )
                for r in results
            ]

        return await asyncio.to_thread(_do)


def _to_qdrant_id(s: str) -> str:
    """Qdrant accepts UUID or unsigned int. Hash arbitrary strings deterministically."""
    try:
        return str(uuid.UUID(s))
    except (ValueError, AttributeError):
        return str(uuid.uuid5(uuid.NAMESPACE_URL, s))


def build_default_store() -> VectorStore:
    """Factory used by main.py — pluggable in tests via monkeypatch."""
    backend = os.environ.get("RAG_BACKEND", "qdrant").lower()
    if backend == "memory":
        return MemoryStore()
    try:
        return QdrantStore()
    except Exception as exc:  # pragma: no cover — runtime fallback
        log.warning("Qdrant init failed (%s) — falling back to memory store", exc)
        return MemoryStore()
