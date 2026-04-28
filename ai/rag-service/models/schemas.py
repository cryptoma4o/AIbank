"""Pydantic v2 schemas for rag-service public API."""
from __future__ import annotations

from typing import Any

from pydantic import BaseModel, Field


class IndexDocument(BaseModel):
    """Single document submitted for indexing."""

    id: str
    text: str
    source: str            # human-readable label, e.g. "115-ФЗ ст.7"
    source_type: str = "law"   # "law" | "regulation" | "internal" | ...
    metadata: dict[str, Any] = Field(default_factory=dict)


class IndexRequest(BaseModel):
    tenant_id: str
    documents: list[IndexDocument]


class IndexResponse(BaseModel):
    tenant_id: str
    indexed: int
    collection: str


class SearchFilters(BaseModel):
    source_type: str | None = None
    source: str | None = None


class SearchRequest(BaseModel):
    tenant_id: str
    query: str
    top_k: int = 5
    filters: SearchFilters | None = None


class SearchHit(BaseModel):
    document_id: str
    score: float
    snippet: str
    source: str
    source_type: str
    metadata: dict[str, Any] = Field(default_factory=dict)


class SearchResponse(BaseModel):
    tenant_id: str
    query: str
    hits: list[SearchHit]
    backend: str          # "qdrant" | "memory" — useful in tests / dev
